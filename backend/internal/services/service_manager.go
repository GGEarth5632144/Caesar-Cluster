package services

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"backend/internal/entity"
)

var (
	ErrPlanNotFound    = errors.New("ไม่พบ plan ที่เลือก (หรือถูกปิดใช้งานแล้ว)")
	ErrServiceNotFound = errors.New("ไม่พบ service นี้ใน namespace ของคุณ")
)

// CreateServiceParams คือ input ของ ServiceManager.Create — ใช้ struct ของ services เอง
// (ไม่ import dto ตรงๆ) เพื่อไม่ให้ service layer ผูกกับ controller/dto layer
//
// เลือกสเปกได้ 2 ทาง: ส่ง PlanID มา (เลือกจาก choice ที่ admin สร้างไว้)
// หรือกรอก CPUMilli/RAMMB เอง — ถ้าส่ง PlanID มา ค่าใน plan จะชนะเสมอ
type CreateServiceParams struct {
	Name     string
	Image    string
	PlanID   *int
	CPUMilli int
	RAMMB    int
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
//   - ถ้าเลือก plan มา → อ่าน plan ที่ is_active แล้วก๊อป cpu/ram มาเป็น snapshot (ดูเหตุผลใน entity/plan.go)
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

	// เลือกจาก choice ที่ admin สร้างไว้ → ใช้สเปกของ plan เป็นหลัก
	if p.PlanID != nil {
		var plan entity.Plan
		err := m.db.WithContext(ctx).
			Where("id = ? AND is_active = true", *p.PlanID).First(&plan).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrPlanNotFound
			}
			return nil, err
		}
		cpuMilli, ramMB = plan.CPUMilli, plan.RAMMB
	}

	svc := &entity.Service{
		NamespaceID: namespaceID,
		Name:        p.Name,
		CreatedBy:   userID,
		PlanID:      p.PlanID,
		Image:       p.Image,
		CPUMilli:    cpuMilli,
		RAMMB:       ramMB,
		Status:      entity.ServiceCreating,
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
		m.db.WithContext(ctx).Delete(&entity.Service{}, svc.ID)
		return nil, err
	}

	if err := m.db.WithContext(ctx).Model(&entity.Service{}).
		Where("id = ?", svc.ID).Update("status", entity.ServiceRunning).Error; err != nil {
		return nil, err
	}
	svc.Status = entity.ServiceRunning
	return svc, nil
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
