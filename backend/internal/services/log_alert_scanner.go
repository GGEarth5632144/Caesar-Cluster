package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend/internal/entity"
)

// ตัวจับระดับความรุนแรงจากบรรทัด log — เรียงจาก "มั่นใจมาก" ไป "มั่นใจน้อย"
//
// ที่ต้องแบ่งชั้นแทนการ grep คำว่า error ตรงๆ เพราะ log ของ container ไม่มีมาตรฐานกลาง:
// จับหลวมไป access log อย่าง `GET /error-page 200` กลายเป็นแจ้งเตือน / จับแน่นไป (ต้องมี level=)
// แอปที่พิมพ์ `panic: ...` เฉยๆ หลุดหมด จึงไล่จากรูปที่ตีความผิดยากที่สุดไปหาคำลอยๆ เป็นด่านท้าย
var (
	// ชั้น 1: log แบบมีโครงสร้าง — level=error, "severity":"critical", lvl=fatal
	structuredErrorRe = regexp.MustCompile(`(?i)(^|[\s\[{("',])(level|lvl|severity)["']?\s*[=:]\s*["']?(error|fatal|critical|crit|panic|emerg)\b`)
	structuredWarnRe  = regexp.MustCompile(`(?i)(^|[\s\[{("',])(level|lvl|severity)["']?\s*[=:]\s*["']?(warn|warning)\b`)

	// ชั้น 2: ป้ายระดับคั่นด้วยวงเล็บ/โคลอน/ขีด — [error] ERROR: | E |
	// พบมากที่สุดในของจริง (nginx, postgres, java, python logging ใช้รูปนี้กันหมด)
	labeledErrorRe = regexp.MustCompile(`(?i)(^|[\s\[|(])(error|fatal|panic|critical|crit|emerg|severe)\s*[\]:|)\-]`)
	labeledWarnRe  = regexp.MustCompile(`(?i)(^|[\s\[|(])(warn|warning)\s*[\]:|)\-]`)

	// ชั้น 3: วลีที่แปลว่าพังโดยตัวมันเอง ไม่ต้องมีป้ายระดับกำกับ
	hardFailureRe = regexp.MustCompile(`(?i)\b(panic:|fatal:|segmentation fault|out of memory|oomkilled|stack overflow|` +
		`unhandled exception|uncaught exception|traceback \(most recent call last\)|` +
		`connection refused|connection reset by peer|no such host|permission denied|` +
		`crashloopbackoff|failed to |failed with |cannot connect|could not connect|` +
		`address already in use|too many open files|deadlock detected)`)

	// ชั้น 4 (อ่อนที่สุด): คำว่า error/exception ลอยๆ — ไม่รวม failed/failure เพราะแอปจำนวนมาก
	// พิมพ์ "0 failed" ในบรรทัดสรุปปกติ
	//
	// ไม่ใช้ \b เพราะ \b ถือว่า - / . _ เป็นขอบคำ ทำให้ `GET /static/error-page.html 200`
	// กลายเป็นแจ้งเตือนทั้งที่ "error" เป็นแค่ชื่อไฟล์ จึงบังคับว่าตัวขนาบต้องไม่ใช่อักขระใน path
	looseErrorRe = regexp.MustCompile(`(?i)(^|[^\w/.\-])(errors?|exceptions?)([^\w/.\-]|$)`)

	// HTTP 5xx ใน access log — ต้องอยู่ในรูปที่เป็น status code จริง ไม่ใช่เลขอะไรก็ได้ที่ขึ้นต้นด้วย 5
	// (ไม่งั้น "GET / 200 512ms" โดนจับเพราะมี 512)
	httpServerErrorRe = regexp.MustCompile(`"\s*[A-Z]{3,7} [^"]*"\s+5\d{2}\b|(?i)\b(status|status_code|code)["']?\s*[=:]\s*["']?5\d{2}\b`)

	// กันบวกปลอมที่เจอบ่อยที่สุด: บรรทัดสรุปที่บอกว่าไม่มี error
	// ("0 errors", "errors=0", "error_count: 0", "no errors found")
	noErrorRe = regexp.MustCompile(`(?i)\b(0|no|zero|none)\s+(errors?|exceptions?|failures?)\b|` +
		`(?i)\b(errors?|error_count|failures?)["']?\s*[=:]\s*["']?(0|false|null|none)\b`)

	// ตัวเลข/hex/uuid ที่ต่างกันทุกบรรทัดแต่เป็น error เดียวกัน — แทนที่ก่อนคำนวณ fingerprint
	// ส่วนตัวเลขไม่ครอบด้วย \b เพราะเลขมักติดหน่วยโดยไม่มีขอบคำคั่น (1230ms, 8080/tcp)
	// ถ้าบังคับ \b ท้าย เลขพวกนี้จะรอดไปทั้งก้อน แล้ว retry แต่ละครั้งกลายเป็นคนละ fingerprint
	fingerprintNoiseRe = regexp.MustCompile(`(?i)\b(0x)?[0-9a-f]{8,}\b|\d+`)
)

// classifyLogLine ตัดสินว่าบรรทัด log นี้ควรกลายเป็นแจ้งเตือนหรือไม่ และรุนแรงระดับไหน
//
// คืน ("", false) = บรรทัดปกติ ไม่ต้องแจ้ง
// คืน (entity.SeverityCritical, true) = error จริง
// คืน (entity.SeverityWarning, true) = คำเตือน (จะถูกส่งต่อหรือไม่ ขึ้นกับ LogScanConfig.IncludeWarnings)
func classifyLogLine(text string) (string, bool) {
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	if noErrorRe.MatchString(text) {
		return "", false
	}

	switch {
	case structuredErrorRe.MatchString(text),
		labeledErrorRe.MatchString(text),
		hardFailureRe.MatchString(text),
		httpServerErrorRe.MatchString(text):
		return entity.SeverityCritical, true

	case structuredWarnRe.MatchString(text), labeledWarnRe.MatchString(text):
		return entity.SeverityWarning, true

	case looseErrorRe.MatchString(text):
		return entity.SeverityCritical, true
	}
	return "", false
}

// alertFingerprint ย่อบรรทัด log ให้เหลือแก่นของเรื่อง แล้วแฮชเป็นตัวชี้ว่าซ้ำกันหรือไม่
//
// ตัวเลขถูกแทนด้วย # ก่อนแฮช เพราะสองบรรทัดนี้เป็นปัญหาเดียวกันเป๊ะ ต่างกันแค่เลขที่ไม่มีความหมาย
//
//	connection refused to 10.0.0.5:5432 (attempt 17)
//	connection refused to 10.0.0.5:5432 (attempt 18)
//
// ถ้าไม่ตัดทิ้ง แต่ละ retry จะได้แจ้งเตือนคนละแถว ซึ่งทำให้ยุบเรื่องซ้ำไม่ได้เลย
func alertFingerprint(serviceID int, text string) string {
	normalized := fingerprintNoiseRe.ReplaceAllString(strings.ToLower(text), "#")
	normalized = strings.Join(strings.Fields(normalized), " ")
	if len(normalized) > 300 {
		normalized = normalized[:300]
	}
	sum := sha256.Sum256([]byte(strconv.Itoa(serviceID) + "|" + normalized))
	return hex.EncodeToString(sum[:16])
}

// LogScanConfig = ค่าที่คุมพฤติกรรมของตัวสแกน (มาจาก env ผ่าน config.Config)
type LogScanConfig struct {
	Enabled         bool
	Interval        time.Duration // ทุกกี่วินาทีจะเดินสแกนหนึ่งรอบ
	MaxLinesPerScan int64         // ดึง log ย้อนหลังสูงสุดกี่บรรทัดต่อ service ต่อรอบ
	IncludeWarnings bool          // true = ส่ง warning เข้าหน้า Alerts ด้วย (default ส่งเฉพาะ error)
}

// logFinding = ผลสรุปของ error หนึ่ง "เรื่อง" ที่เจอในรอบสแกนของ service หนึ่งตัว
type logFinding struct {
	severity    string
	fingerprint string
	sample      string // บรรทัดตัวอย่างล่าสุดที่เจอ (เอาไปโชว์เป็น message)
	count       int
	lastTS      time.Time
}

// LogAlertScanner เดินอ่าน log ของทุก service ที่ running อยู่เป็นรอบๆ หา error แล้วสร้าง UserAlert
//
// data flow ต่อ 1 รอบ:
//
//	SELECT services ที่ status=running + ชื่อ namespace ของมัน
//	→ ต่อ service: prov.Logs(SinceSeconds=หน้าต่างเวลา, Timestamps=true) อ่านจนจบ (ไม่ follow)
//	→ classifyLogLine ทีละบรรทัด → ยุบบรรทัดที่ fingerprint เดียวกันเข้าด้วยกัน
//	→ AlertManager.Raise ให้สมาชิกทุกคนใน namespace นั้น
//
// เลือก poll เป็นรอบแทน follow ค้างทุก service เพราะ follow ต้องเปิด HTTP connection ค้างกับ
// Kubernetes API หนึ่งเส้นต่อหนึ่ง service ตลอดเวลา ซึ่งบน control-plane ตัวเดียวคือภาระถาวร
// ที่โตตามจำนวน service ส่วน poll มีต้นทุนคงที่ และ "ช้าไปหนึ่งรอบ" รับได้สำหรับระบบแจ้งเตือน
// (ไม่ใช่ monitoring แบบ real-time ที่ต้องรู้ภายในวินาที)
type LogAlertScanner struct {
	db     *gorm.DB
	prov   Provisioner
	alerts *AlertManager
	cfg    LogScanConfig

	// cursors = timestamp ของบรรทัดสุดท้ายที่ประมวลผลไปแล้ว แยกตาม service id
	//
	// จำเป็นเพราะแต่ละรอบดึง log ย้อนหลังกว้างกว่าระยะห่างของรอบ (เผื่อ tick เพี้ยน/รอบก่อนช้า)
	// บรรทัดช่วงคาบเกี่ยวจึงถูกอ่านซ้ำ — cursor คือตัวกันไม่ให้นับซ้ำ
	// เก็บใน memory พอ: restart แล้ว cursor หาย อย่างมากคือแจ้งเตือนซ้ำหนึ่งรอบ
	// ซึ่งชั้นยุบด้วย fingerprint ใน AlertManager.Raise รับไว้อยู่แล้ว
	cursors map[int]time.Time
}

// NewLogAlertScanner ประกอบตัวสแกน — ถูกเรียกจาก main แล้วสั่ง Run ใน goroutine แยก
func NewLogAlertScanner(db *gorm.DB, prov Provisioner, alerts *AlertManager, cfg LogScanConfig) *LogAlertScanner {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MaxLinesPerScan <= 0 {
		cfg.MaxLinesPerScan = 500
	}
	return &LogAlertScanner{db: db, prov: prov, alerts: alerts, cfg: cfg, cursors: map[int]time.Time{}}
}

// Run เดินลูปสแกนไปเรื่อยๆ จนกว่า ctx จะถูก cancel (ตอน server ปิด)
// ตั้งใจให้ error ของรอบใดรอบหนึ่งไม่ทำให้ลูปตาย — แค่ log ไว้แล้วไปรอบหน้าต่อ
// เพราะคลัสเตอร์ที่ยังไม่พร้อม/service ที่เพิ่งถูกลบ เป็นเรื่องปกติที่เกิดได้ตลอด
func (s *LogAlertScanner) Run(ctx context.Context) {
	if !s.cfg.Enabled {
		log.Println("log alert scanner: ปิดอยู่ (ALERT_SCAN_ENABLED=false)")
		return
	}
	log.Printf("log alert scanner: เริ่มทำงาน สแกนทุก %s", s.cfg.Interval)

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("log alert scanner: หยุดทำงาน")
			return
		case <-ticker.C:
			if err := s.ScanOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("log alert scanner: รอบนี้ไม่สำเร็จ: %v", err)
			}
		}
	}
}

// serviceRow = ผลลัพธ์ของ join services × namespaces ที่ตัวสแกนต้องใช้ต่อ 1 service
type serviceRow struct {
	ID            int
	Name          string
	NamespaceID   int
	NamespaceName string
}

// ScanOnce เดินสแกนหนึ่งรอบ — แยกออกมาเป็น method สาธารณะเพื่อให้เทสต์เรียกตรงได้
// โดยไม่ต้องรอ ticker (และเพื่อให้ main เรียกครั้งแรกได้ทันทีถ้าอยากได้ผลเร็ว)
func (s *LogAlertScanner) ScanOnce(ctx context.Context) error {
	var rows []serviceRow
	err := s.db.WithContext(ctx).
		Table("services AS s").
		Select("s.id AS id, s.name AS name, s.namespace_id AS namespace_id, n.name AS namespace_name").
		Joins("JOIN namespaces n ON n.id = s.namespace_id").
		Where("s.status = ?", entity.ServiceRunning).
		Scan(&rows).Error
	if err != nil {
		return err
	}

	s.pruneCursors(rows)

	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.scanService(ctx, row); err != nil {
			// service ตัวเดียวอ่าน log ไม่ได้ (เพิ่งถูกลบ / pod ยังไม่ขึ้น / image pull ค้าง)
			// ไม่ควรทำให้ service ที่เหลือในรอบนี้ถูกข้ามไปด้วย
			log.Printf("log alert scanner: อ่าน log ของ %s/%s ไม่สำเร็จ: %v", row.NamespaceName, row.Name, err)
		}
	}
	return nil
}

// pruneCursors ลบ cursor ของ service ที่ไม่ได้อยู่ในรายการแล้ว (ถูกลบ/หยุดไปแล้ว)
// ไม่งั้น map นี้จะโตขึ้นเรื่อยๆ ตามจำนวน service ที่เคยผ่านระบบมาทั้งหมดตลอดอายุ process
func (s *LogAlertScanner) pruneCursors(rows []serviceRow) {
	alive := make(map[int]struct{}, len(rows))
	for _, r := range rows {
		alive[r.ID] = struct{}{}
	}
	for id := range s.cursors {
		if _, ok := alive[id]; !ok {
			delete(s.cursors, id)
		}
	}
}

// scanService ดึง log ของ service ตัวเดียวมาสแกน แล้วสร้างแจ้งเตือนให้สมาชิกทุกคนใน namespace
func (s *LogAlertScanner) scanService(ctx context.Context, row serviceRow) error {
	// ดึงย้อนหลังกว้างกว่าระยะห่างของรอบ 2 เท่า เผื่อรอบก่อนใช้เวลานานจนมีช่วงที่ไม่มีใครอ่าน
	// ส่วนที่คาบเกี่ยวกันถูกตัดทิ้งด้วย cursor อยู่แล้ว จึงไม่เกิดแจ้งเตือนซ้ำ
	since := int64(s.cfg.Interval.Seconds() * 2)
	if since < 1 {
		since = 1
	}

	stream, err := s.prov.Logs(ctx, row.NamespaceName, row.Name, LogOptions{
		Timestamps:   true,
		SinceSeconds: since,
		TailLines:    s.cfg.MaxLinesPerScan,
		Follow:       false, // สำคัญ: ห้าม follow ไม่งั้นลูปนี้จะไม่มีวันจบ
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	findings, newCursor := s.collectFindings(stream, row.ID, s.cursors[row.ID])
	if !newCursor.IsZero() {
		s.cursors[row.ID] = newCursor
	}
	if len(findings) == 0 {
		return nil
	}

	memberIDs, err := s.namespaceMembers(ctx, row.NamespaceID)
	if err != nil {
		return err
	}
	if len(memberIDs) == 0 {
		return nil
	}

	serviceID := row.ID
	for _, f := range findings {
		raiseErr := s.alerts.Raise(ctx, RaiseParams{
			UserIDs:     memberIDs,
			Severity:    f.severity,
			Title:       alertTitleFor(f.severity, row.Name),
			Message:     f.sample,
			SourceType:  entity.AlertSourceServiceLog,
			SourceName:  row.Name,
			ServiceID:   &serviceID,
			Fingerprint: f.fingerprint,
			Count:       f.count,
		})
		if raiseErr != nil {
			return raiseErr
		}
	}
	return nil
}

// maxFindingsPerScan = จำนวน "เรื่อง" สูงสุดที่ยอมสร้างจาก service เดียวในรอบเดียว
// container ที่พังหนักๆ พ่น error คนละข้อความได้เป็นร้อยแบบในไม่กี่วินาที (เช่น stack trace
// ที่ทุกเฟรมเป็นคนละบรรทัด) ถ้าไม่จำกัด หน้า Alerts จะถูกกลบด้วย service ตัวเดียว
const maxFindingsPerScan = 5

// collectFindings อ่าน stream ทีละบรรทัด แล้วยุบบรรทัดที่เป็นเรื่องเดียวกันเข้าด้วยกัน
// คืน findings ที่พร้อมสร้างเป็นแจ้งเตือน + timestamp ของบรรทัดสุดท้าย (cursor ของรอบถัดไป)
//
// รับ io.Reader ตรงๆ เพื่อให้เทสต์ป้อน log ปลอมเข้ามาตรวจได้โดยไม่ต้องมี provisioner
func (s *LogAlertScanner) collectFindings(r io.Reader, serviceID int, cursor time.Time) ([]logFinding, time.Time) {
	sc := bufio.NewScanner(r)
	// บรรทัดเดียวยาวเกิน default 64KB ของ bufio ได้ (เช่น JSON ก้อนใหญ่) — ขยายเพดานไว้
	// ไม่งั้น Scanner หยุดกลางคันด้วย ErrTooLong แล้วเราจะเงียบไปทั้ง service
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	byFingerprint := map[string]*logFinding{}
	order := make([]string, 0, maxFindingsPerScan)
	lastTS := cursor

	for sc.Scan() {
		ts, text := splitLogTimestamp(sc.Text())

		// บรรทัดที่เคยประมวลผลไปแล้วในรอบก่อน (ช่วงเวลาที่ดึงมาคาบเกี่ยวกัน)
		if !ts.IsZero() && !cursor.IsZero() && !ts.After(cursor) {
			continue
		}
		if ts.After(lastTS) {
			lastTS = ts
		}

		severity, hit := classifyLogLine(text)
		if !hit {
			continue
		}
		if severity == entity.SeverityWarning && !s.cfg.IncludeWarnings {
			continue
		}

		fp := alertFingerprint(serviceID, text)
		if existing, ok := byFingerprint[fp]; ok {
			existing.count++
			existing.sample = text // เก็บตัวอย่างล่าสุดเสมอ
			if ts.After(existing.lastTS) {
				existing.lastTS = ts
			}
			continue
		}
		if len(order) >= maxFindingsPerScan {
			continue
		}
		byFingerprint[fp] = &logFinding{
			severity: severity, fingerprint: fp, sample: text, count: 1, lastTS: ts,
		}
		order = append(order, fp)
	}

	out := make([]logFinding, 0, len(order))
	for _, fp := range order {
		out = append(out, *byFingerprint[fp])
	}
	return out, lastTS
}

// namespaceMembers คืน id ของ user ทุกคนที่สังกัด namespace นี้
// (service เป็นของทั้งกลุ่ม สมาชิกทุกคนจึงควรได้รับแจ้งเตือนเหมือนกัน)
func (s *LogAlertScanner) namespaceMembers(ctx context.Context, namespaceID int) ([]int, error) {
	var ids []int
	err := s.db.WithContext(ctx).Model(&entity.User{}).
		Where("namespace_id = ?", namespaceID).Pluck("id", &ids).Error
	return ids, err
}

// splitLogTimestamp แยก timestamp ที่ container runtime ใส่นำหน้ามา (Timestamps=true)
// ออกจากเนื้อ log — รูปแบบเดียวกับที่ frontend ใช้ parse ใน src/api/logs.ts
//
// บรรทัดที่ไม่มี timestamp นำหน้า (ไม่ควรเกิด แต่กันไว้) คืน zero time แล้วถือทั้งบรรทัดเป็นเนื้อ log
func splitLogTimestamp(line string) (time.Time, string) {
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return time.Time{}, line
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:sp])
	if err != nil {
		return time.Time{}, line
	}
	return ts, line[sp+1:]
}

// alertTitleFor ตั้งหัวข้อแจ้งเตือนให้อ่านแล้วรู้เรื่องทันทีจากหน้ารายการ โดยไม่ต้องกดเข้าไปดู
//
// ตัดตามจำนวน "ตัวอักษร" ไม่ใช่จำนวนไบต์ เพราะ varchar(100) ของ Postgres นับเป็นตัวอักษร
// และหัวข้อขึ้นต้นด้วยภาษาไทย (ตัวละ 3 ไบต์) — ตัดด้วย title[:100] จะสั้นกว่าที่คอลัมน์รับได้จริง
// และถ้าจุดตัดไปลงกลางตัวอักษรจะได้ UTF-8 ที่ Postgres ปฏิเสธทั้ง INSERT
func alertTitleFor(severity, serviceName string) string {
	label := "พบ error ใน service"
	if severity == entity.SeverityWarning {
		label = "พบคำเตือนใน service"
	}
	title := []rune(label + " " + serviceName)
	if len(title) > maxAlertTitleRunes {
		title = title[:maxAlertTitleRunes]
	}
	return string(title)
}

// maxAlertTitleRunes = ความยาวสูงสุดของ UserAlert.Title ต้องตรงกับ varchar(100) ใน entity
const maxAlertTitleRunes = 100
