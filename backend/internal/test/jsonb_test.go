package test

import (
	"reflect"
	"testing"

	"backend/internal/entity"
)

func TestJSONB_ValueScan_RoundTrip(t *testing.T) {
	original := entity.JSONB{"gpu": false, "note": "dev"}

	dv, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	raw, ok := dv.([]byte)
	if !ok {
		t.Fatalf("Value() ต้องคืน []byte ได้ (จำลองสิ่งที่ driver ส่งเข้า Scan ต่อ), ได้ %T แทน", dv)
	}

	var scanned entity.JSONB
	if err := scanned.Scan(raw); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if !reflect.DeepEqual(original, scanned) {
		t.Errorf("round-trip ไม่ตรงกัน: original=%v scanned=%v", original, scanned)
	}
}

func TestJSONB_Scan_Nil(t *testing.T) {
	var j entity.JSONB
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) ไม่ควร error: %v", err)
	}
	if j != nil {
		t.Errorf("Scan(nil) ควรได้ nil map, ได้ %v แทน", j)
	}
}
