package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"backend/internal/entity"
)

var (
	ErrAlreadyInNamespace  = errors.New("คุณมี namespace อยู่แล้ว (1 คน = 1 space)")
	ErrNamespaceNotFound   = errors.New("ไม่พบ namespace นี้")
	ErrNameTaken           = errors.New("ชื่อ namespace นี้ถูกใช้แล้ว")
	ErrQuotaOutOfRange     = errors.New("โควตาที่ตั้งเกินเพดานที่อนุญาต")
	ErrNamespaceHasMembers = errors.New("namespace นี้ยังมีสมาชิกคนอื่นอยู่ ต้องให้สมาชิกออกให้หมดก่อน หรือให้แอดมินลบแทน")
)

// NamespaceDetail = namespace + ข้อมูลประกอบที่คำนวณสด (ยอดใช้งาน + จำนวนสมาชิก)
// member_count ไม่ได้เก็บใน DB — นับจาก users ที่ namespace_id ตรงกัน เพื่อไม่ให้ค่าเพี้ยนจากของจริง
type NamespaceDetail struct {
	entity.Namespace
	Usage       NamespaceUsage `json:"usage"`
	MemberCount int            `json:"member_count"`
}

// NamespaceManager ดูแลวงจรชีวิตของ space: สร้าง (เดี่ยว/กลุ่ม), เข้าร่วมกลุ่ม, ดูรายละเอียด, ปรับโควตา
type NamespaceManager struct {
	db    *gorm.DB
	quota *QuotaService
	prov  Provisioner
}

// NewNamespaceManager ประกอบ manager โดยฉีด db/quota/prov — ถูกเรียกจาก main ตอน start
func NewNamespaceManager(db *gorm.DB, quota *QuotaService, prov Provisioner) *NamespaceManager {
	return &NamespaceManager{db: db, quota: quota, prov: prov}
}

// Create สร้าง namespace ใหม่ให้ user แล้วผูก user เข้ากับ space นั้นทันที (เขาเป็นเจ้าของ)
//
// data flow:
//   - รับ userID + ชื่อ + ชนิด (solo/group) จาก NamespaceController
//   - เช็คก่อนว่า user ยังไม่มี space (กติกา 1 คน = 1 space) → ถ้ามีแล้ว → ErrAlreadyInNamespace
//   - ใน transaction: INSERT namespaces (โควตาตั้งต้น 3000m/2048MB) แล้ว UPDATE users.namespace_id
//   - นอก transaction: เรียก prov.EnsureNamespace ไปสร้าง namespace + ResourceQuota จริงบน cluster
//
// ถ้าสร้างบน cluster ไม่สำเร็จ จะ rollback ด้วยการลบ row ทิ้ง — ไม่ปล่อยให้ DB มี space ที่ไม่มีอยู่จริง
func (m *NamespaceManager) Create(ctx context.Context, userID int, name string) (*entity.Namespace, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.NamespaceID != nil {
		return nil, ErrAlreadyInNamespace
	}

	ns := &entity.Namespace{
		Name:          name,
		ContributorID: userID,
		CPULimitMilli: entity.DefaultCPULimitMilli,
		RAMLimitMB:    entity.DefaultRAMLimitMB,
	}

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ns).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation บน uni_namespaces_name
				return ErrNameTaken
			}
			return err // สาเหตุอื่น (NOT NULL, check constraint, ฯลฯ) ให้ขึ้น error จริงแทนที่จะเดาว่าชื่อซ้ำ
		}
		// ผูกเจ้าของเข้ากับ space ที่เพิ่งสร้าง
		return tx.Model(&entity.User{}).Where("id = ?", userID).
			Update("namespace_id", ns.ID).Error
	})
	if err != nil {
		return nil, err
	}

	// สร้างของจริงบน cluster — ถ้าพลาดให้ถอย row ที่เพิ่งสร้างออก (กัน DB กับ cluster ไม่ตรงกัน)
	if err := m.prov.EnsureNamespace(ctx, ns); err != nil {
		m.db.WithContext(ctx).Delete(&entity.Namespace{}, ns.ID) // FK ตั้ง users.namespace_id กลับเป็น NULL ให้เอง
		return nil, fmt.Errorf("สร้าง namespace บน cluster ไม่สำเร็จ: %w", err)
	}
	return ns, nil
}

// Join พา user เข้าร่วม namespace แบบกลุ่มที่มีอยู่แล้ว
//
// data flow:
//   - รับ userID + namespaceID จาก NamespaceController
//   - เช็คว่า user ยังไม่มี space, และ namespace ปลายทางมีจริง
//   - UPDATE users.namespace_id → จบ (ไม่ต้องแตะ cluster เพราะ namespace มีอยู่แล้ว)
//
// member_count เพิ่มขึ้นเองโดยอัตโนมัติ เพราะเรานับจาก COUNT(users) ไม่ได้เก็บตัวเลขไว้
func (m *NamespaceManager) Join(ctx context.Context, userID, namespaceID int) (*entity.Namespace, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.NamespaceID != nil {
		return nil, ErrAlreadyInNamespace
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNamespaceNotFound
		}
		return nil, err
	}

	if err := m.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).
		Update("namespace_id", ns.ID).Error; err != nil {
		return nil, err
	}
	return &ns, nil
}

// Detail คืน namespace + ยอดใช้งาน + จำนวนสมาชิก (ใช้ทั้งหน้า "space ของฉัน" และหน้า admin)
// data flow: รับ namespaceID → อ่าน namespace → ถาม QuotaService.Usage → COUNT สมาชิก → รวมเป็น NamespaceDetail
func (m *NamespaceManager) Detail(ctx context.Context, namespaceID int) (*NamespaceDetail, error) {
	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNamespaceNotFound
		}
		return nil, err
	}

	usage, err := m.quota.Usage(ctx, nil, namespaceID)
	if err != nil {
		return nil, err
	}

	var members int64
	if err := m.db.WithContext(ctx).Model(&entity.User{}).
		Where("namespace_id = ?", namespaceID).Count(&members).Error; err != nil {
		return nil, err
	}

	return &NamespaceDetail{Namespace: ns, Usage: usage, MemberCount: int(members)}, nil
}

// ListAll คืน namespace ทั้งหมดพร้อมยอดใช้งาน — สำหรับหน้า admin ดูภาพรวมทั้งระบบ
// data flow: SELECT namespaces ทั้งหมด → วน Detail ทีละอัน → คืนเป็น slice ให้ AdminController
func (m *NamespaceManager) ListAll(ctx context.Context) ([]NamespaceDetail, error) {
	var all []entity.Namespace
	if err := m.db.WithContext(ctx).Order("id").Find(&all).Error; err != nil {
		return nil, err
	}

	out := make([]NamespaceDetail, 0, len(all))
	for _, ns := range all {
		d, err := m.Detail(ctx, ns.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

// SetQuota ให้ admin ปรับโควตาของ namespace (เช่น อัปจาก 3 core เป็น 8 core)
//
// data flow: รับ namespaceID + โควตาใหม่จาก AdminController → ตรวจว่าไม่เกินเพดานที่อนุญาต
// → UPDATE namespaces → sync โควตาใหม่ขึ้น cluster ผ่าน prov.EnsureNamespace
//
// เพดาน: ทุก namespace ขยายได้ถึง 8 core / 8 GB เท่ากันหมด (หลังเลิกแยกชนิด solo/group)
// ไม่เช็คว่าโควตาใหม่ต่ำกว่ายอดที่ใช้อยู่หรือไม่ — ปล่อยให้ลดได้ (service เดิมยังรันอยู่
// แต่จะ deploy เพิ่มไม่ได้จนกว่าจะลบของเก่าออก) ซึ่งเป็นพฤติกรรมเดียวกับ ResourceQuota ของ k8s
func (m *NamespaceManager) SetQuota(ctx context.Context, namespaceID, cpuMilli, ramMB int) (*NamespaceDetail, error) {
	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNamespaceNotFound
		}
		return nil, err
	}

	if cpuMilli > entity.MaxCPULimitMilli || ramMB > entity.MaxRAMLimitMB {
		return nil, fmt.Errorf("%w: ตั้งได้สูงสุด %dm CPU / %d MB",
			ErrQuotaOutOfRange, entity.MaxCPULimitMilli, entity.MaxRAMLimitMB)
	}

	ns.CPULimitMilli = cpuMilli
	ns.RAMLimitMB = ramMB
	if err := m.db.WithContext(ctx).Model(&entity.Namespace{}).Where("id = ?", ns.ID).
		Updates(map[string]any{
			"cpu_limit_milli": cpuMilli,
			"ram_limit_mb":    ramMB,
		}).Error; err != nil {
		return nil, err
	}

	// ดัน ResourceQuota ใหม่ขึ้น cluster ให้ตรงกับ DB
	if err := m.prov.EnsureNamespace(ctx, &ns); err != nil {
		return nil, fmt.Errorf("อัปเดตโควตาบน cluster ไม่สำเร็จ: %w", err)
	}
	return m.Detail(ctx, ns.ID)
}

// Delete ลบ namespace ทิ้งทั้งก้อนตามดุลยพินิจแอดมิน — ลบได้เสมอไม่ว่าจะมีสมาชิกกี่คน
//
// data flow:
//   - หา namespace ที่จะลบ
//   - เรียก prov.DeleteNamespace ถอนของจริงบนคลัสเตอร์ก่อนเสมอ (แบบเดียวกับ ServiceManager.Delete)
//     กันไม่ให้เหลือของค้างบนคลัสเตอร์โดยไม่มี record ใน DB รองรับ
//   - ลบแถว namespaces — foreign key ที่ตั้งไว้ใน addForeignKeys จัดการที่เหลือให้เอง:
//     services / ai_review_requests / user_containers ในนั้นถูกลบตาม (ON DELETE CASCADE)
//     ส่วนสมาชิกที่เหลือ (ถ้ามี) แค่ namespace_id ถูกตั้งเป็น NULL ไม่ถูกลบบัญชี (ON DELETE SET NULL)
//
// เรียกจาก AdminController.DeleteNamespace โดยตรง และจาก Leave เมื่อผู้ leave เป็น contributor
// และเป็นสมาชิกคนสุดท้ายที่เหลืออยู่ (ดู Leave)
func (m *NamespaceManager) Delete(ctx context.Context, namespaceID int) error {
	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, namespaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNamespaceNotFound
		}
		return err
	}

	if err := m.prov.DeleteNamespace(ctx, ns.Name); err != nil {
		return fmt.Errorf("ลบ namespace บนคลัสเตอร์ไม่สำเร็จ: %w", err)
	}

	return m.db.WithContext(ctx).Delete(&entity.Namespace{}, ns.ID).Error
}

// Leave ให้ผู้ใช้ออกจาก namespace ของตัวเอง — พฤติกรรมต่างกันตามบทบาทในนั้น:
//
//   - เป็นแค่สมาชิก (ไม่ใช่ contributor): แค่ตัด namespace_id ของตัวเองเป็น NULL
//     namespace เดิมและสมาชิกคนอื่นไม่กระทบอะไรเลย
//   - เป็น contributor และเป็นสมาชิกคนเดียวที่เหลืออยู่: leave ของเจ้าของคนสุดท้าย
//     เท่ากับลบ namespace ทั้งก้อน (เรียก Delete ต่อ)
//   - เป็น contributor แต่ยังมีสมาชิกคนอื่นอยู่: ปฏิเสธด้วย ErrNamespaceHasMembers
//     กันไม่ให้เจ้าของออกแล้วพา service ของสมาชิกคนอื่นหายไปด้วยแบบไม่ทันตั้งตัว
//     ต้องให้สมาชิกออกให้หมดก่อน หรือให้แอดมินลบแทน (แอดมินมีดุลยพินิจตัดสินใจเองได้ ดู Delete)
//
// เช็คจำนวนสมาชิกแบบไม่ล็อกแถว (เหมือน Join) — มีโอกาสน้อยมากที่จะมีคนเข้าร่วมพอดีตอนกำลังจะลบ
// ผลเสียที่สุดคือ service ของคนที่เพิ่ง join โดนลบไปด้วย ซึ่งเป็นความเสี่ยงระดับเดียวกับตอนแอดมินลบเอง
func (m *NamespaceManager) Leave(ctx context.Context, userID int) error {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return err
	}
	if user.NamespaceID == nil {
		return ErrNoNamespace
	}
	nsID := *user.NamespaceID

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, nsID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNamespaceNotFound
		}
		return err
	}

	if ns.ContributorID != userID {
		// แค่สมาชิก — ออกเฉยๆ ไม่กระทบ namespace หรือคนอื่น
		return m.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", userID).
			Update("namespace_id", nil).Error
	}

	var memberCount int64
	if err := m.db.WithContext(ctx).Model(&entity.User{}).
		Where("namespace_id = ?", nsID).Count(&memberCount).Error; err != nil {
		return err
	}
	if memberCount > 1 {
		return ErrNamespaceHasMembers
	}

	// เป็น contributor และเป็นคนสุดท้าย — leave ของเจ้าของคนเดียวเท่ากับลบ namespace ทั้งก้อน
	return m.Delete(ctx, nsID)
}
