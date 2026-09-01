package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"backend/internal/config"
	"backend/internal/entity"
)

// TestNewKubernetesProvisionerRejectsBadKubeconfig ยืนยันว่า kubeconfig ที่ใช้ไม่ได้
// ทำให้ server "ไม่ยอม start" ตั้งแต่ต้น ไม่ใช่ปล่อยขึ้นมาแล้วไปพังตอนผู้ใช้กด deploy
//
// เป็นพฤติกรรมที่ main พึ่งอยู่ (log.Fatalf เมื่อ constructor คืน error) และเป็นจุดที่พังเงียบได้ง่าย
// ถ้ามีใครแก้ให้คืน provisioner พร้อม error เป็น nil ในอนาคต
func TestNewKubernetesProvisionerRejectsBadKubeconfig(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(badPath, []byte("this is not a kubeconfig"), 0o600); err != nil {
		t.Fatalf("เขียนไฟล์ทดสอบไม่สำเร็จ: %v", err)
	}

	// ระบุ path ตรงๆ เสมอ ไม่ส่งค่าว่าง — ค่าว่างจะไปหา ~/.kube/config ของเครื่องที่รันเทสต์
	// ทำให้ผลลัพธ์ขึ้นกับว่าเครื่องนั้นมีคลัสเตอร์อยู่หรือเปล่า
	prov, err := NewKubernetesProvisioner(badPath, config.K8sConfig{RequestTimeoutSeconds: 2})
	if err == nil {
		t.Fatal("kubeconfig ที่ใช้ไม่ได้ต้องคืน error เพื่อให้ main หยุด server ตั้งแต่ต้น")
	}
	if prov != nil {
		t.Fatal("เมื่อคืน error แล้วต้องไม่คืน provisioner ที่ใช้งานไม่ได้ออกไปด้วย")
	}
}

// TestBlockedEgressCIDRsDeduplicates — k8s ปฏิเสธ NetworkPolicy ที่มี except ซ้ำกัน
// และ pod CIDR มักถูกใส่มาใน K8S_BLOCKED_EGRESS_CIDRS ซ้ำกับที่ตั้งไว้ใน K8S_POD_CIDR อยู่แล้ว
func TestBlockedEgressCIDRsDeduplicates(t *testing.T) {
	k := &KubernetesProvisioner{cfg: config.K8sConfig{
		PodCIDR:            "172.16.0.0/16",
		BlockedEgressCIDRs: []string{"172.16.0.0/16", " 192.168.100.0/24 ", "", "192.168.100.0/24"},
	}}

	got := k.blockedEgressCIDRs()
	want := []string{"172.16.0.0/16", "192.168.100.0/24"}

	if len(got) != len(want) {
		t.Fatalf("ได้ %v ต้องได้ %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ได้ %v ต้องได้ %v", got, want)
		}
	}
}

// TestAddedCapabilities ล็อกบทเรียนจากการทดสอบจริงบนคลัสเตอร์: ตอนแรก drop ALL แล้วคืนแค่
// NET_BIND_SERVICE ทำให้ nginx:1.27-alpine crash ทันทีด้วย "chown ... Operation not permitted"
// เพราะ entrypoint ของมันต้อง chown cache dir แล้วลดสิทธิ์ตัวเองลงเป็น user nginx ก่อนรันจริง
//
// เทสต์นี้กันไม่ให้ใครเผลอตัดชุด capability ของ baseline กลับไปเป็นแบบเดิม
// และกันไม่ให้ capability อันตรายหลุดเข้ามาในทั้งสองระดับ
func TestAddedCapabilities(t *testing.T) {
	baseline := addedCapabilities(config.PodSecurityBaseline)
	restricted := addedCapabilities(config.PodSecurityRestricted)

	// image ทั่วไปที่รันเป็น root แล้วลดสิทธิ์ตัวเองต้องใช้สามตัวนี้เป็นอย่างน้อย
	for _, need := range []string{"CHOWN", "SETUID", "SETGID", "NET_BIND_SERVICE"} {
		if !hasCapability(baseline, need) {
			t.Errorf("baseline ต้องคืน %s ให้ ไม่งั้น image อย่าง nginx จะ crash ตั้งแต่ start", need)
		}
	}

	// restricted ยอมให้ add ได้แค่ NET_BIND_SERVICE ตัวเดียวตามสเปกของ Pod Security Admission
	if len(restricted) != 1 || !hasCapability(restricted, "NET_BIND_SERVICE") {
		t.Errorf("restricted ต้องคืนแค่ NET_BIND_SERVICE เท่านั้น แต่ได้ %v", restricted)
	}

	// ตัวที่ห้ามหลุดเข้ามาไม่ว่าระดับไหน — พวกนี้คือทางหนีออกจาก container
	for _, level := range [][]corev1.Capability{baseline, restricted} {
		for _, banned := range []string{"NET_RAW", "SYS_ADMIN", "SYS_PTRACE", "SYS_MODULE", "MKNOD"} {
			if hasCapability(level, banned) {
				t.Errorf("ห้ามคืน %s ให้ container ของผู้ใช้เด็ดขาด", banned)
			}
		}
	}
}

func hasCapability(list []corev1.Capability, want string) bool {
	for _, c := range list {
		if string(c) == want {
			return true
		}
	}
	return false
}

// TestEnsureNamespaceClassifiesConflicts ล็อกบทเรียนจากการทดสอบบนคลัสเตอร์จริง:
// ตอนแรก EnsureNamespace คืน error ธรรมดาเวลาเจอ namespace ชื่อซ้ำที่ไม่ใช่ของเรา
// ทำให้ controller แยกประเภทไม่ออกแล้วตอบ 500 INTERNAL ทั้งที่ผู้ใช้แค่ต้องเปลี่ยนชื่อ
// เหตุผลจริงไปโผล่แค่ใน log ฝั่ง server ซึ่งผู้ใช้ไม่มีทางเห็น
//
// ใช้ fake clientset แทนคลัสเตอร์จริง เพราะสิ่งที่ต้องพิสูจน์คือ "แปลงเป็น error ตัวไหน"
// ไม่ใช่ "คุยกับ k8s ได้ไหม" ซึ่งทดสอบไปแล้วด้วยคลัสเตอร์ kind
func TestEnsureNamespaceClassifiesConflicts(t *testing.T) {
	cases := []struct {
		name     string
		existing *corev1.Namespace
		wantErr  error
	}{
		{
			name: "มีชื่อนี้อยู่แล้วแต่ไม่ใช่ของเรา ต้องบอกว่าชื่อซ้ำ ให้ไปตั้งชื่ออื่น",
			existing: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-system"},
			},
			wantErr: ErrNameTaken,
		},
		{
			name: "ของเราเองแต่ยังตายไม่สนิท ต้องบอกให้รอ ไม่ใช่ให้เปลี่ยนชื่อ",
			existing: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "space-x",
					Labels: map[string]string{labelManagedBy: labelManagedValue},
				},
				Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
			},
			wantErr: ErrNamespaceTerminating,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := &KubernetesProvisioner{
				cs:      fake.NewSimpleClientset(tc.existing),
				cfg:     config.K8sConfig{PodCIDR: "172.16.0.0/16", PodSecurity: config.PodSecurityBaseline},
				timeout: 5 * time.Second,
			}

			err := k.EnsureNamespace(context.Background(), &entity.Namespace{
				ID: 1, Name: tc.existing.Name, CPULimitMilli: 3000, RAMLimitMB: 2048,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ต้องได้ error ที่ wrap %v เพื่อให้ controller ตอบ 409 ได้ แต่ได้: %v", tc.wantErr, err)
			}
		})
	}
}
