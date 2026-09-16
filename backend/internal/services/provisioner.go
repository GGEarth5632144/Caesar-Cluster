package services

import (
	"context"
	"fmt"
	"io"

	"backend/internal/entity"
)

// K8sNamespaceName แปลง id ของ entity.Namespace เป็นชื่อ namespace จริงบนคลัสเตอร์
//
// ใช้ id ไม่ใช่คอลัมน์ namespaces.name เพราะชื่อในตารางเป็นชื่อที่ผู้ใช้พิมพ์เอง
// (NamespaceController.Create รับ name จาก body ตรงๆ) จึงมีเว้นวรรค ตัวพิมพ์ใหญ่ หรือภาษาไทยได้
// ซึ่ง k8s ไม่รับ — ชื่อ namespace ต้องเป็น RFC 1123 label (a-z, 0-9, '-' ยาวไม่เกิน 63 ตัว)
// ส่วน id เป็น serial จึงถูกกติกาและไม่ซ้ำเสมอ โดยไม่ต้องไป validate/แปลงชื่อของผู้ใช้เลย
//
// ชื่อที่ผู้ใช้ตั้งไม่ได้หายไปไหน — EnsureNamespace เก็บไว้เป็น annotation บน namespace
// ให้ยังมองจากฝั่งคลัสเตอร์ออกว่า ns-7 คือ space ชื่ออะไรของใคร
//
// ทุก method ของ Provisioner ที่รับ nsName คาดหวังค่าที่ผ่านฟังก์ชันนี้มาแล้ว ไม่ใช่ ns.Name ดิบๆ
func K8sNamespaceName(id int) string { return fmt.Sprintf("ns-%d", id) }

// LogOptions คุมว่าจะดึง log กลับมาแบบไหน — ตรงกับตัวเลือกที่หน้า log viewer ให้ผู้ใช้ปรับได้
// (คล้ายแผง log ของ Cloud Run: จำนวนบรรทัดย้อนหลัง, ช่วงเวลา, และโหมด "ไหลสด" หรือไม่)
type LogOptions struct {
	TailLines    int64 // ดึงกี่บรรทัดล่าสุด — 0 = ใช้ค่า default ของ provisioner
	SinceSeconds int64 // ดึงย้อนหลังกี่วินาที — 0 = ไม่จำกัด (เท่าที่ node ยังเก็บไว้)
	Follow       bool  // true = ไม่ปิด stream หลังส่ง log เดิมหมด รอส่งบรรทัดใหม่ต่อไปเรื่อยๆ
	Timestamps   bool  // true = แต่ละบรรทัดมี timestamp ของ container runtime นำหน้า
}

// WorkloadPhase = สภาพจริงของ workload บนคลัสเตอร์ ณ วินาทีที่ถาม
// (ต่างจาก entity.Service.Status ที่เป็นสิ่งที่ระบบเราบันทึกไว้ — ServiceHealthMonitor เป็นตัวแปลง)
type WorkloadPhase string

const (
	PhasePending   WorkloadPhase = "pending"   // object มีแล้ว แต่ยังไม่มี Pod รัน (รอ node ว่าง / กำลังดึง image)
	PhaseRunning   WorkloadPhase = "running"   // Pod รันอยู่จริงและพร้อมใช้งาน
	PhaseCrashLoop WorkloadPhase = "crashloop" // container ขึ้นแล้วตายซ้ำๆ
	PhaseFailed    WorkloadPhase = "failed"    // พังแบบที่ไม่น่าหายเอง
	PhaseGone      WorkloadPhase = "gone"      // ไม่เจอ workload นี้บนคลัสเตอร์แล้ว
)

// WorkloadStatus = คำตอบของ Provisioner.Status
//
// Reason/Message ต้องกรอกให้ครบทุกครั้งที่ Phase ไม่ใช่ Running เพราะนี่คือข้อมูลเดียว
// ที่ผู้ใช้จะได้เห็นว่าทำไม service ของตัวเองถึงไม่ขึ้น — ปล่อยว่างไว้เท่ากับบอกแค่ว่า "พัง"
type WorkloadStatus struct {
	Phase WorkloadPhase

	// Reason = รหัสสั้นๆ จากคลัสเตอร์ (CrashLoopBackOff, ImagePullBackOff, FailedScheduling, OOMKilled)
	Reason string

	// Message = คำอธิบาย + log ท้ายๆ ก่อน container ตาย — ต้องใส่ log มาด้วยตอน crash loop
	// เพราะ Pod ถูกสร้างใหม่เรื่อยๆ แล้ว log รอบที่บอกสาเหตุจริงหายก่อนผู้ใช้จะทันเปิดดู
	Message string

	// Restarts = จำนวนครั้งที่ container ถูกสร้างใหม่
	Restarts int
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
	// nsName ที่ method นี้และทุก method ด้านล่างรับ คือชื่อบนคลัสเตอร์ที่ได้จาก
	// K8sNamespaceName(ns.ID) ไม่ใช่ ns.Name (ดูเหตุผลที่ K8sNamespaceName)
	//
	// ต้อง idempotent: ถ้า namespace ไม่มีอยู่บนคลัสเตอร์แล้ว ให้คืน nil ไม่ใช่ error
	// เพราะ NamespaceManager.Delete ถอนของบนคลัสเตอร์ก่อนแล้วค่อยลบแถวใน DB — ถ้าล้มกลางคัน
	// การสั่งลบซ้ำต้องเดินจนจบได้ ไม่งั้น namespace นั้นจะค้างใน DB ตลอดกาล (500 วนไปเรื่อยๆ)
	DeleteNamespace(ctx context.Context, nsName string) error

	// DeployService สร้าง workload จริงเข้าไปใน namespace ที่กำหนด
	// (สเปกต่อ 1 Pod มาจาก svc.CPUMilli/RAMMB, จำนวน Pod จาก svc.Replicas, พอร์ตจาก svc.ContainerPort)
	//
	// ทางเดินแยกด้วยสองแกนที่เป็นอิสระต่อกัน:
	//   svc.HasStorage() → workload: false = Deployment, true = StatefulSet + PVC (ตรึง 1 pod)
	//                      StatefulSet ปิด pod เก่าก่อนเปิดตัวใหม่ สอง pod จึงไม่แย่งเขียนดิสก์ก้อนเดียวกัน
	//   svc.IsDatabase   → เครือข่าย: false = Service ชนิด NodePort แล้วเซ็ต svc.NodePort กลับเข้า struct เดิม
	//                      true = Service ชนิด ClusterIP + NetworkPolicy และห้ามจ่าย NodePort (ปล่อย nil)
	//                      — ไม่เคยจองพอร์ตบน node ก็ไม่มีประตูให้เคาะ ซึ่งเชื่อถือได้กว่าการหวังให้ NetworkPolicy ทำงานถูก
	//
	// database มีดิสก์เสมอ (ServiceManager.Create บังคับ) จึงเหลือ 3 แบบ: web, web + ดิสก์ (เช่น Nextcloud), database
	//
	// ServiceManager.Create เป็นคนเอา NodePort ไป UPDATE ลง DB อีกที — provisioner ไม่รู้จัก DB
	DeployService(ctx context.Context, nsName string, svc *entity.Service) error

	// ScaleService เปลี่ยนจำนวน Pod ของ workload ที่ deploy ไปแล้ว (Deployment.spec.replicas)
	// แยกจาก DeployService เพราะแตะแค่จำนวน Pod ไม่ยุ่งกับ Service/NodePort ที่จ่ายไปแล้ว
	// (NodePort ต้องคงเดิม ไม่งั้น URL ที่ผู้ใช้ถืออยู่จะใช้ไม่ได้)
	ScaleService(ctx context.Context, nsName, svcName string, replicas int) error

	// UpdateService ทำให้ workload บนคลัสเตอร์ตรงกับ svc (ค่าใหม่) โดยรู้ว่าของเดิมคือ oldSvc
	//
	// ถ้าชื่อและชนิด workload เท่าเดิม ให้แก้ของเดิมในที่ (pod template, replicas, พอร์ตของ Service)
	// ไม่ลบแล้วสร้างใหม่ เพราะ:
	//   - service ที่มีดิสก์: ลบ = ลบ PVC = ข้อมูลหาย และ PVC ใหม่ชื่อเดิมจะชนกับโฟลเดอร์ NFS
	//     ที่ provisioner ยังลบไม่เสร็จ (pod ค้าง "stale NFS file handle" — docs 030)
	//   - ทุก service: Service เดิมคงอยู่ = เลข NodePort ไม่เปลี่ยน URL ที่ผู้ใช้ถืออยู่ยังใช้ได้
	// และเซ็ต svc.NodePort กลับเป็นเลขเดิม (database ปล่อย nil) ให้ ServiceManager เขียนลง DB
	//
	// service ที่มีดิสก์ห้ามเปลี่ยนชื่อ / ขนาดดิสก์ / data_path / สวิตช์ database / ถอดดิสก์ออก
	// → คืน ErrStorageImmutable (ServiceManager.Update ตรวจก่อนแล้ว ที่นี่กันซ้ำอีกชั้น)
	// service ไม่มีดิสก์ที่เปลี่ยนชื่อหรือขอดิสก์เพิ่ม ต้องเปลี่ยนชนิด/ชื่อ object จึงลบแล้วสร้างใหม่ได้ (ไม่มีข้อมูลให้หาย)
	UpdateService(ctx context.Context, nsName string, oldSvc, svc *entity.Service) error

	// DeleteService ลบ workload ตัวเดียวออกจาก namespace
	//
	// รับทั้ง svc ไม่ใช่แค่ชื่อ เพราะของที่ต้องถอนต่างกันตามสองแกนเดียวกับ DeployService
	// (HasStorage → StatefulSet + PVC แทน Deployment, IsDatabase → NetworkPolicy เพิ่ม)
	//
	// PVC เป็นจุดที่พลาดง่ายที่สุด: k8s ไม่ลบ PVC ที่เกิดจาก volumeClaimTemplates ให้เองตอนลบ
	// StatefulSet ไม่ตามไปลบแล้วดิสก์จะถูกจองค้างโดยไม่มีแถวใน DB ให้ตามเก็บ
	DeleteService(ctx context.Context, nsName string, svc *entity.Service) error

	// Status ถามคลัสเตอร์ว่า workload นี้กำลังทำอะไรอยู่จริงๆ
	//
	// มีไว้เพราะ DeployService คืน nil ไม่ได้แปลว่า workload รันอยู่ — ถ้าไม่มีเมธอดนี้ ระบบจะเขียน
	// running ลง DB ทุกครั้งที่ deploy "ผ่าน" ซึ่งโกหกในสองเคสที่เจอบ่อยที่สุด (ทรัพยากรไม่พอ / env ไม่ครบ)
	//
	// error ที่คืน = "ถามไม่ได้" (คลัสเตอร์ล่ม/เน็ตมีปัญหา) ต่างจาก PhaseGone ที่แปลว่า
	// "ถามได้แล้ว และของไม่อยู่จริงๆ" — ผู้เรียกจัดการสองอย่างนี้คนละแบบ
	Status(ctx context.Context, nsName string, svc *entity.Service) (WorkloadStatus, error)

	// Logs เปิด stream ของ log จาก container ที่รัน service นี้อยู่
	// ผู้เรียกมีหน้าที่ Close() เสมอ ไม่งั้น connection ค้างไว้กับ Kubernetes API
	//
	// opts.Follow = true แล้วการอ่านจะไม่จบเองจนกว่า ctx จะถูก cancel — ผู้เรียกต้องผูก ctx
	// กับอายุของ HTTP request ไว้ ไม่ปล่อยให้เปิดค้างตลอดกาล
	Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error)
}
