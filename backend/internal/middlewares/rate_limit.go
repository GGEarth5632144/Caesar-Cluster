package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/utils"
)

// RateLimit = middleware จำกัดจำนวน request ต่อ IP แบบ fixed-window ในหน่วยความจำ
//
// ใช้กับ endpoint ที่ถูก abuse ได้ง่าย (เช่น /forgot-password — กันสแปม/email-bombing)
// เก็บ state ในแมพ + mutex ไม่พึ่ง Redis/dependency ภายนอก — พอสำหรับ deploy แบบ single instance
// (ถ้าสเกลเป็นหลาย instance ค่อยเปลี่ยนไปใช้ shared store ทีหลัง)
//
// limit  = จำนวน request สูงสุดต่อ 1 หน้าต่างเวลา
// window = ความยาวของหน้าต่าง เช่น RateLimit(3, 15*time.Minute) = 3 ครั้ง/15 นาที ต่อ IP
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	rl := &rateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
	}
	go rl.cleanupLoop()
	return rl.handle
}

// visitor = ตัวนับของ IP หนึ่ง: นับกี่ครั้งแล้วในหน้าต่างปัจจุบัน + หน้าต่างเริ่มเมื่อไร
type visitor struct {
	count       int
	windowStart time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
}

// handle = ตัว middleware จริง: เช็ค IP ปัจจุบันว่าเกิน limit ในหน้าต่างนี้ไหม
func (rl *rateLimiter) handle(c *gin.Context) {
	ip := c.ClientIP()
	now := time.Now()

	rl.mu.Lock()
	v, ok := rl.visitors[ip]
	if !ok || now.Sub(v.windowStart) > rl.window {
		// IP ใหม่ หรือหน้าต่างเก่าหมดอายุ → เริ่มนับใหม่
		rl.visitors[ip] = &visitor{count: 1, windowStart: now}
		rl.mu.Unlock()
		c.Next()
		return
	}
	if v.count >= rl.limit {
		rl.mu.Unlock()
		utils.AbortError(c, http.StatusTooManyRequests, "RATE_LIMITED",
			"คำขอถี่เกินไป กรุณารอสักครู่แล้วลองใหม่อีกครั้ง")
		return
	}
	v.count++
	rl.mu.Unlock()
	c.Next()
}

// cleanupLoop เก็บกวาด IP ที่หน้าต่างหมดอายุแล้วเป็นระยะ กันแมพโตไม่หยุด
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if now.Sub(v.windowStart) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}
