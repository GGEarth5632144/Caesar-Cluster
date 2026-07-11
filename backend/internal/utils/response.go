package utils

import "github.com/gin-gonic/gin"

// OK ห่อ response สำเร็จให้เป็นรูปแบบเดียวกันทั้งระบบ: {"success": true, "data": ...}
// data flow: controller เรียกพร้อม data → เขียน JSON กลับไปหา client ตาม status ที่กำหนด
func OK(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

// Error ห่อ response ที่ผิดพลาดให้เป็นรูปแบบเดียวกัน: {"success": false, "error": {code, message}}
// data flow: controller เรียกพร้อม code/message → เขียน JSON error กลับไปหา client (ยังรัน handler ต่อได้)
func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	})
}

// AbortError เหมือน Error แต่สั่ง "หยุด chain" ด้วย — ใช้ใน middleware เพื่อไม่ให้ handler ตัวถัดไปทำงานต่อ
// data flow: middleware (เช่น Auth/AdminOnly) เรียกตอน reject → เขียน JSON error + Abort ปิด request ทันที
func AbortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message},
	})
}
