// Package mailer ส่งอีเมลผ่าน SMTP ตรงๆ ด้วย net/smtp ใน stdlib กับบัญชี Gmail แบบ no-reply
// จึงไม่ต้องพึ่งบริการส่งเมลภายนอกและไม่มี API key ให้ดูแล
//
// Gmail ปิดการล็อกอิน SMTP ด้วยรหัสผ่านบัญชีปกติตั้งแต่ปี 2022 — ต้องเปิด 2-Step Verification
// แล้วสร้าง "App Password" 16 ตัวมาใส่ SMTP_PASSWORD แทน (ขั้นตอนอยู่ใน .env.example)
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// smtpTimeout = เพดานเวลาของการคุยกับ SMTP server หนึ่งครั้ง (ตั้งแต่ dial ยัน QUIT)
// กัน handler ที่ส่งเมลค้างยาวถ้า Gmail ไม่ตอบ — ฝั่ง client รอ response อยู่
const smtpTimeout = 15 * time.Second

// implicitTLSPort = พอร์ตที่ห่อ TLS ตั้งแต่ต้น (SMTPS) ไม่ได้เริ่มด้วย plaintext แล้ว STARTTLS
// Gmail เปิดทั้ง 587 (STARTTLS) และ 465 (implicit) — เลือกได้จาก SMTP_PORT
const implicitTLSPort = 465

// ค่า purpose ที่ Mailer ติดไปกับ Delivery — ตรงกับค่าคงที่ฝั่ง entity.EmailDelivery
// ประกาศซ้ำไว้ที่นี่เพราะ package mailer ไม่ import entity (เหตุผลเดียวกับที่ Recorder เป็น interface)
const (
	PurposeVerification  = "verification"
	PurposePasswordReset = "password_reset"
)

// Config = ค่าที่ Mailer ต้องใช้ต่อกับ SMTP server — map ตรงกับ env SMTP_*/MAIL_* ใน config.Config
type Config struct {
	Host     string // เช่น smtp.gmail.com
	Port     int    // 587 = STARTTLS (ค่าปกติ), 465 = TLS ตั้งแต่ต้น
	Username string // อีเมลเต็มของบัญชี no-reply เช่น caesar.cluster.noreply@gmail.com
	Password string // App Password 16 ตัวของบัญชีนั้น (ไม่ใช่รหัสผ่านที่ใช้ล็อกอิน Google)
	FromName string // ชื่อที่แสดงหน้าอีเมลผู้ส่ง เช่น "Caesar Cluster"
	// ReplyTo = อีเมลจริงที่ผู้ใช้ตอบกลับได้ (ว่าง = ไม่ใส่ header Reply-To)
	//
	// ไม่ใช่แค่ความสะดวก: กล่อง no-reply ที่ไม่มีทางติดต่อกลับเลยเป็นลักษณะที่ตัวกรองสแปม
	// หักคะแนน และเป็นสิ่งที่คู่มือผู้ส่งของ Gmail แนะนำให้มี
	ReplyTo string
}

// Delivery = ผลการส่งอีเมลหนึ่งฉบับ ส่งต่อให้ Recorder บันทึกไว้
// Err = nil แปลว่า SMTP server ตอบรับ (250) แล้ว — ดูข้อจำกัดของคำว่า "ส่งสำเร็จ" ที่ entity.EmailDelivery
type Delivery struct {
	UserID    *int
	ToEmail   string
	Purpose   string
	MessageID string
	Err       error
	Duration  time.Duration
}

// Recorder = ปลายทางที่ Mailer เขียนผลการส่งทุกฉบับลงไป (ปกติคือ services.MailJournal)
// เป็น interface เพื่อไม่ให้ package นี้รู้จัก gorm/entity และตั้งใจไม่คืน error — บันทึกล้มเหลว
// ต้องไม่ทำให้อีเมลที่ส่งออกไปแล้วกลายเป็นล้มเหลวตาม
type Recorder interface {
	RecordEmail(ctx context.Context, d Delivery)
}

// Mailer ถือค่า SMTP ไว้ใช้ซ้ำ สร้างครั้งเดียวตอน start (ดู controller.NewAuthController)
// ไม่ถือ connection ค้างเพราะระบบนี้ส่งเมลนานๆ ครั้ง คาไว้มีแต่จะโดน Gmail ตัดแล้วต้อง reconnect เอง
type Mailer struct {
	cfg      Config
	recorder Recorder
}

// New ประกอบ Mailer — ค่าทั้งหมดมาจาก config.Config, recorder เป็น nil ได้ (แปลว่าไม่บันทึกผลลง DB)
func New(cfg Config, recorder Recorder) *Mailer {
	return &Mailer{cfg: cfg, recorder: recorder}
}

// Configured บอกว่าตั้งค่า SMTP ครบพอที่จะส่งเมลได้หรือยัง
// ให้ที่เรียกใช้ตัดสินใจล่วงหน้าได้ (เช่น Register ไม่ต้องเปิด transaction ถ้ารู้อยู่แล้วว่าส่งไม่ออก)
func (m *Mailer) Configured() bool {
	return m.cfg.Username != "" && m.cfg.Password != ""
}

// VerifyConnection ลองต่อ SMTP + ล็อกอินจริงหนึ่งรอบแล้ววางสาย ไม่ส่งเมลถึงใคร
// เรียกตอน server start จะได้รู้ตั้งแต่นาทีแรกว่าค่า SMTP ใช้ได้จริง ไม่ใช่ไปรู้ตอนคนแรกกดสมัคร
func (m *Mailer) VerifyConnection(ctx context.Context) error {
	client, err := m.dial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := m.authenticate(client); err != nil {
		return err
	}
	return client.Quit()
}

// SendPasswordResetEmail ส่งอีเมลพร้อมลิงก์รีเซ็ตรหัสผ่านให้ผู้ใช้
//
// data flow: ประกอบ HTML + text จาก actionEmail → ห่อเป็นข้อความ MIME พร้อม header
// → เปิด SMTP session ไป Gmail (STARTTLS + PLAIN auth) → MAIL/RCPT/DATA → QUIT → บันทึกผลลง Recorder
func (m *Mailer) SendPasswordResetEmail(ctx context.Context, userID int, toEmail, toName, resetLink string, ttlMinutes int) error {
	content := actionEmail{
		preheader: fmt.Sprintf("ลิงก์สำหรับตั้งรหัสผ่านใหม่ของคุณ หมดอายุใน %d นาที", ttlMinutes),
		intro:     "เราได้รับคำขอรีเซ็ตรหัสผ่านสำหรับบัญชีของคุณ กดปุ่มด้านล่างเพื่อตั้งรหัสผ่านใหม่",
		button:    "ตั้งรหัสผ่านใหม่",
		link:      resetLink,
		footnote: fmt.Sprintf("ลิงก์นี้จะหมดอายุใน %d นาที และใช้ได้เพียงครั้งเดียว "+
			"ถ้าคุณไม่ได้ร้องขอการรีเซ็ตรหัสผ่าน กรุณาเพิกเฉยต่ออีเมลฉบับนี้ — บัญชีของคุณยังปลอดภัยดี", ttlMinutes),
	}
	return m.send(ctx, outgoing{
		purpose: PurposePasswordReset,
		userID:  &userID,
		to:      toEmail,
		subject: "รีเซ็ตรหัสผ่าน Caesar Cluster",
		text:    content.text(toName),
		html:    content.html(toName),
	})
}

// SendVerificationEmail ส่งลิงก์ยืนยันอีเมลให้ผู้ที่เพิ่งสมัคร (มาแทนรหัส OTP 6 หลักแบบเดิม)
//
// ตั้งใจไม่ใส่ลิงก์ไว้ใน subject/preheader — สองที่นี้คือส่วนที่โผล่บนหน้าจอล็อกของมือถือ
// คนที่หยิบเครื่องขึ้นมาดูเฉยๆ ไม่ควรกดยืนยันแทนเจ้าตัวได้โดยไม่ต้องปลดล็อกเครื่องก่อน
func (m *Mailer) SendVerificationEmail(ctx context.Context, userID int, toEmail, toName, verifyLink string, ttlHours int) error {
	content := actionEmail{
		preheader: fmt.Sprintf("ยืนยันอีเมลเพื่อเปิดใช้งานบัญชีของคุณ ลิงก์หมดอายุใน %d ชั่วโมง", ttlHours),
		intro: "ขอบคุณที่สมัครใช้งาน Caesar Cluster — เหลืออีกขั้นเดียวเท่านั้น " +
			"กดปุ่มด้านล่างเพื่อยืนยันว่าอีเมลนี้เป็นของคุณจริง แล้วจะเข้าสู่ระบบได้ทันที",
		button: "ยืนยันอีเมลของฉัน",
		link:   verifyLink,
		footnote: fmt.Sprintf("ลิงก์นี้จะหมดอายุใน %d ชั่วโมง และใช้ได้เพียงครั้งเดียว "+
			"ถ้าคุณไม่ได้เป็นคนสมัคร กรุณาเพิกเฉยต่ออีเมลฉบับนี้ — บัญชีจะไม่ถูกเปิดใช้งานถ้าไม่มีใครกดยืนยัน", ttlHours),
	}
	return m.send(ctx, outgoing{
		purpose: PurposeVerification,
		userID:  &userID,
		to:      toEmail,
		subject: "ยืนยันอีเมลของคุณ — Caesar Cluster",
		text:    content.text(toName),
		html:    content.html(toName),
	})
}

// outgoing = อีเมลหนึ่งฉบับที่พร้อมส่งแล้ว รวมพารามิเตอร์ไว้เป็นก้อนเดียวแทนการรับเป็น 6 อาร์กิวเมนต์เรียงกัน
type outgoing struct {
	purpose string
	userID  *int
	to      string
	subject string
	text    string
	html    string
}

// send คุยกับ SMTP หนึ่งรอบเพื่อส่งอีเมลหนึ่งฉบับ แล้วบันทึกผลลง Recorder เสมอ
// "สำเร็จ" = ได้รหัสตอบรับ 2xx ครบ แปลว่า Gmail รับช่วงต่อ ไม่ได้แปลว่าเข้ากล่องขาเข้าผู้รับ
func (m *Mailer) send(ctx context.Context, out outgoing) error {
	started := time.Now()
	messageID := newMessageID(m.cfg.Username)
	err := m.deliver(ctx, out, messageID)

	if m.recorder != nil {
		m.recorder.RecordEmail(ctx, Delivery{
			UserID:    out.userID,
			ToEmail:   out.to,
			Purpose:   out.purpose,
			MessageID: messageID,
			Err:       err,
			Duration:  time.Since(started),
		})
	}
	return err
}

// deliver = ขั้นตอน SMTP จริงๆ แยกจาก send เพื่อให้ทุก error ผ่านจุดบันทึกผลจุดเดียวกันหมด
//
// ลำดับ: dial → (STARTTLS ถ้าไม่ใช่ 465) → AUTH → MAIL FROM → RCPT TO → DATA → QUIT
// QUIT สำคัญ: ไม่เรียกแล้ว Gmail อาจถือว่า session ถูกตัดกลางคันแล้วไม่ส่งจริง จึงต้องคืน error ของมันด้วย
func (m *Mailer) deliver(ctx context.Context, out outgoing, messageID string) error {
	if !m.Configured() {
		return errors.New("mailer: ยังไม่ได้ตั้ง SMTP_USERNAME / SMTP_PASSWORD")
	}
	// ที่อยู่ปลายทางถูกเอาไปวางใน header To: ตรงๆ — ถ้ามี CR/LF ปนมาจะกลายเป็นการแทรก header เพิ่มได้
	// (email header injection) mail.ParseAddress ตรวจให้ครบทั้งรูปแบบและอักขระต้องห้ามในตัวเดียว
	recipient, err := mail.ParseAddress(out.to)
	if err != nil {
		return fmt.Errorf("mailer: อีเมลปลายทางไม่ถูกต้อง: %w", err)
	}

	msg, err := m.buildMessage(*recipient, out, messageID)
	if err != nil {
		return fmt.Errorf("mailer: ประกอบเนื้อเมลไม่สำเร็จ: %w", err)
	}

	client, err := m.dial(ctx)
	if err != nil {
		return err
	}
	defer client.Close() // ปิด connection ทิ้งเสมอ แม้ทางที่ error ก่อนถึง Quit

	if err := m.authenticate(client); err != nil {
		return err
	}
	// MAIL FROM ต้องเป็นบัญชีที่เพิ่ง auth ไป ไม่ใช่ชื่อที่โชว์ใน header From:
	// Gmail ปฏิเสธ (หรือเขียนทับ) ถ้าใส่ที่อยู่อื่นที่ไม่ได้ตั้งเป็น alias ไว้
	if err := client.Mail(m.cfg.Username); err != nil {
		return fmt.Errorf("mailer: MAIL FROM ไม่ผ่าน: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("mailer: RCPT TO ไม่ผ่าน (ปลายทางปฏิเสธที่อยู่ %s): %w", recipient.Address, err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: เริ่มส่ง DATA ไม่สำเร็จ: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: เขียนเนื้อเมลไม่สำเร็จ: %w", err)
	}
	// Close คือจุดที่อ่านรหัสตอบรับของทั้งฉบับ (250 = server รับเข้าคิวแล้ว) ไม่ใช่แค่ปิด writer เฉยๆ
	// error ตรงนี้จึงแปลว่า "ปลายทางไม่รับเมลฉบับนี้" ซึ่งเป็นจุดที่ต้องไม่กลืนทิ้งเด็ดขาด
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: ปลายทางไม่รับเนื้อเมล: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mailer: QUIT ไม่สำเร็จ (server อาจไม่ได้ส่งเมลจริง): %w", err)
	}
	return nil
}

// dial เปิด TCP + TLS ไปยัง SMTP server แล้วคืน client ที่พร้อมทำ AUTH
// ตั้ง deadline เดียวครอบทุกขั้นเพราะ net/smtp ไม่มี timeout เอง ถ้า server ค้างกลาง DATA จะรอไม่จบ
func (m *Mailer) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	dialer := &net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mailer: ต่อ %s ไม่ได้: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	if m.cfg.Port == implicitTLSPort {
		conn = tls.Client(conn, tlsCfg)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mailer: เริ่ม SMTP session ไม่สำเร็จ: %w", err)
	}

	if m.cfg.Port != implicitTLSPort {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// ไม่ยอมส่งต่อแบบ plaintext เพราะขั้นถัดไปคือส่ง App Password ข้ามเน็ต
			client.Close()
			return nil, errors.New("mailer: server ไม่รองรับ STARTTLS — ไม่ส่งรหัสผ่านผ่านช่องที่ไม่เข้ารหัส")
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			client.Close()
			return nil, fmt.Errorf("mailer: STARTTLS ไม่สำเร็จ: %w", err)
		}
	}
	return client, nil
}

// authenticate ล็อกอิน SMTP ด้วย App Password — แยกออกมาเพราะทั้งการส่งจริงและ VerifyConnection ใช้ร่วมกัน
func (m *Mailer) authenticate(client *smtp.Client) error {
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("mailer: ล็อกอิน SMTP ด้วยบัญชี %s ไม่ผ่าน "+
			"(Gmail ต้องใช้ App Password 16 ตัว ไม่ใช่รหัสผ่านบัญชีปกติ): %w", m.cfg.Username, err)
	}
	return nil
}

// buildMessage ห่อเนื้อหาเป็นข้อความ MIME ที่ SMTP รับได้
//
// multipart/alternative ใส่ทั้ง text ล้วนและ HTML ในฉบับเดียว — ที่ต้องมี text ด้วยเป็นเรื่อง
// เข้า inbox ไม่ใช่ความสวยงาม เมลที่มีแต่ HTML คือลายเซ็นสแปม (SpamAssassin: MIME_HTML_ONLY)
//
// จุดที่พลาดง่าย: หัวข้อภาษาไทยต้อง encode ตาม RFC 2047, บรรทัดยาวเกิน 998 ตัวต้องเข้ารหัสด้วย
// quoted-printable (ไม่ใช่ base64 ซึ่งตัวกรองหักคะแนน), ทุกบรรทัดต้องจบด้วย CRLF
func (m *Mailer) buildMessage(to mail.Address, out outgoing, messageID string) ([]byte, error) {
	from := mail.Address{Name: m.cfg.FromName, Address: m.cfg.Username}

	var msg bytes.Buffer
	mp := multipart.NewWriter(&msg) // สร้างก่อนเพื่อรู้ boundary ตอนเขียน header

	headers := []string{
		"From: " + from.String(),
		"To: " + to.String(),
	}
	// Reply-To ชี้ไปกล่องที่มีคนอ่านจริง — กล่อง no-reply ที่ติดต่อกลับไม่ได้เลยถูกตัวกรองหักคะแนน
	if replyTo := m.replyToHeader(); replyTo != "" {
		headers = append(headers, "Reply-To: "+replyTo)
	}
	headers = append(headers,
		"Subject: "+mime.QEncoding.Encode("UTF-8", out.subject),
		"Date: "+time.Now().Format(time.RFC1123Z),
		"Message-ID: "+messageID,
		// บอกว่าเป็นเมลที่ระบบสร้างเอง กันตัวตอบกลับอัตโนมัติยิงกลับเป็นลูป (RFC 3834)
		// ใส่คู่กับ X-Auto-Response-Suppress เพราะ Microsoft 365 ไม่ดู Auto-Submitted
		"Auto-Submitted: auto-generated",
		"X-Auto-Response-Suppress: All",
		"Content-Language: th",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="`+mp.Boundary()+`"`,
	)
	for _, h := range headers {
		msg.WriteString(h + "\r\n")
	}
	msg.WriteString("\r\n")

	// เรียง text ก่อน HTML ตาม RFC 2046: ส่วนท้ายสุดคือส่วนที่ "อยากให้แสดงมากที่สุด"
	for _, part := range []struct{ contentType, content string }{
		{`text/plain; charset="UTF-8"`, out.text},
		{`text/html; charset="UTF-8"`, out.html},
	} {
		w, err := mp.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mp.Close(); err != nil {
		return nil, err
	}
	return msg.Bytes(), nil
}

// replyToHeader คืนค่า header Reply-To ที่ encode แล้ว (ว่าง = ไม่ต้องใส่ header นี้)
// ค่าที่ตั้งมาผิดรูปแบบให้ถือว่าไม่ได้ตั้ง — header ที่พังทำให้ทั้งฉบับดูน่าสงสัยกว่าการไม่มี header
func (m *Mailer) replyToHeader() string {
	if m.cfg.ReplyTo == "" {
		return ""
	}
	addr, err := mail.ParseAddress(m.cfg.ReplyTo)
	if err != nil {
		return ""
	}
	return addr.String()
}

// newMessageID สร้าง header Message-ID ให้เมลแต่ละฉบับ — เมลที่ไม่มีถูกตัวกรองหักคะแนน
// (SpamAssassin: MISSING_MID) และค่านี้ถูกเก็บลง email_deliveries ไว้ตามหาเมลฉบับนั้นย้อนหลัง
func newMessageID(fromAddress string) string {
	domain := "localhost"
	if at := strings.LastIndex(fromAddress, "@"); at >= 0 && at+1 < len(fromAddress) {
		domain = fromAddress[at+1:]
	}
	var randomPart [16]byte
	if _, err := rand.Read(randomPart[:]); err != nil {
		// สุ่มไม่ได้ก็ยังต้องมี Message-ID ติดไป — ค่าที่ซ้ำได้ยังดีกว่าไม่มี header เลย
		return fmt.Sprintf("<%d@%s>", time.Now().UnixNano(), domain)
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(randomPart[:]), domain)
}

// actionEmail = เนื้อหาของอีเมลแบบ "มีปุ่มให้กดหนึ่งปุ่ม" ใช้ร่วมกันทั้งลิงก์ยืนยันและลิงก์รีเซ็ตรหัสผ่าน
// รวมเป็น template เดียวเพราะสองฉบับนี้ต้องหน้าตาเหมือนกันตลอด ไม่งั้นวันหนึ่งจะมีฝั่งเดียวที่ถูกแก้
type actionEmail struct {
	preheader string // ข้อความตัวอย่างที่โผล่ข้างหัวข้อในรายการ inbox
	intro     string // ย่อหน้าอธิบายว่าทำไมถึงได้เมลฉบับนี้
	button    string // ข้อความบนปุ่ม
	link      string // URL ปลายทางของปุ่ม (ระบบสร้างเอง: origin + token hex จึงไม่ต้อง escape)
	footnote  string // บรรทัดท้ายเรื่องอายุลิงก์ + จะทำอย่างไรถ้าไม่ได้เป็นคนขอ
}

// actionEmailTemplate = โครง HTML วางแบบ table-based layout + inline style ล้วน
// (จำเป็นสำหรับอีเมล — client อย่าง Outlook ไม่รองรับ flexbox/grid/external CSS) ใช้โทนสีเดียวกับ
// หน้า Login/Register จริงของแอป (#BB6653 การ์ด, #FFF8E8 พื้นหลังครีม, #211a14 ตัวอักษรเข้ม)
const actionEmailTemplate = `<!DOCTYPE html>
<html lang="th">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Caesar Cluster</title>
  </head>
  <body style="margin:0;padding:0;background-color:#FFF8E8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <span style="display:none;max-height:0;max-width:0;overflow:hidden;">__PREHEADER__</span>
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#FFF8E8;">
      <tr>
        <td align="center" style="padding:40px 16px;">
          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;background-color:#ffffff;border-radius:24px;overflow:hidden;">
            <tr>
              <td style="background-color:#BB6653;padding:36px 32px;text-align:center;">
                <div style="font-size:26px;font-weight:700;color:#FFF8E8;letter-spacing:.3px;">Caesar Cluster</div>
                <div style="margin-top:6px;font-size:13px;color:rgba(255,248,232,.8);">Cloud for CPE Students</div>
              </td>
            </tr>
            <tr>
              <td style="padding:40px 36px 8px;">
                <p style="margin:0 0 16px;font-size:15px;line-height:1.6;color:#211a14;">__GREETING__,</p>
                <p style="margin:0;font-size:15px;line-height:1.6;color:#211a14;">__INTRO__</p>
              </td>
            </tr>
            <tr>
              <td align="center" style="padding:32px 36px;">
                <table role="presentation" cellpadding="0" cellspacing="0">
                  <tr>
                    <td align="center" style="border-radius:9999px;background-color:#BB6653;">
                      <a href="__LINK__" style="display:inline-block;padding:14px 40px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;border-radius:9999px;">__BUTTON__</a>
                    </td>
                  </tr>
                </table>
              </td>
            </tr>
            <tr>
              <td style="padding:0 36px 32px;">
                <p style="margin:0 0 8px;font-size:13px;color:#8a7d72;">หรือคัดลอกลิงก์นี้ไปเปิดในเบราว์เซอร์:</p>
                <p style="margin:0;font-size:13px;word-break:break-all;">
                  <a href="__LINK__" style="color:#BB6653;text-decoration:underline;">__LINK__</a>
                </p>
              </td>
            </tr>
            <tr>
              <td style="padding:0 36px;">
                <hr style="border:none;border-top:1px solid #f0e6d6;margin:0;" />
              </td>
            </tr>
            <tr>
              <td style="padding:24px 36px 36px;">
                <p style="margin:0;font-size:12px;line-height:1.6;color:#8a7d72;">__FOOTNOTE__</p>
              </td>
            </tr>
          </table>
          <p style="max-width:520px;margin:20px 0 0;font-size:11px;color:#a89a8c;text-align:center;">อีเมลนี้ส่งจากระบบอัตโนมัติของ Caesar Cluster</p>
        </td>
      </tr>
    </table>
  </body>
</html>`

// actionEmailTextTemplate = เวอร์ชัน text ล้วนของอีเมลฉบับเดียวกัน ต้องสื่อความครบเท่า HTML
// เพราะตัวกรองเทียบสองส่วนนี้ (ต่างกันมาก = สัญญาณของการซ่อนเนื้อหาจากตัวสแกน)
const actionEmailTextTemplate = `__GREETING__,

__INTRO__

__LINK__

__FOOTNOTE__

--
Caesar Cluster - Cloud for CPE Students
อีเมลนี้ส่งจากระบบอัตโนมัติ
`

// html/text ประกอบเนื้อความจาก template — escape เฉพาะชื่อผู้ใช้ซึ่งเป็นค่าเดียวที่มาจากผู้ใช้
// ใช้ NewReplacer ไม่ใช่ Sprintf เพราะ template มี "%" เยอะ (width:100% ฯลฯ) ซึ่งชนกับ verb
func (a actionEmail) html(toName string) string {
	return a.replacer(html.EscapeString(toName)).Replace(actionEmailTemplate)
}

// text ประกอบเนื้อความ text ล้วน — ไม่ต้อง escape ชื่อเพราะ text ล้วนไม่มี markup ให้แทรก
// (และถ้า escape จะได้ &amp; ติดมาให้คนอ่านเห็นด้วยตาเปล่า)
func (a actionEmail) text(toName string) string {
	return a.replacer(toName).Replace(actionEmailTextTemplate)
}

// replacer รวมการแทนค่าที่ทั้งสองเวอร์ชันใช้เหมือนกันไว้ที่เดียว
func (a actionEmail) replacer(name string) *strings.Replacer {
	greeting := "สวัสดี"
	if name != "" {
		greeting = "สวัสดีคุณ " + name
	}
	return strings.NewReplacer(
		"__GREETING__", greeting,
		"__PREHEADER__", a.preheader,
		"__INTRO__", a.intro,
		"__BUTTON__", a.button,
		"__LINK__", a.link,
		"__FOOTNOTE__", a.footnote,
	)
}
