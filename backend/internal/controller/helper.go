package controller

import (
	"net/http"
	"regexp"
	"strings"

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

// องค์ประกอบของ image reference ตามไวยากรณ์จริงของ Docker/OCI
// (อ้างอิง grammar ของ github.com/distribution/reference ซึ่งเป็นตัวที่ containerd ใช้ parse จริง)
//
// ทำไมต้องเช็คเองแทนที่จะปล่อยให้ k8s ปฏิเสธ: ถ้าปล่อยผ่านไป workflow จะเดินไปไกลมาก
// คือจองโควตา → INSERT แถว → สร้าง Deployment สำเร็จ → pod ค่อยพังตอน pull ด้วย
// ErrImagePull ซึ่งผู้ใช้เห็นแค่ service ที่ status เป็น running แต่ใช้งานไม่ได้ (ยังไม่มี
// reconcile loop มาแก้สถานะให้) เช็คตั้งแต่ที่ API จบใน 400 เดียว บอกสาเหตุได้ตรงจุด
var (
	// imageDomainPattern = ส่วน registry เช่น ghcr.io, registry.k8s.io, localhost:5000
	// ยอมให้มีตัวพิมพ์ใหญ่ได้ เพราะ hostname ไม่ case-sensitive (ต่างจากส่วน path)
	imageDomainPattern = regexp.MustCompile(
		`^(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])` +
			`(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))*(?::[0-9]+)?$`)

	// imagePathPattern = ส่วนชื่อ เช่น library/nginx, you/app
	// ต้องเป็นตัวพิมพ์เล็กเท่านั้น — Docker ปฏิเสธตัวพิมพ์ใหญ่ในส่วนนี้ (จุดที่คนพลาดบ่อยที่สุด)
	imagePathPattern = regexp.MustCompile(
		`^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*` +
			`(?:/[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*)*$`)

	// imageTagPattern = ส่วนหลัง ":" เช่น 1.27-alpine (ยาวได้ไม่เกิน 128 ตัวตามสเปก)
	imageTagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

	// imageDigestPattern = ส่วนหลัง "@" เช่น sha256:<hex 64 ตัว>
	imageDigestPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}$`)
)

// maxImageRefLength = ความยาวสูงสุดของ image ให้ตรงกับ varchar(200) ของคอลัมน์ services.image
const maxImageRefLength = 200

// isValidImageRef เช็คว่า image ที่ผู้ใช้กรอกเป็น reference ที่ Docker/containerd parse ได้จริง
//
// data flow: ถูกเรียกจาก ServiceController.Create ก่อนส่งต่อให้ service layer
// (แบบเดียวกับ isValidK8sName) — ตรวจ "รูปแบบ" อย่างเดียว ส่วนจะอนุญาต registry ไหน
// เป็นกติกาธุรกิจที่ ServiceManager.checkImageAllowed ดูแลแยกต่างหาก
//
// รองรับครบทุกรูปที่ Docker ยอมรับ: nginx, nginx:1.27, bitnami/redis,
// ghcr.io/org/app:v1, localhost:5000/app, nginx@sha256:<hex>, และแบบมีทั้ง tag และ digest
func isValidImageRef(image string) bool {
	if image == "" || len(image) > maxImageRefLength {
		return false
	}

	remainder := image

	// digest มาท้ายสุดเสมอ ตัดออกก่อน
	if i := strings.Index(remainder, "@"); i >= 0 {
		if !imageDigestPattern.MatchString(remainder[i+1:]) {
			return false
		}
		remainder = remainder[:i]
	}

	// tag คือ ":" ที่อยู่ "หลัง" slash ตัวสุดท้ายเท่านั้น
	// ถ้าไม่เช็คเงื่อนไขนี้ localhost:5000/app จะถูกอ่านว่า tag = "5000/app" ทั้งที่เป็น port ของ registry
	if i := strings.LastIndex(remainder, ":"); i >= 0 && i > strings.LastIndex(remainder, "/") {
		if !imageTagPattern.MatchString(remainder[i+1:]) {
			return false
		}
		remainder = remainder[:i]
	}

	// ท่อนแรกเป็น registry ก็ต่อเมื่อมีจุดหรือ colon อยู่ในนั้น หรือเป็น localhost
	// (กฎเดียวกับ services.normalizeImageRef ที่ใช้ตอนเทียบ allowlist — ต้องตรงกันเสมอ
	// ไม่งั้นจะมี image ที่ผ่านด่านนี้แต่ไปตกด่าน allowlist ด้วยเหตุผลที่อธิบายไม่ได้)
	name := remainder
	if parts := strings.SplitN(remainder, "/", 2); len(parts) == 2 &&
		(strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		if !imageDomainPattern.MatchString(parts[0]) {
			return false
		}
		name = parts[1]
	}

	return imagePathPattern.MatchString(name)
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
