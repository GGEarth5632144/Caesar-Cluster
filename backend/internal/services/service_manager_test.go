package services

import (
	"errors"
	"testing"
)

// TestCheckImageAllowed ครอบกฎ "จำกัด image ที่ผู้ใช้รันได้" ซึ่งเป็นด่านกัน image ขุดเหรียญ
//
// จุดที่พลาดง่ายที่สุดคือชื่อย่อของ Docker: "nginx" กับ "docker.io/library/nginx" เป็น image เดียวกัน
// ถ้าเทียบ prefix กับ string ดิบๆ allowlist ที่เขียนแบบเต็มจะบล็อกชื่อย่อทิ้งทั้งที่ควรผ่าน
// เทสต์ตัวนี้ล็อกพฤติกรรมนั้นไว้ อยู่ในแพ็กเกจเดียวกันเพราะเรียก checkImageAllowed ตรงๆ
// โดยไม่ต้องมี DB (ฟังก์ชันนี้ไม่แตะ db/quota/prov เลย ส่ง nil เข้าไปได้)
func TestCheckImageAllowed(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		image   string
		wantOK  bool
	}{
		{"ไม่ตั้ง allowlist = ผ่านทุก image", nil, "evil/miner:latest", true},
		{"ชื่อย่อของ docker hub ถูก normalize ก่อนเทียบ", []string{"docker.io/library/"}, "nginx:1.27", true},
		{"ชื่อเต็มที่ตรง prefix ผ่าน", []string{"docker.io/library/"}, "docker.io/library/postgres:16", true},
		{"image ของ user บน docker hub ไม่ใช่ library", []string{"docker.io/library/"}, "someone/miner", false},
		{"registry อื่นที่อยู่ในรายการผ่าน", []string{"ghcr.io/"}, "ghcr.io/org/app:v1", true},
		{"registry อื่นที่ไม่อยู่ในรายการไม่ผ่าน", []string{"ghcr.io/"}, "quay.io/org/app:v1", false},
		{"หลาย prefix ตัวไหนตรงก็ผ่าน", []string{"ghcr.io/", "registry.k8s.io/"}, "registry.k8s.io/pause:3.9", true},
		{"ท่อนแรกที่มี port ถือเป็น registry ไม่ใช่ชื่อ user", []string{"localhost:5000/"}, "localhost:5000/app", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := NewServiceManager(nil, nil, nil, tc.allowed)
			err := mgr.checkImageAllowed(tc.image)

			if tc.wantOK {
				if err != nil {
					t.Fatalf("คาดว่า %q ผ่าน แต่ได้ error: %v", tc.image, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("คาดว่า %q ถูกบล็อก แต่ผ่านไปได้", tc.image)
			}
			// ต้อง wrap sentinel ไว้ ไม่งั้น ServiceController แปลงเป็น 400 ไม่ได้ จะกลายเป็น 500
			if !errors.Is(err, ErrImageNotAllowed) {
				t.Fatalf("error ต้อง wrap ErrImageNotAllowed แต่ได้: %v", err)
			}
		})
	}
}

// TestNormalizeImageRef ล็อกกฎการเติมส่วนที่ Docker ละไว้ ให้ตรงกับที่ Docker เองตีความ
func TestNormalizeImageRef(t *testing.T) {
	cases := map[string]string{
		"nginx":                        "docker.io/library/nginx",
		"nginx:1.27":                   "docker.io/library/nginx:1.27",
		"bitnami/redis":                "docker.io/bitnami/redis",
		"ghcr.io/org/app:v1":           "ghcr.io/org/app:v1",
		"localhost:5000/app":           "localhost:5000/app",
		"registry.example.com/x/y:tag": "registry.example.com/x/y:tag",
	}

	for input, want := range cases {
		if got := normalizeImageRef(input); got != want {
			t.Errorf("normalizeImageRef(%q) = %q ต้องได้ %q", input, got, want)
		}
	}
}
