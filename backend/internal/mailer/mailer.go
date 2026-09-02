// Package mailer ส่งอีเมลผ่าน SMTP ตรงๆ ด้วย net/smtp ใน stdlib
// ตั้งใจใช้กับบัญชี Gmail แบบ no-reply ที่สร้างไว้ให้ระบบนี้โดยเฉพาะ (smtp.gmail.com)
// จึงไม่ต้องพึ่งบริการส่งเมลภายนอกและไม่ต้องมี API key ให้ดูแล/หมดอายุ
//
// ข้อควรรู้เรื่อง Gmail: ตั้งแต่ปี 2022 Google ปิดการล็อกอิน SMTP ด้วยรหัสผ่านบัญชีปกติแล้ว
// ต้องเปิด 2-Step Verification ของบัญชี no-reply นั้นก่อน แล้วสร้าง "App Password" (16 ตัวอักษร)
// มาใส่ใน SMTP_PASSWORD แทน — ดูขั้นตอนใน .env.example
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
// กัน handler ForgotPassword ค้างยาวถ้า Gmail ไม่ตอบ — ฝั่ง client รอ response อยู่
const smtpTimeout = 15 * time.Second

// implicitTLSPort = พอร์ตที่ห่อ TLS ตั้งแต่ต้น (SMTPS) ไม่ได้เริ่มด้วย plaintext แล้ว STARTTLS
// Gmail เปิดทั้ง 587 (STARTTLS) และ 465 (implicit) — เลือกได้จาก SMTP_PORT
const implicitTLSPort = 465

// Config = ค่าที่ Mailer ต้องใช้ต่อกับ SMTP server — map ตรงกับ env SMTP_* ใน config.Config
type Config struct {
	Host     string // เช่น smtp.gmail.com
	Port     int    // 587 = STARTTLS (ค่าปกติ), 465 = TLS ตั้งแต่ต้น
	Username string // อีเมลเต็มของบัญชี no-reply เช่น caesar.cluster.noreply@gmail.com
	Password string // App Password 16 ตัวของบัญชีนั้น (ไม่ใช่รหัสผ่านที่ใช้ล็อกอิน Google)
	FromName string // ชื่อที่แสดงหน้าอีเมลผู้ส่ง เช่น "Caesar Cluster"
}

// Mailer ถือค่า SMTP ไว้ใช้ซ้ำทุก request — สร้างครั้งเดียวตอน start (ดู controller.NewAuthController)
// ไม่ถือ connection ค้าง: เปิด-ปิดต่อการส่งหนึ่งฉบับ เพราะระบบนี้ส่งเมลนานๆ ครั้ง
// การคา connection ไว้เฉยๆ มีแต่จะโดน Gmail ตัดทิ้งแล้วต้องมา handle reconnect เอง
type Mailer struct {
	cfg Config
}

// New ประกอบ Mailer — ค่าทั้งหมดมาจาก config.Config
func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// SendPasswordResetEmail ส่งอีเมลพร้อมลิงก์รีเซ็ตรหัสผ่านให้ผู้ใช้
//
// data flow: ประกอบ HTML body (ใส่ลิงก์ + เวลาหมดอายุ) → ห่อเป็นข้อความ MIME พร้อม header
// → เปิด SMTP session ไป Gmail (STARTTLS + PLAIN auth) → MAIL/RCPT/DATA → QUIT
//
// ถ้ายังไม่ได้ตั้ง SMTP_USERNAME/SMTP_PASSWORD จะคืน error ทันทีโดยไม่ต่อ connection
// ให้ caller (ForgotPassword) log ไว้ แต่ยังตอบ client เป็น generic message ตามเดิม
func (m *Mailer) SendPasswordResetEmail(ctx context.Context, toEmail, toName, resetLink string, ttlMinutes int) error {
	return m.send(
		ctx,
		toEmail,
		"รีเซ็ตรหัสผ่าน Caesar Cluster",
		buildResetText(toName, resetLink, ttlMinutes),
		buildResetHTML(toName, resetLink, ttlMinutes),
	)
}

// SendOTPEmail ส่งรหัสยืนยันตัวตน 6 หลักให้ผู้ใช้ตอนล็อกอินจากเครื่องที่ระบบยังไม่รู้จัก
//
// ตั้งใจไม่ใส่รหัสไว้ใน subject และ preheader ถึงแม้จะสะดวกกว่า เพราะสองที่นี้คือส่วนที่
// โผล่บนหน้าจอล็อกของมือถือ — คนที่หยิบเครื่องขึ้นมาดูเฉยๆ ไม่ควรอ่านรหัสได้โดยไม่ต้องปลดล็อก
func (m *Mailer) SendOTPEmail(ctx context.Context, toEmail, toName, code string, ttlMinutes int) error {
	return m.send(
		ctx,
		toEmail,
		"รหัสยืนยันตัวตน Caesar Cluster",
		buildOTPText(toName, code, ttlMinutes),
		buildOTPHTML(toName, code, ttlMinutes),
	)
}

// send คุยกับ SMTP server หนึ่งรอบเพื่อส่งอีเมล HTML หนึ่งฉบับ
//
// ลำดับตามโปรโตคอล: dial → (STARTTLS ถ้าไม่ใช่พอร์ต 465) → AUTH PLAIN → MAIL FROM → RCPT TO
// → DATA → QUIT ซึ่ง QUIT สำคัญ: ถ้าไม่เรียก Gmail อาจถือว่า session ถูกตัดกลางคันแล้วไม่ส่งจริง
// เลยต้องคืน error ของ Quit ด้วย ไม่ใช่ปิด connection เฉยๆ
func (m *Mailer) send(ctx context.Context, toEmail, subject, textBody, htmlBody string) error {
	if m.cfg.Username == "" || m.cfg.Password == "" {
		return errors.New("mailer: ยังไม่ได้ตั้ง SMTP_USERNAME / SMTP_PASSWORD")
	}
	// ที่อยู่ปลายทางถูกเอาไปวางใน header To: ตรงๆ — ถ้ามี CR/LF ปนมาจะกลายเป็นการแทรก header เพิ่มได้
	// (email header injection) mail.ParseAddress ตรวจให้ครบทั้งรูปแบบและอักขระต้องห้ามในตัวเดียว
	recipient, err := mail.ParseAddress(toEmail)
	if err != nil {
		return fmt.Errorf("mailer: อีเมลปลายทางไม่ถูกต้อง: %w", err)
	}

	msg, err := buildMessage(
		mail.Address{Name: m.cfg.FromName, Address: m.cfg.Username},
		*recipient, subject, textBody, htmlBody,
	)
	if err != nil {
		return fmt.Errorf("mailer: ประกอบเนื้อเมลไม่สำเร็จ: %w", err)
	}

	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	dialer := &net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mailer: ต่อ %s ไม่ได้: %w", addr, err)
	}
	// deadline เดียวครอบทุกขั้นที่เหลือ — net/smtp ไม่มี timeout ของตัวเอง
	// ถ้าไม่ตั้ง แล้ว server ค้างกลาง DATA จะรอไปเรื่อยๆ ไม่มีวันคืน
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
	if m.cfg.Port == implicitTLSPort {
		conn = tls.Client(conn, tlsCfg)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mailer: เริ่ม SMTP session ไม่สำเร็จ: %w", err)
	}
	defer client.Close() // ปิด connection ทิ้งเสมอ แม้ทางที่ error ก่อนถึง Quit

	if m.cfg.Port != implicitTLSPort {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// ไม่ยอมส่งต่อแบบ plaintext เพราะขั้นถัดไปคือส่ง App Password ข้ามเน็ต
			return errors.New("mailer: server ไม่รองรับ STARTTLS — ไม่ส่งรหัสผ่านผ่านช่องที่ไม่เข้ารหัส")
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mailer: STARTTLS ไม่สำเร็จ: %w", err)
		}
	}

	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("mailer: ล็อกอิน SMTP ด้วยบัญชี %s ไม่ผ่าน "+
			"(Gmail ต้องใช้ App Password 16 ตัว ไม่ใช่รหัสผ่านบัญชีปกติ): %w", m.cfg.Username, err)
	}
	// MAIL FROM ต้องเป็นบัญชีที่เพิ่ง auth ไป ไม่ใช่ชื่อที่โชว์ใน header From:
	// Gmail ปฏิเสธ (หรือเขียนทับ) ถ้าใส่ที่อยู่อื่นที่ไม่ได้ตั้งเป็น alias ไว้
	if err := client.Mail(m.cfg.Username); err != nil {
		return fmt.Errorf("mailer: MAIL FROM ไม่ผ่าน: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("mailer: RCPT TO ไม่ผ่าน: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: เริ่มส่ง DATA ไม่สำเร็จ: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mailer: เขียนเนื้อเมลไม่สำเร็จ: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: ปิดท้าย DATA ไม่สำเร็จ: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mailer: QUIT ไม่สำเร็จ (server อาจไม่ได้ส่งเมลจริง): %w", err)
	}
	return nil
}

// buildMessage ห่อเนื้อหาให้เป็นข้อความ MIME ที่ SMTP รับได้
//
// multipart/alternative = ใส่ทั้ง text ล้วนและ HTML ไว้ในฉบับเดียว mail client เลือกส่วนท้ายสุด
// (HTML) ถ้าแสดงได้ ไม่งั้นตกมาที่ text — ที่ต้องมี text ด้วยเป็นเรื่องเข้า inbox ไม่ใช่ความสวยงาม
// อีเมลที่มีแต่ HTML ก้อนเดียวเป็นลายเซ็นคลาสสิกของสแปม (SpamAssassin มีกฎ MIME_HTML_ONLY ตรงๆ)
//
// จุดที่พลาดง่าย:
//   - หัวข้อ/ชื่อผู้ส่งภาษาไทยต้อง encode ตาม RFC 2047 ไม่งั้นขึ้นเป็นตัวขยะ
//     (mail.Address.String() จัดการชื่อผู้ส่งให้เอง ส่วน Subject ใช้ mime.QEncoding)
//   - บรรทัด UTF-8 ยาวเกิน 998 ตัวอักษรตาม RFC 5321 ต้องเข้ารหัส เลือก quoted-printable
//     ไม่ใช่ base64 เพราะตัวกรองหักคะแนนเมลที่หมก text ไว้ใน base64 (เป็นวิธีที่สแปมใช้ซ่อนเนื้อหา)
//   - ทุกบรรทัดต้องจบด้วย CRLF — multipart.Writer และ quotedprintable.Writer จัดการให้แล้ว
func buildMessage(from, to mail.Address, subject, textBody, htmlBody string) ([]byte, error) {
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	// เรียง text ก่อน HTML ตาม RFC 2046: ส่วนที่อยู่ท้ายสุดคือส่วนที่ "อยากให้แสดงมากที่สุด"
	for _, part := range []struct{ contentType, content string }{
		{`text/plain; charset="UTF-8"`, textBody},
		{`text/html; charset="UTF-8"`, htmlBody},
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

	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + to.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: " + messageID(from.Address) + "\r\n")
	// บอกว่าเป็นเมลที่ระบบสร้างเอง ไม่ใช่คนพิมพ์ — กันตัวตอบกลับอัตโนมัติ (out-of-office)
	// ยิงกลับเข้ากล่อง no-reply เป็นลูป ตาม RFC 3834
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + mp.Boundary() + "\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body.String())
	return []byte(b.String()), nil
}

// messageID สร้างค่า header Message-ID ให้เมลแต่ละฉบับ
//
// เมลที่ไม่มี Message-ID ถูกหักคะแนนโดยตัวกรองแทบทุกตัว (SpamAssassin: MISSING_MID)
// เพราะโปรแกรมเมลของจริงใส่มาให้เสมอ มีแต่สคริปต์ลวกๆ ที่ลืม
// ฝั่งขวาของ @ ใช้โดเมนผู้ส่งให้สอดคล้องกับ From (โดเมนมั่วเป็นสัญญาณต้องสงสัยอีกแบบ)
func messageID(fromAddress string) string {
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

// resetEmailTemplate = โครง HTML ของอีเมล วางแบบ table-based layout + inline style ล้วน
// (จำเป็นสำหรับอีเมล — client อย่าง Outlook ไม่รองรับ flexbox/grid/external CSS) ใช้โทนสีเดียวกับ
// หน้า Login/Register จริงของแอป (#BB6653 การ์ด, #FFF8E8 พื้นหลังครีม, #211a14 ตัวอักษรเข้ม)
// ใส่ preheader (span ที่ซ่อนไว้) ให้ตัวอย่างข้อความบน inbox list ดูดีขึ้นด้วย
const resetEmailTemplate = `<!DOCTYPE html>
<html lang="th">
  <body style="margin:0;padding:0;background-color:#FFF8E8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <span style="display:none;max-height:0;max-width:0;overflow:hidden;">ลิงก์สำหรับตั้งรหัสผ่านใหม่ของคุณ หมดอายุใน __TTL__ นาที</span>
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
                <p style="margin:0;font-size:15px;line-height:1.6;color:#211a14;">เราได้รับคำขอรีเซ็ตรหัสผ่านสำหรับบัญชีของคุณ กดปุ่มด้านล่างเพื่อตั้งรหัสผ่านใหม่</p>
              </td>
            </tr>
            <tr>
              <td align="center" style="padding:32px 36px;">
                <table role="presentation" cellpadding="0" cellspacing="0">
                  <tr>
                    <td align="center" style="border-radius:9999px;background-color:#BB6653;">
                      <a href="__LINK__" style="display:inline-block;padding:14px 40px;font-size:15px;font-weight:600;color:#ffffff;text-decoration:none;border-radius:9999px;">ตั้งรหัสผ่านใหม่</a>
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
                <p style="margin:0;font-size:12px;line-height:1.6;color:#8a7d72;">ลิงก์นี้จะหมดอายุใน __TTL__ นาที และใช้ได้เพียงครั้งเดียว ถ้าคุณไม่ได้ร้องขอการรีเซ็ตรหัสผ่าน กรุณาเพิกเฉยต่ออีเมลฉบับนี้ — บัญชีของคุณยังปลอดภัยดี</p>
              </td>
            </tr>
          </table>
          <p style="max-width:520px;margin:20px 0 0;font-size:11px;color:#a89a8c;text-align:center;">อีเมลนี้ส่งจากระบบอัตโนมัติของ Caesar Cluster กรุณาอย่าตอบกลับ</p>
        </td>
      </tr>
    </table>
  </body>
</html>`

// buildResetHTML ประกอบ HTML ของอีเมลจาก resetEmailTemplate
// escape ค่าที่มาจากผู้ใช้ (ชื่อ) กัน HTML injection — resetLink เป็น URL ที่เราสร้างเอง
// (origin + token hex) จึงปลอดภัยพอที่จะใส่ตรงๆ โดยไม่ต้อง escape
//
// ใช้ strings.NewReplacer แทน fmt.Sprintf เพราะ template มี "%" อยู่เยอะ (width:100% ฯลฯ)
// ซึ่งจะชนกับ verb ของ Sprintf ถ้าใช้ %s ตรงๆ
func buildResetHTML(toName, resetLink string, ttlMinutes int) string {
	greeting := "สวัสดี"
	if toName != "" {
		greeting = "สวัสดีคุณ " + html.EscapeString(toName)
	}
	replacer := strings.NewReplacer(
		"__GREETING__", greeting,
		"__LINK__", resetLink,
		"__TTL__", strconv.Itoa(ttlMinutes),
	)
	return replacer.Replace(resetEmailTemplate)
}

// resetTextTemplate = เวอร์ชัน text ล้วนของอีเมลฉบับเดียวกัน
//
// ต้องสื่อความครบเท่า HTML ไม่ใช่ใส่ "กรุณาเปิดด้วยโปรแกรมที่รองรับ HTML" พอเป็นพิธี:
// ตัวกรองเทียบว่าสองส่วนต่างกันมากไหม (ต่างมาก = สัญญาณของการซ่อนเนื้อหาจากตัวสแกน)
// และคนที่อ่านเมลแบบ text ล้วนก็ต้องรีเซ็ตรหัสผ่านได้จริง
const resetTextTemplate = `__GREETING__,

เราได้รับคำขอรีเซ็ตรหัสผ่านสำหรับบัญชี Caesar Cluster ของคุณ
เปิดลิงก์ด้านล่างเพื่อตั้งรหัสผ่านใหม่:

__LINK__

ลิงก์นี้จะหมดอายุใน __TTL__ นาที และใช้ได้เพียงครั้งเดียว
ถ้าคุณไม่ได้ร้องขอการรีเซ็ตรหัสผ่าน กรุณาเพิกเฉยต่ออีเมลฉบับนี้ บัญชีของคุณยังปลอดภัยดี

--
Caesar Cluster - Cloud for CPE Students
อีเมลนี้ส่งจากระบบอัตโนมัติ กรุณาอย่าตอบกลับ
`

// buildResetText ประกอบเนื้อความ text ล้วนจาก resetTextTemplate
// ไม่ต้อง escape ชื่อผู้ใช้เหมือนฝั่ง HTML เพราะ text ล้วนไม่มี markup ให้แทรก
func buildResetText(toName, resetLink string, ttlMinutes int) string {
	greeting := "สวัสดี"
	if toName != "" {
		greeting = "สวัสดีคุณ " + toName
	}
	replacer := strings.NewReplacer(
		"__GREETING__", greeting,
		"__LINK__", resetLink,
		"__TTL__", strconv.Itoa(ttlMinutes),
	)
	return replacer.Replace(resetTextTemplate)
}

// otpEmailTemplate = โครง HTML ของอีเมลรหัสยืนยัน ใช้โทนสี/โครง table เดียวกับ resetEmailTemplate
// ตัวรหัสวางเป็นบล็อกใหญ่ ตัวอักษร monospace + เว้นวรรคระหว่างหลัก ให้อ่านแล้วพิมพ์ตามได้ไม่ผิด
// (เลข 6 หลักติดกันในฟอนต์ปกติ อ่านสลับหลักกันง่ายมากบนจอเล็ก)
const otpEmailTemplate = `<!DOCTYPE html>
<html lang="th">
  <body style="margin:0;padding:0;background-color:#FFF8E8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <span style="display:none;max-height:0;max-width:0;overflow:hidden;">รหัสยืนยันตัวตนสำหรับเข้าสู่ระบบ หมดอายุใน __TTL__ นาที</span>
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
                <p style="margin:0;font-size:15px;line-height:1.6;color:#211a14;">ใช้รหัสด้านล่างเพื่อยืนยันตัวตนและเข้าสู่ระบบ</p>
              </td>
            </tr>
            <tr>
              <td align="center" style="padding:28px 36px 8px;">
                <table role="presentation" cellpadding="0" cellspacing="0" width="100%">
                  <tr>
                    <td align="center" style="background-color:#FFF8E8;border-radius:16px;padding:24px 16px;">
                      <div style="font-family:'Courier New',Courier,monospace;font-size:34px;font-weight:700;letter-spacing:10px;color:#BB6653;">__CODE__</div>
                    </td>
                  </tr>
                </table>
              </td>
            </tr>
            <tr>
              <td style="padding:16px 36px 32px;">
                <p style="margin:0;font-size:13px;line-height:1.6;color:#8a7d72;">รหัสนี้จะหมดอายุใน __TTL__ นาที และใช้ได้เพียงครั้งเดียว</p>
              </td>
            </tr>
            <tr>
              <td style="padding:0 36px;">
                <hr style="border:none;border-top:1px solid #f0e6d6;margin:0;" />
              </td>
            </tr>
            <tr>
              <td style="padding:24px 36px 36px;">
                <p style="margin:0;font-size:12px;line-height:1.6;color:#8a7d72;">ถ้าคุณไม่ได้เป็นคนพยายามเข้าสู่ระบบ กรุณาเพิกเฉยต่ออีเมลฉบับนี้ และควรเปลี่ยนรหัสผ่านของคุณ เพราะการที่รหัสนี้ถูกส่งออกไปแปลว่ามีคนกรอกรหัสผ่านของคุณได้ถูกต้อง</p>
              </td>
            </tr>
          </table>
          <p style="max-width:520px;margin:20px 0 0;font-size:11px;color:#a89a8c;text-align:center;">อีเมลนี้ส่งจากระบบอัตโนมัติของ Caesar Cluster กรุณาอย่าตอบกลับ</p>
        </td>
      </tr>
    </table>
  </body>
</html>`

// otpTextTemplate = เวอร์ชัน text ล้วนของอีเมลรหัสยืนยัน (เหตุผลที่ต้องมีดูที่ buildMessage)
const otpTextTemplate = `__GREETING__,

ใช้รหัสด้านล่างเพื่อยืนยันตัวตนและเข้าสู่ระบบ Caesar Cluster:

    __CODE__

รหัสนี้จะหมดอายุใน __TTL__ นาที และใช้ได้เพียงครั้งเดียว

ถ้าคุณไม่ได้เป็นคนพยายามเข้าสู่ระบบ กรุณาเพิกเฉยต่ออีเมลฉบับนี้ และควรเปลี่ยนรหัสผ่านของคุณ
เพราะการที่รหัสนี้ถูกส่งออกไปแปลว่ามีคนกรอกรหัสผ่านของคุณได้ถูกต้อง

--
Caesar Cluster - Cloud for CPE Students
อีเมลนี้ส่งจากระบบอัตโนมัติ กรุณาอย่าตอบกลับ
`

// buildOTPHTML / buildOTPText ประกอบเนื้อความจาก template — escape ชื่อเฉพาะฝั่ง HTML
// ตัว code เป็นเลข 6 หลักที่ระบบสุ่มเอง ไม่ใช่ค่าจากผู้ใช้ จึงไม่ต้อง escape
func buildOTPHTML(toName, code string, ttlMinutes int) string {
	return otpReplacer(html.EscapeString(toName), code, ttlMinutes).Replace(otpEmailTemplate)
}

func buildOTPText(toName, code string, ttlMinutes int) string {
	return otpReplacer(toName, code, ttlMinutes).Replace(otpTextTemplate)
}

// otpReplacer รวมการแทนค่าที่ทั้งสองเวอร์ชันใช้เหมือนกันไว้ที่เดียว
// รับชื่อที่ escape มาแล้ว (หรือยังไม่ได้ escape สำหรับ text) เพื่อไม่ให้ฝั่ง text ติด &amp; มาด้วย
func otpReplacer(escapedName, code string, ttlMinutes int) *strings.Replacer {
	greeting := "สวัสดี"
	if escapedName != "" {
		greeting = "สวัสดีคุณ " + escapedName
	}
	return strings.NewReplacer(
		"__GREETING__", greeting,
		"__CODE__", code,
		"__TTL__", strconv.Itoa(ttlMinutes),
	)
}
