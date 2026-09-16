package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

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

// dataVolumeName = ชื่อ volumeClaimTemplate ของ service ที่มีดิสก์ถาวร — PVC จริงจะชื่อ data-<service>-0
// ตามกติกาการตั้งชื่อของ StatefulSet (ดู pvcName)
const dataVolumeName = "data"

// pvcStorageClass = StorageClass ของ PVC ทุกตัว (NFS บน NUC) — ต้องตรงกับ deploy/k8s/caesar-nfs-storage.yaml
// ระบุชื่อเองทุกครั้ง เพราะคลัสเตอร์ตั้งใจไม่มี default StorageClass (ไม่ระบุ = PVC ค้าง Pending ตลอด)
const pvcStorageClass = "caesar-nfs"

// ขอบเขตของข้อความสถานะที่ Status ส่งกลับไปให้ ServiceHealthMonitor เขียนลง DB
const (
	crashLogTailLines = 15   // log กี่บรรทัดท้ายที่แนบไปกับสถานะ crash loop
	maxStatusMessage  = 2000 // กัน message ยาวจนตาราง services บวม
	maxStatusReason   = 60   // ต้องเท่ากับความกว้างคอลัมน์ status_reason
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
//  2. ResourceQuota ✓ — requests/limits ของ cpu กับ memory และ requests.storage ตาม limit ของ entity.Namespace
//     (นี่คือตัวบังคับโควตาชั้นสุดท้าย ต่อให้ backend เราพลาด k8s ก็ยังไม่ให้เกิน)
//  3. LimitRange ✓ — กันไม่ให้ container ที่ไม่ได้ระบุ resource แอบกินเกิน
//  4. NetworkPolicy default-deny — กัน traffic ข้าม namespace (ข้อกำหนดเรื่องแยก network) ยังไม่ทำ
//
// database (svc.IsDatabase) พึ่งของสองอย่างบนคลัสเตอร์:
//   - StorageClass default — PVC ไม่ระบุ storageClassName (k3s = local-path) ถ้าไม่มี pod จะค้าง
//     Pending ว่า "unbound PersistentVolumeClaims"
//   - CNI ที่บังคับใช้ NetworkPolicy (k3s เปิดให้ตั้งแต่ต้น) — ไม่งั้น policy ของ database ไม่มีผล
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

		// ServiceHealthMonitor ถามสถานะหลาย service ต่อรอบ — ค่า default ของ client-go (5 req/s) จะโดน throttle
		cfg.QPS = 20
		cfg.Burst = 40

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

// ensureResourceQuota ตั้งเพดานทรัพยากร "รวมทั้ง namespace" ให้ตรงกับ ns.CPULimitMilli / ns.RAMLimitMB / ns.StorageLimitMB
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
// requests.storage = ขนาดรวมของทุก PVC ใน namespace ตรงกับที่ QuotaService หักจาก storage_mb
// (StorageLimitMB = 0 จึงแปลว่าสร้าง database ไม่ได้ ทั้งฝั่ง DB และฝั่งคลัสเตอร์)
//
// ไม่ใส่ count/pods ใน Hard โดยตั้งใจ: DB ไม่ได้นับจำนวน Pod เป็นแกนโควตา (นับแค่ cpu/ram)
// ถ้าใส่เพดานที่ backend มองไม่เห็น ก็จะสร้างเคส "DB บอกพอ แต่คลัสเตอร์ปฏิเสธ" ขึ้นมาเอง
// จำนวน Pod ถูกคุมทางอ้อมอยู่แล้วจาก cpu ขั้นต่ำต่อ service (100m → เต็มที่ 80 Pod ที่โควตาสูงสุด 8 core)
func (k *KubernetesProvisioner) ensureResourceQuota(
	ctx context.Context, cs kubernetes.Interface, nsName string, ns *entity.Namespace,
) error {
	cpu := *resource.NewMilliQuantity(int64(ns.CPULimitMilli), resource.DecimalSI)
	mem := *resource.NewQuantity(int64(ns.RAMLimitMB)*1024*1024, resource.BinarySI)
	disk := *resource.NewQuantity(int64(ns.StorageLimitMB)*1024*1024, resource.BinarySI)

	spec := corev1.ResourceQuotaSpec{
		Hard: corev1.ResourceList{
			corev1.ResourceRequestsCPU:     cpu,
			corev1.ResourceLimitsCPU:       cpu,
			corev1.ResourceRequestsMemory:  mem,
			corev1.ResourceLimitsMemory:    mem,
			corev1.ResourceRequestsStorage: disk,
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
		log.Printf("[k8s] ตั้งโควตา namespace '%s' เป็น %dm CPU / %d MB / ดิสก์ %d MB แล้ว",
			nsName, ns.CPULimitMilli, ns.RAMLimitMB, ns.StorageLimitMB)
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
	log.Printf("[k8s] อัปเดตโควตา namespace '%s' เป็น %dm CPU / %d MB / ดิสก์ %d MB แล้ว",
		nsName, ns.CPULimitMilli, ns.RAMLimitMB, ns.StorageLimitMB)
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
// (service ที่มีดิสก์ถาวรแยกไปทาง deployStateful — ดูสัญญาใน Provisioner)
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
	if svc.HasStorage() {
		return k.deployStateful(ctx, cs, nsName, svc)
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

// deployStateful = StatefulSet + PVC ของ service ที่มีดิสก์ถาวร แล้วเปิดทางเข้าตาม svc.IsDatabase
//   - database → Service ชนิด ClusterIP + NetworkPolicy ไม่แตะ svc.NodePort เลย — ไม่เคยจองพอร์ตบน node
//     ก็ไม่มีประตูให้เคาะจากนอกคลัสเตอร์ ซึ่งเชื่อถือได้กว่าการหวังให้ NetworkPolicy ทำงานถูก
//   - web      → Service ชนิด NodePort แบบเดียวกับทางของ Deployment (เช่น Nextcloud ที่เก็บไฟล์ใน /var/www/html)
//
// StatefulSet เกิดแล้วแต่ชิ้นถัดไปพัง ต้องถอนทิ้งด้วยเหตุผลเดียวกับทางของ Deployment ใน DeployService
// — ใช้ DeleteService ถอนเพราะเก็บครบทุกชิ้น (รวม PVC ที่ StatefulSet controller อาจสร้างไปแล้ว)
func (k *KubernetesProvisioner) deployStateful(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) error {
	if svc.IsTemplateDatabase() {
		if err := k.ensureDatabaseSecret(ctx, cs, nsName, svc); err != nil {
			return err
		}
	}
	if _, err := cs.AppsV1().StatefulSets(nsName).Create(ctx, statefulSetFor(nsName, svc), metav1.CreateOptions{}); err != nil {
		// Secret เกิดไปแล้ว — เก็บทิ้งด้วย ไม่งั้นรหัสผ่านค้างอยู่บนคลัสเตอร์โดยไม่มีแถวใน DB
		if svc.IsTemplateDatabase() {
			delErr := cs.CoreV1().Secrets(nsName).Delete(context.WithoutCancel(ctx), databaseSecretName(svc.Name), metav1.DeleteOptions{})
			if delErr != nil && !apierrors.IsNotFound(delErr) {
				log.Printf("!! สร้าง StatefulSet '%s' ใน namespace '%s' ไม่สำเร็จ และลบ Secret ที่เพิ่งสร้างไม่สำเร็จด้วย: %v", svc.Name, nsName, delErr)
			}
		}
		return fmt.Errorf("สร้าง StatefulSet '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}

	var nodePort int
	var err error
	if svc.IsDatabase {
		err = k.createDatabaseNetwork(ctx, cs, nsName, svc)
	} else {
		nodePort, err = k.createNodePortService(ctx, cs, nsName, svc)
	}
	if err != nil {
		if delErr := k.DeleteService(context.WithoutCancel(ctx), nsName, svc); delErr != nil {
			log.Printf("!! deploy '%s' ใน namespace '%s' ไม่สำเร็จ (%v) และถอนของที่สร้างค้างไว้ไม่สำเร็จด้วย: %v "+
				"— เหลือ StatefulSet/PVC ที่กินโควตาค้างบนคลัสเตอร์โดยไม่มีแถวใน DB ต้องลบมือ", svc.Name, nsName, err, delErr)
		}
		return err
	}

	if svc.IsDatabase {
		log.Printf("[k8s] deploy database '%s' เข้า namespace '%s' แล้ว — %dm CPU / %d MB / ดิสก์ %d MB ที่ %s, เข้าถึงได้เฉพาะใน namespace ที่ %s:%d",
			svc.Name, nsName, svc.CPUMilli, svc.RAMMB, svc.StorageMB, svc.DataPath, svc.Name, svc.ContainerPort)
		return nil
	}
	svc.NodePort = &nodePort
	log.Printf("[k8s] deploy '%s' (StatefulSet) เข้า namespace '%s' แล้ว — %dm CPU / %d MB / ดิสก์ %d MB ที่ %s, เข้าถึงที่ <node-ip>:%d → container port %d",
		svc.Name, nsName, svc.CPUMilli, svc.RAMMB, svc.StorageMB, svc.DataPath, nodePort, svc.ContainerPort)
	return nil
}

// createDatabaseNetwork เปิดทางเข้าให้ database เฉพาะจากใน namespace: Service ClusterIP + NetworkPolicy
func (k *KubernetesProvisioner) createDatabaseNetwork(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) error {
	if _, err := cs.CoreV1().Services(nsName).Create(ctx,
		serviceFor(nsName, svc, corev1.ServiceTypeClusterIP), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("สร้าง Service (ClusterIP) '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies(nsName).Create(ctx,
		networkPolicyFor(nsName, svc), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("สร้าง NetworkPolicy ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	return nil
}

// serviceLabels = ป้ายของทุก object ที่ผูกกับ service ตัวเดียว และเป็น selector ของ workload/Service ด้วย
// Selector แก้ทีหลังไม่ได้ (k8s ห้าม) — ต้องเป็นชุด label ที่ไม่มีวันเปลี่ยนตามค่าที่ผู้ใช้แก้ได้
func serviceLabels(svcName string) map[string]string {
	return map[string]string{
		labelManagedBy:   managedByCaesar,
		labelServiceName: svcName,
	}
}

func podSelector(svcName string) string        { return labelServiceName + "=" + svcName }
func networkPolicyName(svcName string) string  { return svcName + "-namespace-only" }
func pvcName(svcName string) string            { return dataVolumeName + "-" + svcName + "-0" }
func databaseSecretName(svcName string) string { return svcName + "-credentials" }

// databaseSecretFor ประกอบ Secret credential ของ database template (docs 029)
// root-password มีเฉพาะ engine ที่ image บังคับ (MySQL/MariaDB) — backend สุ่มให้ ผู้ใช้ไม่เห็น
func databaseSecretFor(nsName string, svc *entity.Service) *corev1.Secret {
	// ใช้ Data ไม่ใช่ StringData: apiserver แปลงให้เหมือนกัน แต่ fake client ในเทสต์ไม่แปลง
	data := map[string][]byte{
		SecretKeyUsername: []byte(svc.DBUsername),
		SecretKeyPassword: []byte(svc.DBPassword),
		SecretKeyDatabase: []byte(svc.DBName),
	}
	if svc.DBRootPassword != "" {
		data[SecretKeyRootPassword] = []byte(svc.DBRootPassword)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      databaseSecretName(svc.Name),
			Namespace: nsName,
			Labels:    serviceLabels(svc.Name),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

// ensureDatabaseSecret สร้าง Secret ของ database — มีอยู่แล้ว (ค้างจากรอบที่ล้มกลางทางโดยไม่มีแถวใน DB) = เขียนทับ
// เขียนทับได้เพราะชื่อ service ไม่ซ้ำใน namespace (unique ใน DB): Secret ชื่อนี้ไม่มีเจ้าของตัวจริงอยู่
func (k *KubernetesProvisioner) ensureDatabaseSecret(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) error {
	want := databaseSecretFor(nsName, svc)
	_, err := cs.CoreV1().Secrets(nsName).Create(ctx, want, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = cs.CoreV1().Secrets(nsName).Update(ctx, want, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("สร้าง Secret ของ database '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	return nil
}

// DatabaseCredentials อ่าน username/password/database กลับจาก Secret (ดูสัญญาใน Provisioner)
func (k *KubernetesProvisioner) DatabaseCredentials(ctx context.Context, nsName string, svc *entity.Service) (DatabaseCredentials, error) {
	if !svc.IsTemplateDatabase() {
		return DatabaseCredentials{}, ErrNotTemplateDatabase
	}
	cs, err := k.client()
	if err != nil {
		return DatabaseCredentials{}, err
	}
	sec, err := cs.CoreV1().Secrets(nsName).Get(ctx, databaseSecretName(svc.Name), metav1.GetOptions{})
	if err != nil {
		return DatabaseCredentials{}, fmt.Errorf("อ่าน Secret ของ database '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	return DatabaseCredentials{
		Username: string(sec.Data[SecretKeyUsername]),
		Password: string(sec.Data[SecretKeyPassword]),
		Database: string(sec.Data[SecretKeyDatabase]),
	}, nil
}

// databaseEnv = env ของ database template ที่อ่านค่าจาก Secret — ไม่มีรหัสจริงอยู่ใน pod spec
func databaseEnv(svc *entity.Service) []corev1.EnvVar {
	e, ok := findDatabaseEngine(svc.DatabaseEngine)
	if !ok {
		return nil
	}
	ref := func(name, key string) corev1.EnvVar {
		return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: databaseSecretName(svc.Name)},
				Key:                  key,
			},
		}}
	}
	env := []corev1.EnvVar{
		ref(e.envUser, SecretKeyUsername),
		ref(e.envPassword, SecretKeyPassword),
		ref(e.envDatabase, SecretKeyDatabase),
	}
	if e.envRootPassword != "" {
		env = append(env, ref(e.envRootPassword, SecretKeyRootPassword))
	}
	return env
}

// deploymentFor แปลง entity.Service → Deployment object (ยังไม่ยิงไปคลัสเตอร์)
func deploymentFor(nsName string, svc *entity.Service) *appsv1.Deployment {
	labels := serviceLabels(svc.Name)
	replicas := int32(svc.Replicas)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   nsName,
			Labels:      labels,
			Annotations: map[string]string{annContributorID: strconv.Itoa(svc.CreatedBy)},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplateFor(svc, labels),
		},
	}
}

// statefulSetFor แปลง entity.Service ที่มีดิสก์ถาวร (database หรือ web) → StatefulSet ที่มี PVC ของตัวเอง
//
// ใช้ StatefulSet ไม่ใช่ Deployment เพราะ RollingUpdate ของ Deployment ปั้น Pod ใหม่ก่อนฆ่าตัวเก่า
// สองตัวจะแย่ง PVC แบบ ReadWriteOnce ก้อนเดียวกันจน rollout ค้างถาวร
//
// ServiceName ชี้ Service ชื่อเดียวกันซึ่งเป็น ClusterIP (database) หรือ NodePort (web) ไม่ใช่ headless
// — ใช้ได้ทั้งคู่ แค่ไม่ได้ DNS ราย pod (<pod>.<service>) ซึ่งไม่มีใครใช้เพราะมี pod เดียว
func statefulSetFor(nsName string, svc *entity.Service) *appsv1.StatefulSet {
	labels := serviceLabels(svc.Name)
	replicas := int32(entity.StorageReplicas)
	storageClass := pvcStorageClass

	template := podTemplateFor(svc, labels)
	template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
		Name:      dataVolumeName,
		MountPath: svc.DataPath,
	}}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   nsName,
			Labels:      labels,
			Annotations: map[string]string{annContributorID: strconv.Itoa(svc.CreatedBy)},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: svc.Name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template:    template,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: dataVolumeName},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &storageClass,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: *resource.NewQuantity(int64(svc.StorageMB)*1024*1024, resource.BinarySI),
						},
					},
				},
			}},
			// ลบ StatefulSet แล้วให้ k8s ลบ PVC ตาม — DeleteService ยังตามไปลบเองอีกชั้น
			// เผื่อคลัสเตอร์เก่าที่ยังไม่รู้จัก field นี้
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
		},
	}
}

// podTemplateFor ประกอบ pod 1 container — ใช้ร่วมกันทั้ง Deployment และ StatefulSet
//
// requests เท่ากับ limits เสมอ ไม่งั้นชน ResourceQuota ที่ ensureResourceQuota ตั้งไว้: Hard มีทั้ง
// requests.* และ limits.* เป็นค่าเดียวกัน — limits ที่สูงกว่า requests จะทำให้ยอดรวมฝั่ง limits
// ทะลุก่อน ทั้งที่ QuotaService ใน backend คิดว่าโควตายังเหลือ (เคสที่ debug ยากที่สุดของระบบนี้)
//
// readiness probe แบบ TCP ทำให้ Status แยก "รันอยู่" ออกจาก "โปรเซสขึ้นแต่ยังไม่ฟังพอร์ต" ได้
// โดยไม่ต้องรู้จัก image
func podTemplateFor(svc *entity.Service, labels map[string]string) corev1.PodTemplateSpec {
	port := int32(svc.ContainerPort)
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
	if svc.IsTemplateDatabase() {
		env = append(env, databaseEnv(svc)...)
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: labels},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  svc.Name,
				Image: svc.Image,
				Ports: []corev1.ContainerPort{{
					ContainerPort: port,
					Protocol:      corev1.ProtocolTCP,
				}},
				Env:       env,
				Resources: corev1.ResourceRequirements{Requests: res, Limits: res},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       10,
					FailureThreshold:    3,
				},
			}},
		},
	}
}

// serviceFor แปลง entity.Service → k8s Service ที่ชี้เข้า pod ของ service นี้
//
// targetPort ต้องชี้ที่ ContainerPort เสมอ — ตั้งผิดแล้ว Service จะสร้างสำเร็จแต่ traffic เข้าไปไม่มีใครฟัง
// กลายเป็น connection refused ที่ debug ยากเพราะ deploy "ผ่าน"
func serviceFor(nsName string, svc *entity.Service, typ corev1.ServiceType) *corev1.Service {
	labels := serviceLabels(svc.Name)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: nsName,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     typ,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       int32(svc.ContainerPort),
				TargetPort: intstr.FromInt32(int32(svc.ContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// networkPolicyFor รับ ingress เข้า pod ของ database ได้เฉพาะจาก pod ใน namespace เดียวกัน
//
// from: [{podSelector: {}}] แปลว่า pod ทุกตัวใน namespace เดียวกับ policy ซึ่งคือที่ต้องการ
// ห้ามเผลอเขียนเป็น namespaceSelector: {} ที่แปลว่าทุก namespace = เปิดให้ทั้งคลัสเตอร์
func networkPolicyFor(nsName string, svc *entity.Service) *netv1.NetworkPolicy {
	labels := serviceLabels(svc.Name)
	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(int32(svc.ContainerPort))

	return &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyName(svc.Name),
			Namespace: nsName,
			Labels:    labels,
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: labels},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			Ingress: []netv1.NetworkPolicyIngressRule{{
				From:  []netv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{}}},
				Ports: []netv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port}},
			}},
		},
	}
}

// createNodePortService เปิดทางเข้าจากนอกให้ workload แล้วคืนเลข nodePort ที่ k8s จ่ายมา
//
// ไม่ระบุ nodePort เอง ปล่อยให้ k8s สุ่มจากช่วง 30000-32767: ถ้าเราเลือกเลขเองต้องมาจดว่าใครใช้เลขไหน
// แล้วกันชนกันข้าม namespace ซึ่ง k8s ทำให้อยู่แล้ว (คอลัมน์ node_port ใน DB เป็นแค่สำเนาไว้โชว์ URL)
func (k *KubernetesProvisioner) createNodePortService(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) (int, error) {
	created, err := cs.CoreV1().Services(nsName).Create(ctx,
		serviceFor(nsName, svc, corev1.ServiceTypeNodePort), metav1.CreateOptions{})
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

// UpdateService แก้ workload เดิมในที่ให้ตรงกับ svc (ดูสัญญาใน Provisioner)
// data flow: ServiceManager.Update ตรวจกติกาดิสก์ + จองโควตา + บันทึกค่าใหม่ลง DB แล้ว
// → แก้ pod template ของ StatefulSet/Deployment → แก้พอร์ตของ Service (+ NetworkPolicy ของ database)
// → เซ็ต svc.NodePort เป็นเลขเดิมของ Service
//
// ลำดับ: workload ก่อน Service — พอร์ตใหม่ของ Service ชี้ไปที่ container ที่ยังฟังพอร์ตเก่าอยู่
// ช่วงสั้นๆ ระหว่าง rollout ไม่ว่าจะเรียงแบบไหน แต่ถ้า workload แก้ไม่ผ่าน (เช่น ชน LimitRange)
// Service จะยังชี้พอร์ตเดิมที่ใช้งานได้อยู่
//
// ใช้ Get → Update ใน RetryOnConflict ไม่ใช่ merge patch แบบ ScaleService: ต้องแทน pod template ทั้งก้อน
// (env ที่ผู้ใช้ลบออกต้องหายจริง) ซึ่ง merge patch ของ list ทำไม่ได้ — ชน resourceVersion กับ controller
// ที่เขียน status อยู่ได้ จึงวนอ่านใหม่แล้วลองอีกรอบ
func (k *KubernetesProvisioner) UpdateService(ctx context.Context, nsName string, oldSvc, svc *entity.Service) error {
	if oldSvc.HasStorage() {
		if msg := storageChange(oldSvc, svc); msg != "" {
			return fmt.Errorf("%w: %s", ErrStorageImmutable, msg)
		}
	} else if svc.Name != oldSvc.Name || svc.HasStorage() {
		// ชื่อ object หรือชนิด workload เปลี่ยน แก้ในที่ไม่ได้ — ไม่มีดิสก์เดิมให้หาย ลบแล้วสร้างใหม่ได้
		if err := k.DeleteService(ctx, nsName, oldSvc); err != nil {
			return fmt.Errorf("ลบ workload เดิมไม่สำเร็จ: %w", err)
		}
		return k.DeployService(ctx, nsName, svc)
	}

	cs, err := k.client()
	if err != nil {
		return err
	}

	if svc.HasStorage() {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			sts, err := cs.AppsV1().StatefulSets(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			// แทนแค่ pod template — volumeClaimTemplates/selector แก้ไม่ได้ (k8s ห้าม) และไม่ต้องแก้
			// เพราะ storageChange รับประกันแล้วว่าขนาดดิสก์และชื่อเท่าเดิม
			sts.Spec.Template = statefulSetFor(nsName, svc).Spec.Template
			_, err = cs.AppsV1().StatefulSets(nsName).Update(ctx, sts, metav1.UpdateOptions{})
			return err
		})
		if err != nil {
			return fmt.Errorf("แก้ StatefulSet '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
		}
	} else {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			dep, err := cs.AppsV1().Deployments(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			want := deploymentFor(nsName, svc)
			dep.Spec.Template = want.Spec.Template
			dep.Spec.Replicas = want.Spec.Replicas
			_, err = cs.AppsV1().Deployments(nsName).Update(ctx, dep, metav1.UpdateOptions{})
			return err
		})
		if err != nil {
			return fmt.Errorf("แก้ Deployment '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
		}
	}

	nodePort, err := k.updateServicePort(ctx, cs, nsName, svc)
	if err != nil {
		return err
	}
	if svc.IsDatabase {
		if err := k.updateNetworkPolicyPort(ctx, cs, nsName, svc); err != nil {
			return err
		}
		svc.NodePort = nil
	} else {
		svc.NodePort = &nodePort
	}

	log.Printf("[k8s] แก้ service '%s' ใน namespace '%s' ในที่แล้ว (image=%s, %dm CPU / %d MB, port %d) — PVC และ NodePort คงเดิม",
		svc.Name, nsName, svc.Image, svc.CPUMilli, svc.RAMMB, svc.ContainerPort)
	return nil
}

// updateServicePort ชี้ Service เดิมไปที่ ContainerPort ใหม่ แล้วคืนเลข nodePort เดิม (ClusterIP ได้ 0)
// ไม่แตะ nodePort — ปล่อยค่าที่ Get มาไว้ k8s จึงไม่จ่ายเลขใหม่
func (k *KubernetesProvisioner) updateServicePort(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) (int, error) {
	var nodePort int
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cur, err := cs.CoreV1().Services(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(cur.Spec.Ports) == 0 {
			return fmt.Errorf("Service ไม่มีพอร์ต")
		}
		p := &cur.Spec.Ports[0]
		nodePort = int(p.NodePort)
		port := int32(svc.ContainerPort)
		if p.Port == port && p.TargetPort == intstr.FromInt32(port) {
			return nil
		}
		p.Port, p.TargetPort = port, intstr.FromInt32(port)
		_, err = cs.CoreV1().Services(nsName).Update(ctx, cur, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("แก้พอร์ตของ Service '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	if !svc.IsDatabase && nodePort == 0 {
		return 0, fmt.Errorf("Service '%s' ใน namespace '%s' ไม่มี nodePort — ไม่ใช่ Service ที่ DeployService สร้าง", svc.Name, nsName)
	}
	return nodePort, nil
}

// updateNetworkPolicyPort ให้ NetworkPolicy ของ database เปิดพอร์ตเดียวกับ ContainerPort ใหม่
// ลืมแก้แล้ว pod ใน namespace ต่อ database ไม่ได้ทั้งที่ Service ถูกต้อง (policy ยังเปิดพอร์ตเก่า)
func (k *KubernetesProvisioner) updateNetworkPolicyPort(
	ctx context.Context, cs kubernetes.Interface, nsName string, svc *entity.Service,
) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cur, err := cs.NetworkingV1().NetworkPolicies(nsName).Get(ctx, networkPolicyName(svc.Name), metav1.GetOptions{})
		if err != nil {
			return err
		}
		want := networkPolicyFor(nsName, svc).Spec.Ingress
		if len(cur.Spec.Ingress) == 1 && len(cur.Spec.Ingress[0].Ports) == 1 &&
			cur.Spec.Ingress[0].Ports[0].Port != nil && *cur.Spec.Ingress[0].Ports[0].Port == *want[0].Ports[0].Port {
			return nil
		}
		cur.Spec.Ingress = want
		_, err = cs.NetworkingV1().NetworkPolicies(nsName).Update(ctx, cur, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("แก้ NetworkPolicy ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}
	return nil
}

// DeleteService ลบของที่ DeployService สร้างไว้ (ทุกชิ้นชื่อตาม svc.Name)
// data flow: ServiceManager.Delete ส่ง namespace + service มา → ลบ workload → ลบ Service
// → database ลบ NetworkPolicy ต่อ → service ที่มีดิสก์ (รวม database) ลบ PVC ต่อ
//
// NotFound = สำเร็จ ตามสัญญา idempotent เดียวกับ DeleteNamespace: ServiceManager.Delete ลบแถวใน DB
// หลังเราคืน nil เท่านั้น ถ้าลบได้ตัวเดียวแล้วล้ม การกดลบซ้ำต้องเดินผ่านตัวที่หายไปแล้วได้
// ไม่งั้นแถวใน DB ค้างถาวรและโควตาไม่ถูกคืน
//
// ลบ workload ก่อนเพราะเป็นตัวที่กินทรัพยากร — Pod ถูก GC เก็บต่อแบบ async (propagation ของ
// apps/v1 เป็น Background อยู่แล้ว) เราไม่รอให้หมด ด้วยเหตุผลเดียวกับ DeleteNamespace
//
// PVC ต้องตามลบเอง: คลัสเตอร์ที่ไม่รู้จัก PersistentVolumeClaimRetentionPolicy จะไม่ลบ PVC จาก
// volumeClaimTemplates ให้ ไม่ลบแล้วดิสก์จะถูกจองค้างกินโควตาโดยไม่มีแถวใน DB ให้ตามเก็บ
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName string, svc *entity.Service) error {
	cs, err := k.client()
	if err != nil {
		return err
	}

	kind, deleteWorkload := "Deployment", cs.AppsV1().Deployments(nsName).Delete
	if svc.HasStorage() {
		kind, deleteWorkload = "StatefulSet", cs.AppsV1().StatefulSets(nsName).Delete
	}
	if err := deleteWorkload(ctx, svc.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ %s '%s' ใน namespace '%s' ไม่สำเร็จ: %w", kind, svc.Name, nsName, err)
	}
	err = cs.CoreV1().Services(nsName).Delete(ctx, svc.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ Service '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
	}

	if svc.IsDatabase {
		err = cs.NetworkingV1().NetworkPolicies(nsName).Delete(ctx, networkPolicyName(svc.Name), metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("ลบ NetworkPolicy ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
		}
	}
	if svc.IsTemplateDatabase() {
		err = cs.CoreV1().Secrets(nsName).Delete(ctx, databaseSecretName(svc.Name), metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("ลบ Secret ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
		}
	}
	if svc.HasStorage() {
		// PVC มี finalizer กันลบระหว่าง pod ยังใช้อยู่ — คำสั่งนี้จองการลบไว้ ดิสก์หายจริงหลัง pod ตายสนิท
		err = cs.CoreV1().PersistentVolumeClaims(nsName).Delete(ctx, pvcName(svc.Name), metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("ลบ PVC ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svc.Name, nsName, err)
		}
	}

	log.Printf("[k8s] ลบ service '%s' ออกจาก namespace '%s' แล้ว", svc.Name, nsName)
	return nil
}

// Status ถามคลัสเตอร์ว่า workload นี้เป็นยังไงจริงๆ (ดูสัญญาใน Provisioner)
//
// ดูตามลำดับ: object หลักยังอยู่ไหม → มี pod ไหม → pod อยู่ในสภาพไหน
// หลาย pod เอาตัวที่แย่ที่สุดเป็นคำตอบ เพราะ "1 ใน 3 ตายซ้ำๆ" ต้องไม่ถูกกลบด้วยอีกสองตัวที่ยังดี
func (k *KubernetesProvisioner) Status(ctx context.Context, nsName string, svc *entity.Service) (WorkloadStatus, error) {
	cs, err := k.client()
	if err != nil {
		return WorkloadStatus{}, err
	}

	gone := func(kind string) WorkloadStatus {
		return WorkloadStatus{Phase: PhaseGone, Reason: "NotFound",
			Message: fmt.Sprintf("ไม่พบ %s '%s' ใน namespace %s แล้ว — อาจถูกลบจากนอกระบบ", kind, svc.Name, nsName)}
	}

	// ข้อความจากตัวคุม replica ตอนสร้าง pod ไม่ได้เลย (เช่น ชน ResourceQuota) — Deployment
	// รายงานผ่าน condition ReplicaFailure ส่วน StatefulSet ไม่มี condition แบบนี้ให้
	var replicaFailure string
	if svc.HasStorage() {
		_, err := cs.AppsV1().StatefulSets(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return gone("StatefulSet"), nil
		}
		if err != nil {
			return WorkloadStatus{}, err
		}
	} else {
		dep, err := cs.AppsV1().Deployments(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return gone("Deployment"), nil
		}
		if err != nil {
			return WorkloadStatus{}, err
		}
		for _, c := range dep.Status.Conditions {
			if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
				replicaFailure = c.Message
			}
		}
	}

	pods, err := cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{LabelSelector: podSelector(svc.Name)})
	if err != nil {
		return WorkloadStatus{}, err
	}
	live := livePods(pods.Items)
	if len(live) == 0 {
		if replicaFailure != "" {
			return WorkloadStatus{Phase: PhaseFailed, Reason: "ReplicaFailure",
				Message: clip(replicaFailure, maxStatusMessage)}, nil
		}
		return WorkloadStatus{Phase: PhasePending, Reason: "NoPods",
			Message: "คลัสเตอร์ยังไม่ได้สร้าง pod ให้ — ปกติใช้เวลาไม่กี่วินาที ถ้าค้างนานอาจติดโควตาของ namespace บนคลัสเตอร์"}, nil
	}

	worst := inspectPod(&live[0])
	worstPod := live[0].Name
	restarts := worst.Restarts
	for i := 1; i < len(live); i++ {
		st := inspectPod(&live[i])
		restarts += st.Restarts
		if severity(st.Phase) > severity(worst.Phase) {
			worst, worstPod = st, live[i].Name
		}
	}
	worst.Restarts = restarts

	// pod ที่ตายซ้ำๆ ถูกสร้างใหม่เรื่อยๆ log ของรอบที่บอกสาเหตุจริงหายไปก่อนผู้ใช้จะทันเปิดดู
	// จึงเก็บ log ท้ายๆ ของ container รอบก่อนติดไปกับสถานะเลย
	if worst.Phase == PhaseCrashLoop {
		if tail := crashLogs(ctx, cs, nsName, worstPod); tail != "" {
			worst.Message = strings.TrimSpace(worst.Message) + "\n\nlog ท้ายๆ ก่อน container ตาย:\n" + tail
		}
	}
	worst.Reason = clip(worst.Reason, maxStatusReason)
	worst.Message = clip(worst.Message, maxStatusMessage)
	return worst, nil
}

// defaultLogTailLines ใช้เมื่อผู้เรียกไม่ระบุ tail — ไม่ปล่อยให้ k8s ส่ง log ทั้งหมดที่ node เก็บไว้มาทีเดียว
const defaultLogTailLines = 200

// Logs เปิด stream log ของทุก Pod ที่เป็นของ service นี้ (หา Pod จาก label ที่ deploymentFor แปะไว้)
//
// data flow: list Pod ด้วย labelServiceName=svcName → เปิด GetLogs ทีละ Pod
// → Pod เดียว: คืน stream ของ k8s ตรงๆ / หลาย Pod: รวมทีละบรรทัดผ่าน io.Pipe
// พร้อมแทรก "[ชื่อ pod] " หลัง timestamp ให้รู้ว่าบรรทัดนี้มาจาก replica ไหน
// (timestamp ต้องอยู่หน้าสุดเสมอ — หน้าเว็บ parse ตัดที่ช่องว่างตัวแรก)
//
// container ที่ติด CrashLoopBackOff ไม่มี log ของรอบปัจจุบันเพราะยังไม่ได้เริ่ม ต้องขอรอบก่อน
// (Previous) ไม่งั้นผู้ใช้เปิดหน้า Logs แล้วเห็นหน้าว่างทั้งที่ container ตายไปแล้วหลายรอบ
// Pod ที่ container ยังไม่เริ่ม (ImagePullBackOff / ContainerCreating) เปิด log ไม่ได้ ข้ามไป
// ถ้าเปิดไม่ได้เลยสักตัวคืน ErrLogsUnavailable พร้อมเหตุผลจาก k8s ให้ผู้ใช้เห็นว่าติดอะไร
//
// ponytail: เห็นเฉพาะ Pod ที่มีอยู่ตอนเปิด stream — Pod ใหม่จาก scale/restart ระหว่าง follow
// ต้องกดเปิดใหม่ ถ้าต้องการให้ตามเองค่อยเปลี่ยนไปใช้ Pods().Watch
func (k *KubernetesProvisioner) Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error) {
	cs, err := k.client()
	if err != nil {
		return nil, err
	}

	pods, err := cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{LabelSelector: podSelector(svcName)})
	if err != nil {
		return nil, fmt.Errorf("list Pod ของ '%s' ใน namespace '%s' ไม่สำเร็จ: %w", svcName, nsName, err)
	}

	tail := opts.TailLines
	if tail <= 0 {
		tail = defaultLogTailLines
	}

	type podStream struct {
		pod string
		rc  io.ReadCloser
	}
	var streams []podStream
	var firstErr error
	for _, p := range livePods(pods.Items) {
		podOpts := &corev1.PodLogOptions{
			Follow:     opts.Follow,
			Timestamps: opts.Timestamps,
			TailLines:  &tail,
			Previous:   inCrashLoop(&p),
		}
		if opts.SinceSeconds > 0 {
			podOpts.SinceSeconds = &opts.SinceSeconds
		}
		rc, err := cs.CoreV1().Pods(nsName).GetLogs(p.Name, podOpts).Stream(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		streams = append(streams, podStream{p.Name, rc})
	}

	switch len(streams) {
	case 0:
		if firstErr == nil {
			return nil, fmt.Errorf("%w (ไม่พบ Pod ของ service นี้)", ErrLogsUnavailable)
		}
		return nil, fmt.Errorf("%w (%v)", ErrLogsUnavailable, firstErr)
	case 1:
		return streams[0].rc, nil
	}

	// io.Pipe ยอมให้หลาย goroutine Write พร้อมกันได้ (ต่อคิวให้ทีละครั้ง) และเราเขียนทีละบรรทัดเต็ม
	// จึงไม่มีบรรทัดไหนถูกตัดปนกัน — ปิด pw เมื่อทุก Pod จบ (non-follow) หรือ ctx ถูก cancel (follow)
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	for _, s := range streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer s.rc.Close()
			tag := []byte("[" + s.pod + "] ")
			br := bufio.NewReader(s.rc)
			for {
				line, err := br.ReadBytes('\n')
				if len(line) > 0 {
					if _, werr := pw.Write(tagLogLine(line, tag)); werr != nil {
						return // ฝั่งอ่านปิดไปแล้ว
					}
				}
				if err != nil {
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		pw.Close()
	}()
	return pr, nil
}

// tagLogLine แทรก tag หลัง timestamp (ช่องว่างตัวแรก) และรับประกันว่าบรรทัดจบด้วย '\n'
func tagLogLine(line, tag []byte) []byte {
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	i := bytes.IndexByte(line, ' ') + 1 // ไม่มีช่องว่าง = 0 = แปะ tag หน้าสุด
	return slices.Concat(line[:i], tag, line[i:])
}

// ── ตัวช่วยอ่านสภาพ pod ────────────────────────────────────────────────────────────

// livePods ตัด pod ที่กำลังถูกลบออก (ช่วง rolling update / scale ลง) — ถ้าไม่ตัด สถานะจะกระพริบ
// เป็น "ตายซ้ำ" ทุกครั้งที่มี pod เก่ากำลังปิดตัว ทั้งที่ตัวใหม่รันดีอยู่
func livePods(pods []corev1.Pod) []corev1.Pod {
	live := make([]corev1.Pod, 0, len(pods))
	for _, p := range pods {
		if p.DeletionTimestamp == nil {
			live = append(live, p)
		}
	}
	if len(live) == 0 {
		return pods // กำลังถูกลบทั้งหมด — รายงานจากที่มีดีกว่าบอกว่าไม่มี pod
	}
	return live
}

// severity เรียงความร้ายแรงของ phase เพื่อเลือก pod ที่ "แย่ที่สุด" มารายงาน
func severity(p WorkloadPhase) int {
	switch p {
	case PhaseCrashLoop:
		return 3
	case PhaseFailed:
		return 2
	case PhasePending:
		return 1
	default:
		return 0
	}
}

// inspectPod แปลสภาพ pod หนึ่งตัวเป็น WorkloadStatus — ฟังก์ชันล้วนๆ เทสต์ได้โดยไม่ต้องมีคลัสเตอร์
//
// ใช้ชื่อ reason เดียวกับที่ kubectl แสดง เพราะผู้ใช้เอาไปค้นต่อได้ ยกเว้น OOMKilled ที่ยกขึ้นมา
// แทน CrashLoopBackOff เพราะทางแก้คนละเรื่องกัน (เพิ่ม RAM ไม่ใช่แก้ env)
func inspectPod(pod *corev1.Pod) WorkloadStatus {
	restarts := 0
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += int(cs.RestartCount)
	}

	switch pod.Status.Phase {
	case corev1.PodFailed:
		return WorkloadStatus{Phase: PhaseFailed, Reason: orDefault(pod.Status.Reason, "PodFailed"),
			Message: pod.Status.Message, Restarts: restarts}
	case corev1.PodSucceeded:
		return WorkloadStatus{Phase: PhaseCrashLoop, Reason: "Completed",
			Message:  "container จบการทำงานเองทันที (exit 0) — image นี้ไม่ได้รันโปรเซสค้างไว้รอรับงาน",
			Restarts: restarts}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil {
			switch w.Reason {
			case "CrashLoopBackOff":
				reason, msg := "CrashLoopBackOff", w.Message
				if t := cs.LastTerminationState.Terminated; t != nil {
					msg = terminatedSummary(t)
					if t.Reason == "OOMKilled" {
						reason = "OOMKilled"
					}
				}
				return WorkloadStatus{Phase: PhaseCrashLoop, Reason: reason, Message: msg, Restarts: restarts}
			case "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "ErrImageNeverPull",
				"CreateContainerConfigError", "CreateContainerError", "RunContainerError":
				return WorkloadStatus{Phase: PhaseFailed, Reason: w.Reason, Message: w.Message, Restarts: restarts}
			default: // ContainerCreating, PodInitializing — ยังไม่พัง แค่ยังไม่เสร็จ
				return WorkloadStatus{Phase: PhasePending, Reason: orDefault(w.Reason, "ContainerCreating"),
					Message: w.Message, Restarts: restarts}
			}
		}
		if t := cs.State.Terminated; t != nil {
			// ตายแล้วยังไม่ถูกสร้างใหม่ (ช่วงสั้นๆ ก่อน kubelet รีสตาร์ท)
			reason := "CrashLoopBackOff"
			if t.Reason == "OOMKilled" {
				reason = "OOMKilled"
			}
			return WorkloadStatus{Phase: PhaseCrashLoop, Reason: reason, Message: terminatedSummary(t), Restarts: restarts}
		}
		if cs.State.Running != nil && !cs.Ready {
			return WorkloadStatus{Phase: PhasePending, Reason: "NotReady",
				Message:  "container รันอยู่แต่ยังไม่มีอะไรฟังที่พอร์ตที่ระบุไว้ — ถ้าค้างนาน ตรวจสอบว่า Container Port ตรงกับพอร์ตที่ image เปิดจริง",
				Restarts: restarts}
		}
	}

	// ยังไม่มี container เลย = ยังหา node ให้ไม่ได้ หรือกำลังจะเริ่ม
	if len(pod.Status.ContainerStatuses) == 0 {
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
				return WorkloadStatus{Phase: PhasePending, Reason: "FailedScheduling", Message: c.Message, Restarts: restarts}
			}
		}
		return WorkloadStatus{Phase: PhasePending, Reason: orDefault(pod.Status.Reason, "Pending"),
			Message: pod.Status.Message, Restarts: restarts}
	}

	if pod.Status.Phase == corev1.PodRunning {
		return WorkloadStatus{Phase: PhaseRunning, Restarts: restarts}
	}
	return WorkloadStatus{Phase: PhasePending, Reason: orDefault(pod.Status.Reason, string(pod.Status.Phase)),
		Message: pod.Status.Message, Restarts: restarts}
}

func terminatedSummary(t *corev1.ContainerStateTerminated) string {
	s := fmt.Sprintf("container ออกด้วย exit code %d", t.ExitCode)
	if t.Reason != "" && t.Reason != "Error" {
		s += " (" + t.Reason + ")"
	}
	if t.Message != "" {
		s += ": " + t.Message
	}
	return s
}

func inCrashLoop(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason == "CrashLoopBackOff" {
			return true
		}
	}
	return false
}

// crashLogs ดึง log ท้ายๆ ของ container รอบก่อน (ตัวที่ตาย) — ไม่มีก็ลองรอบปัจจุบัน ไม่มีอีกก็ว่าง
// พลาดตรงนี้ไม่ใช่ error ของ Status: สถานะยังถูก แค่ไม่มี log แนบ
func crashLogs(ctx context.Context, cs kubernetes.Interface, nsName, podName string) string {
	tail := int64(crashLogTailLines)
	for _, previous := range []bool{true, false} {
		stream, err := cs.CoreV1().Pods(nsName).
			GetLogs(podName, &corev1.PodLogOptions{Previous: previous, TailLines: &tail}).Stream(ctx)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(stream, 4096))
		stream.Close()
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return ""
}

// clip ตัดข้อความให้ยาวไม่เกิน max ตัวอักษร โดยนับรวม … ที่ต่อท้ายด้วย
//
// ต้องนับเป็นตัวอักษรไม่ใช่ไบต์ เพราะ varchar ของ Postgres นับเป็นตัวอักษร และต้องเผื่อที่ให้ …
// ไม่งั้นผลลัพธ์ยาวเกินเพดาน 1 ตัว แล้ว UPDATE ถูกปฏิเสธตอน service พังพอดี
func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}
