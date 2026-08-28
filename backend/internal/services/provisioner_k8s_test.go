package services

import (
	"os"
	"path/filepath"
	"testing"

	"backend/internal/config"
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
