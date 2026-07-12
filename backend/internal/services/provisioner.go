package services

import (
	"context"

	"backend/internal/entity"
)

// Provisioner คือสัญญาว่า "ตัวสร้างของจริงบน cluster" ต้องทำอะไรได้บ้าง
// เป็นจุดเดียวที่ผูกกับ Kubernetes — ส่วน service layer ที่เหลือไม่รู้จัก k8s เลย
// ทำให้สลับไป mock ตอน dev ได้โดยไม่ต้องแก้ logic ธุรกิจสักบรรทัด
//
// ข้อสังเกตสำคัญ: ที่นี่ไม่มี "เลือก node" เพราะบน k8s เป็นหน้าที่ของ scheduler ของ k8s เอง
// หน้าที่ของเราคือกำหนดขอบเขต (namespace + ResourceQuota) แล้วโยน workload เข้าไป
type Provisioner interface {
	// EnsureNamespace สร้าง namespace บน cluster พร้อม ResourceQuota (ตาม limit ใน entity.Namespace)
	// และ NetworkPolicy แบบ default-deny เพื่อกันไม่ให้ namespace คุยข้ามกัน
	// (ทุก node อยู่บน switch เดียวกัน เลยต้องกั้นที่ระดับ k8s ให้ชัด)
	EnsureNamespace(ctx context.Context, ns *entity.Namespace) error

	// DeleteNamespace ลบ namespace ทิ้งทั้งก้อน (workload ข้างในหายตามหมด)
	DeleteNamespace(ctx context.Context, nsName string) error

	// DeployService สร้าง workload จริงเข้าไปใน namespace ที่กำหนด
	// (resource request/limit ของ container มาจาก svc.CPUMilli / svc.RAMMB)
	DeployService(ctx context.Context, nsName string, svc *entity.Service) error

	// DeleteService ลบ workload ตัวเดียวออกจาก namespace
	DeleteService(ctx context.Context, nsName, svcName string) error
}
