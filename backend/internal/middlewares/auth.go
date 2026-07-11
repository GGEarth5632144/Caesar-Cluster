package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"backend/internal/utils"
)

// Auth = middleware ตรวจ JWT ก่อนเข้า route ที่ต้อง login
// data flow: อ่าน header "Authorization: Bearer <token>" → verify ลายเซ็นด้วย jwtSecret
// → ดึง claims (sub=userID, role) ยัดลง gin.Context ให้ handler ถัดไปใช้ (c.GetInt("userID"))
// ถ้า token หาย/ผิด/หมดอายุ → AbortError 401 หยุด chain ทันที
func Auth(jwtSecret string) gin.HandlerFunc {
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
		role, roleOK := claims["role"].(string)
		if !subOK || !roleOK {
			utils.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}
		c.Set("userID", int(sub))
		c.Set("role", role)
		c.Next()
	}
}

// AdminOnly = middleware กันไว้ให้เฉพาะ admin (ต้องวางหลัง Auth เสมอ เพราะอ่าน role ที่ Auth ตั้งไว้)
// data flow: อ่าน role จาก gin.Context (ที่ Auth เซ็ต) → ถ้าไม่ใช่ "admin" → AbortError 403 หยุด chain
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			utils.AbortError(c, http.StatusForbidden, "ADMIN_ONLY", "admin only")
			return
		}
		c.Next()
	}
}
