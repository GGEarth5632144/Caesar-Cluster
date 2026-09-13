package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"backend/internal/entity"
)

// newFakeProvisioner ประกอบ KubernetesProvisioner ที่ยิงใส่คลัสเตอร์ปลอมของ client-go
//
// once.Do(ไม่ทำอะไร) คือการ "ปิดสวิตช์" ตัวสร้าง clientset ของจริงทิ้ง — พอ sync.Once ถูกใช้ไปแล้ว
// client() จะข้ามการอ่าน kubeconfig แล้วคืน clientset ที่เรายัดไว้แทน ทำให้เทสต์ทั้งไฟล์นี้
// เดินผ่าน method ตัวจริงได้โดยไม่ต้องมีคลัสเตอร์
//
// fake API ไม่มี controller (ไม่มีใครสร้าง pod จาก Deployment หรือจ่าย NodePort ให้) — จึงตรวจได้ว่า
// "object ที่ส่งไปถูกไหม" กับ "อ่านสภาพ pod กลับมาแปลถูกไหม" แต่ยืนยันไม่ได้ว่า container ขึ้นจริง
func newFakeProvisioner(objects ...runtime.Object) (*KubernetesProvisioner, *fake.Clientset) {
	cs := fake.NewClientset(objects...)
	k := &KubernetesProvisioner{clientset: cs}
	k.once.Do(func() {})
	return k, cs
}

func testNamespace() *entity.Namespace {
	return &entity.Namespace{
		ID:             7,
		Name:           "สเปซของเอิร์ธ", // ตั้งใจใช้ภาษาไทย: ชื่อแบบนี้เป็นชื่อ namespace ของ k8s ไม่ได้
		ContributorID:  42,
		CPULimitMilli:  entity.DefaultCPULimitMilli,
		RAMLimitMB:     entity.DefaultRAMLimitMB,
		StorageLimitMB: entity.DefaultStorageLimitMB,
	}
}

// TestEnsureNamespaceCreatesQuotaAndLimits ยืนยันว่าเรียกครั้งเดียวได้ครบทั้ง 3 อย่าง
// และชื่อ namespace มาจาก id ไม่ใช่ชื่อที่ผู้ใช้ตั้ง (ซึ่งในเทสต์นี้เป็นภาษาไทย ใช้ไม่ได้แน่ๆ)
func TestEnsureNamespaceCreatesQuotaAndLimits(t *testing.T) {
	k, cs := newFakeProvisioner()
	ns := testNamespace()
	ctx := context.Background()

	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}

	name := K8sNamespaceName(ns.ID)
	if name != "ns-7" {
		t.Fatalf("ชื่อ namespace บนคลัสเตอร์ต้องมาจาก id ได้ %q", name)
	}

	got, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา namespace ที่เพิ่งสร้างไม่เจอ: %v", err)
	}
	if got.Labels[labelNamespaceID] != "7" {
		t.Errorf("label %s = %q ต้องเป็น \"7\"", labelNamespaceID, got.Labels[labelNamespaceID])
	}
	// ชื่อที่ผู้ใช้ตั้งต้องไม่หายไปไหน — เก็บเป็น annotation ให้ยังสาวกลับได้ว่า ns-7 คือ space ไหน
	if got.Annotations[annDisplayName] != ns.Name {
		t.Errorf("annotation %s = %q ต้องเป็น %q", annDisplayName, got.Annotations[annDisplayName], ns.Name)
	}

	quota, err := cs.CoreV1().ResourceQuotas(name).Get(ctx, quotaObjectName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา ResourceQuota ไม่เจอ: %v", err)
	}
	if cpu := quota.Spec.Hard[corev1.ResourceRequestsCPU]; cpu.MilliValue() != int64(ns.CPULimitMilli) {
		t.Errorf("requests.cpu = %s ต้องเป็น %dm", cpu.String(), ns.CPULimitMilli)
	}
	// limits.* ต้องมีและเท่ากับ requests.* ไม่งั้น container จะ burst เกินโควตาได้ตอนคลัสเตอร์ว่าง
	if cpu := quota.Spec.Hard[corev1.ResourceLimitsCPU]; cpu.MilliValue() != int64(ns.CPULimitMilli) {
		t.Errorf("limits.cpu = %s ต้องเท่ากับ requests.cpu (%dm)", cpu.String(), ns.CPULimitMilli)
	}
	wantMem := int64(ns.RAMLimitMB) * 1024 * 1024
	if mem := quota.Spec.Hard[corev1.ResourceRequestsMemory]; mem.Value() != wantMem {
		t.Errorf("requests.memory = %s ต้องเป็น %d bytes", mem.String(), wantMem)
	}
	wantDisk := int64(ns.StorageLimitMB) * 1024 * 1024
	if disk := quota.Spec.Hard[corev1.ResourceRequestsStorage]; disk.Value() != wantDisk {
		t.Errorf("requests.storage = %s ต้องเป็น %d bytes", disk.String(), wantDisk)
	}

	limits, err := cs.CoreV1().LimitRanges(name).Get(ctx, limitsObjectName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา LimitRange ไม่เจอ: %v", err)
	}
	if len(limits.Spec.Limits) != 1 {
		t.Fatalf("LimitRange ต้องมี 1 รายการ ได้ %d", len(limits.Spec.Limits))
	}
	item := limits.Spec.Limits[0]
	// เพดานต่อ container ต้องตรงกับที่ QuotaService.ReserveAndInsert เช็คไว้ ไม่งั้นสองชั้นขัดกันเอง
	if cpu := item.Max[corev1.ResourceCPU]; cpu.MilliValue() != int64(entity.MaxCPUMilliPerService) {
		t.Errorf("LimitRange max cpu = %s ต้องเป็น %dm", cpu.String(), entity.MaxCPUMilliPerService)
	}
	def, defReq := item.Default[corev1.ResourceCPU], item.DefaultRequest[corev1.ResourceCPU]
	if def.IsZero() || defReq.IsZero() {
		t.Error("ต้องมีทั้ง default และ defaultRequest ไม่งั้น container ที่ไม่ระบุ resource จะโดน ResourceQuota ปฏิเสธ")
	}
}

// TestEnsureNamespaceIdempotentAndUpdatesQuota จำลองเส้นทางของ NamespaceManager.SetQuota:
// เรียก EnsureNamespace ซ้ำกับ namespace เดิมที่โควตาเปลี่ยน ต้องไม่ error และค่าใหม่ต้องถูกทับลงไป
func TestEnsureNamespaceIdempotentAndUpdatesQuota(t *testing.T) {
	k, cs := newFakeProvisioner()
	ns := testNamespace()
	ctx := context.Background()

	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("เรียกครั้งแรก: %v", err)
	}

	ns.CPULimitMilli = entity.MaxCPULimitMilli
	ns.RAMLimitMB = entity.MaxRAMLimitMB
	ns.Name = "ชื่อใหม่"
	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("เรียกซ้ำต้องผ่าน (SetQuota เรียกทุกครั้งที่แอดมินปรับโควตา): %v", err)
	}

	name := K8sNamespaceName(ns.ID)
	quota, err := cs.CoreV1().ResourceQuotas(name).Get(ctx, quotaObjectName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา ResourceQuota ไม่เจอ: %v", err)
	}
	if cpu := quota.Spec.Hard[corev1.ResourceRequestsCPU]; cpu.MilliValue() != int64(entity.MaxCPULimitMilli) {
		t.Errorf("requests.cpu = %s ต้องถูกอัปเดตเป็น %dm", cpu.String(), entity.MaxCPULimitMilli)
	}

	got, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา namespace ไม่เจอ: %v", err)
	}
	if got.Annotations[annDisplayName] != "ชื่อใหม่" {
		t.Errorf("annotation ชื่อ space = %q ต้อง sync ตาม DB เป็น %q", got.Annotations[annDisplayName], "ชื่อใหม่")
	}
}

// TestEnsureNamespaceTerminating ยืนยันว่า namespace เดิมที่ยังลบไม่เสร็จให้ error แบบ "รอแล้วลองใหม่"
// ไม่ใช่ error ทั่วไป — controller พึ่ง errors.Is ตัวนี้ในการตอบ 409 พร้อมคำแนะนำที่ถูก
func TestEnsureNamespaceTerminating(t *testing.T) {
	k, cs := newFakeProvisioner()
	ns := testNamespace()
	ctx := context.Background()

	_, err := cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: K8sNamespaceName(ns.ID)},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("เตรียม namespace ที่กำลังถูกลบไม่สำเร็จ: %v", err)
	}

	if err := k.EnsureNamespace(ctx, ns); !errors.Is(err, ErrNamespaceTerminating) {
		t.Fatalf("ต้องได้ ErrNamespaceTerminating ได้: %v", err)
	}
}

// TestScaleServiceOnlyTouchesReplicas ยืนยันว่า scale เปลี่ยนแค่ replicas (pod template เดิม = ไม่เกิด rollout)
// และ Deployment ที่ไม่มีอยู่ต้องได้ error — ServiceManager.Scale พึ่ง error นี้ในการย้อนโควตาใน DB กลับ
func TestScaleServiceOnlyTouchesReplicas(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	svc := &entity.Service{Name: "web", Image: "nginx", ContainerPort: 80, Replicas: 1, CPUMilli: 100, RAMMB: 128,
		EnvVars: map[string]string{"A": "1"}}

	if _, err := cs.AppsV1().Deployments("ns-7").Create(ctx, deploymentFor("ns-7", svc), metav1.CreateOptions{}); err != nil {
		t.Fatalf("เตรียม Deployment ไม่สำเร็จ: %v", err)
	}
	if err := k.ScaleService(ctx, "ns-7", "web", 3); err != nil {
		t.Fatalf("ScaleService: %v", err)
	}

	got, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("หา Deployment ไม่เจอ: %v", err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != 3 {
		t.Errorf("replicas = %v ต้องเป็น 3", got.Spec.Replicas)
	}
	c := got.Spec.Template.Spec.Containers
	if len(c) != 1 || c[0].Image != "nginx" || len(c[0].Env) != 1 {
		t.Errorf("pod template ต้องไม่เปลี่ยน ได้ %+v", c)
	}

	if err := k.ScaleService(ctx, "ns-7", "ghost", 2); err == nil {
		t.Error("scale Deployment ที่ไม่มีอยู่ต้องได้ error")
	}
}

// TestDeleteNamespaceIdempotent ยืนยันสัญญาใน Provisioner: ลบของที่ไม่มีอยู่แล้วต้องคืน nil
// NamespaceManager.Delete ถอนของบนคลัสเตอร์ก่อนแล้วค่อยลบแถวใน DB — ถ้าล้มกลางคัน
// การสั่งลบซ้ำต้องเดินจนจบได้ ไม่งั้น namespace นั้นค้างใน DB ตลอดกาล
func TestDeleteNamespaceIdempotent(t *testing.T) {
	k, cs := newFakeProvisioner()
	ns := testNamespace()
	ctx := context.Background()
	name := K8sNamespaceName(ns.ID)

	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	if err := k.DeleteNamespace(ctx, name); err != nil {
		t.Fatalf("ลบครั้งแรก: %v", err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{}); err == nil {
		t.Fatal("namespace ต้องหายไปแล้ว")
	}
	if err := k.DeleteNamespace(ctx, name); err != nil {
		t.Fatalf("ลบซ้ำต้องคืน nil ไม่ใช่ error: %v", err)
	}
}

func testDatabaseSvc() *entity.Service {
	return &entity.Service{
		ID: 5, Name: "my-pg", Image: "postgres:16",
		CPUMilli: 500, RAMMB: 512, ContainerPort: 5432, Replicas: 1,
		IsDatabase: true, StorageMB: 2048, DataPath: "/var/lib/postgresql/data",
		EnvVars: entity.EnvVarMap{"POSTGRES_PASSWORD": "s3cret", "POSTGRES_DB": "app"},
	}
}

func testAppSvc() *entity.Service {
	return &entity.Service{
		ID: 6, Name: "web", Image: "nginx:1.27-alpine",
		CPUMilli: 300, RAMMB: 256, ContainerPort: 8080, Replicas: 2,
	}
}

// TestK8sDeployDatabaseIsClosedAndPersistent — สวิตช์ database ต้องออกมาเป็นของ 3 ชิ้นที่ถูกต้อง
// ทุกชิ้น: StatefulSet ที่มีดิสก์ mount ตรงจุด, Service ที่ไม่มี NodePort, NetworkPolicy ที่รับเฉพาะ
// namespace ตัวเอง — ข้อไหนหลุดอาการจะต่างกันคนละแบบและทุกแบบ "ดูเหมือนสำเร็จ"
func TestK8sDeployDatabaseIsClosedAndPersistent(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	svc := testDatabaseSvc()

	if err := k.DeployService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่ได้ NodePort แต่ได้ %d", *svc.NodePort)
	}

	sts, err := cs.AppsV1().StatefulSets("ns-7").Get(ctx, "my-pg", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("StatefulSet ต้องถูกสร้าง: %v", err)
	}
	if got := *sts.Spec.Replicas; got != 1 {
		t.Errorf("replicas = %d ต้องเป็น 1", got)
	}
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("ต้องมี volumeClaimTemplate 1 อัน ได้ %d", len(sts.Spec.VolumeClaimTemplates))
	}
	disk := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	if disk.Value() != 2048<<20 {
		t.Errorf("ขนาดดิสก์ = %s ต้องเป็น 2Gi", disk.String())
	}
	c := sts.Spec.Template.Spec.Containers[0]
	if len(c.VolumeMounts) != 1 || c.VolumeMounts[0].MountPath != "/var/lib/postgresql/data" {
		t.Errorf("ดิสก์ต้อง mount ที่ data_path ที่ผู้ใช้กรอก ได้ %+v", c.VolumeMounts)
	}
	if c.Image != "postgres:16" || c.Ports[0].ContainerPort != 5432 {
		t.Errorf("image/port ไม่ตรง: %s :%d", c.Image, c.Ports[0].ContainerPort)
	}
	if len(c.Env) != 2 || c.Env[0].Name != "POSTGRES_DB" || c.Env[1].Value != "s3cret" {
		t.Errorf("env ต้องถูกส่งต่อครบและเรียงชื่อ ได้ %+v", c.Env)
	}
	if cpu := c.Resources.Limits[corev1.ResourceCPU]; cpu.MilliValue() != 500 {
		t.Errorf("cpu limit = %s ต้องเป็น 500m", cpu.String())
	}
	if c.ReadinessProbe == nil || c.ReadinessProbe.TCPSocket == nil || c.ReadinessProbe.TCPSocket.Port.IntValue() != 5432 {
		t.Error("ต้องมี readiness probe แบบ TCP ที่พอร์ตของ database")
	}
	if p := sts.Spec.PersistentVolumeClaimRetentionPolicy; p == nil || p.WhenDeleted != appsv1.DeletePersistentVolumeClaimRetentionPolicyType {
		t.Error("ลบ StatefulSet แล้วต้องให้ k8s ลบ PVC ตาม")
	}

	k8sSvc, err := cs.CoreV1().Services("ns-7").Get(ctx, "my-pg", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Service ต้องถูกสร้าง: %v", err)
	}
	if k8sSvc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("Service ของ database ต้องเป็น ClusterIP ได้ %s", k8sSvc.Spec.Type)
	}
	if k8sSvc.Spec.Ports[0].TargetPort.IntValue() != 5432 {
		t.Errorf("targetPort ต้องชี้ที่ container port ได้ %v", k8sSvc.Spec.Ports[0].TargetPort)
	}

	np, err := cs.NetworkingV1().NetworkPolicies("ns-7").Get(ctx, networkPolicyName("my-pg"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy ต้องถูกสร้าง: %v", err)
	}
	if np.Spec.PodSelector.MatchLabels[labelServiceName] != "my-pg" {
		t.Errorf("policy ต้องเจาะจง pod ของ database นี้ ได้ %v", np.Spec.PodSelector)
	}
	if len(np.Spec.Ingress) != 1 || len(np.Spec.Ingress[0].From) != 1 {
		t.Fatalf("ต้องมี ingress rule 1 ข้อ from 1 แหล่ง ได้ %+v", np.Spec.Ingress)
	}
	peer := np.Spec.Ingress[0].From[0]
	// podSelector ว่าง = pod ใน namespace เดียวกัน; namespaceSelector ว่าง = ทุก namespace (ผิด)
	if peer.PodSelector == nil || peer.NamespaceSelector != nil || peer.IPBlock != nil {
		t.Errorf("ต้องอนุญาตเฉพาะ podSelector ใน namespace เดียวกัน ได้ %+v", peer)
	}
	if np.Spec.Ingress[0].Ports[0].Port.IntValue() != 5432 {
		t.Errorf("policy ต้องเปิดเฉพาะพอร์ตของ database ได้ %v", np.Spec.Ingress[0].Ports)
	}

	if _, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("database ต้องไม่สร้าง Deployment")
	}
}

// TestK8sDeployAppGetsNodePort — service ธรรมดาต้องได้ Deployment + NodePort ที่คลัสเตอร์จ่ายให้
// และต้องไม่มีของฝั่ง database ติดมา (ดิสก์ / NetworkPolicy ที่จะทำให้ NodePort เข้าไม่ได้)
func TestK8sDeployAppGetsNodePort(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	// fake API ไม่มีตัวจ่าย NodePort — เติมเลขให้ก่อน object ถูกเก็บ เหมือนที่ apiserver จริงทำ
	cs.PrependReactor("create", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
		s := action.(k8stesting.CreateAction).GetObject().(*corev1.Service)
		if s.Spec.Type == corev1.ServiceTypeNodePort {
			s.Spec.Ports[0].NodePort = 30123
		}
		return false, nil, nil
	})
	svc := testAppSvc()

	if err := k.DeployService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if svc.NodePort == nil || *svc.NodePort != 30123 {
		t.Fatalf("ต้องอ่าน NodePort ที่คลัสเตอร์จ่ายกลับมา ได้ %v", svc.NodePort)
	}

	dep, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment ต้องถูกสร้าง: %v", err)
	}
	if *dep.Spec.Replicas != 2 {
		t.Errorf("replicas = %d ต้องเป็น 2", *dep.Spec.Replicas)
	}
	if len(dep.Spec.Template.Spec.Containers[0].VolumeMounts) != 0 {
		t.Error("service ธรรมดาต้องไม่มีดิสก์")
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("ns-7").Get(ctx, networkPolicyName("web"), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("service ธรรมดาต้องไม่มี NetworkPolicy (ไม่งั้น NodePort เข้าไม่ได้)")
	}
}

// TestK8sDeployCleansUpOnFailure — พลาดกลางทางต้องไม่ทิ้งของค้างบนคลัสเตอร์
// เพราะ ServiceManager จะลบแถวใน DB ทันที ของที่เหลืออยู่จะกลายเป็น workload ผี
func TestK8sDeployCleansUpOnFailure(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	// ให้ NetworkPolicy (ชิ้นสุดท้ายของ database) ล้ม — StatefulSet กับ Service สร้างไปแล้ว
	cs.PrependReactor("create", "networkpolicies", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("webhook ปฏิเสธ")
	})

	if err := k.DeployService(ctx, "ns-7", testDatabaseSvc()); err == nil {
		t.Fatal("ต้องคืน error เมื่อสร้างไม่ครบ")
	}
	if _, err := cs.AppsV1().StatefulSets("ns-7").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("StatefulSet ที่สร้างค้างต้องถูกถอนออก")
	}
	if _, err := cs.CoreV1().Services("ns-7").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("Service ที่สร้างค้างต้องถูกถอนออก")
	}
}

// TestK8sDeleteDatabaseRemovesEverything — ลบ database ต้องเก็บ PVC ด้วย และลบซ้ำต้องไม่ error
func TestK8sDeleteDatabaseRemovesEverything(t *testing.T) {
	// PVC ที่ StatefulSet controller จะสร้างจาก volumeClaimTemplates บนคลัสเตอร์จริง
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName("my-pg"), Namespace: "ns-7"}}
	k, cs := newFakeProvisioner(pvc)
	ctx := context.Background()
	svc := testDatabaseSvc()

	if err := k.DeployService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if err := k.DeleteService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	checks := map[string]error{}
	_, checks["StatefulSet"] = cs.AppsV1().StatefulSets("ns-7").Get(ctx, "my-pg", metav1.GetOptions{})
	_, checks["Service"] = cs.CoreV1().Services("ns-7").Get(ctx, "my-pg", metav1.GetOptions{})
	_, checks["NetworkPolicy"] = cs.NetworkingV1().NetworkPolicies("ns-7").Get(ctx, networkPolicyName("my-pg"), metav1.GetOptions{})
	_, checks["PVC"] = cs.CoreV1().PersistentVolumeClaims("ns-7").Get(ctx, pvcName("my-pg"), metav1.GetOptions{})
	for what, err := range checks {
		if !apierrors.IsNotFound(err) {
			t.Errorf("%s ต้องถูกลบ แต่ยังอยู่ (err=%v)", what, err)
		}
	}

	if err := k.DeleteService(ctx, "ns-7", svc); err != nil {
		t.Errorf("ลบซ้ำต้องผ่าน (idempotent) ได้ %v", err)
	}
}

// TestK8sStatusReadsPods — ตารางแปลสภาพ pod จริงๆ ที่ kubelet รายงาน เป็นสถานะที่ระบบเก็บ
// แต่ละเคสคืออาการที่ผู้ใช้เจอบ่อยที่สุดตอน deploy ของตัวเองครั้งแรก
func TestK8sStatusReadsPods(t *testing.T) {
	ctx := context.Background()
	svc := testAppSvc()
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-7"}}
	podWith := func(status corev1.PodStatus) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "ns-7", Labels: serviceLabels("web")},
			Status:     status,
		}
	}

	cases := []struct {
		name        string
		objects     []runtime.Object
		wantPhase   WorkloadPhase
		wantReason  string
		wantRestart int
		wantInMsg   string
	}{
		{
			name:       "ไม่มี Deployment บนคลัสเตอร์",
			objects:    nil,
			wantPhase:  PhaseGone,
			wantReason: "NotFound",
		},
		{
			name:       "มี Deployment แต่ยังไม่มี pod",
			objects:    []runtime.Object{dep},
			wantPhase:  PhasePending,
			wantReason: "NoPods",
		},
		{
			name: "ตายซ้ำเพราะ env ไม่ครบ",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					RestartCount:         4,
					State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
				}},
			})},
			wantPhase: PhaseCrashLoop, wantReason: "CrashLoopBackOff", wantRestart: 4,
			wantInMsg: "exit code 1",
		},
		{
			name: "RAM ไม่พอ",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					RestartCount:         2,
					State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "OOMKilled"}},
				}},
			})},
			wantPhase: PhaseCrashLoop, wantReason: "OOMKilled", wantRestart: 2,
			wantInMsg: "OOMKilled",
		},
		{
			name: "ดึง image ไม่ได้",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff", Message: `Back-off pulling image "nginx:nope"`}},
				}},
			})},
			wantPhase: PhaseFailed, wantReason: "ImagePullBackOff", wantInMsg: "nginx:nope",
		},
		{
			name: "ไม่มี node ว่างพอ",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable",
					Message: "0/3 nodes are available: 3 Insufficient memory.",
				}},
			})},
			wantPhase: PhasePending, wantReason: "FailedScheduling", wantInMsg: "Insufficient memory",
		},
		{
			name: "รันอยู่แต่ยังไม่มีอะไรฟังพอร์ต",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Ready: false, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			})},
			wantPhase: PhasePending, wantReason: "NotReady",
		},
		{
			name: "รันอยู่และพร้อม",
			objects: []runtime.Object{dep, podWith(corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Ready: true, RestartCount: 1, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			})},
			wantPhase: PhaseRunning, wantReason: "", wantRestart: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, _ := newFakeProvisioner(tc.objects...)
			got, err := k.Status(ctx, "ns-7", svc)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if got.Phase != tc.wantPhase || got.Reason != tc.wantReason {
				t.Errorf("ได้ %s/%s ต้องเป็น %s/%s", got.Phase, got.Reason, tc.wantPhase, tc.wantReason)
			}
			if got.Restarts != tc.wantRestart {
				t.Errorf("restarts = %d ต้องเป็น %d", got.Restarts, tc.wantRestart)
			}
			if tc.wantInMsg != "" && !strings.Contains(got.Message, tc.wantInMsg) {
				t.Errorf("message ต้องมี %q ได้ %q", tc.wantInMsg, got.Message)
			}
			if tc.wantPhase != PhaseRunning && got.Reason == "" {
				t.Error("สถานะที่ไม่ใช่ running ต้องมี reason เสมอ")
			}
		})
	}
}

// TestK8sStatusAttachesCrashLogs — crash loop ต้องมี log ของรอบที่ตายแนบมาด้วย
// (fake API ตอบ "fake logs" เสมอ — ที่ตรวจคือเราขอ log และเอามาต่อท้าย message จริง)
func TestK8sStatusAttachesCrashLogs(t *testing.T) {
	ctx := context.Background()
	svc := testDatabaseSvc()
	objects := []runtime.Object{
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "my-pg", Namespace: "ns-7"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pg-0", Namespace: "ns-7", Labels: serviceLabels("my-pg")},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					RestartCount:         3,
					State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
				}},
			},
		},
	}
	k, _ := newFakeProvisioner(objects...)
	got, err := k.Status(ctx, "ns-7", svc)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Phase != PhaseCrashLoop {
		t.Fatalf("ต้องเป็น crashloop ได้ %s", got.Phase)
	}
	if !strings.Contains(got.Message, "fake logs") {
		t.Errorf("message ต้องมี log ของ container ที่ตายแนบมา ได้ %q", got.Message)
	}
}

// TestK8sLogsStreamsFromPod — Logs ต้องหา pod จาก label แล้วเปิด stream ได้; ไม่มี pod ต้องบอกชัด
func TestK8sLogsStreamsFromPod(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeProvisioner()
	if _, err := k.Logs(ctx, "ns-7", "web", LogOptions{}); err == nil {
		t.Error("ไม่มี pod ต้อง error ไม่ใช่คืน stream ว่าง")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-xyz", Namespace: "ns-7", Labels: serviceLabels("web")},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	k, _ = newFakeProvisioner(pod)
	stream, err := k.Logs(ctx, "ns-7", "web", LogOptions{TailLines: 50, Timestamps: true})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer stream.Close()
	b, _ := io.ReadAll(stream)
	if !strings.Contains(string(b), "fake logs") {
		t.Errorf("ต้องได้ log จาก pod ได้ %q", string(b))
	}
}

// TestClipRespectsColumnWidth — ผลลัพธ์ต้องไม่ยาวเกินเพดาน "เป็นตัวอักษร" และต้องไม่ตัดอักษรไทยขาดครึ่ง
//
// regression test ของบั๊กที่เจอตอนตรวจระบบ: clip เดิมนับเป็นไบต์แล้วต่อ … ทีหลัง ผลคือ
// reason ยาว 61 ตัวอักษรลงคอลัมน์ varchar(60) ไม่ได้ Postgres ตีกลับว่า "value too long"
// ซึ่งจะเกิดตอนกำลังบันทึกว่า service พังพอดี คือตอนที่ข้อมูลนี้สำคัญที่สุด
func TestClipRespectsColumnWidth(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"อังกฤษล้วน", strings.Repeat("a", 200)},
		{"ไทยล้วน", strings.Repeat("ก", 200)},
		{"ปนกัน", strings.Repeat("aก", 100)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := clip(tc.in, maxStatusReason)
			if n := utf8.RuneCountInString(got); n > maxStatusReason {
				t.Errorf("ยาว %d ตัวอักษร เกินคอลัมน์ varchar(%d) — INSERT จะถูกปฏิเสธ", n, maxStatusReason)
			}
			if !strings.HasSuffix(got, "…") {
				t.Errorf("ต้องมี … ต่อท้ายเมื่อถูกตัด ได้ %q", got)
			}
			if strings.ContainsRune(got, '�') {
				t.Error("ตัดแล้วอักษรขาดครึ่ง")
			}
		})
	}

	if clip("short", 100) != "short" {
		t.Error("ข้อความสั้นต้องไม่ถูกแตะ")
	}
	if got := clip(strings.Repeat("ก", 60), 60); utf8.RuneCountInString(got) != 60 {
		t.Errorf("ยาวพอดีเพดานต้องไม่ถูกตัด ได้ %d ตัวอักษร", utf8.RuneCountInString(got))
	}
}
