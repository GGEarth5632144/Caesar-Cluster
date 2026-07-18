package services

import (
	"context"
	"fmt"

	"backend/internal/entity"
)

// KubernetesProvisioner = provisioner ของจริงที่จะคุยกับ Kubernetes API (ยังไม่ได้ implement)
// ถูกเลือกใช้ใน main เมื่อ PROVISIONER=kubernetes
//
// ตอนลงมือทำจริง ให้ใช้ k8s.io/client-go แล้วสร้าง clientset จาก kubeConfig
// (ถ้า kubeConfig ว่าง = รันอยู่ใน cluster เอง ให้ใช้ rest.InClusterConfig())
//
// โครงที่ต้องสร้างต่อ namespace 1 อัน:
//  1. Namespace
//  2. ResourceQuota — requests.cpu / requests.memory / count(pods) ตาม limit ของ entity.Namespace
//     (นี่คือตัวบังคับโควตาชั้นสุดท้าย ต่อให้ backend เราพลาด k8s ก็ยังไม่ให้เกิน)
//  3. LimitRange — กันไม่ให้ container ที่ไม่ได้ระบุ resource แอบกินเกิน
//  4. NetworkPolicy default-deny — กัน traffic ข้าม namespace (ข้อกำหนดเรื่องแยก network)
type KubernetesProvisioner struct {
	kubeConfig string // path ของ kubeconfig; ว่าง = in-cluster
}

// NewKubernetesProvisioner ประกอบ provisioner — ถูกเรียกจาก main เมื่อ PROVISIONER=kubernetes
func NewKubernetesProvisioner(kubeConfig string) *KubernetesProvisioner {
	return &KubernetesProvisioner{kubeConfig: kubeConfig}
}

// EnsureNamespace (ยังไม่ทำ) — จะสร้าง Namespace + ResourceQuota + LimitRange + NetworkPolicy
// data flow (แผน): รับ entity.Namespace จาก NamespaceManager → แปลง limit เป็น k8s resource spec → apply เข้า cluster
func (k *KubernetesProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (EnsureNamespace)")
}

// DeleteNamespace (ยังไม่ทำ) — จะลบ Namespace ทิ้งทั้งก้อน
// data flow (แผน): รับชื่อ namespace จาก NamespaceManager → เรียก CoreV1().Namespaces().Delete()
func (k *KubernetesProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (DeleteNamespace)")
}

// DeployService (ยังไม่ทำ) — จะสร้าง Deployment + Service ชนิด NodePort ใน namespace ที่กำหนด
// data flow (แผน): รับ entity.Service จาก ServiceManager.Create → ตั้ง resources.requests/limits
// จาก CPUMilli ("300m") และ RAMMB ("2048Mi") → apply Deployment + Service(type=NodePort) เข้า cluster
// → อ่าน nodePort ที่ k8s สุ่มจ่ายให้ (หรือระบุเองถ้าอยากคุมเลข) → เซ็ตกลับที่ svc.NodePort ก่อน return
func (k *KubernetesProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (DeployService)")
}

// DeleteService (ยังไม่ทำ) — จะลบ Deployment ตัวเดียวออกจาก namespace
// data flow (แผน): รับ namespace + ชื่อ service จาก ServiceManager.Delete → เรียก AppsV1().Deployments().Delete()
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (DeleteService)")
}
