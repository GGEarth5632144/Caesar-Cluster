package services

import (
	"strings"
	"testing"
	"time"

	"backend/internal/entity"
)

// TestClassifyLogLine ยืนยันว่าตัวจับ error แยกบรรทัดที่ควรแจ้งเตือนออกจากบรรทัดปกติได้จริง
//
// กรณี "ไม่ควรจับ" สำคัญไม่แพ้กรณี "ควรจับ": ถ้าจับ access log ปกติไปด้วย ผู้ใช้จะได้แจ้งเตือน
// ทุกนาทีจนเลิกสนใจตัวเลขบน Sidebar ไปเลย ซึ่งแย่กว่าไม่มีฟีเจอร์นี้ตั้งแต่แรก
func TestClassifyLogLine(t *testing.T) {
	errorLines := []string{
		`level=error msg="failed to connect to database"`,
		`{"level":"fatal","msg":"cannot bind port"}`,
		`lvl=critical something broke`,
		`2024/01/02 [error] 31#31: *1 open() "/usr/share/nginx/html/x" failed`,
		`ERROR: relation "users" does not exist`,
		`[ERROR] java.lang.NullPointerException`,
		`panic: runtime error: index out of range [3]`,
		`Traceback (most recent call last):`,
		`dial tcp 10.0.0.5:5432: connect: connection refused`,
		`nginx: [emerg] chown("/var/cache/nginx/client_temp", 101) failed`,
		`Error: listen EADDRINUSE: address already in use :::8080`,
		`10.244.0.1 - - "GET /api/items HTTP/1.1" 503 0`,
		`msg=timeout status=500`,
		`OOMKilled`,
		`failed to pull image "ghcr.io/x/y:v1"`,
	}
	for _, line := range errorLines {
		sev, hit := classifyLogLine(line)
		if !hit {
			t.Errorf("ควรจับได้ว่าเป็น error แต่ปล่อยผ่าน: %q", line)
			continue
		}
		if sev != entity.SeverityCritical {
			t.Errorf("severity ควรเป็น %q แต่ได้ %q: %q", entity.SeverityCritical, sev, line)
		}
	}

	warnLines := []string{
		`level=warn msg="disk usage above 80%"`,
		`[warning] cache miss ratio high`,
		`WARN: deprecated flag --foo`,
	}
	for _, line := range warnLines {
		sev, hit := classifyLogLine(line)
		if !hit || sev != entity.SeverityWarning {
			t.Errorf("ควรจับเป็น warning แต่ได้ (%q, %v): %q", sev, hit, line)
		}
	}

	normalLines := []string{
		``,
		`   `,
		`10.244.0.1 - - "GET / HTTP/1.1" 200 612 "-" "curl/8.0"`,
		`[mock] GET / 200 512ms`,
		`level=info msg="server listening on :8080"`,
		`Server started in 512ms`,
		`request completed with 0 errors`,
		`error_count: 0`,
		`errors=0 warnings=0`,
		`GET /static/error-page.html 200`,
		`processed 500 records`,
		`healthz ok`,
	}
	for _, line := range normalLines {
		if sev, hit := classifyLogLine(line); hit {
			t.Errorf("ไม่ควรจับบรรทัดปกติ แต่จับเป็น %q: %q", sev, line)
		}
	}
}

// TestAlertFingerprintCollapsesNoise ยืนยันว่า error เดียวกันที่ต่างกันแค่ตัวเลข/ที่อยู่หน่วยความจำ
// ได้ fingerprint เดียวกัน (จะได้ยุบเป็นแจ้งเตือนแถวเดียว) ส่วน error คนละเรื่องต้องได้คนละตัว
func TestAlertFingerprintCollapsesNoise(t *testing.T) {
	same := []string{
		`level=error msg="connection refused" attempt=17 elapsed=1230ms`,
		`level=error msg="connection refused" attempt=18 elapsed=1417ms`,
		`level=error msg="connection refused" attempt=999 elapsed=88ms`,
	}
	first := alertFingerprint(1, same[0])
	for _, line := range same[1:] {
		if got := alertFingerprint(1, line); got != first {
			t.Errorf("บรรทัดที่ต่างกันแค่ตัวเลขควรได้ fingerprint เดียวกัน\n  %q\n  %q", same[0], line)
		}
	}

	if alertFingerprint(1, same[0]) == alertFingerprint(1, `level=error msg="permission denied"`) {
		t.Error("error คนละเรื่องไม่ควรได้ fingerprint เดียวกัน")
	}
	// service คนละตัวที่พ่นข้อความเดียวกัน ต้องเป็นคนละเรื่อง ไม่งั้นแจ้งเตือนของ service หนึ่ง
	// จะไปยุบรวมกับอีก service หนึ่งจนผู้ใช้ไม่รู้ว่าตัวไหนพัง
	if alertFingerprint(1, same[0]) == alertFingerprint(2, same[0]) {
		t.Error("service คนละตัวควรได้ fingerprint คนละอัน")
	}
}

// newTestScanner สร้างตัวสแกนที่ไม่มี db/provisioner — พอสำหรับทดสอบ collectFindings
// ซึ่งรับ io.Reader ตรงๆ ไม่แตะทั้งสองอย่าง
func newTestScanner(includeWarnings bool) *LogAlertScanner {
	return NewLogAlertScanner(nil, nil, nil, LogScanConfig{
		Enabled: true, Interval: time.Minute, IncludeWarnings: includeWarnings,
	})
}

// logStream ประกอบบรรทัด log ให้อยู่ในรูปที่ provisioner ส่งจริง: "<RFC3339Nano> <เนื้อ log>"
func logStream(base time.Time, lines ...string) string {
	var b strings.Builder
	for i, line := range lines {
		b.WriteString(base.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano))
		b.WriteByte(' ')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestCollectFindingsCollapsesRepeats — error บรรทัดเดิมซ้ำ 4 รอบต้องกลายเป็น "เรื่องเดียว count=4"
// ไม่ใช่ 4 เรื่อง (นี่คือหัวใจที่ทำให้หน้า Alerts ไม่ถูกกลบด้วย container ที่พังค้าง)
func TestCollectFindingsCollapsesRepeats(t *testing.T) {
	s := newTestScanner(false)
	base := time.Now().Add(-time.Minute)
	stream := logStream(base,
		`GET / 200 12ms`,
		`level=error msg="connection refused" attempt=1`,
		`GET / 200 9ms`,
		`level=error msg="connection refused" attempt=2`,
		`level=error msg="connection refused" attempt=3`,
		`level=error msg="connection refused" attempt=4`,
	)

	findings, cursor := s.collectFindings(strings.NewReader(stream), 1, time.Time{})
	if len(findings) != 1 {
		t.Fatalf("ควรได้ 1 เรื่อง แต่ได้ %d เรื่อง", len(findings))
	}
	if findings[0].count != 4 {
		t.Errorf("count ควรเป็น 4 แต่ได้ %d", findings[0].count)
	}
	if !strings.Contains(findings[0].sample, "attempt=4") {
		t.Errorf("sample ควรเป็นบรรทัดล่าสุด แต่ได้ %q", findings[0].sample)
	}
	if !cursor.After(base) {
		t.Errorf("cursor ควรขยับไปที่บรรทัดสุดท้าย แต่ได้ %v", cursor)
	}
}

// TestCollectFindingsSkipsAlreadySeen — บรรทัดที่ timestamp เก่ากว่า cursor ต้องถูกข้าม
// นี่คือกลไกที่กันไม่ให้ช่วงเวลาที่ดึงมาคาบเกี่ยวกันระหว่างสองรอบ กลายเป็นแจ้งเตือนซ้ำ
func TestCollectFindingsSkipsAlreadySeen(t *testing.T) {
	s := newTestScanner(false)
	base := time.Now().Add(-time.Minute)
	stream := logStream(base,
		`level=error old failure`, // t+0s — อ่านไปแล้วรอบก่อน
		`level=error old failure`, // t+1s — อ่านไปแล้วรอบก่อน
		`level=error new failure`, // t+2s — ของใหม่
	)

	cursor := base.Add(1500 * time.Millisecond)
	findings, newCursor := s.collectFindings(strings.NewReader(stream), 1, cursor)
	if len(findings) != 1 {
		t.Fatalf("ควรเหลือแค่บรรทัดใหม่ 1 เรื่อง แต่ได้ %d เรื่อง", len(findings))
	}
	if !strings.Contains(findings[0].sample, "new failure") {
		t.Errorf("ควรได้บรรทัดใหม่ แต่ได้ %q", findings[0].sample)
	}
	if !newCursor.After(cursor) {
		t.Error("cursor ต้องขยับไปข้างหน้าเสมอ")
	}

	// สแกนซ้ำด้วย cursor ที่ขยับแล้ว = ไม่มีอะไรใหม่ (ไม่แจ้งเตือนซ้ำ)
	again, _ := s.collectFindings(strings.NewReader(stream), 1, newCursor)
	if len(again) != 0 {
		t.Errorf("สแกน log ชุดเดิมซ้ำไม่ควรได้อะไรเพิ่ม แต่ได้ %d เรื่อง", len(again))
	}
}

// TestCollectFindingsWarningGate — warning ต้องถูกกรองทิ้งตาม default ("ส่งเฉพาะที่มัน error")
// และต้องผ่านเข้ามาเมื่อเปิด IncludeWarnings
func TestCollectFindingsWarningGate(t *testing.T) {
	stream := logStream(time.Now().Add(-time.Minute),
		`level=warn msg="slow response"`,
		`level=error msg="boom"`,
	)

	off, _ := newTestScanner(false).collectFindings(strings.NewReader(stream), 1, time.Time{})
	if len(off) != 1 || off[0].severity != entity.SeverityCritical {
		t.Fatalf("ปิด IncludeWarnings ต้องเหลือแต่ error แต่ได้ %+v", off)
	}

	on, _ := newTestScanner(true).collectFindings(strings.NewReader(stream), 1, time.Time{})
	if len(on) != 2 {
		t.Fatalf("เปิด IncludeWarnings ต้องได้ทั้งคู่ แต่ได้ %d เรื่อง", len(on))
	}
}

// TestCollectFindingsCapsDistinctIssues — service ที่พ่น error คนละแบบเป็นสิบๆ ในรอบเดียว
// (เช่น stack trace ที่ทุกเฟรมเป็นคนละบรรทัด) ต้องไม่สร้างแจ้งเตือนเกินเพดานต่อรอบ
func TestCollectFindingsCapsDistinctIssues(t *testing.T) {
	lines := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		lines = append(lines, `level=error msg="distinct failure kind `+string(rune('a'+i))+`"`)
	}
	stream := logStream(time.Now().Add(-time.Minute), lines...)

	findings, _ := newTestScanner(false).collectFindings(strings.NewReader(stream), 1, time.Time{})
	if len(findings) != maxFindingsPerScan {
		t.Errorf("ควรถูกจำกัดไว้ที่ %d เรื่องต่อรอบ แต่ได้ %d", maxFindingsPerScan, len(findings))
	}
}

// TestSplitLogTimestamp ยืนยันว่าแยก timestamp ออกจากเนื้อ log ได้ตรงกับที่ frontend parse
// (src/api/logs.ts ใช้กติกาเดียวกัน: ตัดที่ช่องว่างตัวแรก)
func TestSplitLogTimestamp(t *testing.T) {
	want := time.Now().UTC().Truncate(time.Millisecond)
	ts, text := splitLogTimestamp(want.Format(time.RFC3339Nano) + " hello world")
	if !ts.Equal(want) {
		t.Errorf("timestamp เพี้ยน: อยากได้ %v ได้ %v", want, ts)
	}
	if text != "hello world" {
		t.Errorf("เนื้อ log เพี้ยน: ได้ %q", text)
	}

	// บรรทัดที่ไม่มี timestamp นำหน้า ต้องถือทั้งบรรทัดเป็นเนื้อ log ไม่ใช่ตัดคำแรกทิ้ง
	if ts, text := splitLogTimestamp("no timestamp here"); !ts.IsZero() || text != "no timestamp here" {
		t.Errorf("บรรทัดไม่มี timestamp ควรคืนทั้งบรรทัด แต่ได้ (%v, %q)", ts, text)
	}
}

// TestMockLogLineProducesErrors — mock ต้องมีบรรทัด error ปนอยู่จริง ไม่งั้นทดสอบฟีเจอร์
// แจ้งเตือนบนเครื่อง dev (PROVISIONER=mock) ไม่ได้เลย
func TestMockLogLineProducesErrors(t *testing.T) {
	var errs, warns, normal int
	for n := int64(0); n < 100; n++ {
		switch sev, hit := classifyLogLine(mockLogLine(n)); {
		case !hit:
			normal++
		case sev == entity.SeverityWarning:
			warns++
		default:
			errs++
		}
	}
	if errs == 0 || warns == 0 || normal == 0 {
		t.Errorf("mock ควรมีครบทั้งสามแบบใน 100 บรรทัด แต่ได้ error=%d warn=%d ปกติ=%d", errs, warns, normal)
	}
	// ส่วนใหญ่ต้องยังเป็นบรรทัดปกติ ไม่งั้น log viewer จะดูเหมือนระบบพังตลอดเวลา
	if normal < 60 {
		t.Errorf("บรรทัดปกติควรเป็นส่วนใหญ่ แต่มีแค่ %d จาก 100", normal)
	}
}
