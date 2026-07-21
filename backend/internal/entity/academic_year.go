package entity

import (
	"fmt"
	"strconv"
	"time"
)

// CurrentAcademicYearBE คืนปีการศึกษาปัจจุบัน (พ.ศ.) จากเวลาที่ให้มา
// ปีการศึกษาไทยเริ่มราวเดือนมิถุนายน — ก่อนมิถุนายนถือว่ายังอยู่ปีการศึกษาก่อนหน้า
func CurrentAcademicYearBE(t time.Time) int {
	be := t.Year() + 543
	if t.Month() < time.June {
		be--
	}
	return be
}

// EntryYearFromStudentID แกะปีที่เข้าศึกษา (พ.ศ.) จาก prefix ของรหัสนักศึกษา
// รูปแบบที่รองรับ: 1 ตัวอักษร + เลข 2 หลัก (เช่น "B66..." → เข้าปี 2566)
func EntryYearFromStudentID(studentID string) (int, error) {
	if len(studentID) < 3 {
		return 0, fmt.Errorf("รหัสนักศึกษา %q สั้นเกินไปจะแกะปีที่เข้าศึกษา", studentID)
	}
	c := studentID[0]
	if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return 0, fmt.Errorf("รหัสนักศึกษา %q ไม่ได้ขึ้นต้นด้วยตัวอักษร", studentID)
	}
	yy, err := strconv.Atoi(studentID[1:3])
	if err != nil {
		return 0, fmt.Errorf("รหัสนักศึกษา %q ไม่มีปีที่เข้าศึกษาแบบตัวเลข 2 หลัก: %w", studentID, err)
	}
	return 2500 + yy, nil
}

// YearLevel คำนวณชั้นปีปัจจุบันของ นศ. จากรหัสนักศึกษา เทียบกับเวลา now ที่ให้มา
// สูตร: ปีการศึกษาปัจจุบัน - ปีที่เข้าศึกษา + 1
// จงใจไม่เก็บผลลัพธ์นี้ลง DB — คำนวณสดทุกครั้งที่ต้องใช้ ชั้นปีจะได้ไม่ค้าง (เลื่อนขึ้นเองทุกปีการศึกษาใหม่)
func YearLevel(studentID string, now time.Time) (int, error) {
	entryYear, err := EntryYearFromStudentID(studentID)
	if err != nil {
		return 0, err
	}
	return CurrentAcademicYearBE(now) - entryYear + 1, nil
}
