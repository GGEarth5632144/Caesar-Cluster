# การติดตั้ง Caesar Cluster บน Intel NUC7i5DNHE

คู่มือนี้ครอบคลุมการนำระบบขึ้นรันด้วย Docker บนเครื่อง NUC ที่เป็น control plane ของคลัสเตอร์อยู่แล้ว
และให้ backend สร้าง namespace กับจัดสรรทรัพยากรบนคลัสเตอร์ได้จริง

---

## สิ่งที่จะรันบนเครื่อง

| container | ทำอะไร | พอร์ตที่เปิดออก host | RAM ที่จำกัดไว้ |
|---|---|---|---|
| `caesar-frontend` | nginx เสิร์ฟหน้าเว็บ + proxy `/api` ไป backend | `WEB_PORT` (ค่าเริ่มต้น 8081) | 128 MB |
| `caesar-backend` | Go API คุยกับ Postgres และ Kubernetes | ไม่เปิด | 384 MB |
| `caesar-postgres` | ฐานข้อมูล | ไม่เปิด | 512 MB |

รวมประมาณ 1 GB จาก RAM 8 GB ของเครื่อง ที่เหลือยังเป็นของ control plane และ Prometheus ตามเดิม
พอร์ต 9090 และ 9100 ไม่ถูกแตะเลย

ผู้ใช้เข้าเว็บที่เดียวคือ `http://<ip-ของ-nuc>:8081` ทุกอย่างวิ่งผ่าน nginx ตัวนั้น
backend และ postgres คุยกันในเน็ตเวิร์กของ Docker เท่านั้น ไม่มีพอร์ตโผล่ออกมาให้ยิงจากข้างนอก

---

## สิ่งที่ต้องมีก่อนเริ่ม

- Docker Engine กับ `docker compose` v2 บน NUC (มีอยู่แล้วเพราะ Prometheus รันด้วยชุดนี้)
- `kubectl` ที่ใช้สิทธิ์แอดมินของคลัสเตอร์ได้
- พื้นที่ดิสก์ว่างประมาณ 3 GB สำหรับ image ทั้งหมด

---

## ขั้นตอนติดตั้ง

### 1. เอาโค้ดขึ้นเครื่องแล้วเตรียมค่า config

```bash
git clone <repo-url> caesar-cluster
cd caesar-cluster
cp .env.example .env
```

เปิด `.env` แล้วกรอกค่าที่มีเครื่องหมาย `<<<` ให้ครบ อย่างน้อย 3 ตัวนี้

```bash
openssl rand -hex 24     # เอาไปใส่ POSTGRES_PASSWORD
openssl rand -base64 48  # เอาไปใส่ JWT_SECRET
```

`FRONTEND_ORIGIN` ต้องเป็น URL ที่ผู้ใช้เปิดจริง ครบทั้ง scheme และพอร์ต เช่น `http://192.168.192.1:8081`
ค่านี้ถูกใช้เป็นฐานของลิงก์ในอีเมลรีเซ็ตรหัสผ่าน กรอกผิดแล้วลิงก์ในอีเมลจะพาไปผิดที่

### 2. ให้สิทธิ์ backend บนคลัสเตอร์

```bash
kubectl apply -f deploy/k8s/caesar-backend-rbac.yaml
chmod +x deploy/make-kubeconfig.sh
./deploy/make-kubeconfig.sh
```

สคริปต์จะเขียน `secrets/kubeconfig` ที่ใช้สิทธิ์ของ ServiceAccount ตัวเดียว ไม่ใช่ `admin.conf`
ถ้าสคริปต์บอกว่า API server เป็น `127.0.0.1` ให้รันใหม่พร้อมระบุ IP จริง

```bash
APISERVER=https://192.168.100.1:6443 ./deploy/make-kubeconfig.sh
```

ตรวจว่าสิทธิ์ใช้ได้จริง

```bash
kubectl --kubeconfig secrets/kubeconfig auth can-i create namespaces
kubectl --kubeconfig secrets/kubeconfig auth can-i delete namespaces
```

ทั้งสองคำสั่งต้องตอบ `yes`

### 3. build แล้วเปิดระบบ

```bash
mkdir -p out && sudo chown 10001:10001 out   # ที่เก็บไฟล์ rbac.yaml ที่ export ออกมา
docker compose -f docker-compose.prod.yml up -d --build
```

build ครั้งแรกใช้เวลาสักพักเพราะต้องดาวน์โหลด dependency ของ Go และ npm ทั้งหมด

### 4. ใส่ข้อมูลตั้งต้น

ขั้นนี้ข้ามไม่ได้ ถ้าไม่รันจะไม่มี role `user` ในตาราง `roles` แล้วสมัครสมาชิกไม่ได้เลย

```bash
docker compose -f docker-compose.prod.yml --profile tools run --rm seed
```

รันซ้ำได้ไม่พังและไม่เกิดข้อมูลซ้ำ

### 5. เปลี่ยนรหัสผ่าน admin ทันที

seed สร้างบัญชี `admin` ด้วยรหัสผ่าน `changeme123` ซึ่งเขียนอยู่ในซอร์สโค้ดที่ใครก็อ่านได้
login แล้วเปลี่ยนรหัสผ่านเป็นอย่างแรกก่อนให้ใครมาใช้งาน

seed ยังใส่บัญชีทดสอบและรายชื่อ `eligible_students` ปลอมไว้ด้วย รายละเอียดอยู่ในหัวข้อค้างคาข้างล่าง

---

## ตรวจว่าใช้งานได้จริง

```bash
docker compose -f docker-compose.prod.yml ps        # ทุกตัวต้องเป็น healthy
docker compose -f docker-compose.prod.yml logs backend | head -20
```

log ของ backend ตอน start ต้องมีบรรทัดพวกนี้

```
database connected ✓
schema migrated (AutoMigrate) ✓
kubernetes: ต่อ https://192.168.100.1:6443 สำเร็จ (server v1.xx.x)
provisioner: KUBERNETES
```

ถ้าเห็น `provisioner: MOCK` แปลว่ายังตั้ง `PROVISIONER=kubernetes` ไม่ครบ
ถ้าต่อคลัสเตอร์ไม่ได้ container จะไม่ยอมขึ้นเลยและบอกสาเหตุใน log ตั้งใจให้พังตั้งแต่ต้น
ไม่ใช่ไปพังตอนผู้ใช้กด deploy

ทดสอบครบวงจร: สมัครผู้ใช้ สร้าง space แล้ว deploy service หนึ่งตัว จากนั้นดูว่าของขึ้นจริง

```bash
kubectl get ns -l app.kubernetes.io/managed-by=caesar-cluster
kubectl -n <ชื่อ-space> get deploy,svc,resourcequota,limitrange,networkpolicy
kubectl -n <ชื่อ-space> describe resourcequota caesar-quota
```

เข้าถึง service ที่ deploy ไปได้ที่ `http://<ip-ของ-node-ตัวไหนก็ได้>:<node_port>`
เลข node port อยู่ในหน้าเว็บและในคอลัมน์ `node_port` ของตาราง `services`

---

## สิ่งที่ backend สร้างให้ทุก space

| resource | หน้าที่ |
|---|---|
| `Namespace` | ติด label ของระบบ และ label ที่ Pod Security Admission อ่าน |
| `ResourceQuota` ชื่อ `caesar-quota` | เพดาน CPU และ RAM รวมของทั้ง space ปิดการขอ PVC และ LoadBalancer |
| `LimitRange` ชื่อ `caesar-limits` | ค่า default ของ container ที่ไม่ระบุ resource และเพดานต่อ container |
| `NetworkPolicy` ชื่อ `caesar-isolation` | กัน traffic ข้าม space และห้าม pod เดินเข้าวงภายในของคลัสเตอร์ |

เรื่องที่ตัดสินใจไว้แล้วและควรรู้

- **โควตาถูกบังคับสองชั้น** ชั้นแรกคือ `QuotaService` ที่ล็อกแถวใน DB ก่อนอนุญาต ชั้นที่สองคือ ResourceQuota บนคลัสเตอร์ ถ้าโค้ดฝั่งเราพลาด k8s ยังปฏิเสธให้เอง
- **requests เท่ากับ limits เสมอ** ทำให้ pod ได้ QoS แบบ Guaranteed และทำให้ตัวเลขที่ ResourceQuota หักตรงกับที่ DB คำนวณเป๊ะ
- **ปิด token ของ ServiceAccount ในทุก pod ของผู้ใช้** ใครเข้าถึง container ได้ก็ยิง Kubernetes API ต่อไม่ได้
- **NetworkPolicy เปิดทุกอย่างยกเว้นวง pod** ไม่ใช่ deny ทั้งหมด เพราะ deny ทั้งหมดจะปิดทาง NodePort ไปด้วยจนผู้ใช้เข้า service ตัวเองไม่ได้
- **ไม่ยึด namespace ที่ไม่ได้สร้างเอง** ก่อนแก้หรือลบอะไร ระบบอ่าน label `app.kubernetes.io/managed-by` ก่อนเสมอ คนที่ตั้งชื่อ space ว่า `kube-system` จะไม่สามารถลบของระบบได้

### port ของ container

ระบบหา port ที่แอปฟังอยู่ตามลำดับนี้

1. ค่า `container_port` ที่ผู้ใช้กรอกตอนสร้าง service
2. env var ชื่อ `PORT`, `CONTAINER_PORT`, `APP_PORT` หรือ `HTTP_PORT` ที่ผู้ใช้ใส่มา
3. ค่า `K8S_DEFAULT_CONTAINER_PORT` ใน `.env`

หน้าเว็บยังไม่มีช่องกรอก `container_port` ให้ ตอนนี้ผู้ใช้เลี่ยงด้วยการใส่ env var ชื่อ `PORT` แทนได้

---

## คำสั่งที่ใช้บ่อย

```bash
# ดูสถานะและ log
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f backend

# อัปเดตโค้ดใหม่
git pull
docker compose -f docker-compose.prod.yml up -d --build

# เปลี่ยนแค่ค่าใน .env ของหน้าเว็บ ไม่ต้อง build ใหม่
docker compose -f docker-compose.prod.yml up -d frontend

# generate RBAC manifest จาก namespace ที่มีใน DB
docker compose -f docker-compose.prod.yml --profile tools run --rm export-rbac
kubectl apply -f out/rbac.yaml

# backup ฐานข้อมูล
docker compose -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U caesar cloud_cluster | gzip > backup-$(date +%F).sql.gz

# ต่อ pgAdmin จากเครื่องอื่นโดยไม่เปิดพอร์ตทิ้งไว้ (รันบนเครื่องตัวเอง)
ssh -N -L 5433:localhost:5433 nuc@192.168.192.1
# แล้วบน NUC เปิดพอร์ตชั่วคราวด้วย docker compose ... exec หรือเพิ่ม ports ใน compose ชั่วคราว
```

---

## ปัญหาที่เจอได้

**backend ขึ้นไม่ได้ บอกว่า `APP_ENV=production ต้องตั้ง JWT_SECRET ใหม่`**
ยังไม่ได้กรอก `JWT_SECRET` ใน `.env` หรือกรอกสั้นกว่า 32 ตัวอักษร ตั้งใจให้ไม่ยอม start
เพราะค่าตัวอย่างอยู่ในซอร์สโค้ด ใครก็ปลอม token เป็น admin ได้

**backend ขึ้นไม่ได้ บอกว่าต่อ Kubernetes API ไม่ได้**
เช็คว่า `secrets/kubeconfig` มีอยู่จริงและเป็นไฟล์ ไม่ใช่โฟลเดอร์
ถ้าเผลอ `up` ก่อนสร้างไฟล์ Docker จะสร้างเป็นโฟลเดอร์ให้ ลบทิ้งแล้วรันสคริปต์ใหม่

```bash
docker compose -f docker-compose.prod.yml down
sudo rm -rf secrets/kubeconfig
./deploy/make-kubeconfig.sh
```

อีกสาเหตุคือ `server:` ใน kubeconfig ชี้ไป `127.0.0.1` ซึ่งใน container หมายถึงตัว container เอง

**backend บอก permission denied ตอนอ่าน kubeconfig**
container รันเป็น uid 10001 ไม่ใช่ root

```bash
sudo chown 10001:10001 secrets/kubeconfig
```

**สมัครสมาชิกแล้วได้ 500**
ยังไม่ได้รัน seed ไม่มี role `user` ในตาราง `roles`

**deploy แล้วได้ IMAGE_NOT_ALLOWED**
`ALLOWED_IMAGE_REGISTRIES` ใน `.env` ไม่ครอบคลุม image นั้น เพิ่ม prefix เข้าไปแล้ว restart backend
ปล่อยว่างเพื่ออนุญาตทุก image ได้ แต่หมายถึงนักศึกษารัน image ขุดเหรียญได้ด้วย

**pod ขึ้นแล้วแต่ CrashLoopBackOff ทันที**
มักเป็นเพราะ image ต้องการสิทธิ์ที่ระดับ Pod Security ที่ตั้งไว้ไม่ให้
ดูสาเหตุจริงที่ `kubectl -n <space> describe pod <ชื่อ>` และ `kubectl -n <space> logs <ชื่อ>`
ถ้าตั้ง `K8S_POD_SECURITY=restricted` ไว้ image ที่รันเป็น root จะขึ้นไม่ได้ทั้งหมด
ค่าที่แนะนำสำหรับเริ่มต้นคือ `baseline`

**สร้าง space ชื่อเดิมซ้ำทันทีหลังลบแล้วได้ error ว่า Terminating**
การลบ namespace ของ k8s เป็นแบบไม่รอ รอสัก 10 ถึง 30 วินาทีแล้วลองใหม่

---

## เรื่องที่ยังค้างอยู่

เรียงตามความสำคัญ ไม่มีข้อไหนที่บล็อกการใช้งานปกติ แต่ควรรู้ก่อนเปิดให้นักศึกษาใช้จริง

1. **บัญชีทดสอบใน seed ยังอยู่** `cmd/seed/main.go` สร้างบัญชี `B6618452` รหัสผ่าน `Banana1234` และรายชื่อ eligible ปลอม `B6600001` ถึง `B6600010` ควรลบก่อนเปิดใช้จริง
2. **สถานะ service ไม่ตรงกับของจริง** DB เขียน `running` ตอน deploy สำเร็จครั้งเดียว pod พังทีหลัง DB ยังบอก running อยู่ ต้องมี reconcile loop มาอ่านสถานะ pod กลับเข้ามา สิทธิ์อ่าน pod เตรียมไว้ใน ClusterRole ให้แล้ว
3. **ยังไม่มี HTTPS** ทุกอย่างวิ่งเป็น http ซึ่งรับได้ถ้าเข้าผ่าน ZeroTier อย่างเดียว แต่ถ้าเปิดออกวงองค์กรควรมี TLS
4. **ยังไม่มี persistent storage** ให้ผู้ใช้ ResourceQuota ปิดการขอ PVC ไว้แล้วเพื่อไม่ให้สร้างสิ่งที่ระบบยังจัดการไม่ได้
5. **rate limit เก็บใน memory** ใช้ได้กับ backend ตัวเดียว ถ้าจะขยายเป็นหลาย replica ต้องย้ายไป store ที่แชร์กัน
6. **ยังไม่มี backup อัตโนมัติ** คำสั่ง `pg_dump` อยู่ในหัวข้อคำสั่งที่ใช้บ่อย ต้องเอาไปตั้ง cron เอง
