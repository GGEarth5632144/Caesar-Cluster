package services

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"backend/internal/entity"
)

// เทสต์ของ UpdateService — regression ของ docs 030: เดิมแก้ไข service = DeleteService + DeployService
// ซึ่งลบ PVC ทิ้ง (ข้อมูลหาย) แล้ว PVC ใหม่ชื่อเดิมชนโฟลเดอร์ NFS ที่ provisioner ยังลบไม่เสร็จ

// deleteActions นับคำสั่ง delete ที่ยิงเข้า fake API — แก้ในที่ต้องไม่มีสักคำสั่ง
func deleteActions(cs *fake.Clientset) []string {
	var got []string
	for _, a := range cs.Actions() {
		if a.GetVerb() == "delete" {
			got = append(got, a.GetResource().Resource)
		}
	}
	return got
}

// TestK8sUpdateWebWithStorageKeepsPVCAndNodePort — แก้ image/env/พอร์ตของ Nextcloud ต้องไม่แตะ PVC
// และ NodePort ต้องเป็นเลขเดิม (URL ที่ผู้ใช้ถืออยู่ยังใช้ได้)
func TestK8sUpdateWebWithStorageKeepsPVCAndNodePort(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName("nextcloud"), Namespace: "ns-7"}}
	k, cs := newFakeProvisioner(pvc)
	fakeNodePorts(cs, 30456)
	ctx := context.Background()

	oldSvc := testWebDiskSvc()
	if err := k.DeployService(ctx, "ns-7", oldSvc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	cs.ClearActions()

	svc := *oldSvc
	svc.Image = "nextcloud:31-apache"
	svc.ContainerPort = 8080
	svc.EnvVars = entity.EnvVarMap{"MYSQL_HOST": "nc-db"}
	svc.NodePort = nil // ServiceManager.Update ล้างค่าไว้ก่อนเรียก

	if err := k.UpdateService(ctx, "ns-7", oldSvc, &svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}

	if dels := deleteActions(cs); len(dels) != 0 {
		t.Fatalf("แก้ในที่ต้องไม่ลบอะไรเลย ได้ delete %v", dels)
	}
	if _, err := cs.CoreV1().PersistentVolumeClaims("ns-7").Get(ctx, pvcName("nextcloud"), metav1.GetOptions{}); err != nil {
		t.Errorf("PVC ต้องยังอยู่: %v", err)
	}

	sts, err := cs.AppsV1().StatefulSets("ns-7").Get(ctx, "nextcloud", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("StatefulSet ต้องยังอยู่: %v", err)
	}
	c := sts.Spec.Template.Spec.Containers[0]
	if c.Image != "nextcloud:31-apache" {
		t.Errorf("image = %q ต้องเป็นค่าใหม่", c.Image)
	}
	if len(c.Env) != 1 || c.Env[0].Name != "MYSQL_HOST" || c.Env[0].Value != "nc-db" {
		t.Errorf("env ต้องถูกแทนทั้งชุด ได้ %+v", c.Env)
	}
	if len(c.VolumeMounts) != 1 || c.VolumeMounts[0].MountPath != "/var/www/html" {
		t.Errorf("ดิสก์ต้องยัง mount ที่ data_path เดิม ได้ %+v", c.VolumeMounts)
	}
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Errorf("volumeClaimTemplates ต้องคงเดิม ได้ %d อัน", len(sts.Spec.VolumeClaimTemplates))
	}

	k8sSvc, err := cs.CoreV1().Services("ns-7").Get(ctx, "nextcloud", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Service ต้องยังอยู่: %v", err)
	}
	p := k8sSvc.Spec.Ports[0]
	if p.Port != 8080 || p.TargetPort.IntValue() != 8080 {
		t.Errorf("Service ต้องชี้พอร์ตใหม่ 8080 ได้ port=%d targetPort=%s", p.Port, p.TargetPort.String())
	}
	if p.NodePort != 30456 {
		t.Errorf("nodePort บน Service = %d ต้องคงเดิม 30456", p.NodePort)
	}
	if svc.NodePort == nil || *svc.NodePort != 30456 {
		t.Errorf("svc.NodePort ต้องเป็นเลขเดิม 30456 ได้ %v", svc.NodePort)
	}
}

// TestK8sUpdateDatabasePortUpdatesNetworkPolicy — database เปลี่ยนพอร์ต ต้องแก้ NetworkPolicy ตาม
// ไม่งั้น pod ใน namespace ต่อไม่ได้ทั้งที่ Service ถูก และต้องไม่ได้ NodePort
func TestK8sUpdateDatabasePortUpdatesNetworkPolicy(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()

	oldSvc := testDatabaseSvc()
	if err := k.DeployService(ctx, "ns-7", oldSvc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	svc := *oldSvc
	svc.ContainerPort = 5433

	if err := k.UpdateService(ctx, "ns-7", oldSvc, &svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	if svc.NodePort != nil {
		t.Errorf("database ต้องไม่ได้ NodePort ได้ %d", *svc.NodePort)
	}
	np, err := cs.NetworkingV1().NetworkPolicies("ns-7").Get(ctx, networkPolicyName("my-pg"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy ต้องยังอยู่: %v", err)
	}
	if got := np.Spec.Ingress[0].Ports[0].Port.IntValue(); got != 5433 {
		t.Errorf("NetworkPolicy เปิดพอร์ต %d ต้องเป็น 5433", got)
	}
	if got := np.Spec.Ingress[0].From[0].PodSelector; got == nil {
		t.Error("ต้องยังรับเฉพาะ pod ใน namespace เดียวกัน")
	}
}

// TestK8sUpdateStorageImmutable — ค่าที่ผูกกับ PVC แก้ไม่ได้ ต้องคืน ErrStorageImmutable โดยไม่แตะคลัสเตอร์
func TestK8sUpdateStorageImmutable(t *testing.T) {
	cases := map[string]func(s *entity.Service){
		"เปลี่ยนชื่อ":       func(s *entity.Service) { s.Name = "nextcloud2" },
		"เปลี่ยนขนาดดิสก์":  func(s *entity.Service) { s.StorageMB = 10240 },
		"เปลี่ยน data_path": func(s *entity.Service) { s.DataPath = "/data" },
		"สลับ database":     func(s *entity.Service) { s.IsDatabase = true },
		"ถอดดิสก์":          func(s *entity.Service) { s.StorageMB, s.DataPath = 0, "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			k, cs := newFakeProvisioner()
			fakeNodePorts(cs, 30456)
			ctx := context.Background()
			oldSvc := testWebDiskSvc()
			if err := k.DeployService(ctx, "ns-7", oldSvc); err != nil {
				t.Fatalf("DeployService: %v", err)
			}
			cs.ClearActions()

			svc := *oldSvc
			mutate(&svc)
			if err := k.UpdateService(ctx, "ns-7", oldSvc, &svc); !errors.Is(err, ErrStorageImmutable) {
				t.Fatalf("ต้องได้ ErrStorageImmutable ได้ %v", err)
			}
			for _, a := range cs.Actions() {
				if a.GetVerb() != "get" && a.GetVerb() != "list" {
					t.Errorf("ต้องไม่แตะคลัสเตอร์ ได้ %s %s", a.GetVerb(), a.GetResource().Resource)
				}
			}
		})
	}
}

// TestK8sUpdateAppInPlace — web ไม่มีดิสก์ ชื่อเดิม: แก้ Deployment ในที่ (replicas/image) และ NodePort คงเดิม
func TestK8sUpdateAppInPlace(t *testing.T) {
	k, cs := newFakeProvisioner()
	fakeNodePorts(cs, 30111)
	ctx := context.Background()

	oldSvc := testAppSvc()
	if err := k.DeployService(ctx, "ns-7", oldSvc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	cs.ClearActions()
	fakeNodePorts(cs, 30999) // ถ้าเผลอสร้าง Service ใหม่จะได้เลขนี้แทน

	svc := *oldSvc
	svc.Replicas = 3
	svc.Image = "nginx:1.29-alpine"
	if err := k.UpdateService(ctx, "ns-7", oldSvc, &svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	if dels := deleteActions(cs); len(dels) != 0 {
		t.Fatalf("แก้ในที่ต้องไม่ลบอะไรเลย ได้ delete %v", dels)
	}
	dep, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment ต้องยังอยู่: %v", err)
	}
	if *dep.Spec.Replicas != 3 || dep.Spec.Template.Spec.Containers[0].Image != "nginx:1.29-alpine" {
		t.Errorf("Deployment ต้องเป็นค่าใหม่ ได้ replicas=%d image=%s", *dep.Spec.Replicas, dep.Spec.Template.Spec.Containers[0].Image)
	}
	if svc.NodePort == nil || *svc.NodePort != 30111 {
		t.Errorf("NodePort ต้องคงเดิม 30111 ได้ %v", svc.NodePort)
	}
}

// TestK8sUpdateAppRenameRecreates — web ไม่มีดิสก์เปลี่ยนชื่อ: ชื่อ object เปลี่ยนต้องสร้างใหม่ (ไม่มีข้อมูลให้หาย)
func TestK8sUpdateAppRenameRecreates(t *testing.T) {
	k, cs := newFakeProvisioner()
	fakeNodePorts(cs, 30111)
	ctx := context.Background()

	oldSvc := testAppSvc()
	if err := k.DeployService(ctx, "ns-7", oldSvc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	svc := *oldSvc
	svc.Name = "web2"
	if err := k.UpdateService(ctx, "ns-7", oldSvc, &svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	if _, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("Deployment ชื่อเดิมต้องถูกลบ")
	}
	if _, err := cs.AppsV1().Deployments("ns-7").Get(ctx, "web2", metav1.GetOptions{}); err != nil {
		t.Errorf("Deployment ชื่อใหม่ต้องถูกสร้าง: %v", err)
	}
	if svc.NodePort == nil {
		t.Error("สร้างใหม่แล้วต้องได้ NodePort")
	}
}
