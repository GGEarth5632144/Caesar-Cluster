package services

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"backend/internal/entity"
)

// newFakeProvisioner ประกอบ KubernetesProvisioner ที่ยิงใส่คลัสเตอร์ปลอมของ client-go
//
// once.Do(ไม่ทำอะไร) คือการ "ปิดสวิตช์" ตัวสร้าง clientset ของจริงทิ้ง — พอ sync.Once ถูกใช้ไปแล้ว
// client() จะข้ามการอ่าน kubeconfig แล้วคืน clientset ที่เรายัดไว้แทน ทำให้เทสต์ทั้งไฟล์นี้
// เดินผ่าน EnsureNamespace ตัวจริงได้โดยไม่ต้องมีคลัสเตอร์
func newFakeProvisioner() (*KubernetesProvisioner, kubernetes.Interface) {
	cs := fake.NewClientset()
	k := &KubernetesProvisioner{clientset: cs}
	k.once.Do(func() {})
	return k, cs
}

func testNamespace() *entity.Namespace {
	return &entity.Namespace{
		ID:            7,
		Name:          "สเปซของเอิร์ธ", // ตั้งใจใช้ภาษาไทย: ชื่อแบบนี้เป็นชื่อ namespace ของ k8s ไม่ได้
		ContributorID: 42,
		CPULimitMilli: entity.DefaultCPULimitMilli,
		RAMLimitMB:    entity.DefaultRAMLimitMB,
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

// TestLogsMergesPodsOfService ยืนยันว่าอ่าน log ครบทุก replica ของ service (และไม่ปน Pod ของ service อื่น)
// โดยแต่ละบรรทัดถูกแปะชื่อ pod ไว้ — ส่วนไม่มี Pod เลยต้องได้ ErrLogsUnavailable ให้ controller ตอบ 409
func TestLogsMergesPodsOfService(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()

	if _, err := k.Logs(ctx, "ns-7", "web", LogOptions{}); !errors.Is(err, ErrLogsUnavailable) {
		t.Fatalf("ไม่มี Pod ต้องได้ ErrLogsUnavailable ได้: %v", err)
	}

	for name, svc := range map[string]string{"web-a": "web", "web-b": "web", "db-a": "db"} {
		_, err := cs.CoreV1().Pods("ns-7").Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "ns-7", Labels: map[string]string{labelServiceName: svc},
		}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("เตรียม Pod ไม่สำเร็จ: %v", err)
		}
	}

	stream, err := k.Logs(ctx, "ns-7", "web", LogOptions{Timestamps: true})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer stream.Close()
	out, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("อ่าน stream: %v", err)
	}

	got := string(out)
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "[web-a]") || !strings.Contains(got, "[web-b]") {
		t.Errorf("ต้องได้ 2 บรรทัดจาก web-a กับ web-b ได้ %q", got)
	}
	if strings.Contains(got, "[db-a]") {
		t.Errorf("ต้องไม่มี log ของ service อื่นปน ได้ %q", got)
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
