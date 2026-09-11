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

// เทสต์ชุดนี้ใช้ fake clientset ของ client-go: API server ปลอมในหน่วยความจำที่รับ apply/patch/delete
// ได้เหมือนของจริง แต่ไม่มี controller (ไม่มีใครสร้าง pod จาก Deployment ให้) — จึงตรวจได้ว่า
// "manifest ที่ส่งไปถูกไหม" กับ "อ่านสภาพ pod กลับมาแปลถูกไหม" แต่ยืนยันไม่ได้ว่า container
// จะขึ้นจริงบนคลัสเตอร์ อันนั้นต้องลองบนเครื่องจริง

func newFakeK8s(objects ...runtime.Object) (*KubernetesProvisioner, *fake.Clientset) {
	cs := fake.NewClientset(objects...)
	return &KubernetesProvisioner{client: cs}, cs
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

// TestK8sEnsureNamespaceAppliesQuota — โควตาบน DB ต้องไปโผล่เป็น ResourceQuota บนคลัสเตอร์
// และเรียกซ้ำด้วยตัวเลขใหม่ (admin ปรับโควตา) ต้องอัปเดต ไม่ใช่ล้มด้วย AlreadyExists
func TestK8sEnsureNamespaceAppliesQuota(t *testing.T) {
	k, cs := newFakeK8s()
	ctx := context.Background()
	ns := &entity.Namespace{Name: "ns-user-7", CPULimitMilli: 3000, RAMLimitMB: 2048, StorageLimitMB: 10240}

	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace ต้องถูกสร้าง: %v", err)
	}
	rq, err := cs.CoreV1().ResourceQuotas(ns.Name).Get(ctx, k8sQuotaName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("ResourceQuota ต้องถูกสร้าง: %v", err)
	}
	cpu := rq.Spec.Hard[corev1.ResourceRequestsCPU]
	mem := rq.Spec.Hard[corev1.ResourceLimitsMemory]
	disk := rq.Spec.Hard[corev1.ResourceRequestsStorage]
	if cpu.MilliValue() != 3000 || mem.Value() != 2048<<20 || disk.Value() != 10240<<20 {
		t.Errorf("โควตาไม่ตรง: cpu=%s mem=%s storage=%s", cpu.String(), mem.String(), disk.String())
	}

	ns.CPULimitMilli = 8000
	if err := k.EnsureNamespace(ctx, ns); err != nil {
		t.Fatalf("EnsureNamespace ซ้ำ (ปรับโควตา): %v", err)
	}
	rq, _ = cs.CoreV1().ResourceQuotas(ns.Name).Get(ctx, k8sQuotaName, metav1.GetOptions{})
	if cpu := rq.Spec.Hard[corev1.ResourceRequestsCPU]; cpu.MilliValue() != 8000 {
		t.Errorf("ปรับโควตาแล้วต้องได้ 8000m ได้ %s", cpu.String())
	}
}

// TestK8sDeployDatabaseIsClosedAndPersistent — สวิตช์ database ต้องออกมาเป็นของ 3 ชิ้นที่ถูกต้อง
// ทุกชิ้น: StatefulSet ที่มีดิสก์ mount ตรงจุด, Service ที่ไม่มี NodePort, NetworkPolicy ที่รับเฉพาะ
// namespace ตัวเอง — ข้อไหนหลุดอาการจะต่างกันคนละแบบและทุกแบบ "ดูเหมือนสำเร็จ"
func TestK8sDeployDatabaseIsClosedAndPersistent(t *testing.T) {
	k, cs := newFakeK8s()
	ctx := context.Background()
	svc := testDatabaseSvc()

	if err := k.DeployService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่ได้ NodePort แต่ได้ %d", *svc.NodePort)
	}

	sts, err := cs.AppsV1().StatefulSets("ns-a").Get(ctx, "my-pg", metav1.GetOptions{})
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

	k8sSvc, err := cs.CoreV1().Services("ns-a").Get(ctx, "my-pg", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Service ต้องถูกสร้าง: %v", err)
	}
	if k8sSvc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("Service ของ database ต้องเป็น ClusterIP ได้ %s", k8sSvc.Spec.Type)
	}
	if k8sSvc.Spec.Ports[0].TargetPort.IntValue() != 5432 {
		t.Errorf("targetPort ต้องชี้ที่ container port ได้ %v", k8sSvc.Spec.Ports[0].TargetPort)
	}

	np, err := cs.NetworkingV1().NetworkPolicies("ns-a").Get(ctx, networkPolicyName("my-pg"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy ต้องถูกสร้าง: %v", err)
	}
	if np.Spec.PodSelector.MatchLabels[labelApp] != "my-pg" {
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

	if _, err := cs.AppsV1().Deployments("ns-a").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("database ต้องไม่สร้าง Deployment")
	}
}

// TestK8sDeployAppGetsNodePort — service ธรรมดาต้องได้ Deployment + NodePort ที่คลัสเตอร์จ่ายให้
//
// fake API ไม่มีตัวจ่าย NodePort จึงวาง Service ที่มี nodePort ไว้ก่อน แล้วดูว่า apply ของเรา
// "ไม่ทับ" เลขที่คลัสเตอร์ถืออยู่ — พฤติกรรมเดียวกับ apply ซ้ำบนของจริง
func TestK8sDeployAppGetsNodePort(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-a"},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{{Name: "main", Protocol: corev1.ProtocolTCP, Port: 8080, NodePort: 30123}},
		},
	}
	k, cs := newFakeK8s(existing)
	ctx := context.Background()
	svc := testAppSvc()

	if err := k.DeployService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if svc.NodePort == nil || *svc.NodePort != 30123 {
		t.Fatalf("ต้องอ่าน NodePort ที่คลัสเตอร์ถืออยู่กลับมา ได้ %v", svc.NodePort)
	}

	dep, err := cs.AppsV1().Deployments("ns-a").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment ต้องถูกสร้าง: %v", err)
	}
	if *dep.Spec.Replicas != 2 {
		t.Errorf("replicas = %d ต้องเป็น 2", *dep.Spec.Replicas)
	}
	if len(dep.Spec.Template.Spec.Containers[0].VolumeMounts) != 0 {
		t.Error("service ธรรมดาต้องไม่มีดิสก์")
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("ns-a").Get(ctx, networkPolicyName("web"), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("service ธรรมดาต้องไม่มี NetworkPolicy (ไม่งั้น NodePort เข้าไม่ได้)")
	}
}

// TestK8sDeployCleansUpOnFailure — พลาดกลางทางต้องไม่ทิ้งของค้างบนคลัสเตอร์
// เพราะ ServiceManager จะลบแถวใน DB ทันที ของที่เหลืออยู่จะกลายเป็น workload ผี
func TestK8sDeployCleansUpOnFailure(t *testing.T) {
	k, cs := newFakeK8s()
	ctx := context.Background()
	// ให้ NetworkPolicy (ชิ้นสุดท้ายของ database) ล้ม — StatefulSet กับ Service สร้างไปแล้ว
	cs.PrependReactor("patch", "networkpolicies", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("webhook ปฏิเสธ")
	})

	if err := k.DeployService(ctx, "ns-a", testDatabaseSvc()); err == nil {
		t.Fatal("ต้องคืน error เมื่อสร้างไม่ครบ")
	}
	if _, err := cs.AppsV1().StatefulSets("ns-a").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("StatefulSet ที่สร้างค้างต้องถูกถอนออก")
	}
	if _, err := cs.CoreV1().Services("ns-a").Get(ctx, "my-pg", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("Service ที่สร้างค้างต้องถูกถอนออก")
	}
}

// TestK8sDeleteDatabaseRemovesEverything — ลบ database ต้องเก็บ PVC ด้วย และลบซ้ำต้องไม่ error
func TestK8sDeleteDatabaseRemovesEverything(t *testing.T) {
	// PVC ที่ StatefulSet controller จะสร้างจาก volumeClaimTemplates บนคลัสเตอร์จริง
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName("my-pg"), Namespace: "ns-a"}}
	k, cs := newFakeK8s(pvc)
	ctx := context.Background()
	svc := testDatabaseSvc()

	if err := k.DeployService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	if err := k.DeleteService(ctx, "ns-a", svc); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	checks := map[string]error{}
	_, checks["StatefulSet"] = cs.AppsV1().StatefulSets("ns-a").Get(ctx, "my-pg", metav1.GetOptions{})
	_, checks["Service"] = cs.CoreV1().Services("ns-a").Get(ctx, "my-pg", metav1.GetOptions{})
	_, checks["NetworkPolicy"] = cs.NetworkingV1().NetworkPolicies("ns-a").Get(ctx, networkPolicyName("my-pg"), metav1.GetOptions{})
	_, checks["PVC"] = cs.CoreV1().PersistentVolumeClaims("ns-a").Get(ctx, pvcName("my-pg"), metav1.GetOptions{})
	for what, err := range checks {
		if !apierrors.IsNotFound(err) {
			t.Errorf("%s ต้องถูกลบ แต่ยังอยู่ (err=%v)", what, err)
		}
	}

	if err := k.DeleteService(ctx, "ns-a", svc); err != nil {
		t.Errorf("ลบซ้ำต้องผ่าน (idempotent) ได้ %v", err)
	}
}

// TestK8sScaleUpdatesReplicas — scale แตะแค่ replicas ของ Deployment
func TestK8sScaleUpdatesReplicas(t *testing.T) {
	k, cs := newFakeK8s()
	ctx := context.Background()
	svc := testAppSvc()
	if err := k.deployApp(ctx, "ns-a", svc); err != nil && !strings.Contains(err.Error(), "NodePort") {
		t.Fatalf("deployApp: %v", err) // fake ไม่จ่าย NodePort — Deployment ถูกสร้างแล้วก่อนถึงจุดนั้น
	}

	if err := k.ScaleService(ctx, "ns-a", "web", 5); err != nil {
		t.Fatalf("ScaleService: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("ns-a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 5 {
		t.Errorf("replicas = %d ต้องเป็น 5", *dep.Spec.Replicas)
	}
	if err := k.ScaleService(ctx, "ns-a", "ghost", 2); err == nil {
		t.Error("scale ของที่ไม่มีอยู่ต้อง error เพื่อให้ ServiceManager ย้อน DB กลับ")
	}
}

// TestK8sStatusReadsPods — ตารางแปลสภาพ pod จริงๆ ที่ kubelet รายงาน เป็นสถานะที่ระบบเก็บ
// แต่ละเคสคืออาการที่ผู้ใช้เจอบ่อยที่สุดตอน deploy ของตัวเองครั้งแรก
func TestK8sStatusReadsPods(t *testing.T) {
	ctx := context.Background()
	svc := testAppSvc()
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns-a"}}
	podWith := func(status corev1.PodStatus) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "ns-a", Labels: map[string]string{labelApp: "web"}},
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
			k, _ := newFakeK8s(tc.objects...)
			got, err := k.Status(ctx, "ns-a", svc)
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
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "my-pg", Namespace: "ns-a"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pg-0", Namespace: "ns-a", Labels: map[string]string{labelApp: "my-pg"}},
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
	k, _ := newFakeK8s(objects...)
	got, err := k.Status(ctx, "ns-a", svc)
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
	k, _ := newFakeK8s()
	if _, err := k.Logs(ctx, "ns-a", "web", LogOptions{}); err == nil {
		t.Error("ไม่มี pod ต้อง error ไม่ใช่คืน stream ว่าง")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-xyz", Namespace: "ns-a", Labels: map[string]string{labelApp: "web"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	k, _ = newFakeK8s(pod)
	stream, err := k.Logs(ctx, "ns-a", "web", LogOptions{TailLines: 50, Timestamps: true})
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
