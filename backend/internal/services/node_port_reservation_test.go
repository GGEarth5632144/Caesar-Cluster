package services

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	k8stesting "k8s.io/client-go/testing"

	"backend/internal/entity"
)

// fixedPortsProv = mock ที่บอกว่าคลัสเตอร์ใช้พอร์ตไหนอยู่แล้วบ้าง
type fixedPortsProv struct {
	*MockProvisioner
	used map[int]bool
}

func (p *fixedPortsProv) UsedNodePorts(context.Context) (map[int]bool, error) {
	out := map[int]bool{}
	for k, v := range p.used {
		out[k] = v
	}
	return out, nil
}

// TestNodePortReservations — กติกาของใบจอง: เลขเดิมเมื่อเปิดฟอร์มซ้ำ, คนอื่นใช้ใบจองของเราไม่ได้,
// หมดอายุแล้วใช้ไม่ได้, ไม่จ่ายเลขที่คลัสเตอร์หรือ DB ใช้อยู่
func TestNodePortReservations(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// ใช้ทุกพอร์ตยกเว้นสองเลข — การสุ่มต้องได้หนึ่งในสองเลขนี้เท่านั้น
	used := map[int]bool{}
	for p := entity.MinNodePort; p <= entity.MaxNodePort; p++ {
		used[p] = true
	}
	delete(used, 20001)
	delete(used, 32767)
	r := NewNodePortReservations(db, &fixedPortsProv{MockProvisioner: NewMockProvisioner(), used: used})
	clock := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return clock }

	a, err := r.Reserve(ctx, 1, 10)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if a.NodePort != 20001 && a.NodePort != 32767 {
		t.Fatalf("ต้องได้พอร์ตที่ว่างเท่านั้น ได้ %d", a.NodePort)
	}
	again, _ := r.Reserve(ctx, 1, 10)
	if again.NodePort != a.NodePort {
		t.Errorf("เปิดฟอร์มซ้ำต้องได้เลขเดิม %d ได้ %d", a.NodePort, again.NodePort)
	}

	b, err := r.Reserve(ctx, 2, 10)
	if err != nil || b.NodePort == a.NodePort {
		t.Fatalf("ผู้ใช้อีกคนต้องได้อีกเลข ได้ %d err=%v", b.NodePort, err)
	}
	if _, err := r.Reserve(ctx, 3, 10); !errors.Is(err, ErrNodePortExhausted) {
		t.Errorf("พอร์ตหมดต้องได้ ErrNodePortExhausted ได้ %v", err)
	}

	if err := r.Check(1, 10, a.NodePort); err != nil {
		t.Errorf("เจ้าของใบจองต้องใช้ได้: %v", err)
	}
	if err := r.Check(2, 10, a.NodePort); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Errorf("คนอื่นใช้ใบจองของเราต้องไม่ได้ ได้ %v", err)
	}
	if err := r.Check(1, 11, a.NodePort); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Errorf("ใบจองต้องผูกกับกลุ่ม ได้ %v", err)
	}
	if err := r.Check(1, 10, 25000); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Errorf("เลขที่ไม่ได้จอง (เลือกเอง) ต้องไม่ได้ ได้ %v", err)
	}

	clock = clock.Add(NodePortReservationTTL)
	if err := r.Check(1, 10, a.NodePort); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Errorf("ใบจองหมดอายุต้องใช้ไม่ได้ ได้ %v", err)
	}
	// หมดอายุแล้วเลขว่าง ผู้ใช้คนที่สามจองได้
	if _, err := r.Reserve(ctx, 3, 10); err != nil {
		t.Errorf("หลังใบจองหมดอายุต้องจองได้: %v", err)
	}
}

// TestCreateServiceUsesReservedNodePort — deploy ต้องได้เลขที่จองไว้เป๊ะ แล้วใบจองถูกคืน
func TestCreateServiceUsesReservedNodePort(t *testing.T) {
	db := testDB(t)
	ns := dbTestNamespace(t, db, "nodeport-reserve-test-ns")
	ctx := context.Background()
	mgr := NewServiceManager(db, NewQuotaService(db), NewMockProvisioner())

	params := CreateServiceParams{Name: "web", Image: "nginx", CPUMilli: 100, RAMMB: 128, NodePort: 21000}
	if _, err := mgr.Create(ctx, 1, ns.ID, params); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Fatalf("เลขที่ไม่ได้จองต้องถูกปฏิเสธ ได้ %v", err)
	}

	res, err := mgr.ReserveNodePort(ctx, 1, ns.ID)
	if err != nil {
		t.Fatalf("ReserveNodePort: %v", err)
	}
	params.NodePort = res.NodePort
	svc, err := mgr.Create(ctx, 1, ns.ID, params)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if svc.NodePort == nil || *svc.NodePort != res.NodePort {
		t.Errorf("ต้องได้ NodePort %d ตามที่จอง ได้ %v", res.NodePort, svc.NodePort)
	}
	var row entity.Service
	if err := db.First(&row, svc.ID).Error; err != nil || row.NodePort == nil || *row.NodePort != res.NodePort {
		t.Errorf("DB ต้องเก็บเลขที่จอง ได้ %v err=%v", row.NodePort, err)
	}
	if err := mgr.ports.Check(1, ns.ID, res.NodePort); !errors.Is(err, ErrNodePortReservationInvalid) {
		t.Error("deploy สำเร็จแล้วใบจองต้องถูกคืน")
	}
	// เลขที่อยู่ใน DB แล้วต้องไม่ถูกจ่ายซ้ำ
	for i := 0; i < 20; i++ {
		again, err := mgr.ReserveNodePort(ctx, 100+i, ns.ID)
		if err != nil {
			t.Fatal(err)
		}
		if again.NodePort == res.NodePort {
			t.Fatalf("จ่ายเลข %d ที่ service ใช้อยู่ซ้ำ", res.NodePort)
		}
	}
}

// TestK8sCreateServiceWithReservedNodePort — ส่งเลขที่จองไปให้ apiserver และแปลง "เลขถูกใช้แล้ว" เป็น ErrNodePortTaken
func TestK8sCreateServiceWithReservedNodePort(t *testing.T) {
	k, cs := newFakeProvisioner()
	ctx := context.Background()
	svc := testAppSvc()
	port := 23456
	svc.NodePort = &port

	if err := k.DeployService(ctx, "ns-7", svc); err != nil {
		t.Fatalf("DeployService: %v", err)
	}
	got, err := cs.CoreV1().Services("ns-7").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Ports[0].NodePort != 23456 || *svc.NodePort != 23456 {
		t.Errorf("Service ต้องขอ nodePort 23456 ได้ %d", got.Spec.Ports[0].NodePort)
	}

	used, err := k.UsedNodePorts(ctx)
	if err != nil || !used[23456] {
		t.Errorf("UsedNodePorts ต้องเห็น 23456 ได้ %v err=%v", used, err)
	}

	// apiserver ปฏิเสธเลขที่ถูกใช้แล้ว → ErrNodePortTaken และไม่ทิ้ง Deployment ค้าง
	k2, cs2 := newFakeProvisioner()
	cs2.PrependReactor("create", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInvalid(schema.GroupKind{Kind: "Service"}, "web", field.ErrorList{
			field.Invalid(field.NewPath("spec", "ports").Index(0).Child("nodePort"), 23456, "provided port is already allocated"),
		})
	})
	svc2 := testAppSvc()
	svc2.NodePort = &port
	if err := k2.DeployService(ctx, "ns-7", svc2); !errors.Is(err, ErrNodePortTaken) {
		t.Fatalf("ต้องได้ ErrNodePortTaken ได้ %v", err)
	}
	if _, err := cs2.AppsV1().Deployments("ns-7").Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Error("Deployment ต้องถูกถอนเมื่อสร้าง Service ไม่ผ่าน")
	}
}
