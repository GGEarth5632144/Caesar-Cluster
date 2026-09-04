package mailer

import (
	"context"
	"mime"
	"net/mail"
	"strings"
	"testing"
)

// testConfig = ค่า SMTP ปลอมที่ "ครบพอให้ผ่านด่าน Configured" — เทสต์ในไฟล์นี้ไม่แตะเน็ตเลย
// ทุกเคสหยุดก่อนถึงขั้น dial (ประกอบข้อความอย่างเดียว หรือพังตั้งแต่ตรวจที่อยู่ปลายทาง)
func testConfig() Config {
	return Config{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "no-reply@example.com",
		Password: "app-password",
		FromName: "Caesar Cluster",
	}
}

func testOutgoing() outgoing {
	return outgoing{
		purpose: PurposeVerification,
		to:      "student@g.sut.ac.th",
		subject: "ยืนยันอีเมลของคุณ — Caesar Cluster",
		text:    "สวัสดี",
		html:    "<p>สวัสดี</p>",
	}
}

func buildForTest(t *testing.T, cfg Config) string {
	t.Helper()
	to, err := mail.ParseAddress("student@g.sut.ac.th")
	if err != nil {
		t.Fatalf("ที่อยู่ทดสอบใช้ไม่ได้: %v", err)
	}
	msg, err := New(cfg, nil).buildMessage(*to, testOutgoing(), "<abc@example.com>")
	if err != nil {
		t.Fatalf("buildMessage ล้มเหลว: %v", err)
	}
	return string(msg)
}

// TestBuildMessageHasDeliverabilityHeaders คุม header ชุดที่มีผลกับการเข้า inbox โดยตรง
// ขาดตัวใดตัวหนึ่งไปแล้วเมลจะถูกหักคะแนนเงียบๆ ซึ่งเป็นบั๊กที่ไม่มีทางเห็นจาก log ฝั่งเรา
func TestBuildMessageHasDeliverabilityHeaders(t *testing.T) {
	got := buildForTest(t, testConfig())

	for _, want := range []string{
		"From: \"Caesar Cluster\" <no-reply@example.com>",
		"To: <student@g.sut.ac.th>",
		"Message-ID: <abc@example.com>",
		"MIME-Version: 1.0",
		"Auto-Submitted: auto-generated",
		"X-Auto-Response-Suppress: All",
		"Content-Language: th",
		"Content-Type: multipart/alternative;",
		"Date: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ไม่พบ header %q ในเมลที่ประกอบได้", want)
		}
	}
}

// TestBuildMessageEncodesThaiSubject — หัวข้อภาษาไทยที่ไม่ผ่าน RFC 2047 จะขึ้นเป็นตัวขยะฝั่งผู้รับ
// เทียบกับผลของ decoder จริงแทนการเทียบสตริงที่ encode ไว้ตายตัว จะได้ไม่ผูกกับรูปแบบการ encode
func TestBuildMessageEncodesThaiSubject(t *testing.T) {
	got := buildForTest(t, testConfig())

	var raw string
	for _, line := range strings.Split(got, "\r\n") {
		if after, ok := strings.CutPrefix(line, "Subject: "); ok {
			raw = after
			break
		}
	}
	if raw == "" {
		t.Fatal("ไม่พบ header Subject")
	}
	if strings.Contains(raw, "ยืนยัน") {
		t.Error("Subject ภาษาไทยถูกใส่ดิบๆ ไม่ได้ encode ตาม RFC 2047")
	}

	decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
	if err != nil {
		t.Fatalf("decode Subject ไม่ได้: %v", err)
	}
	if decoded != testOutgoing().subject {
		t.Errorf("Subject หลัง decode = %q, ต้องการ %q", decoded, testOutgoing().subject)
	}
}

// TestBuildMessageHasBothTextAndHTMLParts — เมลที่มีแต่ HTML ก้อนเดียวคือลายเซ็นสแปมคลาสสิก
// (SpamAssassin: MIME_HTML_ONLY) ส่วน text ล้วนจึงไม่ใช่ของประดับ ต้องมีจริงทุกฉบับ
func TestBuildMessageHasBothTextAndHTMLParts(t *testing.T) {
	got := buildForTest(t, testConfig())

	for _, want := range []string{
		`Content-Type: text/plain; charset="UTF-8"`,
		`Content-Type: text/html; charset="UTF-8"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ไม่พบส่วน %q — เมลต้องมีทั้ง text และ HTML", want)
		}
	}
	if strings.Index(got, "text/plain") > strings.Index(got, "text/html") {
		t.Error("ส่วน text ต้องมาก่อน HTML ตาม RFC 2046 (ท้ายสุด = ส่วนที่อยากให้แสดงมากที่สุด)")
	}
}

// TestReplyToHeaderOptional — ตั้ง MAIL_REPLY_TO แล้วต้องมี header, ไม่ตั้งแล้วต้องไม่มี
// และค่าที่ผิดรูปแบบต้องถูกมองข้าม ไม่ใช่ปล่อยให้ header พังติดไปทั้งฉบับ
func TestReplyToHeaderOptional(t *testing.T) {
	if got := buildForTest(t, testConfig()); strings.Contains(got, "Reply-To:") {
		t.Error("ไม่ได้ตั้ง ReplyTo แต่ยังมี header Reply-To ติดไป")
	}

	cfg := testConfig()
	cfg.ReplyTo = "admin@example.com"
	if got := buildForTest(t, cfg); !strings.Contains(got, "Reply-To: <admin@example.com>") {
		t.Error("ตั้ง ReplyTo แล้วแต่ไม่มี header Reply-To")
	}

	cfg.ReplyTo = "ไม่ใช่อีเมล"
	if got := buildForTest(t, cfg); strings.Contains(got, "Reply-To:") {
		t.Error("ค่า ReplyTo ที่ผิดรูปแบบต้องถูกมองข้าม ไม่ใช่ใส่ลงไปทั้งอย่างนั้น")
	}
}

// recorderSpy = Recorder ปลอมไว้ดูว่า Mailer รายงานผลการส่งจริงหรือไม่
type recorderSpy struct{ got []Delivery }

func (r *recorderSpy) RecordEmail(_ context.Context, d Delivery) { r.got = append(r.got, d) }

// TestSendRecordsFailures — ทั้งระบบพึ่งตาราง email_deliveries ในการตอบว่า "ส่งไปจริงไหม"
// ทางที่ล้มเหลวหลุดไปโดยไม่ถูกบันทึกแย่กว่าไม่มีตารางเลย ใช้ที่อยู่ผิดรูปแบบให้พังก่อนถึงขั้น dial
func TestSendRecordsFailures(t *testing.T) {
	spy := &recorderSpy{}
	m := New(testConfig(), spy)

	out := testOutgoing()
	out.to = "ไม่ใช่อีเมล"
	if err := m.send(context.Background(), out); err == nil {
		t.Fatal("ที่อยู่ปลายทางผิดรูปแบบแต่ send ไม่คืน error")
	}

	if len(spy.got) != 1 {
		t.Fatalf("ต้องบันทึกผล 1 รายการ แต่ได้ %d", len(spy.got))
	}
	rec := spy.got[0]
	if rec.Err == nil {
		t.Error("บันทึกผลแล้วแต่ไม่มี error ติดไปด้วย")
	}
	if rec.Purpose != PurposeVerification || rec.ToEmail != out.to {
		t.Errorf("บันทึกผลผิดรายการ: purpose=%q to=%q", rec.Purpose, rec.ToEmail)
	}
	if rec.MessageID == "" {
		t.Error("ต้องบันทึก Message-ID ไว้เสมอ ไม่งั้นตามหาเมลฉบับนั้นย้อนหลังไม่ได้")
	}
}

// TestSendRejectsUnconfiguredMailer — ไม่ตั้ง SMTP แล้วต้องคืน error ทันทีโดยไม่ไปต่อ
// (Register พึ่งพฤติกรรมนี้ในการ rollback แทนที่จะสร้างบัญชีค้างไว้)
func TestSendRejectsUnconfiguredMailer(t *testing.T) {
	m := New(Config{Host: "smtp.example.com", Port: 587}, nil)
	if m.Configured() {
		t.Fatal("ไม่ได้ตั้ง username/password แต่ Configured คืน true")
	}
	if err := m.send(context.Background(), testOutgoing()); err == nil {
		t.Error("ไม่ได้ตั้ง SMTP แต่ send ไม่คืน error")
	}
}

// TestActionEmailEscapesNameInHTMLOnly — ชื่อผู้ใช้เป็นค่าเดียวในเมลที่มาจากผู้ใช้
// ฝั่ง HTML ต้อง escape (กันแทรก markup) ส่วนฝั่ง text ต้องไม่ escape ไม่งั้นคนอ่านจะเห็น &amp;
func TestActionEmailEscapesNameInHTMLOnly(t *testing.T) {
	content := actionEmail{
		preheader: "p",
		intro:     "i",
		button:    "b",
		link:      "https://example.com/verify-email?token=abc",
		footnote:  "f",
	}
	const name = "<script>x</script>"

	if got := content.html(name); strings.Contains(got, "<script>") {
		t.Error("ชื่อผู้ใช้ฝั่ง HTML ไม่ถูก escape")
	}
	if got := content.text(name); !strings.Contains(got, name) {
		t.Error("ชื่อผู้ใช้ฝั่ง text ไม่ควรถูก escape")
	}
	if got := content.html(""); !strings.Contains(got, "สวัสดี") {
		t.Error("ไม่มีชื่อก็ต้องยังทักทายได้")
	}
	if got := content.html(name); !strings.Contains(got, content.link) {
		t.Error("ลิงก์ปลายทางหายไปจากเนื้อเมล")
	}
}
