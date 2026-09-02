package services

import (
	"context"
	"errors"
	"io"
	"log"

	"gorm.io/gorm"

	"backend/internal/entity"
)

var (
	ErrRequestTemplateNotFound = errors.New("ไม่พบ template ที่เลือก (หรือถูกปิดใช้งานแล้ว)")
	ErrServiceNotFound         = errors.New("ไม่พบ service นี้ใน namespace ของคุณ")
)

// CreateServiceParams คือ input ของ ServiceManager.Create — ใช้ struct ของ services เอง
// (ไม่ import dto ตรงๆ) เพื่อไม่ให้ service layer ผูกกับ controller/dto layer
//
// เลือกสเปกได้ 2 ทาง: ส่ง RequestTemplateID มา (เลือกจาก choice ที่ admin สร้างไว้)
// หรือกรอก CPUMilli/RAMMB เอง — ถ้าส่ง RequestTemplateID มา ค่าใน template จะชนะเสมอ
type CreateServiceParams struct {
	Name              string
	Image             string
	RequestTemplateID *int
	CPUMilli          int
	RAMMB             int
	EnvVars           map[string]string
}

// ServiceManager = business logic ของ workload: เช็คโควตา → บันทึก DB → deploy จริงขึ้น cluster
// (มาแทน VMService เดิม)
type ServiceManager struct {
	db    *gorm.DB
	quota *QuotaService
	prov  Provisioner
}

// NewServiceManager ประกอบ manager โดยฉีด db/quota/prov — ถูกเรียกจาก main ตอน start
func NewServiceManager(db *gorm.DB, quota *QuotaService, prov Provisioner) *ServiceManager {
	return &ServiceManager{db: db, quota: quota, prov: prov}
}

// ListByNamespace คืน service ทั้งหมดใน namespace เรียงใหม่→เก่า
//
// data flow: รับ namespaceID (มาจาก user.namespace_id ที่ controller อ่านมา) → SELECT services → คืน slice
//
// หมายเหตุ: มองเป็นของ "ทั้ง space" ไม่ใช่ของรายคน — สมาชิกทุกคนในกลุ่มเห็น service ของกลุ่มเหมือนกันหมด
// (สอดคล้องกับที่โควตาเป็นของ namespace ร่วมกัน ไม่ใช่ของใครคนเดียว)
func (m *ServiceManager) ListByNamespace(ctx context.Context, namespaceID int) ([]entity.Service, error) {
	var list []entity.Service
	err := m.db.WithContext(ctx).
		Where("namespace_id = ?", namespaceID).
		Order("created_at DESC").
		Find(&list).Error
	return list, err
}

// Create deploy service ใหม่เข้า namespace ของผู้ใช้
//
// data flow:
//   - รับ userID + namespaceID + params จาก ServiceController
//   - ถ้าเลือก template มา → อ่าน template ที่ is_active แล้วก๊อป cpu/ram มาเป็น snapshot (ดูเหตุผลใน entity/request_template.go)
//   - QuotaService.ReserveAndInsert ล็อกแถว namespace → เช็คโควตารวม → INSERT service (status=creating) ใน tx เดียว
//   - นอก transaction: prov.DeployService สร้าง workload จริงบน cluster
//   - สำเร็จ → update status=running ; ล้มเหลว → ลบ row ทิ้งเพื่อ "คืนโควตา" แล้วคืน error
//
// ทำไมล้มเหลวแล้วต้องลบ row (ไม่ mark failed ค้างไว้):
// โควตาคิดจาก SUM ของ service ทุกแถวใน namespace — ถ้าปล่อยแถว failed ค้างไว้ มันจะกินโควตาไปเรื่อยๆ
// ทั้งที่ไม่มี workload อยู่จริงบน cluster (นี่คือบั๊กแบบเดียวกับที่ VMService เดิมมี แต่รอบนี้ปิดไปเลย)
//
// เรียก provisioner นอก transaction เพราะการ deploy ช้า/พลาดได้ ไม่ควรถือ lock ของ namespace ค้างไว้ตอนรอ
func (m *ServiceManager) Create(ctx context.Context, userID, namespaceID int, p CreateServiceParams) (*entity.Service, error) {
	cpuMilli, ramMB := p.CPUMilli, p.RAMMB

	// เลือกจาก choice ที่ admin สร้างไว้ → ใช้สเปกของ template เป็นหลัก
	if p.RequestTemplateID != nil {
		var tmpl entity.RequestTemplate
		err := m.db.WithContext(ctx).
			Where("id = ? AND is_active = true", *p.RequestTemplateID).First(&tmpl).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrRequestTemplateNotFound
			}
			return nil, err
		}
		cpuMilli, ramMB = tmpl.CPULimitMilli, tmpl.RAMLimitMB
	}

	svc := &entity.Service{
		NamespaceID:       namespaceID,
		Name:              p.Name,
		CreatedBy:         userID,
		RequestTemplateID: p.RequestTemplateID,
		Image:             p.Image,
		CPUMilli:          cpuMilli,
		RAMMB:             ramMB,
		Status:            entity.ServiceCreating,
		EnvVars:           entity.EnvVarMap(p.EnvVars),
	}

	// เช็คโควตาของ namespace แล้ว INSERT ภายใน transaction เดียวกับที่ล็อก namespace ไว้
	err := m.quota.ReserveAndInsert(ctx, namespaceID, cpuMilli, ramMB, func(tx *gorm.DB) error {
		return tx.Create(svc).Error
	})
	if err != nil {
		return nil, err
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		return nil, err
	}

	// deploy ของจริงขึ้น cluster
	if err := m.prov.DeployService(ctx, ns.Name, svc); err != nil {
		// deploy ไม่สำเร็จ → ลบ row ทิ้ง เพื่อคืนโควตาให้ namespace ทันที
		m.releaseReservation(ctx, svc.ID, err)
		return nil, err
	}

	// prov.DeployService เซ็ต svc.NodePort กลับมาแล้ว — persist คู่กับ status ในทีเดียว
	//
	// ตัด cancel ออกด้วยเหตุผลเดียวกับ releaseReservation: ของถูกสร้างบนคลัสเตอร์ไปแล้วจริง
	// ถ้าเขียนสถานะกลับไม่ได้เพราะผู้ใช้ปิดหน้าเว็บ แถวนี้จะค้างเป็น "creating" ตลอดกาล
	// ทั้งที่ workload รันอยู่ และไม่มีอะไรมาแก้ให้
	if err := m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).
		Where("id = ?", svc.ID).
		Updates(map[string]any{"status": entity.ServiceRunning, "node_port": svc.NodePort}).Error; err != nil {
		return nil, err
	}
	svc.Status = entity.ServiceRunning
	return svc, nil
}

// releaseReservation ลบแถว service ที่จองโควตาไว้ทิ้ง หลัง deploy ล้มเหลว
//
// ต้องใช้ context.WithoutCancel: สาเหตุที่ deploy ล้มบ่อยที่สุดสาเหตุหนึ่งคือผู้ใช้ปิดหน้าเว็บ
// ระหว่างรอ ซึ่ง cancel ctx ของ HTTP request ไปด้วย ถ้าใช้ ctx ตัวเดิมมาลบ DELETE จะล้มทันที
// ด้วย "context canceled" แล้วแถวที่จองโควตาไว้ค้างตลอดไปโดยไม่มี workload จริง
// (เคยเกิดจริง: โควตาหาย 400m/256MB โดยผู้ใช้เห็นแค่ข้อความ error)
//
// error ของการลบต้อง log เสมอ ไม่กลืนทิ้ง — ลบไม่สำเร็จแปลว่าโควตารั่วจริง ต้องมีร่องรอยให้ตามเก็บ
func (m *ServiceManager) releaseReservation(ctx context.Context, serviceID int, cause error) {
	err := m.db.WithContext(context.WithoutCancel(ctx)).
		Delete(&entity.Service{}, serviceID).Error
	if err != nil {
		log.Printf("!! deploy service id=%d ล้มเหลว (%v) และคืนโควตาไม่สำเร็จด้วย: %v "+
			"— แถวนี้ยังกินโควตาของ namespace อยู่ ต้องลบมือ", serviceID, cause, err)
	}
}

// Delete ลบ service ออกจาก namespace: ถอนของจริงบน cluster ก่อน แล้วค่อยลบ row (คืนโควตา)
//
// data flow:
//   - รับ serviceID + namespaceID ของผู้ใช้จาก ServiceController
//   - SELECT service ที่ id ตรง "และ" อยู่ใน namespace ของผู้ใช้ — กันไม่ให้ลบของ space อื่น
//   - prov.DeleteService ถอน workload จริงก่อน → สำเร็จค่อย DELETE row
//
// เรียงลำดับนี้กันไม่ให้เหลือ workload ค้างบน cluster โดยไม่มี record ใน DB (กลายเป็นของผีที่กินทรัพยากรฟรี)
func (m *ServiceManager) Delete(ctx context.Context, serviceID, namespaceID int) error {
	var svc entity.Service
	err := m.db.WithContext(ctx).
		Where("id = ? AND namespace_id = ?", serviceID, namespaceID).First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrServiceNotFound
		}
		return err
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		return err
	}

	if err := m.prov.DeleteService(ctx, ns.Name, svc.Name); err != nil {
		return err
	}
	return m.db.WithContext(ctx).Delete(&entity.Service{}, svc.ID).Error
}

// Logs เปิด stream ของ log จาก service หนึ่งตัว — ระบบไม่เก็บสำเนา log ไว้ อ่านสดจากคลัสเตอร์ทุกครั้ง
// แตะ DB แค่เช็คว่า service นี้อยู่ใน namespace ของผู้เรียกจริง (กันดู log ข้าม space)
//
// data flow: ตรวจสิทธิ์แบบเดียวกับ Delete → หาชื่อ namespace บนคลัสเตอร์ → ให้ provisioner
// เปิด stream → คืน io.ReadCloser ให้ ServiceController อ่านต่อออก HTTP response
// ผู้เรียกมีหน้าที่ Close() เสมอ
func (m *ServiceManager) Logs(ctx context.Context, serviceID, namespaceID int, opts LogOptions) (io.ReadCloser, error) {
	var svc entity.Service
	err := m.db.WithContext(ctx).
		Where("id = ? AND namespace_id = ?", serviceID, namespaceID).First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		return nil, err
	}

	return m.prov.Logs(ctx, ns.Name, svc.Name, opts)
}
