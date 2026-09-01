package services

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"backend/internal/entity"
)

var (
	ErrNotContributor            = errors.New("เฉพาะเจ้าของ space เท่านั้นที่เชิญสมาชิกใหม่ได้")
	ErrInviteSelf                = errors.New("เชิญตัวเองไม่ได้")
	ErrStudentNotEligible        = errors.New("ไม่พบรหัสนักศึกษานี้ในฐานข้อมูล")
	ErrStudentNotCPE             = errors.New("เชิญได้เฉพาะนักศึกษาสาขาวิศวกรรมคอมพิวเตอร์ (CPE) เท่านั้น")
	ErrStudentNotActive          = errors.New("สถานภาพนักศึกษาของรหัสนี้ไม่สามารถเข้าร่วมระบบได้")
	ErrInviteeAlreadyInNamespace = errors.New("คนที่เชิญมี namespace อยู่แล้ว")
	ErrInviteAlreadyPending      = errors.New("มีคำเชิญที่รอตอบรับของคนนี้อยู่แล้วใน space นี้")
	ErrInviteNotFound            = errors.New("ไม่พบคำเชิญนี้")
	ErrInviteNotPending          = errors.New("คำเชิญนี้ถูกดำเนินการไปแล้ว")
	ErrInviteWrongUser           = errors.New("คำเชิญนี้ไม่ได้ส่งถึงคุณ")
)

// InviteDetail = คำเชิญ + ชื่อ namespace/ผู้เชิญที่ resolve มาให้แล้ว กัน frontend ต้องยิง query
// เพิ่มเพื่อโชว์ "คุณถูกเชิญเข้ากลุ่ม X โดย Y"
type InviteDetail struct {
	entity.NamespaceInvite
	NamespaceName string `json:"namespace_name"`
	InvitedByName string `json:"invited_by_name"`
}

// InviteManager ดูแลวงจรชีวิตของคำเชิญเข้ากลุ่ม: เจ้าของส่ง/ดู/ยกเลิก, ผู้ถูกเชิญดู/ตอบรับ/ปฏิเสธ
// ไม่พึ่ง Provisioner เลย เพราะคำเชิญเป็นแค่ความสัมพันธ์ระดับ DB ไม่แตะ cluster
// (พอ Accept แล้วถึงจะเทียบเท่า Join จริงๆ ซึ่งก็ไม่แตะ cluster เหมือนกัน — ดู NamespaceManager.Join)
type InviteManager struct {
	db *gorm.DB
}

// NewInviteManager ประกอบ manager โดยฉีด db — ถูกเรียกจาก main ตอน start
func NewInviteManager(db *gorm.DB) *InviteManager {
	return &InviteManager{db: db}
}

// Create ให้เจ้าของ (contributor) ของ namespace เชิญ student_id คนหนึ่งเข้ากลุ่ม
//
// data flow:
//   - หา namespace ของผู้เชิญ → ต้องเป็น contributor เท่านั้นถึงเชิญได้ (สมาชิกธรรมดาเชิญไม่ได้ —
//     กันสมาชิกคนหนึ่งชวนคนนอกเข้ามาแชร์โควตาโดยเจ้าของไม่รู้เรื่อง)
//   - เช็ค student_id ที่เชิญผ่านด่านเดียวกับ AuthController.Register ทุกด่าน (อยู่ในรายชื่อ/เป็น CPE/
//     สถานภาพยัง active) เพราะถ้าคนนั้นสมัคร/login ไม่ได้ตั้งแต่ต้น เชิญไปก็ตอบรับไม่ได้อยู่ดี
//     ดีกว่าปล่อยให้เจ้าของเห็นคำเชิญค้าง "pending" ตลอดไปโดยไม่รู้สาเหตุ
//   - ถ้าคนที่ถูกเชิญลงทะเบียนแล้ว ต้องยังไม่มี namespace ของตัวเอง (กติกา 1 คน = 1 space)
//   - กันเชิญซ้ำ (มี pending อยู่แล้วสำหรับคู่ namespace+student_id เดียวกัน)
func (m *InviteManager) Create(ctx context.Context, userID int, studentID string) (*entity.NamespaceInvite, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.NamespaceID == nil {
		return nil, ErrNoNamespace
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, *user.NamespaceID).Error; err != nil {
		return nil, err
	}
	if ns.ContributorID != userID {
		return nil, ErrNotContributor
	}

	if studentID == user.StudentID {
		return nil, ErrInviteSelf
	}

	var eligible entity.EligibleStudent
	if err := m.db.WithContext(ctx).Where("student_id = ?", studentID).First(&eligible).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStudentNotEligible
		}
		return nil, err
	}
	if eligible.Major != entity.MajorCPE {
		return nil, ErrStudentNotCPE
	}
	if !entity.ActiveEnrollmentStatuses[eligible.EnrollmentStatus] {
		return nil, ErrStudentNotActive
	}

	var invitee entity.User
	err := m.db.WithContext(ctx).Where("student_id = ?", studentID).First(&invitee).Error
	switch {
	case err == nil:
		if invitee.NamespaceID != nil {
			return nil, ErrInviteeAlreadyInNamespace
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// ยังไม่ได้ลงทะเบียน — เชิญล่วงหน้าได้ พอ register/login แล้วจะเห็นคำเชิญเอง (ดู Mine)
	default:
		return nil, err
	}

	var pendingCount int64
	if err := m.db.WithContext(ctx).Model(&entity.NamespaceInvite{}).
		Where("namespace_id = ? AND invited_student_id = ? AND status = ?",
			ns.ID, studentID, entity.InviteStatusPending).
		Count(&pendingCount).Error; err != nil {
		return nil, err
	}
	if pendingCount > 0 {
		return nil, ErrInviteAlreadyPending
	}

	invite := &entity.NamespaceInvite{
		NamespaceID:      ns.ID,
		InvitedStudentID: studentID,
		InvitedBy:        userID,
		Status:           entity.InviteStatusPending,
	}
	if err := m.db.WithContext(ctx).Create(invite).Error; err != nil {
		return nil, err
	}
	return invite, nil
}

// Mine คืนคำเชิญที่ pending อยู่ ส่งถึง student_id ของผู้ใช้ที่ login อยู่ตอนนี้
// (ไม่สน namespace_id ปัจจุบันของผู้ใช้ — ต่อให้ยังไม่มี namespace ก็ต้องเห็นคำเชิญได้)
func (m *InviteManager) Mine(ctx context.Context, userID int) ([]InviteDetail, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}

	var invites []entity.NamespaceInvite
	if err := m.db.WithContext(ctx).
		Where("invited_student_id = ? AND status = ?", user.StudentID, entity.InviteStatusPending).
		Order("created_at DESC").Find(&invites).Error; err != nil {
		return nil, err
	}
	return m.enrich(ctx, invites)
}

// Sent คืนคำเชิญทั้งหมด (ทุกสถานะ) ที่ contributor ของ namespace ตัวเองเคยส่งออกไป
// ให้เจ้าของเห็นว่าใคร pending/accepted/declined อยู่ กันเชิญซ้ำมั่วๆ
func (m *InviteManager) Sent(ctx context.Context, userID int) ([]InviteDetail, error) {
	ns, err := m.namespaceOwnedBy(ctx, userID)
	if err != nil {
		return nil, err
	}

	var invites []entity.NamespaceInvite
	if err := m.db.WithContext(ctx).Where("namespace_id = ?", ns.ID).
		Order("created_at DESC").Find(&invites).Error; err != nil {
		return nil, err
	}
	return m.enrich(ctx, invites)
}

// Accept ให้ผู้ถูกเชิญตอบรับ — เท่ากับ Join namespace ที่เชิญมา (ผูก user เข้า namespace ทันที)
//
// data flow: หา invite ตาม id → ต้องเป็นของ student_id ผู้ใช้ที่ login อยู่จริง (กันคนอื่นมากด accept
// แทน) → ต้องยัง pending อยู่ → ผู้ใช้ต้องยังไม่มี namespace (เผื่อไป Join/Accept ที่อื่นระหว่างรอ)
// → namespace ที่เชิญต้องยังอยู่จริง (เผื่อถูกลบไประหว่างรอ) → UPDATE users.namespace_id +
// invite.status = accepted ในทรานแซกชันเดียว (สอง write ต้องสำเร็จคู่กันเท่านั้น)
func (m *InviteManager) Accept(ctx context.Context, userID, inviteID int) (*entity.Namespace, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}

	invite, err := m.findMyInvite(ctx, user.StudentID, inviteID)
	if err != nil {
		return nil, err
	}
	if invite.Status != entity.InviteStatusPending {
		return nil, ErrInviteNotPending
	}
	if user.NamespaceID != nil {
		return nil, ErrAlreadyInNamespace
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, invite.NamespaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNamespaceNotFound
		}
		return nil, err
	}

	// เงื่อนไขทั้งสองข้อถูกเช็คไปแล้วข้างบน แต่ต้องเช็คซ้ำ "ในคำสั่ง UPDATE เอง" อีกที
	// เพราะการเช็คข้างบนอยู่นอก transaction: ระหว่างนั้นผู้ใช้อาจกด accept คำเชิญอีกใบพร้อมกัน
	// (หรือกดใบเดิมสองครั้ง) แล้วทั้งสอง request ผ่านด่านมาได้ทั้งคู่ ผลคือคนที่เขียนทีหลังชนะ
	// และคำเชิญอีกใบถูกทำเครื่องหมายว่า accepted ทั้งที่ไม่ได้พาเข้า namespace นั้นจริง
	//
	// ให้ฐานข้อมูลตัดสินแทน: ใครมาถึงก่อนได้ไป คนที่มาทีหลังได้ RowsAffected = 0 แล้ว rollback
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		joined := tx.Model(&entity.User{}).Where("id = ? AND namespace_id IS NULL", userID).
			Update("namespace_id", ns.ID)
		if joined.Error != nil {
			return joined.Error
		}
		if joined.RowsAffected == 0 {
			return ErrAlreadyInNamespace
		}

		accepted := tx.Model(&entity.NamespaceInvite{}).
			Where("id = ? AND status = ?", invite.ID, entity.InviteStatusPending).
			Update("status", entity.InviteStatusAccepted)
		if accepted.Error != nil {
			return accepted.Error
		}
		if accepted.RowsAffected == 0 {
			return ErrInviteNotPending
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ns, nil
}

// Decline ให้ผู้ถูกเชิญปฏิเสธคำเชิญ — แค่พลิกสถานะ ไม่กระทบ namespace ใดๆ
func (m *InviteManager) Decline(ctx context.Context, userID, inviteID int) error {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return err
	}

	invite, err := m.findMyInvite(ctx, user.StudentID, inviteID)
	if err != nil {
		return err
	}
	if invite.Status != entity.InviteStatusPending {
		return ErrInviteNotPending
	}

	return m.db.WithContext(ctx).Model(&entity.NamespaceInvite{}).Where("id = ?", invite.ID).
		Update("status", entity.InviteStatusDeclined).Error
}

// Cancel ให้เจ้าของ (contributor) ยกเลิกคำเชิญที่ตัวเองส่งไป (เผื่อเชิญผิดคน) — ลบแถวทิ้งเลย
// เพราะเป็นคำเชิญที่ยังไม่มีใครตัดสินใจอะไร ต่างจาก accepted/declined ที่เป็นเหตุการณ์จริงควรเก็บไว้ดูใน Sent
func (m *InviteManager) Cancel(ctx context.Context, userID, inviteID int) error {
	ns, err := m.namespaceOwnedBy(ctx, userID)
	if err != nil {
		return err
	}

	var invite entity.NamespaceInvite
	if err := m.db.WithContext(ctx).
		Where("id = ? AND namespace_id = ?", inviteID, ns.ID).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInviteNotFound
		}
		return err
	}
	if invite.Status != entity.InviteStatusPending {
		return ErrInviteNotPending
	}

	return m.db.WithContext(ctx).Delete(&entity.NamespaceInvite{}, invite.ID).Error
}

// namespaceOwnedBy คืน namespace ของ userID แต่ต้องเป็น contributor เท่านั้น — ใช้ร่วมกันใน
// Sent/Cancel ที่ทั้งคู่เป็นมุมมองของเจ้าของ
func (m *InviteManager) namespaceOwnedBy(ctx context.Context, userID int) (*entity.Namespace, error) {
	var user entity.User
	if err := m.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.NamespaceID == nil {
		return nil, ErrNoNamespace
	}

	var ns entity.Namespace
	if err := m.db.WithContext(ctx).First(&ns, *user.NamespaceID).Error; err != nil {
		return nil, err
	}
	if ns.ContributorID != userID {
		return nil, ErrNotContributor
	}
	return &ns, nil
}

// findMyInvite หา invite ตาม id แล้วยืนยันว่าเป็นของ studentID จริง — ใช้ร่วมกันใน Accept/Decline
// กันคนอื่นเดา invite id แล้วไปตอบรับ/ปฏิเสธแทนเจ้าตัว
func (m *InviteManager) findMyInvite(ctx context.Context, studentID string, inviteID int) (*entity.NamespaceInvite, error) {
	var invite entity.NamespaceInvite
	if err := m.db.WithContext(ctx).First(&invite, inviteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInviteNotFound
		}
		return nil, err
	}
	if invite.InvitedStudentID != studentID {
		return nil, ErrInviteWrongUser
	}
	return &invite, nil
}

// enrich เติมชื่อ namespace + ชื่อผู้เชิญให้ invite แต่ละแถว โดย query แบบ batch กัน N+1
// (รูปแบบเดียวกับ AdminController.ListAllRequests ที่เก็บ id ที่พบมาถามเป็นก้อนเดียว)
func (m *InviteManager) enrich(ctx context.Context, invites []entity.NamespaceInvite) ([]InviteDetail, error) {
	out := make([]InviteDetail, 0, len(invites))
	if len(invites) == 0 {
		return out, nil
	}

	nsSeen := make(map[int]bool, len(invites))
	nsIDs := make([]int, 0, len(invites))
	userSeen := make(map[int]bool, len(invites))
	inviterIDs := make([]int, 0, len(invites))
	for _, inv := range invites {
		if !nsSeen[inv.NamespaceID] {
			nsSeen[inv.NamespaceID] = true
			nsIDs = append(nsIDs, inv.NamespaceID)
		}
		if !userSeen[inv.InvitedBy] {
			userSeen[inv.InvitedBy] = true
			inviterIDs = append(inviterIDs, inv.InvitedBy)
		}
	}

	var namespaces []entity.Namespace
	if err := m.db.WithContext(ctx).Where("id IN ?", nsIDs).Find(&namespaces).Error; err != nil {
		return nil, err
	}
	nameByNS := make(map[int]string, len(namespaces))
	for _, ns := range namespaces {
		nameByNS[ns.ID] = ns.Name
	}

	var inviters []entity.User
	if err := m.db.WithContext(ctx).Where("id IN ?", inviterIDs).Find(&inviters).Error; err != nil {
		return nil, err
	}
	nameByUser := make(map[int]string, len(inviters))
	for _, u := range inviters {
		nameByUser[u.ID] = u.RealName
	}

	for _, inv := range invites {
		out = append(out, InviteDetail{
			NamespaceInvite: inv,
			NamespaceName:   nameByNS[inv.NamespaceID],
			InvitedByName:   nameByUser[inv.InvitedBy],
		})
	}
	return out, nil
}
