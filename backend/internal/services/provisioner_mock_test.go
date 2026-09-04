package services

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// TestMockProvisionerLogsTail ยืนยันว่าโหมดไม่ follow ปิด stream เองหลังส่ง log ครบ (ไม่ใช่ค้างรอตลอดกาล)
// และจำนวนบรรทัดที่ได้ตรงกับ TailLines ที่ขอ (บวก 2 บรรทัดเปิดเครื่องที่ mock ใส่ให้เสมอ)
func TestMockProvisionerLogsTail(t *testing.T) {
	m := NewMockProvisioner()
	stream, err := m.Logs(context.Background(), "space-x", "web-a", LogOptions{TailLines: 5})
	if err != nil {
		t.Fatalf("Logs คืน error ไม่ควรเกิดขึ้นเลยสำหรับ mock: %v", err)
	}
	defer stream.Close()

	lines := readAllLines(t, stream, 5*time.Second)
	const wantLines = 2 + 5 // starting + listening + tail
	if len(lines) != wantLines {
		t.Fatalf("ได้ %d บรรทัด ต้องได้ %d บรรทัด (TailLines=5 ไม่ follow ต้องปิดเองพอดี): %v",
			len(lines), wantLines, lines)
	}
	if !strings.Contains(lines[0], "web-a") || !strings.Contains(lines[0], "space-x") {
		t.Errorf("บรรทัดแรกต้องมีชื่อ service และ namespace กำกับไว้ ได้: %q", lines[0])
	}
}

// TestMockProvisionerLogsTimestampPrefix ยืนยันว่าทุกบรรทัดขึ้นต้นด้วย RFC3339 timestamp
// ตามสัญญาเดียวกับที่ KubernetesProvisioner ส่งมาจริงตอน Timestamps=true (ดู provisioner_k8s.go)
// หน้าเว็บพึ่งรูปแบบนี้ในการแยก timestamp ออกจากเนื้อ log — ต้องตรงกันทั้งสอง provisioner
func TestMockProvisionerLogsTimestampPrefix(t *testing.T) {
	m := NewMockProvisioner()
	stream, err := m.Logs(context.Background(), "space-x", "web-a", LogOptions{TailLines: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	lines := readAllLines(t, stream, 5*time.Second)
	for _, line := range lines {
		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			t.Fatalf("บรรทัด %q ต้องมี timestamp คั่นด้วยช่องว่างก่อนเนื้อ log", line)
		}
		if _, err := time.Parse(time.RFC3339Nano, fields[0]); err != nil {
			t.Errorf("timestamp %q parse ด้วย RFC3339Nano ไม่ได้: %v", fields[0], err)
		}
	}
}

// TestMockProvisionerLogsStopsOnContextCancel ยืนยันว่าโหมด follow หยุดเขียนทันทีที่ ctx ถูก cancel
// (ผู้ใช้ปิดหน้าเว็บ) — ถ้าไม่หยุด goroutine จะรันค้างกินทรัพยากรไปเรื่อยๆ ทุกครั้งที่มีคนเปิดหน้า log
func TestMockProvisionerLogsStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewMockProvisioner()
	stream, err := m.Logs(ctx, "space-x", "web-a", LogOptions{TailLines: 1, Follow: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	// อ่าน 2 บรรทัดแรก (starting + listening) ให้แน่ใจว่า goroutine เริ่มทำงานแล้วจริงๆ
	r := bufio.NewReader(stream)
	for i := 0; i < 2; i++ {
		if _, err := r.ReadString('\n'); err != nil {
			t.Fatalf("อ่านบรรทัดที่ %d ไม่สำเร็จ: %v", i+1, err)
		}
	}

	cancel() // จำลองผู้ใช้ปิดหน้าเว็บ

	done := make(chan struct{})
	go func() {
		// เมื่อ ctx ถูก cancel goroutine ฝั่งเขียนต้องปิด pipe (EOF) ไม่ใช่เขียนต่อไปเรื่อยๆ
		for {
			if _, err := r.ReadString('\n'); err != nil {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		// ผ่าน — stream ปิดหลัง cancel ตามที่คาด
	case <-time.After(5 * time.Second):
		t.Fatal("stream ไม่ปิดภายใน 5 วินาทีหลัง cancel ctx — goroutine รั่วไหล")
	}
}

// readAllLines อ่านทุกบรรทัดจนกว่า stream จะปิด (EOF) มี timeout กันเทสต์ค้างถ้า mock พังแล้วไม่ปิด stream
func readAllLines(t *testing.T, r io.Reader, timeout time.Duration) []string {
	t.Helper()
	done := make(chan []string, 1)
	go func() {
		var lines []string
		br := bufio.NewReader(r)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				lines = append(lines, strings.TrimRight(line, "\n"))
			}
			if err != nil {
				done <- lines
				return
			}
		}
	}()

	select {
	case lines := <-done:
		return lines
	case <-time.After(timeout):
		t.Fatal("readAllLines timeout — stream ไม่ปิดเองตามที่คาด (ไม่ follow ต้องปิดหลังส่ง log ครบ)")
		return nil
	}
}
