package services

import (
	"context"
	"io"

	"backend/internal/entity"
)

// LogOptions คุมว่าจะดึง log กลับมาแบบไหน — ตรงกับตัวเลือกที่หน้า log viewer ให้ผู้ใช้ปรับได้
// (คล้ายแผง log ของ Cloud Run: จำนวนบรรทัดย้อนหลัง, ช่วงเวลา, และโหมด "ไหลสด" หรือไม่)
type LogOptions struct {
	TailLines    int64 // ดึงกี่บรรทัดล่าสุด — 0 = ใช้ค่า default ของ provisioner
	SinceSeconds int64 // ดึงย้อนหลังกี่วินาที — 0 = ไม่จำกัด (เท่าที่ node ยังเก็บไว้)
	Follow       bool  // true = ไม่ปิด stream หลังส่ง log เดิมหมด รอส่งบรรทัดใหม่ต่อไปเรื่อยๆ
	Timestamps   bool  // true = แต่ละบรรทัดมี timestamp ของ container runtime นำหน้า
}

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
	//
	// ต้อง idempotent: ถ้า namespace ไม่มีอยู่บนคลัสเตอร์แล้ว ให้คืน nil ไม่ใช่ error
	// เพราะ NamespaceManager.Delete ถอนของบนคลัสเตอร์ก่อนแล้วค่อยลบแถวใน DB — ถ้าล้มกลางคัน
	// การสั่งลบซ้ำต้องเดินจนจบได้ ไม่งั้น namespace นั้นจะค้างใน DB ตลอดกาล (500 วนไปเรื่อยๆ)
	DeleteNamespace(ctx context.Context, nsName string) error

	// DeployService สร้าง workload จริงเข้าไปใน namespace ที่กำหนด
	// (resource request/limit ของ container มาจาก svc.CPUMilli / svc.RAMMB)
	// สำเร็จแล้วต้องเซ็ต svc.NodePort กลับเข้า struct เดิม (k8s Service ชนิด NodePort เป็นตัวจ่าย port ให้)
	// ServiceManager.Create เป็นคนเอาไป UPDATE ลง DB อีกที — provisioner ไม่รู้จัก DB
	DeployService(ctx context.Context, nsName string, svc *entity.Service) error

	// DeleteService ลบ workload ตัวเดียวออกจาก namespace
	DeleteService(ctx context.Context, nsName, svcName string) error

	// Logs เปิด stream ของ log จาก container ที่รัน service นี้อยู่
	// ผู้เรียกมีหน้าที่ Close() เสมอ ไม่งั้น connection ค้างไว้กับ Kubernetes API
	//
	// opts.Follow = true แล้วการอ่านจะไม่จบเองจนกว่า ctx จะถูก cancel — ผู้เรียกต้องผูก ctx
	// กับอายุของ HTTP request ไว้ ไม่ปล่อยให้เปิดค้างตลอดกาล
	Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error)
}
