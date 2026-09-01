package controller

import "testing"

// TestIsValidImageRef ล็อกกฎรูปแบบ image reference ไว้
//
// จุดที่พลาดง่ายที่สุดคือ colon ทำหน้าที่ได้ 2 อย่าง: เป็น port ของ registry (localhost:5000/app)
// กับเป็นตัวคั่น tag (nginx:1.27) แยกกันด้วยตำแหน่งเทียบกับ slash ตัวสุดท้ายเท่านั้น
// ถ้า parse ผิด localhost:5000/app จะถูกอ่านว่า tag = "5000/app" แล้วถูกปฏิเสธทั้งที่ถูกต้อง
func TestIsValidImageRef(t *testing.T) {
	valid := []string{
		"nginx",
		"nginx:1.27-alpine",
		"nginx:latest",
		"bitnami/redis",
		"bitnami/redis:7.2",
		"ghcr.io/org/app:v1.2.3",
		"registry.k8s.io/pause:3.9",
		"docker.io/library/postgres:16",
		"localhost:5000/app",                 // colon เป็น port ไม่ใช่ tag
		"localhost:5000/app:dev",             // มีทั้ง port และ tag
		"registry.example.com:8443/a/b/c:v1", // path ซ้อนหลายชั้น
		"my-app_1/sub.name",                  // separator ที่สเปกอนุญาตในส่วน path
		"nginx@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"ghcr.io/org/app:v1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, image := range valid {
		if !isValidImageRef(image) {
			t.Errorf("%q ควรผ่าน แต่ถูกปฏิเสธ", image)
		}
	}

	invalid := map[string]string{
		"":                     "ค่าว่าง",
		"   ":                  "ช่องว่างล้วน",
		"nginx latest":         "มีช่องว่างอยู่ข้างใน",
		"NGINX":                "ชื่อ image ห้ามเป็นตัวพิมพ์ใหญ่",
		"Bitnami/redis":        "ตัวพิมพ์ใหญ่ในส่วน path",
		"nginx:":               "มี colon แต่ไม่มี tag",
		"nginx:-bad":           "tag ห้ามขึ้นต้นด้วยขีดกลาง",
		"nginx::1.27":          "colon ซ้อน",
		"/nginx":               "ขึ้นต้นด้วย slash",
		"nginx/":               "ลงท้ายด้วย slash",
		"nginx//app":           "slash ซ้อน",
		"-nginx":               "ขึ้นต้นด้วยขีดกลาง",
		"nginx@sha256:xyz":     "digest ไม่ใช่ hex",
		"nginx@sha256:abc123":  "digest สั้นเกินไป",
		"http://nginx":         "ใส่ scheme มาด้วย ซึ่ง image reference ไม่มี",
		"nginx:tag with space": "tag มีช่องว่าง",
		// ท่อนแรกมีจุด Docker จึงตีความเป็น hostname ของ registry ทันที
		// แล้ว hostname ห้ามมี underscore — Docker เองก็ปฏิเสธ reference นี้เหมือนกัน
		"my-app_1.test/sub-name": "underscore ในส่วนที่ถูกตีความเป็น registry",
	}
	for image, reason := range invalid {
		if isValidImageRef(image) {
			t.Errorf("%q ควรถูกปฏิเสธ (%s) แต่ผ่านไปได้", image, reason)
		}
	}
}

// TestIsValidImageRefLength — คอลัมน์ services.image เป็น varchar(200)
// ถ้าปล่อยยาวเกินจะไปพังตอน INSERT หลังจองโควตาไปแล้ว ซึ่งกู้ยากกว่ามาก
func TestIsValidImageRefLength(t *testing.T) {
	long := "ghcr.io/org/"
	for len(long) <= maxImageRefLength {
		long += "a"
	}
	if isValidImageRef(long) {
		t.Errorf("image ที่ยาวเกิน %d ตัวต้องถูกปฏิเสธ", maxImageRefLength)
	}
}
