package middlewares

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// auditTitles = ชื่อการกระทำที่โชว์ในหน้า Audit Log คีย์คือ "METHOD route-template" (c.FullPath())
// ค่า "" = ไม่ต้องบันทึก (เส้นที่ไม่ได้เปลี่ยนอะไรจริง) ส่วนเส้นที่ไม่อยู่ในนี้ยังถูกบันทึก
// โดยใช้ METHOD + path เป็นชื่อแทน — route ใหม่ที่ลืมเติมตรงนี้จะไม่หลุดจาก audit เงียบๆ
var auditTitles = map[string]string{
	"POST /api/admin/eligible-students":              "นำเข้ารายชื่อผู้มีสิทธิ์",
	"POST /api/admin/eligible-students/single":       "เพิ่มผู้มีสิทธิ์",
	"POST /api/admin/eligible-students/preview":      "",
	"PATCH /api/admin/eligible-students/:studentId":  "แก้ไขผู้มีสิทธิ์",
	"DELETE /api/admin/eligible-students/:studentId": "ลบผู้มีสิทธิ์",
	"POST /api/admin/request-templates":              "สร้างแม่แบบคำขอ",
	"PATCH /api/admin/request-templates/:id":         "แก้ไขแม่แบบคำขอ",
	"DELETE /api/admin/request-templates/:id":        "ลบแม่แบบคำขอ",
	"PATCH /api/admin/namespaces/:id/quota":          "แก้โควตาเนมสเปซ",
	"DELETE /api/admin/namespaces/:id":               "ลบเนมสเปซ",
	"DELETE /api/admin/services/:id":                 "ลบ service ทันที",
	"POST /api/admin/services/:id/schedule-delete":   "ตั้งเวลาลบ service",
	"DELETE /api/admin/services/:id/schedule-delete": "ยกเลิกการตั้งเวลาลบ service",
	"PATCH /api/admin/requests/:id/approve":          "อนุมัติคำขอทรัพยากร",
	"PATCH /api/admin/requests/:id/deny":             "ปฏิเสธคำขอทรัพยากร",
	"PATCH /api/admin/users/:id":                     "แก้ไขผู้ใช้",
	"DELETE /api/admin/users/:id":                    "ลบผู้ใช้",
	"PATCH /api/me":                                  "แก้ไขโปรไฟล์",
	"POST /api/me/password":                          "เปลี่ยนรหัสผ่าน",
	"POST /api/requests":                             "ยื่นคำขอทรัพยากร",
	"POST /api/namespaces":                           "สร้างเนมสเปซ",
	"POST /api/namespaces/join":                      "เข้าร่วมเนมสเปซ",
	"DELETE /api/namespaces":                         "ออกจากเนมสเปซ",
	"POST /api/namespaces/invites":                   "เชิญสมาชิก",
	"PATCH /api/namespaces/invites/:id/accept":       "ตอบรับคำเชิญ",
	"PATCH /api/namespaces/invites/:id/decline":      "ปฏิเสธคำเชิญ",
	"DELETE /api/namespaces/invites/:id":             "ยกเลิกคำเชิญ",
	"POST /api/services":                             "สร้าง service",
	"POST /api/services/node-port-reservation":       "", // ยิงทุกครั้งที่เปิดฟอร์ม ไม่ใช่การกระทำ
	"PATCH /api/services/:id":                        "แก้ไข service",
	"PATCH /api/services/:id/scale":                  "ปรับจำนวน pod",
	"DELETE /api/services/:id":                       "ลบ service",
	"POST /api/databases":                            "สร้าง database",
	"POST /api/ai-review-requests":                   "ส่ง deploy ให้ AI ตรวจ",
}

// auditEvents = method → event_type (ตรงกับ CHECK ของ audit_logs) — method ที่ไม่อยู่ในนี้ไม่บันทึก
var auditEvents = map[string]string{http.MethodPost: "CREATE", http.MethodPatch: "UPDATE", http.MethodDelete: "DELETE"}

// Audit = middleware บันทึกทุก request ที่เปลี่ยนข้อมูล (POST/PATCH/DELETE) และสำเร็จ ลงตาราง audit_logs
// ต้องวางหลัง Auth เพราะอ่านตัวตนผู้กระทำจาก context — GET ไม่บันทึก, request ที่ตอบ 4xx/5xx ไม่บันทึก
//
// ไม่เก็บ request body โดยตั้งใจ: มีรหัสผ่าน/credential ปนอยู่หลายเส้น ส่วน path มี id ของเป้าหมายพอให้ตามต่อได้
// ponytail: detail มีแค่ method+path (id) ไม่มีชื่อเป้าหมาย — ถ้าต้องการชื่อ ให้ handler เขียน detail เองทีละเส้น
func Audit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		method := c.Request.Method
		event := auditEvents[method]
		if event == "" || c.Writer.Status() >= 400 {
			return
		}
		if strings.HasSuffix(c.FullPath(), "/approve") {
			event = "APPROVE"
		}

		key := method + " " + c.FullPath()
		title, known := auditTitles[key]
		if known && title == "" {
			return
		}
		if !known {
			title = key
		}

		// ไม่ผูกกับ c.Request.Context() — client ปิดแท็บทันทีหลังได้ response ต้องไม่ทำให้ audit หาย
		err := db.Table("audit_logs").Create(map[string]any{
			"event_type":   event,
			"actor_role":   c.GetString(CtxRole),
			"actor_name":   c.GetString(CtxRealName),
			"action_title": title,
			"detail":       method + " " + c.Request.URL.Path,
			"source_ip":    c.ClientIP(),
		}).Error
		if err != nil {
			log.Printf("audit: บันทึก %q ไม่สำเร็จ: %v", key, err)
		}
	}
}
