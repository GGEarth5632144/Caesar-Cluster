package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"maps"
	"slices"
	"strconv"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"backend/internal/entity"
)

// ป้ายกำกับที่เราแปะไว้บนทุก object ที่ระบบนี้สร้าง
//
// แยกเป็น label กับ annotation ตามข้อจำกัดของ k8s: ค่าของ label ต้องเป็น [a-zA-Z0-9._-]
// ยาวไม่เกิน 63 ตัว จึงใส่ได้แค่ค่าที่เรารู้รูปแบบแน่ (id, ชื่อระบบ) ส่วนชื่อที่ผู้ใช้พิมพ์เอง
// (มีเว้นวรรค/ภาษาไทยได้) ต้องไปอยู่ใน annotation ที่ไม่จำกัดรูปแบบ
const (
	labelManagedBy   = "app.kubernetes.io/managed-by"
	labelNamespaceID = "caesar-cluster.io/namespace-id"

	// labelServiceName เป็นทั้งป้ายบน Pod และ selector ของ Deployment/Service — ใช้ชื่อ service ดิบได้
	// เพราะ ServiceController.Create บังคับ isValidK8sName มาแล้ว (ตัวพิมพ์เล็ก/เลข/ขีดกลาง)
	labelServiceName = "caesar-cluster.io/service"

	annDisplayName   = "caesar-cluster.io/display-name"
	annContributorID = "caesar-cluster.io/contributor-id"

	managedByCaesar = "caesar-cluster"
)

// ชื่อ object ที่เราสร้างไว้ใน namespace ของผู้ใช้ — ตั้งตายตัวเพราะมีอันเดียวต่อ namespace
// (ไม่ได้ตั้งตาม id เพราะมันอยู่ใน namespace ของตัวเองอยู่แล้ว ไม่มีทางชนกับของ namespace อื่น)
const (
	quotaObjectName  = "caesar-quota"
	limitsObjectName = "caesar-limits"
)

// ค่า default ที่ LimitRange เติมให้ container ที่ไม่ได้ระบุ resource มาเอง
//
// ตัวเลขตรงกับค่าต่ำสุดที่ dto.CreateServiceRequest ยอมรับ (cpu_milli min=100, ram_mb min=128)
// เพื่อให้ "ก้อนที่เล็กที่สุดที่ผู้ใช้ขอผ่าน API ได้" กับ "ก้อนที่คลัสเตอร์แจกให้ฟรีเมื่อไม่ระบุ"
// เป็นขนาดเดียวกัน — ไม่มีทางที่ pod ซึ่ง backend ยอมให้เกิด จะโดน LimitRange ตีกลับ
const (
	defaultContainerCPUMilli = 100
	defaultContainerRAMMB    = 128
)

// KubernetesProvisioner = provisioner ของจริงที่คุยกับ Kubernetes API ผ่าน client-go
// ถูกเลือกใช้ใน main เมื่อ PROVISIONER=kubernetes
//
// โครงที่มีต่อ namespace 1 อัน (ทำไปแล้ว = ✓):
//  1. Namespace ✓
//  2. ResourceQuota ✓ — requests/limits ของ cpu กับ memory ตาม limit ของ entity.Namespace
//     (นี่คือตัวบังคับโควตาชั้นสุดท้าย ต่อให้ backend เราพลาด k8s ก็ยังไม่ให้เกิน)
//  3. LimitRange ✓ — กันไม่ให้ container ที่ไม่ได้ระบุ resource แอบกินเกิน
//  4. NetworkPolicy default-deny — กัน traffic ข้าม namespace (ข้อกำหนดเรื่องแยก network) ยังไม่ทำ
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

// EnsureNamespace สร้าง/ปรับ namespace บนคลัสเตอร์ให้ตรงกับแถวใน DB
//
// data flow: NamespaceManager.Create (หรือ SetQuota) ส่ง entity.Namespace ที่เพิ่งบันทึกลง DB มา
// → แปลง ns.ID เป็นชื่อบนคลัสเตอร์ด้วย K8sNamespaceName → สร้าง 3 อย่างตามลำดับ:
// Namespace → ResourceQuota (โควตารวมของทั้ง space) → LimitRange (ค่า default/เพดานต่อ container)
//
// ต้อง idempotent ทั้งก้อนเพราะถูกเรียกซ้ำได้จริง: SetQuota เรียก method นี้ทุกครั้งที่แอดมิน
// ปรับโควตาของ space เดิม เจอของเดิมอยู่แล้วจึงไม่ใช่ error — ให้ทับค่าใหม่ลงไปแทน
func (k *KubernetesProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	cs, err := k.client()
	if err != nil {
		return err
	}
	name := K8sNamespaceName(ns.ID)

	created, err := k.ensureNamespaceObject(ctx, cs, name, ns)
	if err != nil {
		return err
	}

	// ตั้งโควตาต่อ — พลาดตรงนี้แล้ว namespace จะ "มีอยู่แต่ไม่มีเพดาน" ซึ่งอันตรายกว่าไม่มีเลย
	// (ผู้ใช้ deploy ได้ไม่จำกัดจนกินทั้งคลัสเตอร์) จึงต้องเก็บกวาดให้เรียบร้อยก่อนคืน error
	if err := k.ensureResourceQuota(ctx, cs, name, ns); err != nil {
		k.rollbackFreshNamespace(ctx, cs, name, created, err)
		return err
	}
	if err := k.ensureLimitRange(ctx, cs, name); err != nil {
		k.rollbackFreshNamespace(ctx, cs, name, created, err)
		return err
	}
	return nil
}

// rollbackFreshNamespace ลบ namespace ที่ "เราเพิ่งสร้างในการเรียกครั้งนี้" ทิ้ง เมื่อขั้นตอนถัดไปพัง
//
// เงื่อนไข created สำคัญมาก: EnsureNamespace ถูกเรียกซ้ำจาก SetQuota กับ namespace ที่มี service
// ของผู้ใช้รันอยู่จริง ถ้าเผลอลบเพราะแค่ตั้ง ResourceQuota พลาด งานของทั้งกลุ่มหายทันที
// ลบได้เฉพาะตอนที่มันเพิ่งเกิดจากการเรียกครั้งนี้เท่านั้น (ยังไม่มีอะไรอยู่ข้างในแน่นอน)
//
// WithoutCancel ด้วยเหตุผลเดียวกับ NamespaceManager.Create: ถ้าที่พังคือ ctx ถูก cancel
// (ผู้ใช้ปิดหน้าเว็บ) การลบด้วย ctx ตัวเดิมจะล้มตามทันที แล้ว namespace เปล่าค้างบนคลัสเตอร์ถาวร
func (k *KubernetesProvisioner) rollbackFreshNamespace(
ctx context.Context, cs kubernetes.Interface, name string, created bool, cause error,
) {
	if !created {
		return
	}
	err := cs.CoreV1().Namespaces().Delete(context.WithoutCancel(ctx), name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Printf("!! ตั้งค่า namespace '%s' ไม่สำเร็จ (%v) และลบตัวที่เพิ่งสร้างทิ้งไม่สำเร็จด้วย: %v "+
			"— เหลือ namespace เปล่าที่ไม่มีเพดานค้างบนคลัสเตอร์ ต้องลบมือ", name, cause, err)
	}
}

// ensureNamespaceObject สร้างตัว Namespace เอง — คืน created=true เมื่อเป็นการสร้างใหม่จริงในรอบนี้
//
// กรณีที่ต้องแยกให้ออกคือ namespace เดิมยังอยู่ในสถานะ Terminating (การลบ namespace ของ k8s
// เป็น async ใช้เวลาเก็บของข้างในสักพัก) — อันนี้ "รอแล้วลองใหม่ได้" ไม่ใช่ชื่อซ้ำถาวร
// จึงคืน ErrNamespaceTerminating ให้ controller แปลงเป็น 409 พร้อมข้อความว่าให้รอ
// (ดู NamespaceManager.Create ที่ส่ง error ตัวนี้ต่อแบบไม่ห่อทับ)
func (k *KubernetesProvisioner) ensureNamespaceObject(
	ctx context.Context, cs kubernetes.Interface, name string, ns *entity.Namespace,
) (bool, error) {
	labels := map[string]string{
		labelManagedBy:   managedByCaesar,
		labelNamespaceID: strconv.Itoa(ns.ID),
	}
	annotations := map[string]string{
		annDisplayName:   ns.Name,
		annContributorID: strconv.Itoa(ns.ContributorID),
	}

	_, err := cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}, metav1.CreateOptions{})
	if err == nil {
		log.Printf("[k8s] สร้าง namespace '%s' (space '%s' ของ user id=%d) แล้ว",
			name, ns.Name, ns.ContributorID)
		return true, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("สร้าง namespace '%s' บนคลัสเตอร์ไม่สำเร็จ: %w", name, err)
	}

	// มีอยู่แล้ว — เป็นได้ทั้ง "เรียกซ้ำตามปกติ" และ "ตัวเดิมยังลบไม่เสร็จ" ต้องอ่านมาดูก่อนว่าอันไหน
	existing, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// หายไประหว่าง Create กับ Get พอดี = ตัวเดิมเพิ่งลบเสร็จ สั่งใหม่อีกรอบได้เลย
			return false, fmt.Errorf("%w (namespace '%s')", ErrNamespaceTerminating, name)
		}
		return false, fmt.Errorf("อ่าน namespace '%s' บนคลัสเตอร์ไม่สำเร็จ: %w", name, err)
	}
	if existing.Status.Phase == corev1.NamespaceTerminating || existing.DeletionTimestamp != nil {
		return false, fmt.Errorf("%w (namespace '%s')", ErrNamespaceTerminating, name)
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
		return false, nil
	}

	if _, err := cs.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		// ไม่คืน error: สิ่งที่ผู้เรียกต้องการคือ "namespace นี้มีอยู่จริงบนคลัสเตอร์" ซึ่งจริงแล้ว
		// ส่วนที่พลาดเป็นแค่ป้ายชื่อไว้ให้คนอ่าน ถ้าคืน error ขึ้นไป NamespaceManager.Create
		// จะถอยไปลบแถวใน DB ทิ้งทั้งที่ namespace ใช้งานได้ปกติ — เสียหายกว่าป้ายไม่ตรงเยอะ
		log.Printf("!! อัปเดต metadata ของ namespace '%s' ไม่สำเร็จ (namespace ใช้งานได้ปกติ): %v", name, err)
	}
	return false, nil
}

// ensureResourceQuota ตั้งเพดานทรัพยากร "รวมทั้ง namespace" ให้ตรงกับ ns.CPULimitMilli / ns.RAMLimitMB
//
// นี่คือชั้นบังคับจริง ส่วน QuotaService ใน backend เป็นแค่ชั้นที่ตอบผู้ใช้ให้เร็วและมีข้อความสวยๆ
// ทั้งสองชั้นต้องคิดเลขแบบเดียวกันเป๊ะ ไม่งั้นผู้ใช้จะเจอ "DB บอกว่าโควตาพอ แต่ deploy แล้วโดนปฏิเสธ"
// ซึ่ง debug ยากมาก — QuotaService หักโควตาเป็น cpu_milli × replicas (ยอดรวมทุก Pod)
// ตรงกับที่ ResourceQuota นับ requests.cpu รวมทุก Pod ในnamespace พอดี
//
// ทำไมตั้งทั้ง requests.* และ limits.* เป็นค่าเดียวกัน:
//   - ใส่ limits.* ใน Hard ด้วย = บังคับให้ทุก container ต้องมี limit (ไม่มี = ถูกปฏิเสธ)
//     ซึ่ง LimitRange ด้านล่างเติมให้อยู่แล้ว จึงไม่มีทาง deploy ไม่ผ่านเพราะข้อนี้
//   - ตั้งเท่ากับ requests = ห้าม burst เกินที่จองไว้ ตรงกับความหมายของ "โควตา 300%" ที่ตกลงกัน
//     ถ้าปล่อยให้ limits สูงกว่า requests ได้ ผู้ใช้จะแอบใช้เกินโควตาตอนคนอื่นว่าง
//
// ไม่ใส่ count/pods ใน Hard โดยตั้งใจ: DB ไม่ได้นับจำนวน Pod เป็นแกนโควตา (นับแค่ cpu/ram)
// ถ้าใส่เพดานที่ backend มองไม่เห็น ก็จะสร้างเคส "DB บอกพอ แต่คลัสเตอร์ปฏิเสธ" ขึ้นมาเอง
// จำนวน Pod ถูกคุมทางอ้อมอยู่แล้วจาก cpu ขั้นต่ำต่อ service (100m → เต็มที่ 80 Pod ที่โควตาสูงสุด 8 core)
func (k *KubernetesProvisioner) ensureResourceQuota(
	ctx context.Context, cs kubernetes.Interface, nsName string, ns *entity.Namespace,
) error {
	cpu := *resource.NewMilliQuantity(int64(ns.CPULimitMilli), resource.DecimalSI)
	mem := *resource.NewQuantity(int64(ns.RAMLimitMB)*1024*1024, resource.BinarySI)

	spec := corev1.ResourceQuotaSpec{
		Hard: corev1.ResourceList{
			corev1.ResourceRequestsCPU:    cpu,
			corev1.ResourceLimitsCPU:      cpu,
			corev1.ResourceRequestsMemory: mem,
			corev1.ResourceLimitsMemory:   mem,
		},
	}
	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaObjectName,
			Namespace: nsName,
			Labels:    map[string]string{labelManagedBy: managedByCaesar},
		},
		Spec: spec,
	}

	quotas := cs.CoreV1().ResourceQuotas(nsName)
	if _, err := quotas.Create(ctx, desired, metav1.CreateOptions{}); err == nil {
		log.Printf("[k8s] ตั้งโควตา namespace '%s' เป็น %dm CPU / %d MB แล้ว",
			nsName, ns.CPULimitMilli, ns.RAMLimitMB)
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("ตั้ง ResourceQuota ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}

	// มีอยู่แล้ว (มาจาก SetQuota) → ทับ spec ด้วยค่าใหม่
	//
	// k8s ยอมให้ลดเพดานลงต่ำกว่ายอดที่ใช้อยู่: Pod เดิมรันต่อได้ แต่สร้างเพิ่มไม่ได้จนกว่าจะลบของเก่า
	// ซึ่งเป็นพฤติกรรมเดียวกับที่ NamespaceManager.SetQuota ระบุไว้ จึงไม่ต้องเช็คยอดใช้ก่อน
	existing, err := quotas.Get(ctx, quotaObjectName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("อ่าน ResourceQuota ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}
	existing.Spec = spec
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels[labelManagedBy] = managedByCaesar

	if _, err := quotas.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต ResourceQuota ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}
	log.Printf("[k8s] อัปเดตโควตา namespace '%s' เป็น %dm CPU / %d MB แล้ว",
		nsName, ns.CPULimitMilli, ns.RAMLimitMB)
	return nil
}

// ensureLimitRange ตั้งกติกาต่อ "1 container" ใน namespace — คนละชั้นกับ ResourceQuota ที่คุมยอดรวม
//
// ทำ 2 อย่าง:
//   - Default / DefaultRequest: container ที่ไม่ระบุ resource มาเอง จะถูกเติมค่าให้อัตโนมัติ
//     จำเป็นเพราะ ResourceQuota ด้านบนมี requests.*/limits.* อยู่ใน Hard — container ที่ไม่มี
//     ค่าเหล่านี้จะถูก k8s ปฏิเสธทันที LimitRange คือตัวที่ทำให้ไม่มีทางเกิดเคสนั้น
//   - Max: เพดานของ container เดี่ยวๆ ตรงกับ entity.MaxCPUMilliPerService / MaxRAMMBPerService
//     ที่ QuotaService.ReserveAndInsert เช็คไว้แล้ว — ตั้งให้ตรงกันเพื่อไม่ให้สองชั้นขัดกันเอง
//
// ไม่ตั้ง Min เพราะไม่มีประโยชน์: ทุก Pod ในนี้เกิดจาก DeployService ของเราเอง ซึ่งผ่านด่าน
// min=100m/128Mi ของ dto.CreateServiceRequest มาแล้ว การเพิ่ม Min มีแต่จะสร้างโอกาสตั้งค่าขัดกัน
//
// LimitRange ไม่ขึ้นกับโควตาของ namespace เลย (ทุก namespace ได้ค่าชุดเดียวกัน) จึงไม่ต้องรับ ns
func (k *KubernetesProvisioner) ensureLimitRange(
	ctx context.Context, cs kubernetes.Interface, nsName string,
) error {
	spec := corev1.LimitRangeSpec{
		Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Default: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(defaultContainerCPUMilli, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(defaultContainerRAMMB*1024*1024, resource.BinarySI),
			},
			DefaultRequest: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(defaultContainerCPUMilli, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(defaultContainerRAMMB*1024*1024, resource.BinarySI),
			},
			Max: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(entity.MaxCPUMilliPerService, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(entity.MaxRAMMBPerService*1024*1024, resource.BinarySI),
			},
		}},
	}
	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      limitsObjectName,
			Namespace: nsName,
			Labels:    map[string]string{labelManagedBy: managedByCaesar},
		},
		Spec: spec,
	}

	limits := cs.CoreV1().LimitRanges(nsName)
	if _, err := limits.Create(ctx, desired, metav1.CreateOptions{}); err == nil {
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("ตั้ง LimitRange ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}

	existing, err := limits.Get(ctx, limitsObjectName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("อ่าน LimitRange ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}
	existing.Spec = spec
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels[labelManagedBy] = managedByCaesar

	if _, err := limits.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต LimitRange ของ namespace '%s' ไม่สำเร็จ: %w", nsName, err)
	}
	return nil
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

// DeployService สร้าง Deployment + Service ชนิด NodePort ใน namespace ที่กำหนด
//
// data flow: รับ entity.Service จาก ServiceManager.Create (สเปกถูก snapshot + จองโควตาใน DB มาแล้ว)
// → สร้าง Deployment (replicas = svc.Replicas, resources จาก CPUMilli/RAMMB ต่อ 1 Pod, env จาก svc.EnvVars)
// → สร้าง Service type=NodePort ที่ selector ชี้ Pod ของ Deployment นั้น
// → อ่าน nodePort ที่ k8s จ่ายให้ → เซ็ตกลับที่ svc.NodePort ให้ ServiceManager เอาไป UPDATE ลง DB
//
// ทั้งสอง object ใช้ชื่อเดียวกับ svc.Name — ไม่ต้องเก็บชื่อบนคลัสเตอร์ไว้ใน DB อีกคอลัมน์
// และ ScaleService/DeleteService/Logs ที่รับมาแค่ svcName ก็หาเจอทันที (ชื่อไม่ซ้ำใน namespace
// เพราะตาราง services มี unique (namespace_id, name) อยู่แล้ว)
func (k *KubernetesProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	if _, err := cs.AppsV1().Deployments(nsName).Create(ctx, deploymentFor(nsName, svc), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("สร้าง Deployment '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}

	nodePort, err := k.createNodePortService(ctx, cs, nsName, svc)
	if err != nil {
		// Deployment เกิดไปแล้วแต่ไม่มีทางเข้าถึง — ต้องเก็บทิ้ง เพราะ ServiceManager.Create จะลบแถว
		// ใน DB (คืนโควตา) เมื่อได้ error ตัวนี้ ปล่อยไว้ = Pod รันกินทรัพยากรจริงโดยไม่มีใครเห็นและลบผ่าน UI ไม่ได้
		// WithoutCancel เพราะสาเหตุที่พังบ่อยที่สุดคือ ctx ถูก cancel (ผู้ใช้ปิดหน้าเว็บ) — ลบด้วยตัวเดิมจะล้มตาม
		delErr := cs.AppsV1().Deployments(nsName).Delete(context.WithoutCancel(ctx), svc.Name, metav1.DeleteOptions{})
		if delErr != nil && !apierrors.IsNotFound(delErr) {
			log.Printf("!! สร้าง Service ของ '%s' ใน namespace '%s' ไม่สำเร็จ (%v) และลบ Deployment ที่เพิ่งสร้างทิ้งไม่สำเร็จด้วย: %v "+
				"— เหลือ Pod ที่กินโควตาค้างบนคลัสเตอร์โดยไม่มีแถวใน DB ต้องลบมือ", svc.Name, nsName, err, delErr)
		}
		return err
	}

	svc.NodePort = &nodePort
	log.Printf("[k8s] deploy '%s' เข้า namespace '%s' แล้ว — %d replica × (%dm CPU / %d MB), เข้าถึงที่ <node-ip>:%d → container port %d",
		svc.Name, nsName, svc.Replicas, svc.CPUMilli, svc.RAMMB, nodePort, svc.ContainerPort)
	return nil
}

// deploymentFor แปลง entity.Service → Deployment object (ยังไม่ยิงไปคลัสเตอร์)
//
// requests เท่ากับ limits เสมอ ไม่งั้นชน ResourceQuota ที่ ensureResourceQuota ตั้งไว้: Hard มีทั้ง
// requests.* และ limits.* เป็นค่าเดียวกัน — limits ที่สูงกว่า requests จะทำให้ยอดรวมฝั่ง limits
// ทะลุก่อน ทั้งที่ QuotaService ใน backend คิดว่าโควตายังเหลือ (เคสที่ debug ยากที่สุดของระบบนี้)
func deploymentFor(nsName string, svc *entity.Service) *appsv1.Deployment {
	labels := map[string]string{
		labelManagedBy:   managedByCaesar,
		labelServiceName: svc.Name,
	}
	replicas := int32(svc.Replicas)
	res := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(svc.CPUMilli), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(int64(svc.RAMMB)*1024*1024, resource.BinarySI),
	}

	// เรียง key ก่อนแปลงเป็น env — ลำดับของ map ใน Go สุ่มทุกรอบ ปล่อยไว้แล้ว spec จะ "เปลี่ยน"
	// ทุกครั้งที่ประกอบใหม่ทั้งที่ค่าเท่าเดิม (กวน diff และทำให้ ScaleService/แก้ไขทีหลังสั่ง rollout เปล่าๆ)
	env := make([]corev1.EnvVar, 0, len(svc.EnvVars))
	for _, key := range slices.Sorted(maps.Keys(svc.EnvVars)) {
		env = append(env, corev1.EnvVar{Name: key, Value: svc.EnvVars[key]})
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   nsName,
			Labels:      labels,
			Annotations: map[string]string{annContributorID: strconv.Itoa(svc.CreatedBy)},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			// Selector แก้ทีหลังไม่ได้ (k8s ห้าม) — ต้องเป็นชุด label ที่ไม่มีวันเปลี่ยนตามค่าที่ผู้ใช้แก้ได้
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  svc.Name,
						Image: svc.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(svc.ContainerPort),
							Protocol:      corev1.ProtocolTCP,
						}},
						Env:       env,
						Resources: corev1.ResourceRequirements{Requests: res, Limits: res},
					}},
				},
			},
		},
	}
}

// createNodePortService เปิดทางเข้าจากนอกให้ workload แล้วคืนเลข nodePort ที่ k8s จ่ายมา
//
// targetPort ต้องชี้ที่ ContainerPort เสมอ — ตั้งผิดแล้ว Service จะสร้างสำเร็จแต่ traffic เข้าไปไม่มีใครฟัง
// กลายเป็น connection refused ที่ debug ยากเพราะ deploy "ผ่าน"
//
// ไม่ระบุ nodePort เอง ปล่อยให้ k8s สุ่มจากช่วง 30000-32767: ถ้าเราเลือกเลขเองต้องมาจดว่าใครใช้เลขไหน
// แล้วกันชนกันข้าม namespace ซึ่ง k8s ทำให้อยู่แล้ว (คอลัมน์ node_port ใน DB เป็นแค่สำเนาไว้โชว์ URL)
func (k *KubernetesProvisioner) createNodePortService(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) (int, error) {
	labels := map[string]string{
		labelManagedBy:   managedByCaesar,
		labelServiceName: svc.Name,
	}
	created, err := cs.CoreV1().Services(nsName).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: nsName,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       int32(svc.ContainerPort),
				TargetPort: intstr.FromInt32(int32(svc.ContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return 0, fmt.Errorf("สร้าง Service (NodePort) '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}

	// ปกติ k8s จ่ายเลขมาพร้อม response ของ Create เลย — ถ้าไม่มีแปลว่าช่วง NodePort เต็มหรือคลัสเตอร์
	// ตั้งค่าไว้แปลก คืน error ไปเลยดีกว่าปล่อยผ่าน เพราะ svc.NodePort ที่ว่างแปลว่าผู้ใช้ไม่มี URL ให้เข้า
	// (ServiceManager.Create จะเก็บกวาดของบนคลัสเตอร์ + คืนโควตาให้เอง)
	if len(created.Spec.Ports) == 0 || created.Spec.Ports[0].NodePort == 0 {
		return 0, fmt.Errorf("คลัสเตอร์ไม่ได้จ่าย nodePort ให้ service '%s' ใน namespace '%s' "+
			"(ช่วง 30000-32767 อาจเต็ม)", svc.Name, nsName)
	}
	return int(created.Spec.Ports[0].NodePort), nil
}

// ScaleService แก้เฉพาะ Deployment.spec.replicas ของ workload ที่ DeployService สร้างไว้ (ชื่อ = svcName)
// data flow: ServiceManager.Scale จองโควตา + UPDATE replicas ใน DB แล้ว → merge patch replicas บนคลัสเตอร์
//
// ใช้ merge patch แทน Get→Update: ยิงครั้งเดียว ไม่มีช่อง conflict กับ controller ที่เขียน status อยู่
// และไม่แตะ field อื่นของ spec (pod template ไม่เปลี่ยน = ไม่เกิด rollout ใหม่ Pod เดิมรันต่อ)
//
// ห้ามแตะ Service/NodePort ที่จ่ายไปแล้ว — ผู้ใช้ถือ URL <node-ip>:<node_port> อยู่
//
// NotFound ไม่ถือว่าสำเร็จ (ต่างจาก DeleteService): ต้องคืน error ให้ ServiceManager.Scale ย้อน replicas
// ใน DB กลับ ไม่งั้นโควตาถูกจองไว้ให้ Pod ที่ไม่มีอยู่จริง
func (k *KubernetesProvisioner) ScaleService(ctx context.Context, nsName, svcName string, replicas int) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	patch := fmt.Appendf(nil, `{"spec":{"replicas":%d}}`, replicas)
	_, err = cs.AppsV1().Deployments(nsName).Patch(ctx, svcName, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("ปรับจำนวน replica ของ Deployment '%s' ใน namespace '%s' เป็น %d ไม่สำเร็จ: %w",
			svcName, nsName, replicas, err)
	}
	log.Printf("[k8s] scale '%s' ใน namespace '%s' เป็น %d replica แล้ว", svcName, nsName, replicas)
	return nil
}

// DeleteService ลบ Deployment + Service (NodePort) ที่ DeployService สร้างไว้ (ชื่อเดียวกับ svcName ทั้งคู่)
// data flow: ServiceManager.Delete ส่ง namespace + ชื่อ service มา → ลบ Deployment → ลบ Service
//
// NotFound = สำเร็จ ตามสัญญา idempotent เดียวกับ DeleteNamespace: ServiceManager.Delete ลบแถวใน DB
// หลังเราคืน nil เท่านั้น ถ้าลบได้ตัวเดียวแล้วล้ม การกดลบซ้ำต้องเดินผ่านตัวที่หายไปแล้วได้
// ไม่งั้นแถวใน DB ค้างถาวรและโควตาไม่ถูกคืน
//
// ลบ Deployment ก่อนเพราะเป็นตัวที่กินทรัพยากร — Pod ถูก GC เก็บต่อแบบ async (propagation ของ
// apps/v1 เป็น Background อยู่แล้ว) เราไม่รอให้หมด ด้วยเหตุผลเดียวกับ DeleteNamespace
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	err = cs.AppsV1().Deployments(nsName).Delete(ctx, svcName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ Deployment '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svcName, nsName, err)
	}
	err = cs.CoreV1().Services(nsName).Delete(ctx, svcName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ Service '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svcName, nsName, err)
	}
	log.Printf("[k8s] ลบ service '%s' ออกจาก namespace '%s' แล้ว", svcName, nsName)
	return nil
}

// Logs (ยังไม่ทำ) — จะเปิด stream ของ log จาก container ที่รัน service นี้อยู่
func (k *KubernetesProvisioner) Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error) {
	return nil, fmt.Errorf("kubernetes provisioner: ยังไม่ได้ implement (Logs)")
}
