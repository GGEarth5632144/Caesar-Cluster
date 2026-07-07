package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"backend/internal/response"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.AbortError(c, http.StatusUnauthorized, "NO_TOKEN", "ต้องแนบ token")
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
			response.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			response.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}
		sub, subOK := claims["sub"].(float64)
		role, roleOK := claims["role"].(string)
		if !subOK || !roleOK {
			response.AbortError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token ไม่ถูกต้องหรือหมดอายุ")
			return
		}
		c.Set("userID", int(sub))
		c.Set("role", role)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			response.AbortError(c, http.StatusForbidden, "ADMIN_ONLY", "admin only")
			return
		}
		c.Next()
	}
}
