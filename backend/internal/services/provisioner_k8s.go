package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"backend/internal/entity"
)

// ป้ายกำกับที่เราแปะไว้บน namespace ทุกอันที่ระบบนี้สร้าง
//
// แยกเป็น label กับ annotation ตามข้อจำกัดของ k8s: ค่าของ label ต้องเป็น [a-zA-Z0-9._-]
// ยาวไม่เกิน 63 ตัว จึงใส่ได้แค่ค่าที่เรารู้รูปแบบแน่ (id, ชื่อระบบ) ส่วนชื่อที่ผู้ใช้พิมพ์เอง
// (มีเว้นวรรค/ภาษาไทยได้) ต้องไปอยู่ใน annotation ที่ไม่จำกัดรูปแบบ
const (
	labelManagedBy   = "app.kubernetes.io/managed-by"
	labelNamespaceID = "caesar-cluster.io/namespace-id"

	annDisplayName   = "caesar-cluster.io/display-name"
	annContributorID = "caesar-cluster.io/contributor-id"

	managedByCaesar = "caesar-cluster"
)

// KubernetesProvisioner = provisioner ของจริงที่คุยกับ Kubernetes API ผ่าน client-go
// ถูกเลือกใช้ใน main เมื่อ PROVISIONER=kubernetes
//
// โครงที่ต้องมีต่อ namespace 1 อัน (ทำไปแล้ว = ✓):
//  1. Namespace ✓
//  2. ResourceQuota — requests.cpu / requests.memory / count(pods) ตาม limit ของ entity.Namespace
//     (นี่คือตัวบังคับโควตาชั้นสุดท้าย ต่อให้ backend เราพลาด k8s ก็ยังไม่ให้เกิน)
//  3. LimitRange — กันไม่ให้ container ที่ไม่ได้ระบุ resource แอบกินเกิน
//  4. NetworkPolicy default-deny — กัน traffic ข้าม namespace (ข้อกำหนดเรื่องแยก network)
type KubernetesProvisioner struct {
	kubeConfig string // path ของ kubeconfig; ว่าง = in-cluster

	// clientset ถูกสร้างครั้งเดียวตอนถูกใช้ครั้งแรก ไม่ใช่ตอน New
	//
	// ที่ไม่สร้างใน constructor เพราะ NewKubernetesProvisioner ไม่คืน error (main เรียกแบบ
	// assign ตรงๆ) และการอ่าน kubeconfig ก็ล้มได้จริง — เก็บ error ไว้แล้วคืนทุกครั้งที่ถูกเรียก
	// ดีกว่า panic ตอน start หรือกลืน error ไปเงียบๆ
	//
	// หมายเหตุ: การสร้าง clientset ไม่ได้ต่อเน็ตเวิร์ก แค่ parse config — คลัสเตอร์ล่มจะรู้ตอนยิงจริง
	once      sync.Once
	clientset kubernetes.Interface
	clientErr error
}

// NewKubernetesProvisioner ประกอบ provisioner — ถูกเรียกจาก main เมื่อ PROVISIONER=kubernetes
func NewKubernetesProvisioner(kubeConfig string) *KubernetesProvisioner {
	return &KubernetesProvisioner{kubeConfig: kubeConfig}
}

// client คืน clientset ที่พร้อมใช้ (สร้างครั้งแรกที่ถูกเรียก แล้วใช้ซ้ำตลอดอายุ process)
// kubeConfig ว่าง = สมมติว่ารันเป็น Pod อยู่ในคลัสเตอร์ ให้ใช้ ServiceAccount ที่ mount มาให้
func (k *KubernetesProvisioner) client() (kubernetes.Interface, error) {
	k.once.Do(func() {
		var cfg *rest.Config
		var err error

		if k.kubeConfig == "" {
			cfg, err = rest.InClusterConfig()
			if err != nil {
				k.clientErr = fmt.Errorf("อ่าน in-cluster config ไม่สำเร็จ "+
					"(ถ้ารัน backend นอกคลัสเตอร์ ต้องตั้ง KUBECONFIG ให้ชี้ไฟล์ kubeconfig): %w", err)
				return
			}
		} else {
			cfg, err = clientcmd.BuildConfigFromFlags("", k.kubeConfig)
			if err != nil {
				k.clientErr = fmt.Errorf("อ่าน kubeconfig %q ไม่สำเร็จ: %w", k.kubeConfig, err)
				return
			}
		}

		cs, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			k.clientErr = fmt.Errorf("สร้าง kubernetes client ไม่สำเร็จ: %w", err)
			return
		}
		k.clientset = cs
	})
	return k.clientset, k.clientErr
}

// EnsureNamespace สร้าง namespace บนคลัสเตอร์ให้ตรงกับแถวใน DB — ตอนนี้ทำแค่ตัว Namespace เอง
// (ResourceQuota / LimitRange / NetworkPolicy ยังไม่ได้ทำ ดู TODO ท้าย method)
//
// data flow: NamespaceManager.Create (หรือ SetQuota) ส่ง entity.Namespace ที่เพิ่งบันทึกลง DB มา
// → แปลง ns.ID เป็นชื่อบนคลัสเตอร์ด้วย K8sNamespaceName → Create เข้า cluster
//
// ต้อง idempotent เพราะถูกเรียกซ้ำได้จริง: SetQuota เรียกทุกครั้งที่แอดมินปรับโควตาของ space เดิม
// เจอของเดิมอยู่แล้วจึงไม่ใช่ error — แค่ sync metadata (ชื่อที่ผู้ใช้ตั้ง/เจ้าของ) ให้ตรงกับ DB
//
// กรณีที่ต้องแยกให้ออกคือ namespace เดิมยังอยู่ในสถานะ Terminating (การลบ namespace ของ k8s
// เป็น async ใช้เวลาเก็บของข้างในสักพัก) — อันนี้ "รอแล้วลองใหม่ได้" ไม่ใช่ชื่อซ้ำถาวร
// จึงคืน ErrNamespaceTerminating ให้ controller แปลงเป็น 409 พร้อมข้อความว่าให้รอ
// (ดู NamespaceManager.Create ที่ส่ง error ตัวนี้ต่อแบบไม่ห่อทับ)
func (k *KubernetesProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	name := K8sNamespaceName(ns.ID)
	labels := map[string]string{
		labelManagedBy:   managedByCaesar,
		labelNamespaceID: strconv.Itoa(ns.ID),
	}
	annotations := map[string]string{
		annDisplayName:   ns.Name,
		annContributorID: strconv.Itoa(ns.ContributorID),
	}

	_, err = cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}, metav1.CreateOptions{})
	if err == nil {
		log.Printf("[k8s] สร้าง namespace '%s' (space '%s' ของ user id=%d) แล้ว",
			name, ns.Name, ns.ContributorID)
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("สร้าง namespace '%s' บนคลัสเตอร์ไม่สำเร็จ: %w", name, err)
	}

	// มีอยู่แล้ว — เป็นได้ทั้ง "เรียกซ้ำตามปกติ" และ "ตัวเดิมยังลบไม่เสร็จ" ต้องอ่านมาดูก่อนว่าอันไหน
	existing, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// หายไประหว่าง Create กับ Get พอดี = ตัวเดิมเพิ่งลบเสร็จ สั่งใหม่อีกรอบได้เลย
			return fmt.Errorf("%w (namespace '%s')", ErrNamespaceTerminating, name)
		}
		return fmt.Errorf("อ่าน namespace '%s' บนคลัสเตอร์ไม่สำเร็จ: %w", name, err)
	}
	if existing.Status.Phase == corev1.NamespaceTerminating || existing.DeletionTimestamp != nil {
		return fmt.Errorf("%w (namespace '%s')", ErrNamespaceTerminating, name)
	}

	// ของเดิมใช้ได้ — เหลือแค่ดัน metadata ให้ตรงกับ DB (ผู้ใช้อาจเปลี่ยนชื่อ space ทีหลัง)
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	changed := false
	for key, val := range labels {
		if existing.Labels[key] != val {
			existing.Labels[key] = val
			changed = true
		}
	}
	for key, val := range annotations {
		if existing.Annotations[key] != val {
			existing.Annotations[key] = val
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if _, err := cs.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		// ไม่คืน error: สิ่งที่ผู้เรียกต้องการคือ "namespace นี้มีอยู่จริงบนคลัสเตอร์" ซึ่งจริงแล้ว
		// ส่วนที่พลาดเป็นแค่ป้ายชื่อไว้ให้คนอ่าน ถ้าคืน error ขึ้นไป NamespaceManager.Create
		// จะถอยไปลบแถวใน DB ทิ้งทั้งที่ namespace ใช้งานได้ปกติ — เสียหายกว่าป้ายไม่ตรงเยอะ
		log.Printf("!! อัปเดต metadata ของ namespace '%s' ไม่สำเร็จ (namespace ใช้งานได้ปกติ): %v", name, err)
	}
	return nil

	// TODO ขั้นถัดไปในนี้: ResourceQuota (ns.CPULimitMilli / ns.RAMLimitMB),
	// LimitRange ค่า default ต่อ container, และ NetworkPolicy default-deny
	// ทั้งสามตัวต้อง idempotent แบบเดียวกัน (Create → AlreadyExists → Update)
}

// DeleteNamespace ลบ namespace ทิ้งทั้งก้อน — workload ข้างในถูกเก็บตามไปด้วยโดย k8s เอง
// data flow: NamespaceManager.Delete ส่งชื่อบนคลัสเตอร์มา → CoreV1().Namespaces().Delete()
//
// NotFound = สำเร็จ (คืน nil) ตามสัญญา idempotent ใน Provisioner — NamespaceManager.Delete
// ถอนของบนคลัสเตอร์ก่อนแล้วค่อยลบแถวใน DB ถ้าล้มตรงกลาง การสั่งลบซ้ำต้องเดินจนจบได้
//
// การลบเป็น async: กลับมาแล้ว namespace ยังค้างสถานะ Terminating อยู่พักหนึ่ง เราไม่รอให้จบ
// (จะบล็อก HTTP request นานเกินไป) — คนที่รีบสร้าง space ชื่อเดิมทันทีจะเจอ ErrNamespaceTerminating
// จาก EnsureNamespace ซึ่งบอกให้รอแล้วลองใหม่ ไม่ใช่ 500 ที่เดาสาเหตุไม่ได้
func (k *KubernetesProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	if err := cs.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("ลบ namespace '%s' บนคลัสเตอร์ไม่สำเร็จ: %w", nsName, err)
	}
	log.Printf("[k8s] สั่งลบ namespace '%s' แล้ว (k8s เก็บของข้างในต่อแบบ async)", nsName)
	return nil
}

// DeployService (ยังไม่ทำ) — จะสร้าง Deployment + Service ชนิด NodePort ใน namespace ที่กำหนด
// data flow (แผน): รับ entity.Service จาก ServiceManager.Create → ตั้ง resources.requests/limits
// จาก CPUMilli ("300m") และ RAMMB ("2048Mi") ต่อ 1 Pod → Deployment.spec.replicas = svc.Replicas
// → containerPort = svc.ContainerPort → svc.EnvVars ใส่เป็น container env (corev1.EnvVar)
// → apply Deployment + Service(type=NodePort, targetPort=svc.ContainerPort) เข้า cluster
// → อ่าน nodePort ที่ k8s สุ่มจ่ายให้ (หรือระบุเองถ้าอยากคุมเลข) → เซ็ตกลับที่ svc.NodePort ก่อน return
//
// targetPort ต้องชี้ที่ ContainerPort เสมอ — ตั้งผิดแล้ว Service จะสร้างสำเร็จแต่ traffic เข้าไปไม่มีใครฟัง
// กลายเป็น connection refused ที่ debug ยากเพราะ deploy "ผ่าน"
func (k *KubernetesProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (DeployService)")
}

// ScaleService (ยังไม่ทำ) — จะ Patch เฉพาะ Deployment.spec.replicas ของ workload ที่มีอยู่แล้ว
// (หรือใช้ UpdateScale ผ่าน scale subresource)
//
// ห้ามแตะ Service/NodePort ที่จ่ายไปแล้ว — ผู้ใช้ถือ URL <node-ip>:<node_port> อยู่
func (k *KubernetesProvisioner) ScaleService(ctx context.Context, nsName, svcName string, replicas int) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (ScaleService)")
}

// DeleteService (ยังไม่ทำ) — จะลบ Deployment ตัวเดียวออกจาก namespace
// data flow (แผน): รับ namespace + ชื่อ service จาก ServiceManager.Delete → เรียก AppsV1().Deployments().Delete()
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	return fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (DeleteService)")
}

// Logs (ยังไม่ทำ) — จะเปิด stream ของ log จาก container ที่รัน service นี้อยู่
func (k *KubernetesProvisioner) Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error) {
	return nil, fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (Logs)")
}
