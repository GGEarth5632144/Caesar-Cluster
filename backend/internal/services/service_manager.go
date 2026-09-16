package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

var (
	ErrRequestTemplateNotFound = errors.New("ไม่พบ template ที่เลือก (หรือถูกปิดใช้งานแล้ว)")
	ErrServiceNotFound         = errors.New("ไม่พบ service นี้ใน namespace ของคุณ")
	ErrServiceNotReady         = errors.New("service ยังไม่พร้อม — รอให้ deploy เสร็จก่อนค่อยปรับจำนวน replica")
	ErrLogsUnavailable         = errors.New("ยังอ่าน log ไม่ได้ — container ของ service นี้ยังไม่เริ่มทำงาน (ถ้าเพิ่ง deploy รอสักครู่แล้วกดลองใหม่)")

	// ── error ของดิสก์ถาวร (ดู entity.Service.HasStorage) ────────────────────────────

	// ระบบไม่เดาจุด mount ให้ไม่ว่าจะเป็น image อะไร เพราะเดาผิดแล้วได้ PVC ที่ไม่มีใครเขียนลง
	// ข้อมูลหายตอน Pod restart ทั้งที่ทุกอย่างดูเหมือนสำเร็จ
	ErrDataPathRequired = errors.New("ต้องระบุตำแหน่งที่ image นี้เก็บข้อมูล")

	// ตอบเป็น error แทนการแก้ค่าให้เงียบๆ ไม่งั้นผู้เรียก API จะเข้าใจว่าได้ 3 Pod ทั้งที่ระบบให้ 1
	ErrStorageReplicas = errors.New("service ที่มีดิสก์ถาวรต้องมี 1 pod เท่านั้น — สอง pod เขียนดิสก์ก้อนเดียวกันคือข้อมูลพัง")
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
	// เป็น 0 ได้ = ไม่ได้ระบุมา แล้ว Create เติม default ให้ (8080 / 1 replica) — ทำที่ชั้นนี้
	// ไม่ใช่ที่ DTO เพราะ default เป็นกติกาของ domain ไม่ใช่ของ HTTP layer
	ContainerPort int
	Replicas      int
	EnvVars       map[string]string

	// IsDatabase = สวิตช์ "เข้าได้เฉพาะใน namespace" จากหน้าเว็บ — database ต้องมีดิสก์เสมอ
	// Create จึงบังคับกติกาดิสก์ถาวรให้ด้วย (ดู entity.Service.IsDatabase)
	IsDatabase bool
	// StorageMB > 0 หรือส่ง DataPath มา = ขอดิสก์ถาวร (ได้ทั้ง database และ web)
	// ขอดิสก์แต่ StorageMB เป็น 0 = ใช้ entity.DefaultStorageMBPerService
	StorageMB int
	// DataPath = จุดที่ image เก็บข้อมูล — บังคับกรอกทุกครั้งที่ขอดิสก์
	DataPath string
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

	containerPort := p.ContainerPort
	if containerPort == 0 {
		containerPort = entity.DefaultContainerPort
	}
	replicas := p.Replicas
	if replicas == 0 {
		replicas = entity.DefaultReplicas
	}
	envVars := p.EnvVars
	storageMB := 0
	dataPath := ""

	// ── ดิสก์ถาวร: แปลงคำขอเป็นกติกาทั้งชุด (StatefulSet + PVC + 1 pod) ─────────────────
	//
	// ขอดิสก์ได้สองทาง: เปิดสวิตช์ database (database ต้องมีดิสก์เสมอ) หรือเป็น web ที่ส่ง
	// storage_mb/data_path มา เช่น Nextcloud ที่เก็บไฟล์ผู้ใช้ไว้ใน /var/www/html
	// ส่งมาช่องใดช่องหนึ่งก็ถือว่าขอดิสก์ แล้วบังคับให้ครบ (ขนาดมี default ส่วน path ไม่เดาให้)
	// ไม่ใช่ทิ้ง data_path ที่ส่งมาเงียบๆ — ผู้ใช้จะคิดว่าข้อมูลถาวรทั้งที่ไม่ใช่
	//
	// ตรวจให้จบก่อนแตะ DB เพราะเป็นการตรวจคำขอล้วนๆ ไม่ต้องถือ lock ระหว่างทำ
	wantsStorage := p.IsDatabase || p.StorageMB > 0 || strings.TrimSpace(p.DataPath) != ""
	if wantsStorage {
		if p.Replicas > entity.StorageReplicas {
			return nil, ErrStorageReplicas
		}
		replicas = entity.StorageReplicas
		storageMB = p.StorageMB
		if storageMB == 0 {
			storageMB = entity.DefaultStorageMBPerService
		}
		if msg := ValidateDataPath(p.DataPath); msg != "" {
			return nil, fmt.Errorf("%w: %s", ErrDataPathRequired, msg)
		}
		dataPath = strings.TrimRight(strings.TrimSpace(p.DataPath), "/")
	}

	svc := &entity.Service{
		NamespaceID:       namespaceID,
		Name:              p.Name,
		CreatedBy:         userID,
		RequestTemplateID: p.RequestTemplateID,
		Image:             p.Image,
		CPUMilli:          cpuMilli,
		RAMMB:             ramMB,
		ContainerPort:     containerPort,
		Replicas:          replicas,
		Status:            entity.ServiceCreating,
		EnvVars:           entity.EnvVarMap(envVars),
		IsDatabase:        p.IsDatabase,
		StorageMB:         storageMB,
		DataPath:          dataPath,
	}

	// เช็คโควตาของ namespace แล้ว INSERT ภายใน transaction เดียวกับที่ล็อก namespace ไว้
	// (โควตาที่หักคือ cpu/ram/ดิสก์ × replicas — ดู ReserveAndInsert)
	err := m.quota.ReserveAndInsert(ctx, namespaceID, ResourceRequest{
		CPUMilli:  cpuMilli,
		RAMMB:     ramMB,
		StorageMB: storageMB,
		Replicas:  replicas,
	}, func(tx *gorm.DB) error {
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
	if err := m.prov.DeployService(ctx, K8sNamespaceName(ns.ID), svc); err != nil {
		// deploy ไม่สำเร็จ → ลบ row ทิ้ง เพื่อคืนโควตาให้ namespace ทันที
		m.releaseReservation(ctx, svc.ID, err)
		return nil, err
	}

	// prov.DeployService เซ็ต svc.NodePort กลับมาแล้ว — persist ไว้
	//
	// จงใจไม่เขียน status=running ตรงนี้ทั้งที่ deploy ผ่าน เพราะ k8s รับ object แล้วตอบสำเร็จทันที
	// ต่อให้ไม่มี node ว่างหรือ container จะตายทันทีที่ขึ้น ปล่อยเป็น creating ไว้ให้
	// ServiceHealthMonitor ไปถามคลัสเตอร์ก่อนแล้วค่อยเขียนสถานะจริง (ดู service_health.go)
	//
	// ตัด cancel ออกด้วยเหตุผลเดียวกับ releaseReservation: ของถูกสร้างบนคลัสเตอร์ไปแล้วจริง
	// เขียน node_port ไม่ลงเพราะผู้ใช้ปิดหน้าเว็บ = ผู้ใช้ไม่มีทางรู้พอร์ตของตัวเอง
	if svc.NodePort != nil {
		if err := m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).
			Where("id = ?", svc.ID).
			Update("node_port", svc.NodePort).Error; err != nil {
			return nil, err
		}
	}
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

// Scale ปรับจำนวน Pod ของ service ที่ deploy ไปแล้ว (ใช้ตอนโหลดเยอะจนต้องเพิ่ม Pod มารับ)
//
// data flow: ตรวจสิทธิ์แบบเดียวกับ Delete → ReserveScale ล็อก namespace + เช็คโควตา + UPDATE ใน tx เดียว
// → นอก transaction สั่ง prov.ScaleService → คลัสเตอร์ไม่รับก็เขียนค่าเดิมกลับ
//
// ลำดับ "DB ก่อน แล้วค่อยคลัสเตอร์" ตรงข้ามกับ Delete โดยตั้งใจ: ที่นี่ DB เป็นตัวถือโควตา
// ถ้าไปเพิ่ม Pod บนคลัสเตอร์ก่อนโดยยังไม่จอง มีสิทธิ์แซงโควตาที่คนอื่นกำลังจองพร้อมกันอยู่
func (m *ServiceManager) Scale(ctx context.Context, serviceID, namespaceID, replicas int) (*entity.Service, error) {
	var svc entity.Service
	err := m.db.WithContext(ctx).
		Where("id = ? AND namespace_id = ?", serviceID, namespaceID).First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}

	// เช็คก่อนเรื่องสถานะ เพราะถ้าเช็คทีหลัง การ scale service ที่มีดิสก์ (รวม database) ซึ่งยังไม่ขึ้น
	// จะได้ error ว่า "ยังไม่พร้อม" ซึ่งชวนให้เข้าใจผิดว่ารอแล้วทำได้ (ต้องกันที่นี่ ไม่ใช่แค่ซ่อน dropdown บนหน้าเว็บ)
	// และ ScaleService ของจริงแก้แค่ Deployment — ปล่อยผ่านมาจะได้ NotFound จาก StatefulSet
	if svc.HasStorage() {
		return nil, ErrStorageReplicas
	}

	// ห้าม scale ระหว่างที่ยังไม่ได้ยืนยันว่า workload ขึ้นจริง ไม่งั้น DB กับคลัสเตอร์จะ drift ถาวร:
	// Create อ่านค่า Replicas ไว้ตั้งแต่ก่อนเรียก provisioner แล้วไป apply ด้วยจำนวนเดิม
	// ส่วน Scale เขียนจำนวนใหม่ลง DB ไปแล้ว — จบมาคลัสเตอร์ได้จำนวนเก่า และไม่มีอะไรมาปรับให้ตรงกันทีหลัง
	//
	if svc.Status != entity.ServiceRunning {
		return nil, fmt.Errorf("%w (สถานะตอนนี้: %s)", ErrServiceNotReady, svc.Status)
	}

	previous := svc.Replicas
	if previous == replicas {
		return &svc, nil // ไม่มีอะไรเปลี่ยน ไม่ต้องกวนคลัสเตอร์
	}

	err = m.quota.ReserveScale(ctx, namespaceID, serviceID, ResourceRequest{
		CPUMilli:  svc.CPUMilli,
		RAMMB:     svc.RAMMB,
		StorageMB: svc.StorageMB,
		Replicas:  replicas,
	},
		func(tx *gorm.DB) error {
			// UPDATE ที่ไม่โดนแถวไหน GORM ไม่ถือเป็น error — ถ้ามีคนลบ service แซงตอนเรารอ lock
			// Scale จะตอบ success แล้วไปสั่งคลัสเตอร์ scale ของที่ถูกลบไปแล้ว
			res := tx.Model(&entity.Service{}).
				Where("id = ? AND namespace_id = ?", serviceID, namespaceID).
				Update("replicas", replicas)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrServiceNotFound
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		return nil, err
	}

	if err := m.prov.ScaleService(ctx, K8sNamespaceName(ns.ID), svc.Name, replicas); err != nil {
		// คลัสเตอร์ไม่รับ → คืนค่าเดิม ไม่งั้นโควตาถูกจองไว้เกินของจริง
		// WithoutCancel ด้วยเหตุผลเดียวกับ releaseReservation (ผู้ใช้ปิดหน้าเว็บระหว่างรอ)
		if rbErr := m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).
			Where("id = ?", serviceID).
			Update("replicas", previous).Error; rbErr != nil {
			log.Printf("!! scale service id=%d ล้มเหลว (%v) และย้อน replicas กลับเป็น %d ไม่สำเร็จ: %v "+
				"— DB บอก %d replica แต่บนคลัสเตอร์ยังเป็น %d ต้องแก้มือ",
				serviceID, err, previous, rbErr, replicas, previous)
		}
		return nil, err
	}

	svc.Replicas = replicas
	return &svc, nil
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

	// ส่งทั้ง svc ไปเพราะมีดิสก์ต้องถอน PVC และ database ต้องถอน NetworkPolicy เพิ่มด้วย (ดู Provisioner.DeleteService)
	if err := m.prov.DeleteService(ctx, K8sNamespaceName(ns.ID), &svc); err != nil {
		return err
	}
	return m.db.WithContext(ctx).Delete(&entity.Service{}, svc.ID).Error
}

// ScheduledDeleteDelay = ระยะเวลาก่อนลบ service ที่แอดมินตั้งเวลาไว้
const ScheduledDeleteDelay = 24 * time.Hour

const scheduledDeleteCheckInterval = time.Minute

// AdminService = service + ชื่อ namespace และผู้สร้าง สำหรับหน้า admin
type AdminService struct {
	entity.Service
	NamespaceName    string `json:"namespace_name"`
	CreatorName      string `json:"creator_name"`
	CreatorStudentID string `json:"creator_student_id"`
}

// ListAll คืน service ทั้งระบบ เรียงใหม่→เก่า
func (m *ServiceManager) ListAll(ctx context.Context) ([]AdminService, error) {
	var list []AdminService
	err := m.db.WithContext(ctx).Table("services AS s").
		Select(`s.*, n.name AS namespace_name,
			COALESCE(u.real_name, '') AS creator_name,
			COALESCE(u.student_id, '') AS creator_student_id`).
		Joins("JOIN namespaces n ON n.id = s.namespace_id").
		Joins("LEFT JOIN users u ON u.id = s.created_by").
		Order("s.created_at DESC").
		Scan(&list).Error
	return list, err
}

// DeleteByID ลบ service โดยไม่จำกัด namespace — ใช้กับฝั่ง admin
func (m *ServiceManager) DeleteByID(ctx context.Context, serviceID int) error {
	var svc entity.Service
	if err := m.db.WithContext(ctx).Select("id", "namespace_id").First(&svc, serviceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrServiceNotFound
		}
		return err
	}
	return m.Delete(ctx, svc.ID, svc.NamespaceID)
}

// ScheduleDelete ตั้งเวลาลบ service ใน ScheduledDeleteDelay (ตั้งซ้ำ = นับใหม่)
func (m *ServiceManager) ScheduleDelete(ctx context.Context, serviceID int) (time.Time, error) {
	deleteAt := time.Now().UTC().Add(ScheduledDeleteDelay)
	return deleteAt, m.setDeleteAt(ctx, serviceID, gorm.Expr("?", deleteAt))
}

// CancelScheduledDelete ยกเลิกเวลาลบที่ตั้งไว้
func (m *ServiceManager) CancelScheduledDelete(ctx context.Context, serviceID int) error {
	return m.setDeleteAt(ctx, serviceID, gorm.Expr("NULL"))
}

func (m *ServiceManager) setDeleteAt(ctx context.Context, serviceID int, value any) error {
	res := m.db.WithContext(ctx).Model(&entity.Service{}).Where("id = ?", serviceID).Update("delete_at", value)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// StartScheduledDeletion เปิด worker ลบ service ที่ถึงเวลาลบ — หยุดเมื่อ ctx ถูก cancel
func (m *ServiceManager) StartScheduledDeletion(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scheduledDeleteCheckInterval)
		defer ticker.Stop()
		for {
			m.deleteDue(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (m *ServiceManager) deleteDue(ctx context.Context) {
	var due []entity.Service
	if err := m.db.WithContext(ctx).Select("id", "namespace_id", "name").
		Where("delete_at IS NOT NULL AND delete_at <= ?", time.Now().UTC()).
		Find(&due).Error; err != nil {
		log.Printf("scheduled delete: อ่านรายการไม่สำเร็จ: %v", err)
		return
	}
	for _, svc := range due {
		if err := m.Delete(ctx, svc.ID, svc.NamespaceID); err != nil && !errors.Is(err, ErrServiceNotFound) {
			log.Printf("scheduled delete: ลบ service '%s' (id=%d) ไม่สำเร็จ: %v", svc.Name, svc.ID, err)
			continue
		}
		log.Printf("scheduled delete: ลบ service '%s' (id=%d) ตามเวลาที่ตั้งไว้แล้ว", svc.Name, svc.ID)
	}
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

	return m.prov.Logs(ctx, K8sNamespaceName(ns.ID), svc.Name, opts)
}

// ── จุด mount ของดิสก์ถาวร ─────────────────────────────────────────────────────────

// reservedMountRoots = โฟลเดอร์ที่ห้าม mount PVC ทับ เพราะ PVC จะบังไฟล์เดิมของ image ทั้งหมด
// แล้ว container ขึ้นไม่ได้ด้วย error ที่เดาสาเหตุไม่ออก (binary หาย, ไลบรารีหาย)
var reservedMountRoots = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/lib", "/lib64",
	"/proc", "/root", "/sbin", "/sys", "/usr",
}

// ValidateDataPath ตรวจจุด mount ที่ผู้ใช้กรอกมา — คืนข้อความอธิบาย สตริงว่าง = ผ่าน
//
// ตรวจได้แค่ว่า mount แล้วไม่พังระบบ ตรวจไม่ได้ว่าตรงกับที่ image เขียนข้อมูลลงจริงหรือเปล่า
// อันหลังต้องพึ่งผู้ใช้ จึงเป็นเหตุผลที่ฟอร์มต้องเตือนให้ชัด
func ValidateDataPath(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return "ต้องระบุตำแหน่งที่ image นี้เก็บข้อมูล"
	case !strings.HasPrefix(p, "/"):
		return "ต้องขึ้นต้นด้วย / (เช่น /var/lib/mydb)"
	case len(p) > 200:
		return "ยาวเกินไป (สูงสุด 200 ตัวอักษร)"
	case strings.Contains(p, ".."):
		return "ห้ามมี .. ในเส้นทาง"
	case strings.Contains(p, "//"):
		return "ห้ามมี / ติดกันสองตัว"
	}

	// เทียบทั้งตัวมันเองและการเป็นโฟลเดอร์ย่อย — "/usr" กับ "/usr/local/data" ผิดทั้งคู่
	// แต่ "/usrdata" ไม่ผิด จึงต้องเทียบ prefix แบบมี "/" ต่อท้าย ไม่ใช่ HasPrefix เปล่าๆ
	clean := strings.TrimRight(p, "/")
	if clean == "" {
		return "mount ที่ / ไม่ได้ — จะบังไฟล์ทั้งหมดของ image"
	}
	for _, root := range reservedMountRoots {
		if clean == root || (root != "/" && strings.HasPrefix(clean+"/", root+"/")) {
			return "mount ทับโฟลเดอร์ระบบ (" + root + ") ไม่ได้ — container จะขึ้นไม่ได้"
		}
	}
	return ""
}
