package services

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// ── จอง NodePort ตอนเปิดฟอร์ม New Service (docs 031) ────────────────────────────────
//
// ผู้ใช้ต้องเห็น "<host>:<พอร์ต>" ตั้งแต่ก่อนกด deploy จึงสุ่มพอร์ตว่างให้ตอนเปิดฟอร์มแล้วจองไว้
// ผู้ใช้เลือกพอร์ตเองไม่ได้: POST /api/services รับได้เฉพาะเลขที่ผู้ใช้คนนั้นจองไว้ในกลุ่มเดียวกัน
//
// จองไว้ในหน่วยความจำ (backend รันตัวเดียว) — รีสตาร์ทแล้วหาย ผู้ใช้ได้ 409 แล้วหน้าเว็บขอเลขใหม่
// การจองเป็นแค่การกันไม่ให้ backend จ่ายเลขซ้ำกันเอง ตัวตัดสินสุดท้ายคือ apiserver ที่ปฏิเสธเลขที่ถูกใช้แล้ว

// NodePortReservationTTL = อายุใบจอง — นานพอให้กรอกฟอร์มยาวๆ แต่ไม่กักพอร์ตไว้ข้ามวัน
const NodePortReservationTTL = 30 * time.Minute

var (
	ErrNodePortReservationInvalid = errors.New("พอร์ตที่จองไว้หมดอายุหรือไม่ใช่ของคุณ — ระบบจะจองพอร์ตใหม่ให้")
	ErrNodePortTaken              = errors.New("พอร์ตนี้ถูกใช้ไปแล้ว — ระบบจะจองพอร์ตใหม่ให้")
	ErrNodePortExhausted          = errors.New("ไม่เหลือพอร์ตว่างในคลัสเตอร์")
)

// NodePortReservation = ใบจองที่ส่งกลับให้หน้าเว็บ
type NodePortReservation struct {
	NodePort  int       `json:"node_port"`
	ExpiresAt time.Time `json:"expires_at"`
}

type nodePortHold struct {
	userID, namespaceID int
	expiresAt           time.Time
}

// NodePortReservations เก็บใบจองทั้งหมด — ปลอดภัยต่อการเรียกพร้อมกัน
type NodePortReservations struct {
	db   *gorm.DB
	prov Provisioner
	now  func() time.Time

	mu    sync.Mutex
	holds map[int]nodePortHold // key = เลขพอร์ต
}

func NewNodePortReservations(db *gorm.DB, prov Provisioner) *NodePortReservations {
	return &NodePortReservations{db: db, prov: prov, now: time.Now, holds: make(map[int]nodePortHold)}
}

// Reserve คืนใบจองของผู้ใช้ในกลุ่มนี้ — มีใบที่ยังไม่หมดอายุอยู่แล้ว = คืนเลขเดิม (เปิดฟอร์มซ้ำได้เลขเดิม)
// ไม่มี = สุ่มพอร์ตที่ไม่ถูกใช้บนคลัสเตอร์, ไม่อยู่ใน DB และไม่มีใครจองค้าง
func (r *NodePortReservations) Reserve(ctx context.Context, userID, namespaceID int) (NodePortReservation, error) {
	// ถามคลัสเตอร์/DB นอก lock — ช้าได้ และไม่ต้องกันกันเองระหว่างถาม (เช็คซ้ำตอนถือ lock ด้านล่าง)
	used, err := r.usedPorts(ctx)
	if err != nil {
		return NodePortReservation{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.dropExpired(now)

	for port, h := range r.holds {
		if h.userID == userID && h.namespaceID == namespaceID {
			return NodePortReservation{NodePort: port, ExpiresAt: h.expiresAt}, nil
		}
	}

	free := make([]int, 0, entity.MaxNodePort-entity.MinNodePort+1)
	for p := entity.MinNodePort; p <= entity.MaxNodePort; p++ {
		if _, held := r.holds[p]; !held && !used[p] {
			free = append(free, p)
		}
	}
	if len(free) == 0 {
		return NodePortReservation{}, ErrNodePortExhausted
	}
	i, err := rand.Int(rand.Reader, big.NewInt(int64(len(free))))
	if err != nil {
		return NodePortReservation{}, err
	}
	port := free[i.Int64()]
	h := nodePortHold{userID: userID, namespaceID: namespaceID, expiresAt: now.Add(NodePortReservationTTL)}
	r.holds[port] = h
	return NodePortReservation{NodePort: port, ExpiresAt: h.expiresAt}, nil
}

// Check ยืนยันว่า port เป็นใบจองที่ยังไม่หมดอายุของผู้ใช้คนนี้ในกลุ่มนี้
func (r *NodePortReservations) Check(userID, namespaceID, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.holds[port]
	if !ok || h.userID != userID || h.namespaceID != namespaceID || !r.now().Before(h.expiresAt) {
		return ErrNodePortReservationInvalid
	}
	return nil
}

// Release คืนใบจอง — หลัง deploy สำเร็จ (เลขอยู่ใน DB/คลัสเตอร์แล้ว) หรือเลขนั้นถูกคนนอกใช้ไปแล้ว
func (r *NodePortReservations) Release(port int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.holds, port)
}

func (r *NodePortReservations) dropExpired(now time.Time) {
	for p, h := range r.holds {
		if !now.Before(h.expiresAt) {
			delete(r.holds, p)
		}
	}
}

// usedPorts = พอร์ตที่ Service บนคลัสเตอร์ใช้อยู่ (รวมของระบบที่ไม่อยู่ใน DB) ∪ node_port ใน DB
// (DB ครอบ service ที่กำลัง deploy — แถวถูก INSERT พร้อมเลขก่อน Service บนคลัสเตอร์จะเกิด)
func (r *NodePortReservations) usedPorts(ctx context.Context) (map[int]bool, error) {
	used, err := r.prov.UsedNodePorts(ctx)
	if err != nil {
		return nil, err
	}
	var inDB []int
	if err := r.db.WithContext(ctx).Model(&entity.Service{}).
		Where("node_port IS NOT NULL").Pluck("node_port", &inDB).Error; err != nil {
		return nil, err
	}
	for _, p := range inDB {
		used[p] = true
	}
	return used, nil
}
