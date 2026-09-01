package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"backend/internal/config"
	"backend/internal/entity"
)

// label ที่ติดไว้กับทุก resource ที่ระบบนี้สร้าง
//
// labelManagedBy คือกลไกความปลอดภัยหลักของไฟล์นี้ ไม่ใช่แค่ป้ายไว้ดูสวย:
// ก่อนจะ "แก้" หรือ "ลบ" อะไรบนคลัสเตอร์ เราอ่าน label นี้ก่อนเสมอ ถ้าไม่ใช่ของเรา = ไม่แตะ
// ที่ต้องทำแบบนี้เพราะชื่อ namespace มาจากที่ผู้ใช้กรอกเองตรงๆ (ดู NamespaceController.Create)
// ถ้าไม่มีด่านนี้ คนที่ตั้งชื่อ space ว่า kube-system แล้วกดลบ จะลบ namespace ของระบบทิ้งได้จริง
const (
	labelManagedBy    = "app.kubernetes.io/managed-by"
	labelManagedValue = "caesar-cluster"
	labelAppName      = "app.kubernetes.io/name"
	labelNamespaceID  = "caesar-cluster/namespace-id"
	labelServiceID    = "caesar-cluster/service-id"
)

// ชื่อ resource ที่สร้างไว้ประจำทุก namespace (ชื่อตายตัวเพื่อให้ apply ซ้ำแล้วทับตัวเดิมเสมอ)
const (
	quotaObjectName  = "caesar-quota"
	limitsObjectName = "caesar-limits"
	netpolObjectName = "caesar-isolation"
)

// KubernetesProvisioner = provisioner ของจริงที่คุยกับ Kubernetes API ผ่าน client-go
// ถูกเลือกใช้ใน main เมื่อ PROVISIONER=kubernetes
//
// สิ่งที่สร้างให้ 1 namespace (EnsureNamespace):
//  1. Namespace          — ติด label ของเรา + label ของ Pod Security Admission
//  2. ResourceQuota      — เพดาน CPU/RAM รวมของทั้ง space (บังคับซ้ำอีกชั้นจากที่ QuotaService เช็คใน DB)
//  3. LimitRange         — ค่า default ของ container ที่ไม่ระบุ resource + เพดานต่อ container
//  4. NetworkPolicy      — กัน traffic ข้าม namespace และกันไม่ให้ pod เดินเข้าวงภายในของคลัสเตอร์
//
// สิ่งที่สร้างให้ 1 service (DeployService): Deployment + Service ชนิด NodePort
type KubernetesProvisioner struct {
	cs      kubernetes.Interface
	cfg     config.K8sConfig
	timeout time.Duration
}

// NewKubernetesProvisioner ต่อเข้า Kubernetes API แล้วทดสอบการเชื่อมต่อทันที
//
// data flow: รับ path ของ kubeconfig (ว่าง = ลอง in-cluster ก่อน แล้วค่อยตกไปที่ ~/.kube/config)
// → สร้าง clientset → ถาม server version เพื่อพิสูจน์ว่าคุยได้จริง → คืน provisioner ให้ main
//
// ตั้งใจให้ fail ตั้งแต่ตอน start ถ้าต่อคลัสเตอร์ไม่ได้ ดีกว่าปล่อยให้ server ขึ้นมาแล้วค่อยพัง
// ตอนผู้ใช้กด deploy ครั้งแรก (ตอนนั้นแยกไม่ออกว่า kubeconfig ผิดหรือคลัสเตอร์ล่ม)
func NewKubernetesProvisioner(kubeConfigPath string, cfg config.K8sConfig) (*KubernetesProvisioner, error) {
	timeout := time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	restCfg, err := buildRestConfig(kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("อ่าน kubeconfig ไม่สำเร็จ: %w", err)
	}
	restCfg.Timeout = timeout

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("สร้าง kubernetes client ไม่สำเร็จ: %w", err)
	}

	ver, err := cs.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("ต่อ Kubernetes API ที่ %s ไม่ได้: %w", restCfg.Host, err)
	}
	log.Printf("kubernetes: ต่อ %s สำเร็จ (server %s)", restCfg.Host, ver.GitVersion)

	return &KubernetesProvisioner{cs: cs, cfg: cfg, timeout: timeout}, nil
}

// buildRestConfig หาทางต่อคลัสเตอร์ตามลำดับ: path ที่ระบุมา → in-cluster → ~/.kube/config
// รองรับทั้งกรณีรันเป็น container บน NUC (mount kubeconfig เข้ามา) และกรณีย้ายไปรันเป็น pod ในคลัสเตอร์เอง
func buildRestConfig(path string) (*rest.Config, error) {
	if path != "" {
		return clientcmd.BuildConfigFromFlags("", path)
	}
	if inCluster, err := rest.InClusterConfig(); err == nil {
		return inCluster, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

// EnsureNamespace สร้าง (หรืออัปเดต) namespace ของ space หนึ่งให้ตรงกับสิ่งที่บันทึกไว้ใน DB
//
// data flow: รับ entity.Namespace จาก NamespaceManager.Create / SetQuota
// → สร้าง Namespace → ResourceQuota → LimitRange → NetworkPolicy ตามลำดับ
//
// idempotent: เรียกซ้ำได้ตลอด ครั้งที่สองเป็นต้นไปคือการ "sync ค่าใหม่" ทับของเดิม
// (SetQuota ใช้ประโยชน์จากจุดนี้ตรงๆ — ปรับโควตาใน DB แล้วเรียกตัวนี้ให้ดันค่าใหม่ขึ้นคลัสเตอร์)
func (k *KubernetesProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()

	if err := k.ensureNamespaceObject(ctx, ns); err != nil {
		return err
	}
	if err := k.ensureResourceQuota(ctx, ns); err != nil {
		return err
	}
	if err := k.ensureLimitRange(ctx, ns.Name); err != nil {
		return err
	}
	return k.ensureNetworkPolicy(ctx, ns.Name)
}

// ensureNamespaceObject สร้าง Namespace เอง พร้อม label ของเราและ label ของ Pod Security Admission
//
// ถ้ามี namespace ชื่อนี้อยู่แล้วแต่ไม่ใช่ของเรา จะปฏิเสธทันที ไม่ยึดมาเป็นของตัวเอง —
// นี่คือด่านที่กัน "ตั้งชื่อ space ว่า kube-system" ไม่ให้ไปยุ่งกับของระบบ
func (k *KubernetesProvisioner) ensureNamespaceObject(ctx context.Context, ns *entity.Namespace) error {
	desired := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns.Name,
			Labels: k.namespaceLabels(ns),
		},
	}

	existing, err := k.cs.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = k.cs.CoreV1().Namespaces().Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("สร้าง namespace %q ไม่สำเร็จ: %w", ns.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน namespace %q ไม่สำเร็จ: %w", ns.Name, err)
	}
	// มีชื่อนี้อยู่แล้วแต่ไม่ใช่ของเรา = ไม่ยึดมาใช้เด็ดขาด (ดูเหตุผลที่ labelManagedBy)
	//
	// คืนเป็น ErrNameTaken เพราะจากมุมผู้ใช้มันคือ "ชื่อนี้ถูกใช้แล้ว ไปตั้งชื่ออื่น" เหมือนกับตอนชนกัน
	// ใน DB ทุกประการ ทำให้ controller แปลงเป็น 409 ได้โดยไม่ต้องรู้ว่ามี k8s อยู่
	// ส่วนเหตุผลจริงที่แอดมินต้องรู้ log ไว้ที่นี่ ไม่ส่งออกไปให้ผู้ใช้ทั่วไปเห็นโครงสร้างคลัสเตอร์
	if !isManaged(existing.Labels) {
		log.Printf("ปฏิเสธการใช้ namespace %q: มีอยู่บนคลัสเตอร์แล้วแต่ไม่มี label %s=%s "+
			"— ไม่ยึดมาใช้เพื่อความปลอดภัย", ns.Name, labelManagedBy, labelManagedValue)
		return fmt.Errorf("%w (มี namespace ชื่อนี้อยู่บนคลัสเตอร์แล้ว)", ErrNameTaken)
	}
	// namespace ที่กำลังถูกลบจะรับ resource ใหม่ไม่ได้ ต้องรอให้ตายสนิทก่อน
	// เจอบ่อยตอนลบ space แล้วสร้างใหม่ชื่อเดิมทันที (การลบของ k8s เป็น async)
	if existing.Status.Phase == corev1.NamespaceTerminating {
		return fmt.Errorf("%w (namespace %q ยังอยู่ในสถานะ Terminating)", ErrNamespaceTerminating, ns.Name)
	}

	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, val := range desired.Labels {
		existing.Labels[key] = val
	}
	if _, err := k.cs.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต label ของ namespace %q ไม่สำเร็จ: %w", ns.Name, err)
	}
	return nil
}

// ensureResourceQuota ดัน ResourceQuota ให้ตรงกับ limit ใน DB
//
// นี่คือการบังคับโควตา "ชั้นสุดท้าย": QuotaService เช็คใน DB ก่อนอนุญาต แต่ถ้าโค้ดเราพลาด
// หรือมีใครสร้าง workload ผ่านทางอื่น k8s จะยังปฏิเสธให้เองตรงนี้
//
// นอกจาก CPU/RAM ยังปิด 2 อย่างที่ระบบยังไม่รองรับและเปิดไว้แล้วอันตราย:
//   - persistentvolumeclaims = 0 — ยังไม่มีฟีเจอร์ storage (ดูข้อ 6 ใน backend/README.md)
//   - services.loadbalancers = 0 — MetalLB มี IP แค่ 21 ตัว (192.168.100.200-220) ถ้าปล่อยให้ขอได้
//     นักศึกษาไม่กี่คนก็ดูด IP หมด pool ทั้งคลัสเตอร์
func (k *KubernetesProvisioner) ensureResourceQuota(ctx context.Context, ns *entity.Namespace) error {
	cpu := *resource.NewMilliQuantity(int64(ns.CPULimitMilli), resource.DecimalSI)
	mem := *resource.NewQuantity(int64(ns.RAMLimitMB)*1024*1024, resource.BinarySI)
	zero := *resource.NewQuantity(0, resource.DecimalSI)

	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaObjectName,
			Namespace: ns.Name,
			Labels:    managedLabels(),
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:            cpu,
				corev1.ResourceLimitsCPU:              cpu,
				corev1.ResourceRequestsMemory:         mem,
				corev1.ResourceLimitsMemory:           mem,
				corev1.ResourcePersistentVolumeClaims: zero,
				corev1.ResourceServicesLoadBalancers:  zero,
			},
		},
	}

	client := k.cs.CoreV1().ResourceQuotas(ns.Name)
	existing, err := client.Get(ctx, quotaObjectName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("สร้าง ResourceQuota ของ %q ไม่สำเร็จ: %w", ns.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน ResourceQuota ของ %q ไม่สำเร็จ: %w", ns.Name, err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต ResourceQuota ของ %q ไม่สำเร็จ: %w", ns.Name, err)
	}
	return nil
}

// ensureLimitRange ตั้งค่า default + เพดานของ container 1 ตัวใน namespace
//
// จำเป็นคู่กับ ResourceQuota เสมอ: เมื่อ namespace มี ResourceQuota ที่คุม requests/limits อยู่
// k8s จะไม่ยอมรับ pod ที่ไม่ได้ระบุ resource เลย — LimitRange เป็นตัวเติมค่า default ให้
// (workload ที่ระบบนี้สร้างระบุค่ามาครบอยู่แล้ว ตัวนี้กันกรณีมีของแปลกปลอมหลุดเข้ามา)
func (k *KubernetesProvisioner) ensureLimitRange(ctx context.Context, nsName string) error {
	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      limitsObjectName,
			Namespace: nsName,
			Labels:    managedLabels(),
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Default: corev1.ResourceList{
					corev1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
					corev1.ResourceMemory: *resource.NewQuantity(256*1024*1024, resource.BinarySI),
				},
				DefaultRequest: corev1.ResourceList{
					corev1.ResourceCPU:    *resource.NewMilliQuantity(100, resource.DecimalSI),
					corev1.ResourceMemory: *resource.NewQuantity(128*1024*1024, resource.BinarySI),
				},
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(entity.MaxCPUMilliPerService), resource.DecimalSI),
					corev1.ResourceMemory: *resource.NewQuantity(int64(entity.MaxRAMMBPerService)*1024*1024, resource.BinarySI),
				},
			}},
		},
	}

	client := k.cs.CoreV1().LimitRanges(nsName)
	existing, err := client.Get(ctx, limitsObjectName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("สร้าง LimitRange ของ %q ไม่สำเร็จ: %w", nsName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน LimitRange ของ %q ไม่สำเร็จ: %w", nsName, err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต LimitRange ของ %q ไม่สำเร็จ: %w", nsName, err)
	}
	return nil
}

// ensureNetworkPolicy วางกฎเครือข่ายของ namespace
//
// โจทย์คือ "กัน traffic ข้าม namespace" แต่ยังต้องให้ผู้ใช้เข้าถึง service ของตัวเองผ่าน NodePort ได้
// เลยใช้วิธี allow ทุกอย่างยกเว้นวง pod แทนการ deny ทั้งหมด (deny-all ธรรมดาจะปิด NodePort ไปด้วย):
//
//	ขาเข้า  — รับได้จาก pod ใน namespace เดียวกัน และจากทุก IP ที่ไม่ใช่ pod CIDR
//	          (traffic ที่มาทาง NodePort ต้นทางเป็น IP ของ node ในวง VLAN100 จึงผ่าน
//	           ส่วน pod ของ space อื่นอยู่ใน pod CIDR จึงถูกตัด)
//	ขาออก   — เปิด DNS ไป kube-system, คุยกับ pod ในกลุ่มตัวเองได้, ออกอินเทอร์เน็ตได้
//	          แต่ห้ามเข้า pod CIDR และห้ามเข้าวงที่ตั้งไว้ใน K8S_BLOCKED_EGRESS_CIDRS
//	          (default = 192.168.100.0/24 กันไม่ให้ container ยิงเข้า SSH/kubelet ของ node)
//
// ต้องมี CNI ที่บังคับ NetworkPolicy ได้ถึงจะมีผลจริง — คลัสเตอร์นี้ใช้ Calico จึงบังคับได้
func (k *KubernetesProvisioner) ensureNetworkPolicy(ctx context.Context, nsName string) error {
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP
	dnsPort := intstr.FromInt32(53)

	sameNamespace := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{}}

	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netpolObjectName,
			Namespace: nsName,
			Labels:    managedLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // ทุก pod ใน namespace นี้
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{
					sameNamespace,
					{IPBlock: &networkingv1.IPBlock{
						CIDR:   "0.0.0.0/0",
						Except: []string{k.cfg.PodCIDR},
					}},
				},
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				},
				{To: []networkingv1.NetworkPolicyPeer{sameNamespace}},
				{To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "0.0.0.0/0",
						Except: k.blockedEgressCIDRs(),
					},
				}}},
			},
		},
	}

	client := k.cs.NetworkingV1().NetworkPolicies(nsName)
	existing, err := client.Get(ctx, netpolObjectName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("สร้าง NetworkPolicy ของ %q ไม่สำเร็จ: %w", nsName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน NetworkPolicy ของ %q ไม่สำเร็จ: %w", nsName, err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if _, err := client.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต NetworkPolicy ของ %q ไม่สำเร็จ: %w", nsName, err)
	}
	return nil
}

// blockedEgressCIDRs รวม pod CIDR เข้ากับวงที่ห้ามออกตาม config แล้วตัดตัวซ้ำทิ้ง
// (k8s ไม่ยอมรับ except ที่ซ้ำกัน และ pod CIDR มักถูกใส่มาใน config ซ้ำอยู่แล้ว)
func (k *KubernetesProvisioner) blockedEgressCIDRs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(k.cfg.BlockedEgressCIDRs)+1)
	for _, cidr := range append([]string{k.cfg.PodCIDR}, k.cfg.BlockedEgressCIDRs...) {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" || seen[cidr] {
			continue
		}
		seen[cidr] = true
		out = append(out, cidr)
	}
	return out
}

// DeleteNamespace ลบ namespace ทิ้งทั้งก้อน (workload ข้างในหายตามหมด)
//
// idempotent ตามสัญญาใน Provisioner: ไม่มีอยู่แล้ว หรือกำลังถูกลบอยู่ ถือว่าสำเร็จทั้งคู่
// เพราะ NamespaceManager.Delete ถอนของบนคลัสเตอร์ก่อนแล้วค่อยลบแถวใน DB — ถ้าล้มกลางคัน
// การสั่งลบซ้ำต้องเดินจนจบได้ ไม่งั้น namespace นั้นค้างใน DB ตลอดกาล
func (k *KubernetesProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()

	existing, err := k.cs.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน namespace %q ไม่สำเร็จ: %w", nsName, err)
	}
	if !isManaged(existing.Labels) {
		return fmt.Errorf("ปฏิเสธการลบ namespace %q เพราะไม่ได้ถูกสร้างโดย Caesar Cluster", nsName)
	}
	if existing.Status.Phase == corev1.NamespaceTerminating {
		return nil
	}

	err = k.cs.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ namespace %q ไม่สำเร็จ: %w", nsName, err)
	}
	return nil
}

// DeployService สร้าง Deployment + Service ชนิด NodePort ให้ workload 1 ตัว
//
// data flow: รับ entity.Service ที่ ServiceManager.Create เพิ่ง INSERT ลง DB (ผ่านการเช็คโควตาแล้ว)
// → สร้าง Deployment (resource requests = limits = ค่าที่ผู้ใช้ขอ) → สร้าง Service ชนิด NodePort
// → อ่านเลข nodePort ที่ k8s จ่ายให้ แล้วเซ็ตกลับที่ svc.NodePort ให้ ServiceManager เอาไป UPDATE ลง DB
//
// ถ้าขั้นตอนหลังพัง จะถอน Deployment ที่เพิ่งสร้างออกให้ด้วย — ไม่ทิ้ง workload ที่ไม่มี record ใน DB
// ค้างกินทรัพยากรบนคลัสเตอร์ (ServiceManager จะลบแถวใน DB ทิ้งเมื่อเราคืน error)
//
// ไม่รอจน pod พร้อมใช้งาน: การดึง image ครั้งแรกบน node Atom ใช้เวลาได้เป็นนาที ถ้ารอจะกลายเป็น
// HTTP request ที่ค้างยาวจน timeout ทั้งที่ deploy สำเร็จ (สถานะที่ตรงกับของจริงต้องทำด้วย
// reconcile loop แยก ซึ่งเป็นงานที่ยังไม่ได้ทำ — ดูข้อ 5 ใน backend/README.md)
func (k *KubernetesProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()

	if err := k.assertManagedNamespace(ctx, nsName); err != nil {
		return err
	}

	port := k.containerPortFor(svc)

	if err := k.applyDeployment(ctx, nsName, svc, port); err != nil {
		return err
	}

	nodePort, err := k.applyNodePortService(ctx, nsName, svc, port)
	if err != nil {
		// Deployment สร้างไปแล้วแต่ Service ไม่ผ่าน — ถอนออกก่อนคืน error ไม่งั้นเหลือของค้าง
		if cleanupErr := k.deleteWorkload(ctx, nsName, svc.Name); cleanupErr != nil {
			log.Printf("!! ถอน Deployment %q ใน %q หลัง deploy ล้มเหลวไม่สำเร็จ: %v",
				svc.Name, nsName, cleanupErr)
		}
		return err
	}

	svc.NodePort = &nodePort
	log.Printf("kubernetes: deploy %q เข้า %q สำเร็จ — %dm CPU / %d MB เข้าถึงที่ <node-ip>:%d",
		svc.Name, nsName, svc.CPUMilli, svc.RAMMB, nodePort)
	return nil
}

// applyDeployment สร้างหรืออัปเดต Deployment ของ service หนึ่ง
//
// requests เท่ากับ limits โดยตั้งใจ: ทำให้ pod ได้ QoS class Guaranteed และทำให้ยอดที่ ResourceQuota
// หักไปตรงกับที่ QuotaService คำนวณใน DB เป๊ะ (ถ้า requests น้อยกว่า limits ตัวเลขสองฝั่งจะไม่ตรงกัน)
func (k *KubernetesProvisioner) applyDeployment(ctx context.Context, nsName string, svc *entity.Service, port int) error {
	replicas := int32(1)
	noEscalate := false
	privileged := false
	noAutomount := false
	noServiceLinks := false

	selector := map[string]string{
		labelAppName:   svc.Name,
		labelManagedBy: labelManagedValue,
	}
	podLabels := map[string]string{
		labelAppName:   svc.Name,
		labelManagedBy: labelManagedValue,
		labelServiceID: strconv.Itoa(svc.ID),
	}

	security := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &noEscalate,
		Privileged:               &privileged,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
			Add:  addedCapabilities(k.cfg.PodSecurity),
		},
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
	if k.cfg.PodSecurity == config.PodSecurityRestricted {
		runAsNonRoot := true
		security.RunAsNonRoot = &runAsNonRoot
	}

	cpu := *resource.NewMilliQuantity(int64(svc.CPUMilli), resource.DecimalSI)
	mem := *resource.NewQuantity(int64(svc.RAMMB)*1024*1024, resource.BinarySI)

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: nsName,
			Labels:    podLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					// ปิด token ของ ServiceAccount: pod ของนักศึกษาไม่มีเหตุผลต้องคุยกับ Kubernetes API
					// ถ้าเปิดทิ้งไว้ ใครก็ตามที่เข้าถึง container ได้จะยิง API ด้วยสิทธิ์ของ default SA ได้ทันที
					AutomountServiceAccountToken: &noAutomount,
					EnableServiceLinks:           &noServiceLinks,
					Containers: []corev1.Container{{
						Name:            "app",
						Image:           svc.Image,
						ImagePullPolicy: corev1.PullPolicy(k.cfg.ImagePullPolicy),
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: int32(port),
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: envVarsOf(svc.EnvVars),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: mem},
						},
						SecurityContext: security,
					}},
				},
			},
		},
	}

	client := k.cs.AppsV1().Deployments(nsName)
	existing, err := client.Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("สร้าง Deployment %q ไม่สำเร็จ: %w", svc.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("อ่าน Deployment %q ไม่สำเร็จ: %w", svc.Name, err)
	}
	if !isManaged(existing.Labels) {
		return fmt.Errorf("มี Deployment ชื่อ %q อยู่แล้วใน %q แต่ไม่ได้ถูกสร้างโดย Caesar Cluster", svc.Name, nsName)
	}

	// selector ของ Deployment แก้ไม่ได้หลังสร้าง — คงของเดิมไว้แล้วอัปเดตเฉพาะส่วนที่แก้ได้
	desired.Spec.Selector = existing.Spec.Selector
	desired.ResourceVersion = existing.ResourceVersion
	if _, err := client.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("อัปเดต Deployment %q ไม่สำเร็จ: %w", svc.Name, err)
	}
	return nil
}

// applyNodePortService สร้างหรืออัปเดต Service ชนิด NodePort แล้วคืนเลข port ที่ k8s จ่ายให้
//
// ถ้าแถวใน DB เคยมี node_port อยู่แล้ว จะขอเลขเดิมกลับมา (deploy ซ้ำแล้ว URL ที่ผู้ใช้จดไว้ไม่เปลี่ยน)
func (k *KubernetesProvisioner) applyNodePortService(ctx context.Context, nsName string, svc *entity.Service, port int) (int, error) {
	servicePort := corev1.ServicePort{
		Name:       "http",
		Protocol:   corev1.ProtocolTCP,
		Port:       int32(port),
		TargetPort: intstr.FromInt32(int32(port)),
	}
	if svc.NodePort != nil && *svc.NodePort > 0 {
		servicePort.NodePort = int32(*svc.NodePort)
	}

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: nsName,
			Labels: map[string]string{
				labelAppName:   svc.Name,
				labelManagedBy: labelManagedValue,
				labelServiceID: strconv.Itoa(svc.ID),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				labelAppName:   svc.Name,
				labelManagedBy: labelManagedValue,
			},
			Ports: []corev1.ServicePort{servicePort},
		},
	}

	client := k.cs.CoreV1().Services(nsName)
	result, err := client.Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		result, err = client.Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			return 0, fmt.Errorf("สร้าง Service %q ไม่สำเร็จ: %w", svc.Name, err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("อ่าน Service %q ไม่สำเร็จ: %w", svc.Name, err)
	} else {
		if !isManaged(result.Labels) {
			return 0, fmt.Errorf("มี Service ชื่อ %q อยู่แล้วใน %q แต่ไม่ได้ถูกสร้างโดย Caesar Cluster", svc.Name, nsName)
		}
		// ClusterIP กำหนดมาแล้วตอนสร้างและแก้ไม่ได้ ต้องส่งค่าเดิมกลับไปไม่งั้น API ปฏิเสธ
		// (ClusterIPs กับ IPFamilies เป็นชุดเดียวกันในคลัสเตอร์ที่เปิด dual-stack ต้องคงไว้ด้วย)
		// เช่นเดียวกับ nodePort ที่เคยได้มา — คงเลขเดิมไว้ให้ URL ของผู้ใช้ไม่เปลี่ยน
		desired.Spec.ClusterIP = result.Spec.ClusterIP
		desired.Spec.ClusterIPs = result.Spec.ClusterIPs
		desired.Spec.IPFamilies = result.Spec.IPFamilies
		desired.Spec.IPFamilyPolicy = result.Spec.IPFamilyPolicy
		desired.ResourceVersion = result.ResourceVersion
		if desired.Spec.Ports[0].NodePort == 0 && len(result.Spec.Ports) > 0 {
			desired.Spec.Ports[0].NodePort = result.Spec.Ports[0].NodePort
		}
		result, err = client.Update(ctx, desired, metav1.UpdateOptions{})
		if err != nil {
			return 0, fmt.Errorf("อัปเดต Service %q ไม่สำเร็จ: %w", svc.Name, err)
		}
	}

	if len(result.Spec.Ports) == 0 || result.Spec.Ports[0].NodePort == 0 {
		return 0, fmt.Errorf("Service %q ถูกสร้างแล้วแต่คลัสเตอร์ยังไม่จ่าย NodePort มาให้", svc.Name)
	}
	return int(result.Spec.Ports[0].NodePort), nil
}

// DeleteService ลบ workload ตัวเดียวออกจาก namespace (ทั้ง Deployment และ Service)
// idempotent เช่นเดียวกับ DeleteNamespace — ของหายไปแล้วถือว่าสำเร็จ
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName, svcName string) error {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()

	// namespace หายไปแล้ว = workload ข้างในหายตามไปแล้ว ไม่ต้องทำอะไรต่อ
	if err := k.assertManagedNamespace(ctx, nsName); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return k.deleteWorkload(ctx, nsName, svcName)
}

// deleteWorkload ลบ Deployment + Service ของ workload หนึ่ง โดยกลืน NotFound ให้เป็นสำเร็จ
// ใช้ทั้งจาก DeleteService และจาก DeployService ตอนถอยหลัง deploy ล้มเหลวกลางทาง
func (k *KubernetesProvisioner) deleteWorkload(ctx context.Context, nsName, svcName string) error {
	err := k.cs.CoreV1().Services(nsName).Delete(ctx, svcName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ Service %q ใน %q ไม่สำเร็จ: %w", svcName, nsName, err)
	}

	err = k.cs.AppsV1().Deployments(nsName).Delete(ctx, svcName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ Deployment %q ใน %q ไม่สำเร็จ: %w", svcName, nsName, err)
	}
	return nil
}

// findPod หา pod ของ service หนึ่งตัว — Deployment ของระบบนี้มี replica เดียวเสมอ (ดู applyDeployment)
// จึงเลือกตัวที่สถานะเป็น Running ก่อน ถ้าไม่มีสักตัว (เช่นกำลัง CrashLoopBackOff) ใช้ตัวแรกที่เจอแทน
// เพื่อให้ยังดึง log ของ container ที่พังออกมาดูสาเหตุได้ ไม่ใช่ปฏิเสธเฉยๆ ว่า "ไม่มี pod ที่ Running"
func (k *KubernetesProvisioner) findPod(ctx context.Context, nsName, svcName string) (*corev1.Pod, error) {
	list, err := k.cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", labelAppName, svcName, labelManagedBy, labelManagedValue),
	})
	if err != nil {
		return nil, fmt.Errorf("หา pod ของ %q ไม่สำเร็จ: %w", svcName, err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("ไม่พบ pod ของ service %q ใน %q (อาจกำลังสร้างอยู่ ลองใหม่อีกครั้งในอีกสักครู่)", svcName, nsName)
	}
	for i := range list.Items {
		if list.Items[i].Status.Phase == corev1.PodRunning {
			return &list.Items[i], nil
		}
	}
	return &list.Items[0], nil
}

// assertManagedNamespace ยืนยันว่า namespace ปลายทางเป็นของระบบนี้จริงก่อนจะไปแตะอะไรข้างใน
// คืน error ที่ apierrors.IsNotFound เป็นจริงได้ เพื่อให้ผู้เรียกแยกกรณี "ไม่มีแล้ว" ออกจากกรณีอื่น
func (k *KubernetesProvisioner) assertManagedNamespace(ctx context.Context, nsName string) error {
	ns, err := k.cs.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !isManaged(ns.Labels) {
		return fmt.Errorf("namespace %q ไม่ได้ถูกสร้างโดย Caesar Cluster — ปฏิเสธการเข้าไปแก้ไข", nsName)
	}
	return nil
}

// containerPortFor หา port ที่ container ฟังอยู่ ตามลำดับความน่าเชื่อถือ:
// ค่าที่ผู้ใช้กรอกมาตรงๆ → env var ที่เป็นธรรมเนียมของ image ทั่วไป → ค่า default จาก config
func (k *KubernetesProvisioner) containerPortFor(svc *entity.Service) int {
	if svc.ContainerPort != nil && *svc.ContainerPort > 0 && *svc.ContainerPort < 65536 {
		return *svc.ContainerPort
	}
	for _, key := range []string{"PORT", "CONTAINER_PORT", "APP_PORT", "HTTP_PORT"} {
		v, ok := svc.EnvVars[key]
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	return k.cfg.DefaultContainerPort
}

// addedCapabilities คืน capability ที่ "คืนกลับ" ให้ container หลัง drop ALL ไปแล้ว
//
// เราเริ่มจาก drop ALL เสมอ แล้วค่อยคืนเฉพาะที่จำเป็น ซึ่งตัดตัวอันตรายออกหมดตั้งแต่ต้น:
// NET_RAW (ดักแพ็กเก็ตของคนอื่น), SYS_ADMIN, SYS_PTRACE, SYS_MODULE, MKNOD ไม่มีทางได้คืน
//
// ที่ต้องแยกตามระดับ เพราะ image ทั่วไปเกือบทั้งหมดเริ่มต้นด้วยการรันเป็น root
// เพื่อเตรียมไฟล์ แล้วค่อยลดสิทธิ์ตัวเองลงเป็น user ธรรมดาก่อนรันจริง ขั้นตอนนั้นต้องใช้
// CHOWN, SETUID, SETGID เป็นอย่างน้อย ถ้าไม่คืนให้ image ยอดนิยมอย่าง nginx จะ crash ทันที
// ด้วย "chown ... Operation not permitted" ตั้งแต่ยังไม่ทันเริ่มทำงาน
//
// baseline   = คืนชุดที่ image ปกติต้องใช้ตอน init (ค่าที่แนะนำสำหรับแพลตฟอร์มนักศึกษา)
// restricted = คืนแค่สิทธิ์ผูก port ต่ำกว่า 1024 ซึ่งเป็นตัวเดียวที่ระดับ restricted ของ
//
//	Pod Security Admission ยอมให้ add ได้ — ใช้คู่กับ runAsNonRoot
//	image ต้องถูกสร้างมาให้รันเป็น non-root ตั้งแต่แรกถึงจะใช้โหมดนี้ได้
func addedCapabilities(podSecurity string) []corev1.Capability {
	if podSecurity == config.PodSecurityRestricted {
		return []corev1.Capability{"NET_BIND_SERVICE"}
	}
	return []corev1.Capability{
		"CHOWN",            // เปลี่ยนเจ้าของไฟล์ที่ตัวเองสร้าง เช่น cache dir ของ nginx
		"DAC_OVERRIDE",     // เขียนไฟล์ที่ permission ไม่ตรงระหว่างเตรียมตัว
		"FOWNER",           // แก้ permission ของไฟล์ที่ตัวเองเป็นเจ้าของ
		"SETGID",           // ลดสิทธิ์ตัวเองลงเป็น group ธรรมดาก่อนรันจริง
		"SETUID",           // ลดสิทธิ์ตัวเองลงเป็น user ธรรมดาก่อนรันจริง
		"NET_BIND_SERVICE", // ผูก port ต่ำกว่า 1024 เช่น nginx ที่ฟัง port 80
	}
}

// namespaceLabels ประกอบ label ของ Namespace: label ของเรา + label ที่ Pod Security Admission อ่าน
//
// PSA เป็นกลไกในตัวของ k8s ที่บังคับจากระดับ namespace — ต่อให้ manifest ของ pod เขียนมายังไง
// ถ้าเกินระดับที่ enforce ไว้ API server จะปฏิเสธตั้งแต่ตอนสร้าง (กัน privileged, hostPath, hostNetwork)
func (k *KubernetesProvisioner) namespaceLabels(ns *entity.Namespace) map[string]string {
	level := k.cfg.PodSecurity
	return map[string]string{
		labelManagedBy:   labelManagedValue,
		labelNamespaceID: strconv.Itoa(ns.ID),

		"pod-security.kubernetes.io/enforce": level,
		"pod-security.kubernetes.io/audit":   level,
		"pod-security.kubernetes.io/warn":    level,
	}
}

// managedLabels = label ชุดพื้นฐานสำหรับ resource ที่ไม่ได้ผูกกับ service ตัวใดตัวหนึ่ง
func managedLabels() map[string]string {
	return map[string]string{labelManagedBy: labelManagedValue}
}

// isManaged บอกว่า resource นี้เป็นของระบบเราหรือไม่ — ด่านเดียวที่กันไม่ให้ไปลบของคนอื่น
func isManaged(labels map[string]string) bool {
	return labels[labelManagedBy] == labelManagedValue
}

// envVarsOf แปลง map ของ env vars เป็น slice ของ k8s โดยเรียงตามชื่อ
// เรียงเพื่อให้ manifest ที่ได้เหมือนเดิมทุกครั้งที่ deploy ซ้ำ — ไม่งั้นลำดับ map ที่สุ่มของ Go
// จะทำให้ k8s เห็นว่า spec เปลี่ยนแล้ว rollout pod ใหม่ทั้งที่ค่าเท่าเดิม
func envVarsOf(env entity.EnvVarMap) []corev1.EnvVar {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]corev1.EnvVar, 0, len(keys))
	for _, key := range keys {
		out = append(out, corev1.EnvVar{Name: key, Value: env[key]})
	}
	return out
}

// withTimeout ครอบ context ที่รับมาด้วย timeout ของการคุย Kubernetes API
// กันไม่ให้ HTTP request ของผู้ใช้ค้างยาวเมื่อ API server ไม่ตอบ
func (k *KubernetesProvisioner) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, k.timeout)
}
