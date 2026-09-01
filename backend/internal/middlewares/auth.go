package middlewares

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"backend/internal/utils"
)

// คีย์ที่ Auth เซ็ตไว้ใน gin.Context ให้ handler ถัดไปใช้ต่อ
//
// ประกาศเป็นค่าคงที่แทนการพิมพ์สตริงซ้ำในหลายไฟล์ — พิมพ์ผิดหนึ่งตัวใน c.GetString("role")
// จะได้ค่าว่างเงียบๆ ซึ่งแปลว่า "ไม่ใช่ admin" พอดี ทำให้บั๊กแบบนั้นไม่มีอาการให้เห็นเลย
const (
	CtxUserID      = "userID"
	CtxRole        = "role"
	CtxNamespaceID = "namespaceID" // *int — nil = ผู้ใช้ยังไม่มี space
)

// authUser = ข้อมูลเท่าที่ middleware ต้องใช้ ดึงมาในคำสั่งเดียวพร้อมชื่อ role
type authUser struct {
	ID          int
	NamespaceID *int
	RoleName    string
}

// Auth = middleware ตรวจ JWT ก่อนเข้า route ที่ต้อง login
//
// data flow: อ่าน header "Authorization: Bearer <token>" → verify ลายเซ็นด้วย jwtSecret
// → เอา sub (userID) ไปอ่านสถานะล่าสุดของบัญชีจาก DB → ยัดลง gin.Context ให้ handler ถัดไปใช้
//
// # ทำไมต้องอ่าน DB ทุก request ไม่เชื่อ claim ในโทเคนอย่างเดียว
//
// JWT เป็นของที่ "เซ็นแล้วเรียกคืนไม่ได้" — ข้อมูลข้างในคือภาพ ณ วินาทีที่ล็อกอิน ไม่ใช่ ณ ตอนนี้
// ของเดิมอ่าน role จาก claim ตรงๆ แปลว่า:
//
//   - แอดมินลดสิทธิ์ใครจาก admin เป็น user → คนนั้นยังสั่งงาน endpoint ของแอดมินได้ต่อ
//     จนกว่าโทเคนจะหมดอายุ ซึ่งนานถึง 30 วันถ้าตอนล็อกอินติ๊ก "Remember For 30 Days"
//   - แอดมินลบบัญชีทิ้ง → โทเคนของบัญชีที่ไม่มีอยู่แล้วยังผ่านด่านนี้ได้ ไปตายเอาข้างในแทน
//   - เลื่อนใครขึ้นเป็น admin → เจ้าตัวต้องออกจากระบบแล้วเข้าใหม่ถึงจะใช้ได้ ทั้งที่ DB เปลี่ยนแล้ว
//
// การอ่าน DB ทุกครั้งทำให้การถอนสิทธิ์มีผลทันที ราคาที่จ่ายคือ SELECT ด้วย primary key
// หนึ่งครั้งต่อ request บนตารางที่มีไม่กี่ร้อยแถว ซึ่งถูกกว่าที่ handler ส่วนใหญ่ทำอยู่แล้วเสียอีก
// (ของเดิมหลาย handler ก็ยิง First(&user, userID) ของตัวเองซ้ำอยู่ดี — ตอนนี้อ่านรอบเดียวแล้วแชร์กัน
// ผ่าน context ดู currentNamespaceID ใน controller/helper.go)
//
// claim "role" ในโทเคนยังออกให้เหมือนเดิมเพื่อความเข้ากันได้ แต่ไม่ถูกใช้ตัดสินสิทธิ์อีกต่อไป
func Auth(jwtSecret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			utils.AbortError(c, http.StatusUnauthorized, "NO_TOKEN", "ต้องแนบ token")
			return
		}

		token, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "),
			func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
		if err != nil || !token.Valid {
			utils.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			utils.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}
		sub, subOK := claims["sub"].(float64)
		if !subOK {
			utils.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}

		// LEFT JOIN ไม่ใช่ JOIN: ถ้า role_id ชี้ไปหาแถวที่ไม่มีอยู่ (seed ไม่ครบ/ข้อมูลเพี้ยน)
		// INNER JOIN จะคืนศูนย์แถว ซึ่งแยกไม่ออกจาก "ไม่มีบัญชีนี้แล้ว" แล้วผู้ใช้จะโดนเตะออก
		// พร้อมข้อความว่าบัญชีถูกลบ ทั้งที่บัญชียังอยู่ครบ ปัญหาจริงอยู่ที่ตาราง roles
		var u authUser
		err = db.WithContext(c.Request.Context()).
			Table("users AS u").
			Select("u.id AS id, u.namespace_id AS namespace_id, COALESCE(r.name, '') AS role_name").
			Joins("LEFT JOIN roles r ON r.id = u.role_id").
			Where("u.id = ?", int(sub)).
			Scan(&u).Error
		if err != nil {
			log.Printf("auth: อ่านบัญชี id=%d ไม่สำเร็จ: %v", int(sub), err)
			utils.AbortError(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
		if u.ID == 0 {
			// โทเคนถูกต้องทุกอย่าง แต่บัญชีถูกลบไปแล้ว — แยก code ออกจาก INVALID_TOKEN
			// เพื่อให้หน้าเว็บบอกผู้ใช้ได้ตรงว่าเกิดอะไรขึ้น ไม่ใช่แค่ "token ไม่ถูกต้อง"
			utils.AbortError(c, http.StatusUnauthorized, "ACCOUNT_GONE", "บัญชีนี้ถูกลบออกจากระบบแล้ว")
			return
		}
		if u.RoleName == "" {
			log.Printf("auth: บัญชี id=%d มี role_id ที่ไม่มีอยู่ในตาราง roles (ลืมรัน seed?)", u.ID)
			utils.AbortError(c, http.StatusInternalServerError, "INTERNAL", "ข้อมูลสิทธิ์ของบัญชีนี้ไม่สมบูรณ์")
			return
		}

		c.Set(CtxUserID, u.ID)
		c.Set(CtxRole, u.RoleName)
		c.Set(CtxNamespaceID, u.NamespaceID)
		c.Next()
	}
}

// AdminOnly = middleware กันไว้ให้เฉพาะ admin (ต้องวางหลัง Auth เสมอ เพราะอ่าน role ที่ Auth ตั้งไว้)
// data flow: อ่าน role จาก gin.Context (ที่ Auth อ่านมาจาก DB สดๆ) → ไม่ใช่ "admin" → AbortError 403
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CtxRole) != entityRoleAdmin {
			utils.AbortError(c, http.StatusForbidden, "ADMIN_ONLY", "admin only")
			return
		}
		c.Next()
	}
}

// entityRoleAdmin ซ้ำกับ entity.RoleAdmin โดยตั้งใจ — package middlewares ไม่ import entity
// เพื่อไม่ให้ชั้น middleware ผูกกับ data model (ค่านี้เป็นสตริงที่อยู่ในตาราง roles ไม่เปลี่ยน)
const entityRoleAdmin = "admin"
