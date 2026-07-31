package controller

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/entity"
	"backend/internal/utils"
)

// dns1123 = กฎชื่อที่ Kubernetes ยอมรับสำหรับ namespace/resource
// (ตัวพิมพ์เล็ก/ตัวเลข/ขีดกลาง ขึ้นต้นและลงท้ายด้วยตัวอักษรหรือตัวเลขเท่านั้น)
// เราตั้งชื่อ namespace บน cluster ตามชื่อที่ user กรอกตรงๆ เลยต้องกันชื่อผิดกฎตั้งแต่ที่ API
var dns1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// isValidK8sName เช็คว่าชื่อที่ user กรอกเอาไปตั้งเป็นชื่อ resource บน k8s ได้ไหม
// data flow: ถูกเรียกจาก NamespaceController.Create และ ServiceController.Create ก่อนส่งต่อให้ service layer
func isValidK8sName(name string) bool {
	return dns1123.MatchString(name)
}

// envVarKeyPattern = กฎชื่อ environment variable ตามที่ shell/container runtime ทั่วไปยอมรับ
// (ตัวอักษร/เลข/underscore เท่านั้น ขึ้นต้นด้วยตัวอักษรหรือ underscore — ห้ามขึ้นต้นด้วยตัวเลข)
var envVarKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const (
	maxEnvVarCount = 20   // กันไม่ให้ payload/row ใหญ่เกินไปโดยไม่จำเป็น
	maxEnvVarValue = 4000 // ค่าต่อ 1 ตัวยาวสุด (bytes) — เผื่อไว้สำหรับ config ยาวๆ เช่น cert/JSON
)

// isValidEnvVars เช็คว่า env vars ที่ user กรอกมาปลอดภัยพอจะเก็บ/ส่งต่อให้ provisioner
// data flow: ถูกเรียกจาก ServiceController.Create ก่อนส่งต่อให้ service layer (แบบเดียวกับ isValidK8sName)
func isValidEnvVars(env map[string]string) bool {
	if len(env) > maxEnvVarCount {
		return false
	}
	for k, v := range env {
		if !envVarKeyPattern.MatchString(k) {
			return false
		}
		if len(v) > maxEnvVarValue {
			return false
		}
	}
	return true
}

// currentNamespaceID ดึง namespace ของผู้ใช้ที่ล็อกอินอยู่ (กติกา 1 คน = 1 space)
//
// data flow: อ่าน userID ที่ middleware Auth ตั้งไว้ → SELECT users → คืน users.namespace_id
// ถ้ายังไม่มี space (NULL) จะตอบ 409 NO_NAMESPACE ให้เลย แล้วคืน ok=false เพื่อให้ handler หยุดทำงานต่อ
//
// controller ที่ต้องใช้ namespace (service, namespace/me) เรียกตัวนี้เป็นด่านแรกเสมอ
// จะได้ไม่ต้องเขียน logic เดิมซ้ำในทุก handler
func currentNamespaceID(c *gin.Context, db *gorm.DB) (int, bool) {
	var user entity.User
	if err := db.WithContext(c.Request.Context()).First(&user, c.GetInt("userID")).Error; err != nil {
		utils.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบผู้ใช้")
		return 0, false
	}
	if user.NamespaceID == nil {
		utils.Error(c, http.StatusConflict, "NO_NAMESPACE",
			"คุณยังไม่มี namespace — สร้าง space ของตัวเองหรือเข้าร่วมกลุ่มก่อน")
		return 0, false
	}
	return *user.NamespaceID, true
}
