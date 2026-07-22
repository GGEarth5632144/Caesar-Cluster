// Package mailer ส่งอีเมลผ่าน Resend API (https://resend.com) ด้วย net/http ตรงๆ
// ไม่พึ่ง SDK ภายนอก — เข้ากับสไตล์โปรเจกต์นี้ที่มี dependency น้อย
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// resendEndpoint = endpoint ส่งอีเมลของ Resend
const resendEndpoint = "https://api.resend.com/emails"

// Mailer ถือ API key + ผู้ส่ง (from) ไว้ยิง request ไปหา Resend
// สร้างครั้งเดียวตอน start (ดู controller.NewAuthController) แล้วใช้ซ้ำได้ทุก request
type Mailer struct {
	apiKey string
	from   string
	client *http.Client
}

// New ประกอบ Mailer — apiKey/from มาจาก config
func New(apiKey, from string) *Mailer {
	return &Mailer{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// resendPayload = body ของ POST /emails ตามสเปกของ Resend
type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// SendPasswordResetEmail ส่งอีเมลพร้อมลิงก์รีเซ็ตรหัสผ่านให้ผู้ใช้
//
// data flow: ประกอบ HTML body (ใส่ลิงก์ + เวลาหมดอายุ) → marshal → POST ไป Resend
// พร้อม header Authorization: Bearer <apiKey> → คืน error ถ้าสร้าง request/ยิงไม่สำเร็จ หรือ Resend ตอบ >= 300
//
// ถ้า apiKey ว่าง (ยังไม่ได้ตั้ง RESEND_API_KEY) จะคืน error ทันทีโดยไม่ยิง request
// ให้ caller (ForgotPassword) log ไว้ แต่ยังตอบ client เป็น generic message ตามเดิม
func (m *Mailer) SendPasswordResetEmail(ctx context.Context, toEmail, toName, resetLink string, ttlMinutes int) error {
	if m.apiKey == "" {
		return fmt.Errorf("mailer: RESEND_API_KEY ยังไม่ได้ตั้งค่า")
	}

	payload := resendPayload{
		From:    m.from,
		To:      []string{toEmail},
		Subject: "รีเซ็ตรหัสผ่าน Caesar Cluster",
		HTML:    buildResetHTML(toName, resetLink, ttlMinutes),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend ตอบกลับ %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// resetEmailTemplate = โครง HTML ของอีเมล วางแบบ table-based layout + inline style ล้วน
// (จำเป็นสำหรับอีเมล — client อย่าง Outlook ไม่รองรับ flexbox/grid/external CSS) ใช้โทนสีเดียวกับ
// หน้า Login/Register จริงของแอป (#BB6653 การ์ด, #FFF8E8 พื้นหลังครีม, #211a14 ตัวอักษรเข้ม)
// ใส่ preheader (span ที่ซ่อนไว้) ให้ตัวอย่างข้อความบน inbox list ดูดีขึ้นด้วย
const resetEmailTemplate = `<!DOCTYPE html>
<html lang="th">
  <body style="margin:0;padding:0;background-color:#FFF8E8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
    <span style="display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;color:#FFF8E8;">ลิงก์สำหรับตั้งรหัสผ่านใหม่ของคุณ หมดอายุใน __TTL__ นาที</span>
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
