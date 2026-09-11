package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	netv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"backend/internal/entity"
)

// KubernetesProvisioner = provisioner ของจริง คุยกับ Kubernetes API ผ่าน client-go
// ถูกเลือกใช้ใน main เมื่อ PROVISIONER=kubernetes
//
// ทุกอย่างใช้ server-side apply (field manager "caesar-cluster") จึงเรียกซ้ำได้ปลอดภัย:
// deploy ซ้ำชื่อเดิม = อัปเดต ไม่ต้องแยกทาง create/update เอง
//
// ข้อควรรู้ตอนเอาขึ้นคลัสเตอร์จริง:
//   - PVC ไม่ระบุ storageClassName จึงใช้ StorageClass default ของคลัสเตอร์ (k3s = local-path)
//     ถ้าคลัสเตอร์ไม่มี default pod ของ database จะค้าง Pending ว่า "unbound PersistentVolumeClaims"
//   - NetworkPolicy มีผลก็ต่อเมื่อ CNI บังคับใช้ (k3s เปิดให้ตั้งแต่ต้น) ทดสอบได้ด้วยการยิงจาก
//     pod ใน namespace อื่นเข้าพอร์ตของ database แล้วต้อง timeout
//   - สิทธิ์ที่ kubeconfig/ServiceAccount ต้องมี: namespaces + resourcequotas, และใน namespace
//     deployments, statefulsets, services, networkpolicies, pvc, pods กับ pods/log
type KubernetesProvisioner struct {
	client kubernetes.Interface
}

const (
	k8sFieldManager = "caesar-cluster"

	// labelApp = label ที่ผูก Pod / Service / NetworkPolicy ของ service เดียวกันไว้ด้วยกัน
	// Status กับ Logs ใช้ label นี้หา pod — เปลี่ยนแล้วต้องเปลี่ยนทุกที่พร้อมกัน
	labelApp = "app"

	k8sQuotaName  = "caesar-quota"
	k8sVolumeName = "data" // ชื่อ volume ใน StatefulSet → PVC จะชื่อ data-<service>-0

	crashLogTailLines = 15   // log กี่บรรทัดท้ายที่เก็บติดไปกับสถานะ crash loop
	maxStatusMessage  = 2000 // กัน message ยาวจนตาราง services บวม
	maxStatusReason   = 60   // ต้องเท่ากับความกว้างคอลัมน์ status_reason
)

// NewKubernetesProvisioner ต่อคลัสเตอร์แล้วยืนยันว่าคุยกันรู้เรื่องก่อนคืน — ต่อไม่ได้ต้องรู้ตั้งแต่
// start ไม่ใช่ตอนมีคนกด deploy ครั้งแรก
//
// kubeConfig ว่าง = รันอยู่ใน cluster เอง (ใช้ ServiceAccount ของ pod)
func NewKubernetesProvisioner(kubeConfig string) (*KubernetesProvisioner, error) {
	var (
		cfg *rest.Config
		err error
	)
	if kubeConfig == "" {
		cfg, err = rest.InClusterConfig()
	} else {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("โหลด kubeconfig ไม่ได้: %w", err)
	}
	// ServiceHealthMonitor ยิงถามหลาย service ต่อรอบ — ค่า default (5 req/s) จะโดน throttle
	cfg.QPS = 20
	cfg.Burst = 40

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("สร้าง client ไม่ได้: %w", err)
	}
	ver, err := cs.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("ต่อ Kubernetes API ไม่ได้: %w", err)
	}
	log.Printf("kubernetes provisioner: ต่อคลัสเตอร์ได้ (server %s)", ver.GitVersion)
	return &KubernetesProvisioner{client: cs}, nil
}

func applyOpts() metav1.ApplyOptions {
	return metav1.ApplyOptions{FieldManager: k8sFieldManager, Force: true}
}

// EnsureNamespace สร้างหรืออัปเดต Namespace + ResourceQuota ให้ตรงกับ limit ใน DB
// เรียกได้ทั้งตอนสร้างใหม่และตอน admin ปรับโควตา — ResourceQuota คือด่านสุดท้ายของโควตา
// ต่อให้ backend คิดเลขพลาด k8s ก็ยังไม่ให้เกิน และการใส่ limits.* ไว้ด้วยทำให้ pod ที่ไม่ระบุ
// เพดานของตัวเองถูกปฏิเสธไปเลย
func (k *KubernetesProvisioner) EnsureNamespace(ctx context.Context, ns *entity.Namespace) error {
	nsAC := corev1ac.Namespace(ns.Name).
		WithLabels(map[string]string{"caesar-cluster.io/managed": "true"})
	if _, err := k.client.CoreV1().Namespaces().Apply(ctx, nsAC, applyOpts()); err != nil {
		return fmt.Errorf("สร้าง namespace %s: %w", ns.Name, err)
	}

	hard := corev1.ResourceList{
		corev1.ResourceRequestsCPU:     milliCPU(ns.CPULimitMilli),
		corev1.ResourceLimitsCPU:       milliCPU(ns.CPULimitMilli),
		corev1.ResourceRequestsMemory:  mebibytes(ns.RAMLimitMB),
		corev1.ResourceLimitsMemory:    mebibytes(ns.RAMLimitMB),
		corev1.ResourceRequestsStorage: mebibytes(ns.StorageLimitMB),
	}
	quota := corev1ac.ResourceQuota(k8sQuotaName, ns.Name).
		WithSpec(corev1ac.ResourceQuotaSpec().WithHard(hard))
	if _, err := k.client.CoreV1().ResourceQuotas(ns.Name).Apply(ctx, quota, applyOpts()); err != nil {
		return fmt.Errorf("ตั้ง ResourceQuota ของ %s: %w", ns.Name, err)
	}
	return nil
}

// DeleteNamespace ลบ namespace ทิ้งทั้งก้อน — workload, PVC, NetworkPolicy ข้างในหายตามหมด
// NotFound = สำเร็จ ตามสัญญา idempotent ใน Provisioner (สั่งลบซ้ำต้องเดินจนจบได้)
func (k *KubernetesProvisioner) DeleteNamespace(ctx context.Context, nsName string) error {
	err := k.client.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("ลบ namespace %s: %w", nsName, err)
	}
	return nil
}

// DeployService สร้าง workload จริงตาม svc.IsDatabase (ดูสัญญาใน Provisioner)
//
// พลาดกลางทางแล้วต้องถอนของที่สร้างไปแล้วออกก่อนคืน error เพราะ ServiceManager กำลังจะลบแถว
// ใน DB อยู่แล้ว — ไม่ถอนก็จะเหลือ workload ผีที่กินทรัพยากรโดยไม่มีใครเห็นในระบบ
func (k *KubernetesProvisioner) DeployService(ctx context.Context, nsName string, svc *entity.Service) error {
	var err error
	if svc.IsDatabase {
		err = k.deployDatabase(ctx, nsName, svc)
	} else {
		err = k.deployApp(ctx, nsName, svc)
	}
	if err == nil {
		return nil
	}
	// WithoutCancel: ผู้ใช้ปิดหน้าเว็บคือสาเหตุยอดฮิตที่ deploy ล้ม — การเก็บกวาดต้องเดินต่อได้
	if cleanupErr := k.DeleteService(context.WithoutCancel(ctx), nsName, svc); cleanupErr != nil {
		log.Printf("!! deploy '%s' ใน %s ล้มเหลว (%v) และถอนของที่สร้างค้างไว้ไม่สำเร็จด้วย: %v — ต้องลบมือ",
			svc.Name, nsName, err, cleanupErr)
	}
	return err
}

// deployApp = Deployment + Service ชนิด NodePort แล้วอ่านพอร์ตที่ k8s จ่ายให้กลับมาใส่ svc.NodePort
func (k *KubernetesProvisioner) deployApp(ctx context.Context, nsName string, svc *entity.Service) error {
	labels := labelsFor(svc)
	dep := appsv1ac.Deployment(svc.Name, nsName).WithLabels(labels).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(int32(svc.Replicas)).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(podTemplate(svc, labels, nil)))
	if _, err := k.client.AppsV1().Deployments(nsName).Apply(ctx, dep, applyOpts()); err != nil {
		return fmt.Errorf("สร้าง Deployment: %w", err)
	}

	result, err := k.client.CoreV1().Services(nsName).
		Apply(ctx, serviceFor(svc, nsName, corev1.ServiceTypeNodePort), applyOpts())
	if err != nil {
		return fmt.Errorf("สร้าง Service: %w", err)
	}
	// ไม่ได้ระบุ nodePort ไป k8s จึงเป็นคนสุ่มจ่าย (30000-32767) และคงเลขเดิมไว้ถ้า apply ซ้ำ
	port := assignedNodePort(result)
	if port == 0 {
		return errors.New("คลัสเตอร์ไม่ได้จ่าย NodePort กลับมา")
	}
	svc.NodePort = &port
	return nil
}

// deployDatabase = StatefulSet + PVC + Service ชนิด ClusterIP + NetworkPolicy
// ห้ามแตะ svc.NodePort — ปล่อย nil ไว้ นี่คือชั้นแรกของ "เข้าถึงได้เฉพาะใน namespace"
func (k *KubernetesProvisioner) deployDatabase(ctx context.Context, nsName string, svc *entity.Service) error {
	labels := labelsFor(svc)
	mount := corev1ac.VolumeMount().WithName(k8sVolumeName).WithMountPath(svc.DataPath)

	// ใช้ struct เปล่าแทน corev1ac.PersistentVolumeClaim() เพราะตัวนั้นใส่ kind/apiVersion/namespace
	// มาให้ด้วย ซึ่งไม่ใช่ที่ของมันใน volumeClaimTemplates
	pvc := (&corev1ac.PersistentVolumeClaimApplyConfiguration{}).
		WithName(k8sVolumeName).
		WithSpec(corev1ac.PersistentVolumeClaimSpec().
			WithAccessModes(corev1.ReadWriteOnce).
			WithResources(corev1ac.VolumeResourceRequirements().
				WithRequests(corev1.ResourceList{corev1.ResourceStorage: mebibytes(svc.StorageMB)})))

	sts := appsv1ac.StatefulSet(svc.Name, nsName).WithLabels(labels).
		WithSpec(appsv1ac.StatefulSetSpec().
			WithReplicas(int32(entity.DatabaseReplicas)).
			WithServiceName(svc.Name).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(podTemplate(svc, labels, mount)).
			WithVolumeClaimTemplates(pvc).
			// ลบ StatefulSet แล้วให้ k8s ลบ PVC ตาม (k8s 1.27+) — DeleteService ยังตามไปลบเองอีกชั้น
			// เผื่อคลัสเตอร์เก่าที่ยังไม่รู้จัก field นี้
			WithPersistentVolumeClaimRetentionPolicy(appsv1ac.StatefulSetPersistentVolumeClaimRetentionPolicy().
				WithWhenDeleted(appsv1.DeletePersistentVolumeClaimRetentionPolicyType).
				WithWhenScaled(appsv1.RetainPersistentVolumeClaimRetentionPolicyType)))
	if _, err := k.client.AppsV1().StatefulSets(nsName).Apply(ctx, sts, applyOpts()); err != nil {
		return fmt.Errorf("สร้าง StatefulSet: %w", err)
	}

	if _, err := k.client.CoreV1().Services(nsName).
		Apply(ctx, serviceFor(svc, nsName, corev1.ServiceTypeClusterIP), applyOpts()); err != nil {
		return fmt.Errorf("สร้าง Service: %w", err)
	}

	if _, err := k.client.NetworkingV1().NetworkPolicies(nsName).
		Apply(ctx, networkPolicyFor(svc, nsName), applyOpts()); err != nil {
		return fmt.Errorf("สร้าง NetworkPolicy: %w", err)
	}
	return nil
}

// ScaleService แก้เฉพาะ spec.replicas ของ Deployment — ไม่แตะ Service/NodePort ที่จ่ายไปแล้ว
func (k *KubernetesProvisioner) ScaleService(ctx context.Context, nsName, svcName string, replicas int) error {
	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := k.client.AppsV1().Deployments(nsName).
		Patch(ctx, svcName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("scale Deployment %s: %w", svcName, err)
	}
	return nil
}

// DeleteService ถอนทุกชิ้นของ service ตัวเดียว — NotFound ทุกชิ้นถือว่าสำเร็จ (idempotent)
//
// database ต้องลบ PVC เองด้วย: k8s ไม่ลบ PVC ที่เกิดจาก volumeClaimTemplates ให้ตอนลบ StatefulSet
// (ยกเว้นตั้ง retention policy ซึ่งคลัสเตอร์เก่าไม่รู้จัก) ไม่ลบแล้วดิสก์จะถูกจองค้างกินโควตาไปเรื่อยๆ
func (k *KubernetesProvisioner) DeleteService(ctx context.Context, nsName string, svc *entity.Service) error {
	propagation := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &propagation}

	var errs []error
	collect := func(what string, err error) {
		if err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("ลบ %s: %w", what, err))
		}
	}

	if svc.IsDatabase {
		collect("StatefulSet", k.client.AppsV1().StatefulSets(nsName).Delete(ctx, svc.Name, opts))
		collect("NetworkPolicy", k.client.NetworkingV1().NetworkPolicies(nsName).
			Delete(ctx, networkPolicyName(svc.Name), opts))
		// PVC มี finalizer กันลบระหว่าง pod ยังใช้อยู่ — คำสั่งนี้จองการลบไว้ ดิสก์หายจริงหลัง pod ตายสนิท
		collect("PVC", k.client.CoreV1().PersistentVolumeClaims(nsName).Delete(ctx, pvcName(svc.Name), opts))
	} else {
		collect("Deployment", k.client.AppsV1().Deployments(nsName).Delete(ctx, svc.Name, opts))
	}
	collect("Service", k.client.CoreV1().Services(nsName).Delete(ctx, svc.Name, opts))
	return errors.Join(errs...)
}

// Status ถามคลัสเตอร์ว่า workload นี้เป็นยังไงจริงๆ (ดูสัญญาใน Provisioner)
//
// ดูตามลำดับ: object หลักยังอยู่ไหม → มี pod ไหม → pod อยู่ในสภาพไหน
// หลาย pod เอาตัวที่แย่ที่สุดเป็นคำตอบ เพราะ "1 ใน 3 ตายซ้ำๆ" ต้องไม่ถูกกลบด้วยอีกสองตัวที่ยังดี
func (k *KubernetesProvisioner) Status(ctx context.Context, nsName string, svc *entity.Service) (WorkloadStatus, error) {
	gone := func(kind string) WorkloadStatus {
		return WorkloadStatus{Phase: PhaseGone, Reason: "NotFound",
			Message: fmt.Sprintf("ไม่พบ %s '%s' ใน namespace %s แล้ว — อาจถูกลบจากนอกระบบ", kind, svc.Name, nsName)}
	}

	// ข้อความจากตัวคุม replica ตอนสร้าง pod ไม่ได้เลย (เช่น ชน ResourceQuota) — Deployment
	// รายงานผ่าน condition ReplicaFailure ส่วน StatefulSet ไม่มี condition แบบนี้ให้
	var replicaFailure string
	if svc.IsDatabase {
		_, err := k.client.AppsV1().StatefulSets(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return gone("StatefulSet"), nil
		}
		if err != nil {
			return WorkloadStatus{}, err
		}
	} else {
		dep, err := k.client.AppsV1().Deployments(nsName).Get(ctx, svc.Name, metav1.GetOptions{})
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

	pods, err := k.client.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{LabelSelector: podSelector(svc.Name)})
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
		if tail := k.crashLogs(ctx, nsName, worstPod); tail != "" {
			worst.Message = strings.TrimSpace(worst.Message) + "\n\nlog ท้ายๆ ก่อน container ตาย:\n" + tail
		}
	}
	worst.Reason = clip(worst.Reason, maxStatusReason)
	worst.Message = clip(worst.Message, maxStatusMessage)
	return worst, nil
}

// Logs เปิด stream log จาก pod ของ service นี้ — มีหลาย pod เอาตัวที่ใหม่สุดที่กำลังรันอยู่
//
// container ที่ติด CrashLoopBackOff ไม่มี log ของรอบปัจจุบันเพราะยังไม่ได้เริ่ม ต้องขอรอบก่อน
// (Previous) ไม่งั้นผู้ใช้เปิดหน้า Logs แล้วเห็นหน้าว่างทั้งที่ container ตายไปแล้วหลายรอบ
func (k *KubernetesProvisioner) Logs(ctx context.Context, nsName, svcName string, opts LogOptions) (io.ReadCloser, error) {
	pods, err := k.client.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{LabelSelector: podSelector(svcName)})
	if err != nil {
		return nil, err
	}
	pod := pickLogPod(livePods(pods.Items))
	if pod == nil {
		return nil, fmt.Errorf("service '%s' ยังไม่มี pod บนคลัสเตอร์ จึงยังไม่มี log ให้ดู", svcName)
	}

	logOpts := &corev1.PodLogOptions{
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Previous:   inCrashLoop(pod),
	}
	if opts.TailLines > 0 {
		logOpts.TailLines = &opts.TailLines
	}
	if opts.SinceSeconds > 0 {
		logOpts.SinceSeconds = &opts.SinceSeconds
	}
	return k.client.CoreV1().Pods(nsName).GetLogs(pod.Name, logOpts).Stream(ctx)
}

// ── ตัวช่วยประกอบ manifest ──────────────────────────────────────────────────────────

func labelsFor(svc *entity.Service) map[string]string {
	return map[string]string{labelApp: svc.Name}
}

func podSelector(svcName string) string       { return labelApp + "=" + svcName }
func networkPolicyName(svcName string) string { return svcName + "-namespace-only" }
func pvcName(svcName string) string           { return k8sVolumeName + "-" + svcName + "-0" }

func milliCPU(m int) resource.Quantity {
	return *resource.NewMilliQuantity(int64(m), resource.DecimalSI)
}

func mebibytes(mb int) resource.Quantity {
	return *resource.NewQuantity(int64(mb)*1024*1024, resource.BinarySI)
}

// podTemplate ประกอบ pod 1 container — ใช้ร่วมกันทั้ง Deployment และ StatefulSet
//
// requests = limits เพื่อให้ที่จองในโควตาเท่ากับที่ใช้ได้จริง ตรงกับตัวเลขที่ผู้ใช้เห็น ไม่มี overcommit
// readiness probe แบบ TCP ทำให้ Status แยก "รันอยู่" ออกจาก "โปรเซสขึ้นแต่ยังไม่ฟังพอร์ต" ได้
// โดยไม่ต้องรู้จัก image
func podTemplate(svc *entity.Service, labels map[string]string, mount *corev1ac.VolumeMountApplyConfiguration) *corev1ac.PodTemplateSpecApplyConfiguration {
	port := int32(svc.ContainerPort)
	res := corev1.ResourceList{
		corev1.ResourceCPU:    milliCPU(svc.CPUMilli),
		corev1.ResourceMemory: mebibytes(svc.RAMMB),
	}
	container := corev1ac.Container().
		WithName(svc.Name).
		WithImage(svc.Image).
		WithPorts(corev1ac.ContainerPort().WithName("main").WithContainerPort(port).WithProtocol(corev1.ProtocolTCP)).
		WithEnv(envFor(svc.EnvVars)...).
		WithResources(corev1ac.ResourceRequirements().WithRequests(res).WithLimits(res)).
		WithReadinessProbe(corev1ac.Probe().
			WithTCPSocket(corev1ac.TCPSocketAction().WithPort(intstr.FromInt32(port))).
			WithInitialDelaySeconds(5).
			WithPeriodSeconds(10).
			WithFailureThreshold(3))
	if mount != nil {
		container = container.WithVolumeMounts(mount)
	}
	return corev1ac.PodTemplateSpec().
		WithLabels(labels).
		WithSpec(corev1ac.PodSpec().WithContainers(container))
}

// envFor แปลง map เป็น env ของ container — เรียงชื่อให้ manifest นิ่ง (apply ซ้ำแล้วไม่ถือว่าเปลี่ยน)
func envFor(vars entity.EnvVarMap) []*corev1ac.EnvVarApplyConfiguration {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*corev1ac.EnvVarApplyConfiguration, 0, len(keys))
	for _, k := range keys {
		out = append(out, corev1ac.EnvVar().WithName(k).WithValue(vars[k]))
	}
	return out
}

// serviceFor = k8s Service ที่ชี้เข้า pod ของ service นี้ — targetPort ต้องเท่ากับ ContainerPort เสมอ
// ตั้งผิดแล้วสร้างสำเร็จแต่ traffic เข้าไปไม่มีใครฟัง (connection refused ที่ debug ยากเพราะ deploy "ผ่าน")
func serviceFor(svc *entity.Service, nsName string, typ corev1.ServiceType) *corev1ac.ServiceApplyConfiguration {
	port := int32(svc.ContainerPort)
	return corev1ac.Service(svc.Name, nsName).WithLabels(labelsFor(svc)).
		WithSpec(corev1ac.ServiceSpec().
			WithType(typ).
			WithSelector(labelsFor(svc)).
			WithPorts(corev1ac.ServicePort().
				WithName("main").
				WithProtocol(corev1.ProtocolTCP).
				WithPort(port).
				WithTargetPort(intstr.FromInt32(port))))
}

// networkPolicyFor = รับ ingress เข้า pod ของ database ได้เฉพาะจาก pod ใน namespace เดียวกัน
//
// from: [{podSelector: {}}] แปลว่า pod ทุกตัวใน namespace เดียวกับ policy ซึ่งคือที่ต้องการ
// ห้ามเผลอเขียนเป็น namespaceSelector: {} ที่แปลว่าทุก namespace = เปิดให้ทั้งคลัสเตอร์
func networkPolicyFor(svc *entity.Service, nsName string) *netv1ac.NetworkPolicyApplyConfiguration {
	return netv1ac.NetworkPolicy(networkPolicyName(svc.Name), nsName).WithLabels(labelsFor(svc)).
		WithSpec(netv1ac.NetworkPolicySpec().
			WithPodSelector(metav1ac.LabelSelector().WithMatchLabels(labelsFor(svc))).
			WithPolicyTypes(netv1.PolicyTypeIngress).
			WithIngress(netv1ac.NetworkPolicyIngressRule().
				WithFrom(netv1ac.NetworkPolicyPeer().WithPodSelector(metav1ac.LabelSelector())).
				WithPorts(netv1ac.NetworkPolicyPort().
					WithProtocol(corev1.ProtocolTCP).
					WithPort(intstr.FromInt32(int32(svc.ContainerPort))))))
}

func assignedNodePort(s *corev1.Service) int {
	for _, p := range s.Spec.Ports {
		if p.NodePort != 0 {
			return int(p.NodePort)
		}
	}
	return 0
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
func (k *KubernetesProvisioner) crashLogs(ctx context.Context, nsName, podName string) string {
	tail := int64(crashLogTailLines)
	for _, previous := range []bool{true, false} {
		stream, err := k.client.CoreV1().Pods(nsName).
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

// pickLogPod เลือก pod ที่ใหม่สุดในกลุ่มที่กำลังรัน — ไม่มีตัวรันก็เอาตัวใหม่สุดที่มี
func pickLogPod(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}
	sort.SliceStable(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.After(pods[j].CreationTimestamp.Time)
	})
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning {
			return &pods[i]
		}
	}
	return &pods[0]
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
