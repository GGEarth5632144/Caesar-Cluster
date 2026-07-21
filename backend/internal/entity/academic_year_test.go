package entity

import (
	"testing"
	"time"
)

func TestYearLevel(t *testing.T) {
	// วันที่ 2026-07-21 (หลังมิถุนายน) → ปีการศึกษา 2569, B66 เข้าปี 2566 → ชั้นปี 4
	now := time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC)

	level, err := YearLevel("B6600907", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != 4 {
		t.Fatalf("expected year level 4, got %d", level)
	}
}

func TestYearLevel_InvalidStudentID(t *testing.T) {
	if _, err := YearLevel("12345", time.Now()); err == nil {
		t.Fatal("expected error for student id without a leading letter, got nil")
	}
	if _, err := YearLevel("B6", time.Now()); err == nil {
		t.Fatal("expected error for too-short student id, got nil")
	}
}

func TestCurrentAcademicYearBE_JuneBoundary(t *testing.T) {
	beforeJune := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	if got := CurrentAcademicYearBE(beforeJune); got != 2568 {
		t.Fatalf("expected 2568 before June, got %d", got)
	}

	afterJune := time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC)
	if got := CurrentAcademicYearBE(afterJune); got != 2569 {
		t.Fatalf("expected 2569 after June, got %d", got)
	}
}
