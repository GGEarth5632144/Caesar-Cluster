package services

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// errRollback ใช้คืนจาก transaction ของเทสต์เพื่อให้ทุกอย่างถูก rollback — ไม่ทิ้งแถวไว้ใน DB ทดสอบ
var errRollback = errors.New("rollback")

// TestCheckStoragePool ตรวจด่านดิสก์รวม (NFS_POOL_GB) กับ Postgres จริง
// (pg_advisory_xact_lock + SUM ต้องรันบน Postgres) — DB ทดสอบอาจมี namespace ค้างอยู่ก่อน
// จึงตั้งเพดานเป็น "ยอดที่จองอยู่แล้ว + 2 GB" แทนการสมมติว่าตารางว่าง
func TestCheckStoragePool(t *testing.T) {
	db := testDB(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		var reservedMB int
		if err := tx.Model(&entity.Namespace{}).
			Select("COALESCE(SUM(storage_limit_mb), 0)").Scan(&reservedMB).Error; err != nil {
			t.Fatalf("อ่านยอดดิสก์ที่จองไว้ไม่สำเร็จ: %v", err)
		}
		m := &NamespaceManager{poolMB: reservedMB + 2048}

		if err := m.checkStoragePool(tx, 0, 2048); err != nil {
			t.Errorf("จองพอดีเพดานต้องผ่าน ได้ %v", err)
		}
		if err := m.checkStoragePool(tx, 0, 2049); !errors.Is(err, ErrStoragePoolFull) {
			t.Errorf("จองเกินเพดาน 1 MB ต้องได้ ErrStoragePoolFull ได้ %v", err)
		}

		// ปรับโควตาของ namespace ที่มีอยู่: ค่าเดิมของตัวเองต้องไม่ถูกนับซ้ำ
		ns := &entity.Namespace{Name: "pool-test-ns", ContributorID: 1,
			CPULimitMilli: 100, RAMLimitMB: 128, StorageLimitMB: 1024}
		if err := tx.Create(ns).Error; err != nil {
			t.Fatalf("สร้าง namespace ทดสอบไม่สำเร็จ: %v", err)
		}
		// ยอดรวมตอนนี้ = reserved + 1024 · ขยายตัวเองเป็น 3072 = reserved + 3072 > เพดาน → ต้องไม่ผ่าน
		if err := m.checkStoragePool(tx, ns.ID, 3072); !errors.Is(err, ErrStoragePoolFull) {
			t.Errorf("ขยายเกินเพดานต้องได้ ErrStoragePoolFull ได้ %v", err)
		}
		// ขยายเป็น 2048 = reserved + 2048 พอดีเพดาน (ไม่นับ 1024 เดิมซ้ำ) → ต้องผ่าน
		if err := m.checkStoragePool(tx, ns.ID, 2048); err != nil {
			t.Errorf("ขยายพอดีเพดานต้องผ่าน (ไม่นับค่าเดิมของตัวเอง) ได้ %v", err)
		}

		// ปิดด่าน (poolMB = 0) หรือขอดิสก์ 0 → ผ่านเสมอ
		off := &NamespaceManager{}
		if err := off.checkStoragePool(tx, 0, 1<<30); err != nil {
			t.Errorf("poolMB = 0 ต้องไม่เช็ค ได้ %v", err)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("transaction ของเทสต์จบผิดปกติ: %v", err)
	}
}
