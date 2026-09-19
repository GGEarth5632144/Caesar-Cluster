package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// CreateDatabaseParams = input ของ ServiceManager.CreateDatabase (body ของ POST /api/databases)
//
// ผู้ใช้กำหนดแค่ engine/version + credential 3 ช่อง + ชื่อ/ดิสก์/สเปก — image, พอร์ต, จุด mount
// และ env มาจาก catalog ทั้งหมด (docs 029) ผู้ใช้จึงตั้งค่า database ผิดจนข้อมูลหายไม่ได้อีก
type CreateDatabaseParams struct {
	Engine  string
	Version string // ว่าง = version ที่ catalog ตั้งเป็น default

	Name     string
	Username string
	Password string
	Database string

	StorageMB         int // 0 = entity.DefaultStorageMBPerService
	RequestTemplateID *int
	CPUMilli          int
	RAMMB             int
}

// CreateDatabase deploy database จาก template เข้า namespace ของผู้ใช้
//
// data flow: ตรวจ engine/version (กรอง EOL) + credential → สเปกจาก template หรือที่กรอก → เช็ค RAM ขั้นต่ำของ engine
// → reserveAndDeploy (จองโควตา + INSERT ไม่มีรหัส → provisioner สร้าง Secret → StatefulSet → ClusterIP + NetworkPolicy)
//
// รหัสผ่านเดินทางใน entity.Service.DBPassword (gorm:"-") ไปถึง provisioner เท่านั้น ไม่ลง DB
func (m *ServiceManager) CreateDatabase(ctx context.Context, userID, namespaceID int, p CreateDatabaseParams) (*entity.Service, error) {
	if err := m.requireNamespaceOwner(ctx, userID, namespaceID); err != nil {
		return nil, err
	}

	engine, version, err := resolveDatabaseVersion(p.Engine, p.Version, time.Now())
	if err != nil {
		return nil, err
	}
	if msg := validateDatabaseCredentials(engine.Engine, p.Username, p.Password, p.Database); msg != "" {
		return nil, fmt.Errorf("%w: %s", ErrDatabaseInvalidInput, msg)
	}

	cpuMilli, ramMB, err := m.resolveSpec(ctx, p.RequestTemplateID, p.CPUMilli, p.RAMMB)
	if err != nil {
		return nil, err
	}
	// ต่ำกว่านี้ database ขึ้นไม่ได้ (OOMKilled วนไป) ทั้งที่ deploy "สำเร็จ" — กันตั้งแต่ตอนขอ
	if ramMB < engine.MinRAMMB {
		return nil, fmt.Errorf("%w: %s ต้องใช้ RAM อย่างน้อย %d MB (ขอมา %d MB)", ErrDatabaseRAMTooLow, engine.Label, engine.MinRAMMB, ramMB)
	}

	storageMB := p.StorageMB
	if storageMB == 0 {
		storageMB = entity.DefaultStorageMBPerService
	}

	svc := &entity.Service{
		NamespaceID:       namespaceID,
		Name:              p.Name,
		CreatedBy:         userID,
		RequestTemplateID: p.RequestTemplateID,
		Image:             version.image,
		CPUMilli:          cpuMilli,
		RAMMB:             ramMB,
		ContainerPort:     engine.Port,
		Replicas:          entity.StorageReplicas,
		Status:            entity.ServiceCreating,
		EnvVars:           entity.EnvVarMap{},
		IsDatabase:        true,
		StorageMB:         storageMB,
		DataPath:          version.dataPath,
		DatabaseEngine:    engine.Engine,
		DatabaseVersion:   version.Version,
		DBUsername:        p.Username,
		DBName:            p.Database,
		DBPassword:        p.Password,
	}
	if engine.envRootPassword != "" {
		if svc.DBRootPassword, err = randomDatabasePassword(); err != nil {
			return nil, err
		}
	}
	return m.reserveAndDeploy(ctx, namespaceID, svc)
}

// Connection คืนข้อมูลเชื่อมต่อ database template (รวมรหัสผ่านที่อ่านจาก Secret) ให้สมาชิก namespace
// แยกจาก List โดยตั้งใจ: รายการ service ไม่เคยมีรหัสผ่าน หน้าเว็บเรียกเส้นนี้เมื่อผู้ใช้กด "แสดง" เท่านั้น
func (m *ServiceManager) Connection(ctx context.Context, serviceID, namespaceID int) (*DatabaseConnection, error) {
	var svc entity.Service
	err := m.db.WithContext(ctx).Where("id = ? AND namespace_id = ?", serviceID, namespaceID).First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrServiceNotFound
		}
		return nil, err
	}
	if !svc.IsTemplateDatabase() {
		return nil, ErrNotTemplateDatabase
	}

	nsName := K8sNamespaceName(namespaceID)
	cred, err := m.prov.DatabaseCredentials(ctx, nsName, &svc)
	if err != nil {
		return nil, err
	}
	conn, err := newDatabaseConnection(&svc, nsName, cred)
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// databaseUpdateChange บอกว่าคำขอแก้ database template ไปแตะค่าที่แก้ไม่ได้หรือไม่ ("" = แก้แค่ CPU/RAM)
//
// image อ่าน credential/สร้างโครงข้อมูลครั้งเดียวตอนเริ่มครั้งแรก เปลี่ยน image/version ทีหลังเสี่ยงข้อมูลอ่านไม่ได้
// (PostgreSQL ข้าม major ต้อง pg_upgrade) — ตอบ error ดีกว่ารับค่ามาแล้วแอบทิ้ง
func databaseUpdateChange(old *entity.Service, p UpdateServiceParams) string {
	switch {
	case p.Name != old.Name:
		return "ชื่อ"
	case p.Image != old.Image:
		return "image"
	case p.ContainerPort != 0 && p.ContainerPort != old.ContainerPort:
		return "พอร์ต"
	case len(p.EnvVars) > 0:
		return "env vars (database template ตั้ง env เองจาก credential)"
	case p.Replicas > entity.StorageReplicas:
		return "จำนวน replica"
	case p.StorageMB != 0 && p.StorageMB != old.StorageMB:
		return "ขนาดดิสก์"
	case p.DataPath != "" && p.DataPath != old.DataPath:
		return "data_path"
	}
	return ""
}

// updateTemplateDatabase = ทางของ Update สำหรับ database template: แก้ได้เฉพาะ CPU/RAM (ขั้นต่ำตาม engine)
func (m *ServiceManager) updateTemplateDatabase(ctx context.Context, svc entity.Service, p UpdateServiceParams) (*entity.Service, error) {
	if what := databaseUpdateChange(&svc, p); what != "" {
		return nil, fmt.Errorf("%w (แก้ %s)", ErrDatabaseImmutable, what)
	}
	if engine, ok := findDatabaseEngine(svc.DatabaseEngine); ok && p.RAMMB < engine.MinRAMMB {
		return nil, fmt.Errorf("%w: %s ต้องใช้ RAM อย่างน้อย %d MB (ขอมา %d MB)", ErrDatabaseRAMTooLow, engine.Label, engine.MinRAMMB, p.RAMMB)
	}

	oldSvc := svc
	err := m.quota.ReserveScale(ctx, svc.NamespaceID, svc.ID, ResourceRequest{
		CPUMilli:  p.CPUMilli,
		RAMMB:     p.RAMMB,
		StorageMB: svc.StorageMB,
		Replicas:  svc.Replicas,
	}, func(tx *gorm.DB) error {
		svc.CPUMilli = p.CPUMilli
		svc.RAMMB = p.RAMMB
		svc.Status = entity.ServiceCreating
		return tx.Save(&svc).Error
	})
	if err != nil {
		return nil, err
	}

	if err := m.prov.UpdateService(ctx, K8sNamespaceName(svc.NamespaceID), &oldSvc, &svc); err != nil {
		m.db.WithContext(context.WithoutCancel(ctx)).Model(&entity.Service{}).Where("id = ?", svc.ID).Update("status", entity.ServiceFailed)
		return nil, err
	}
	return &svc, nil
}
