# Caesar-Caster — Backend

ระบบให้นักศึกษา (เฉพาะสาขาที่กำหนด) ขอทรัพยากรไปรัน service ของตัวเองได้ฟรี
แนวคิดคล้าย [PebbleHost](https://pebblehost.com/) แต่**ปิดกลุ่ม ไม่มีค่าใช้จ่าย และมีโควตาจำกัด**

ผู้ใช้เลือกได้ว่าจะเอา CPU/RAM เท่าไหร่ (ภายในเพดาน) แล้วระบบจะไป deploy ให้บน Kubernetes

---

## สถานะปัจจุบัน — อ่านก่อน

| ส่วน | สถานะ |
|---|---|
| Database, schema, migration | ✅ ของจริง |
| Auth (bcrypt + JWT), middleware | ✅ ของจริง |
| **การบังคับโควตา** (transaction + row lock) | ✅ ของจริง กัน overcommit ได้จริง |
| กติกาทั้งหมด (eligible gate, 1 คน 1 space, limits) | ✅ ของจริง |
| **สร้าง namespace / deploy container จริงบน k8s** | ✅ ของจริง — `KubernetesProvisioner` เขียนเสร็จแล้ว |
| จำกัด image ที่ผู้ใช้รันได้ | ✅ มีแล้ว ผ่าน `ALLOWED_IMAGE_REGISTRIES` (ว่าง = ไม่จำกัด) |
| sync สถานะ pod กลับเข้า DB | ❌ ยังไม่มี reconcile loop |
| persistent storage (volume) | ❌ ยังไม่รองรับ (ResourceQuota ปิด PVC ไว้) |

ตั้ง `PROVISIONER=kubernetes` แล้วระบบจะสร้างของจริงบนคลัสเตอร์ทันที
ต่อคลัสเตอร์ไม่ได้ = server ไม่ยอม start เลย (ตั้งใจให้ fail ตั้งแต่ต้น)

วิธีนำขึ้นเครื่องจริงพร้อม Docker อยู่ใน [DEPLOY.md](../DEPLOY.md) ที่ root ของ repo

### สิ่งที่ provisioner สร้างให้ต่อ 1 namespace

| resource | หน้าที่ |
|---|---|
| `Namespace` | ติด label ของระบบ + label ของ Pod Security Admission |
| `ResourceQuota` (`caesar-quota`) | เพดาน CPU/RAM รวมของ space ปิดการขอ PVC และ LoadBalancer |
| `LimitRange` (`caesar-limits`) | ค่า default ของ container ที่ไม่ระบุ resource + เพดานต่อ container |
| `NetworkPolicy` (`caesar-isolation`) | กัน traffic ข้าม namespace + ห้าม pod เดินเข้าวงภายในของคลัสเตอร์ |

ต่อ 1 service สร้าง `Deployment` + `Service` ชนิด NodePort โดย requests เท่ากับ limits เสมอ
(ได้ QoS แบบ Guaranteed และทำให้ยอดที่ ResourceQuota หักตรงกับที่ `QuotaService` คำนวณใน DB เป๊ะ)

ทุก resource ติด label `app.kubernetes.io/managed-by=caesar-cluster` และ provisioner จะอ่าน label นี้
ก่อนแก้/ลบอะไรเสมอ — คนที่ตั้งชื่อ space ว่า `kube-system` จึงลบ namespace ของระบบไม่ได้

---

## เริ่มใช้งาน

```bash
cd backend
cp .env.example .env
docker compose up -d      # postgres (port 5433)

go run ./cmd/seed         # ** จำเป็น ** สร้าง roles + admin + request_templates ตั้งต้น
go run ./cmd/server       # http://localhost:8080
```

> **ห้ามข้าม `cmd/seed`** — ถ้าไม่รัน จะไม่มี role `user` ในตาราง `roles`
> ทำให้ **สมัครสมาชิกไม่ได้เลย** (Register หา role ไม่เจอ → 500)

admin ตั้งต้น: `student_id=admin` / `password=changeme123` → **เปลี่ยนทันทีหลัง login ครั้งแรก**

seed รันซ้ำได้ ไม่พัง ไม่เกิดข้อมูลซ้ำ (idempotent)

### เชื่อมต่อฐานข้อมูลด้วย pgAdmin 4

ต้องรัน `docker compose up -d` ให้ container postgres ทำงานอยู่ก่อน ถึงจะเชื่อมต่อได้
(ค่าทั้งหมดด้านล่างมาจาก `docker-compose.yml` / `.env.example` — ถ้าแก้ค่าพวกนี้เองต้องใช้ค่าที่แก้แทน)

1. เปิด pgAdmin 4 → ในแถบ **Object Explorer** ด้านซ้าย คลิกขวาที่ **Servers** → **Register** → **Server...**
2. แท็บ **General** → ช่อง **Name** ใส่ชื่ออะไรก็ได้ เช่น `Caesar Cluster (local)`
3. แท็บ **Connection** กรอกตามนี้:

   | ช่อง | ค่า |
   |---|---|
   | Host name/address | `localhost` |
   | Port | `5433` |
   | Maintenance database | `cloud_cluster` |
   | Username | `postgres` |
   | Password | `password` |

4. (ไม่บังคับ) ติ๊ก **Save password?** ไว้ จะได้ไม่ต้องกรอกรหัสผ่านซ้ำทุกครั้ง
5. กด **Save** — ถ้าเชื่อมต่อสำเร็จจะเห็น server ใหม่ขึ้นในรายการ ไล่เข้าไปที่
   `Servers → Caesar Cluster (local) → Databases → cloud_cluster → Schemas → public → Tables`
   จะเห็นตารางทั้งหมด (`users`, `eligible_students`, `namespaces`, `services`, ...)

> Port เป็น `5433` ไม่ใช่ `5432` เพราะ `docker-compose.yml` แม็ป container ออกมาที่ port นี้ (กันชนกับ
> postgres ที่อาจติดตั้งไว้ในเครื่องอยู่แล้วที่ port 5432 ปกติ)

### ปัญหาที่เจอได้ถ้ามี Postgres volume ค้างจากก่อนหน้า

ถ้า `go run ./cmd/seed` ขึ้น error แบบ `null value in column "..." violates not-null constraint`
สาเหตุคือมี container/volume ของ postgres ที่เคยรันมาก่อนตอน schema ยังหน้าตาไม่เหมือนตอนนี้ค้างอยู่
(`AutoMigrate` ของ GORM เพิ่ม/แก้ column ให้เท่านั้น **ไม่เคย drop column เก่าที่ไม่มีใน struct แล้ว**)

แก้โดยล้าง volume แล้วให้ `AutoMigrate` สร้าง schema ใหม่ทั้งหมด (ปลอดภัย เพราะเป็น dev DB local):

```bash
docker compose down -v     # -v = ลบ volume postgres ทิ้งด้วย
docker compose up -d
go run ./cmd/seed
```

---

## แนวคิดหลัก (เข้าใจ 4 ข้อนี้ก่อน แล้วโค้ดที่เหลือจะอ่านง่ายขึ้นมาก)

**1. `namespace` คือหน่วยที่ถือโควตา — ไม่ใช่ `node`**

นี่คือจุดที่คนมักเข้าใจผิด บน Kubernetes **เราไม่เลือกเครื่องเอง** — scheduler ของ k8s เลือกให้
หน้าที่ของ backend นี้คือคุมว่า *namespace หนึ่งใช้ทรัพยากรรวมกันได้ไม่เกินเท่าไหร่* เท่านั้น
(โค้ดเวอร์ชันเก่าเคยไล่หา node ที่ว่างเอง — ตอนนี้เอาออกหมดแล้ว)

**2. 1 คน = 1 space**

`users.namespace_id` ชี้ไป namespace เดียว จะเป็นแบบใช้คนเดียว (`solo`) หรือรวมกลุ่ม (`group`) ก็ได้
สมาชิกในกลุ่ม **แชร์โควตาก้อนเดียวกัน** และเห็น service ของกลุ่มเหมือนกันหมด

**3. CPU เก็บเป็น millicore** (1 core = 1000m) เพื่อให้เลือกเป็น % ได้ เช่น 300% = `3000m`

**4. สมัครได้เฉพาะ นศ. สาขา CPE ที่มีอยู่ในฐานข้อมูล**

`eligible_students` (คือตาราง `match` ใน ERD) — admin import รายชื่อ นศ. เข้ามาก่อน (ทุกสาขา)
ตอนสมัคร เช็ค 2 ชั้น: (1) มี student_id นี้ในตารางไหม (2) major ตรงกับ CPE ไหม — ไม่ผ่านชั้นไหนก็สมัครไม่ได้
นอกจากนี้ยังมี FK `users.student_id → eligible_students.student_id` กันอีกชั้นที่ระดับ DB

### เพดานทรัพยากร

ค่าคงที่ทั้งหมดอยู่ใน [`internal/entity/namespace.go`](internal/entity/namespace.go)

| อย่าง | ค่า |
|---|---|
| โควตาตั้งต้นต่อ namespace (**รวมทั้ง space** ไม่ใช่ต่อ service) | 3000m CPU / 2048 MB / 2 services |
| เพดานที่ admin ปรับให้ `group` ได้ | 8000m CPU / 8192 MB |
| เพดานที่ admin ปรับให้ `solo` ได้ | เท่าค่าตั้งต้น |
| เพดานของ service เดี่ยวๆ 1 ตัว | 3000m (300%) / 2048 MB |

---

## Data model

```
eligible_students (= "match")   รายชื่อ นศ. ที่มีสิทธิ์สมัคร — admin import
        ▲ FK
        │
      users ──────► roles          (user / admin)
        │
        │ namespace_id  (1 คน = 1 space, NULL ได้ถ้ายังไม่มี)
        ▼
   namespaces  ◄── contributor_id ── users      << หน่วยที่ถือโควตา
        │            cpu_limit_milli / ram_limit_mb
        ▼
    services ──────► request_templates  ("choices" ที่ admin สร้างไว้ให้เลือก)
      cpu_milli / ram_mb = snapshot ที่ก๊อปมาจาก template ตอนสร้าง
```

**สิ่งที่จงใจ *ไม่* เก็บใน DB:**

- `member_count` → นับสดจาก `COUNT(users WHERE namespace_id = ?)` ทุกครั้ง
- ยอดทรัพยากรที่ใช้ไป → คำนวณสดจาก `SUM(services)` ทุกครั้ง

ทั้งคู่เป็นค่าที่ derive ได้ ถ้าเก็บซ้ำไว้มีโอกาสเพี้ยนจากของจริง (เขียนตัวนึงแล้วลืมอัปเดตอีกตัว)

---

## โครงสร้างโฟลเดอร์

```
cmd/
  server/         entry point ของ API server
  seed/           ยัดข้อมูลตั้งต้น (roles, admin, request_templates) — idempotent
  export-rbac/    generate Kubernetes RBAC manifest (Role/RoleBinding) จาก namespace ใน DB
internal/
  config/         อ่าน env + เปิด DB + AutoMigrate + FK
  entity/         struct ที่ map กับตาราง (schema มาจาก tag ที่นี่ ไม่มีไฟล์ .sql แล้ว)
  dto/            request body + validation tag
  controller/     ชั้น HTTP: bind → เรียก service → ตอบ JSON (ไม่มี business logic)
  services/       business logic ทั้งหมดอยู่ที่นี่
  middlewares/    JWT auth + AdminOnly
  router/         ผูก route → handler
  utils/          response envelope
  test/           unit test
```

### ไฟล์สำคัญใน `services/`

| ไฟล์ | หน้าที่ |
|---|---|
| `quota_service.go` | **หัวใจของระบบ** — บังคับโควตา ล็อกแถว namespace ด้วย `SELECT ... FOR UPDATE` แล้วเช็คก่อนอนุญาต deploy |
| `namespace_manager.go` | สร้าง / เข้าร่วม space, ปรับโควตา |
| `service_manager.go` | deploy / ลบ workload |
| `provisioner.go` | **interface** — จุดเดียวที่ผูกกับ k8s ที่เหลือไม่รู้จัก k8s เลย |
| `provisioner_mock.go` | ตัวที่ใช้อยู่ตอนนี้ (แค่ log) |
| `provisioner_k8s.go` | ของจริงที่คุยกับ Kubernetes ผ่าน client-go (ใช้เมื่อ `PROVISIONER=kubernetes`) |

### Export RBAC manifest

DB (namespace + สมาชิก + โควตา) เป็น source of truth เดียว — ไม่ต้องแก้ YAML มือแล้วซิงก์เอง
รันคำสั่งนี้เมื่อไรก็ได้ manifest ที่ตรงกับ DB ปัจจุบันเสมอ (deterministic, รันซ้ำได้ผลลัพธ์เดิม):

```bash
go run ./cmd/export-rbac --out=rbac.yaml
kubectl apply -f rbac.yaml   # apply เข้า cluster เอง (เครื่องมือนี้แค่ generate ไม่ได้ apply ให้)
```

ต่อ 1 namespace ได้ `Role` (สิทธิ์ตรงกับที่ backend อนุญาตผ่าน API เป๊ะ: CRUD deployments/services,
อ่าน pods/pods-log อย่างเดียว — **ไม่มี** `pods/exec` เพราะ ERD กำกับไว้ว่า monitoring ได้ ใช้ terminal ไม่ได้)
+ `RoleBinding` ผูกกับสมาชิก**ทุกคน**ในกลุ่ม (ไม่ใช่แค่เจ้าของ — ตรงกับที่ backend ให้สมาชิกทุกคนสร้าง/ลบ
service ของกลุ่มร่วมกันเท่ากันอยู่แล้ว) เข้า Role เดียวกัน แต่ละ subject เป็น `caesar-user-<student_id>`
พร้อม annotation บอกว่าใครเป็นเจ้าของ + โควตาไว้ให้ดูเทียบง่ายๆ

> Caesar Cluster ไม่มี auth เข้า Kubernetes โดยตรง (auth ตอนนี้เป็น JWT ระดับ backend API เท่านั้น)
> `caesar-user-<student_id>` เลยเป็นแค่ชื่อ subject ที่ตั้งไว้ก่อนเฉยๆ ยังไม่มีระบบไหน map identity จริง
> เข้ากับชื่อนี้ — ต้องออกแบบ auth layer ตรงนี้เพิ่มก่อนถึงจะเอา manifest นี้ไป apply ใช้งานจริงได้

### ทำไมต้องล็อกแถว namespace ตอนเช็คโควตา

ถ้า 2 request ขอ deploy พร้อมกัน ทั้งคู่จะอ่านยอดใช้เดิม (เช่น 0) แล้วต่างคนต่างคิดว่าโควตาพอ → **ใช้เกิน**
`SELECT ... FOR UPDATE` ทำให้คนที่สองต้องรอ แล้วเห็นยอดที่คนแรก INSERT ไปแล้ว จึงคำนวณถูก
(เช็คโควตา + INSERT อยู่ใน transaction เดียวกันเสมอ)

---

## API

ทุก response ห่อด้วยรูปแบบเดียวกัน:
`{"success": true, "data": ...}` หรือ `{"success": false, "error": {"code": "...", "message": "..."}}`

### Public

| Method | Path | หมายเหตุ |
|---|---|---|
| GET | `/health` | ping DB |
| POST | `/api/register` | ต้องอยู่ใน `eligible_students` และเป็นสาขา CPE ไม่งั้น 403 |
| POST | `/api/login` | คืน JWT (อายุ 24 ชม.) |

### ต้อง login (`Authorization: Bearer <token>`)

| Method | Path | หมายเหตุ |
|---|---|---|
| GET | `/api/me` | ดู `namespace_id` ว่ามี space แล้วยัง |
| GET | `/api/request-templates` | choices ที่ admin เปิดไว้ |
| POST | `/api/namespaces` | สร้าง space (`type`: `solo` \| `group`) |
| POST | `/api/namespaces/join` | เข้าร่วม space แบบ `group` |
| GET | `/api/namespaces/me` | space ของฉัน + ยอดใช้งาน + จำนวนสมาชิก |
| DELETE | `/api/namespaces` | ออกจาก space ของตัวเอง (สมาชิกธรรมดา = แค่ออก **ต้องลบ service ของตัวเองให้หมดก่อน**, เจ้าของคนสุดท้าย = ลบทั้งก้อน) |
| POST | `/api/namespaces/invites` | เชิญ `student_id` เข้ากลุ่ม — **เฉพาะเจ้าของ (contributor)** เท่านั้น |
| GET | `/api/namespaces/invites/mine` | คำเชิญที่ pending ส่งถึงฉัน (เห็นได้ไม่ว่าจะมี space แล้วหรือยัง) |
| GET | `/api/namespaces/invites/sent` | คำเชิญทั้งหมดที่ฉันส่งจาก space ของตัวเอง (ทุกสถานะ) — เจ้าของเท่านั้น |
| PATCH | `/api/namespaces/invites/:id/accept` | ตอบรับ → join namespace ที่เชิญมาทันที |
| PATCH | `/api/namespaces/invites/:id/decline` | ปฏิเสธ |
| DELETE | `/api/namespaces/invites/:id` | เจ้าของยกเลิกคำเชิญที่ยังไม่มีคนตอบ (เผื่อเชิญผิดคน) |
| GET | `/api/services` | service ทั้งหมดใน space |
| POST | `/api/services` | deploy (เลือก `request_template_id` หรือกรอก `cpu_milli`/`ram_mb` เอง) |
| DELETE | `/api/services/:id` | ลบ → **คืนโควตาทันที** |

### Admin เท่านั้น

| Method | Path | หมายเหตุ |
|---|---|---|
| POST | `/api/admin/eligible-students` | import รายชื่อ นศ. (ทีละหลายคนได้) |
| POST | `/api/admin/request-templates` | สร้าง choice ใหม่ |
| GET | `/api/admin/namespaces` | ภาพรวมทุก space + ยอดใช้งาน |
| PATCH | `/api/admin/namespaces/:id/quota` | ปรับโควตา (group ≤ 8 core) |
| DELETE | `/api/admin/namespaces/:id` | ลบ namespace ทิ้ง (ลบได้แม้มีสมาชิกอยู่ ตามดุลยพินิจแอดมิน) |
| DELETE | `/api/admin/users/:id` | ลบผู้ใช้ — ถอน namespace ที่เขาเป็นเจ้าของ + service ที่ไปสร้างค้างไว้ใน space คนอื่น ออกจากคลัสเตอร์ให้ก่อน |

### ลำดับที่ผู้ใช้ต้องเดิน

```
admin import รายชื่อ → user register → login
   → สร้าง namespace (หรือ join กลุ่ม)     ← ข้ามไม่ได้ ไม่งั้น deploy จะได้ 409 NO_NAMESPACE
   → deploy service
```

### error code ที่เจอบ่อย

| code | ความหมาย |
|---|---|
| `STUDENT_NOT_FOUND` | รหัส นศ. ไม่มีอยู่ในฐานข้อมูลเลย |
| `NOT_CPE` | มีรหัส นศ. นี้ในฐานข้อมูล แต่ไม่ใช่สาขา CPE |
| `NO_NAMESPACE` | ยังไม่มี space ต้องสร้าง/เข้ากลุ่มก่อน |
| `QUOTA_EXCEEDED` | ทรัพยากรที่ขอเกินโควตาที่เหลือ |
| `SERVICE_LIMIT` | จำนวน service ใน space เต็มแล้ว |
| `ALREADY_IN_NAMESPACE` | มี space อยู่แล้ว (1 คน = 1 space) |
| `NAMESPACE_HAS_MEMBERS` | เจ้าของพยายามออก/ลบ namespace ทั้งที่ยังมีสมาชิกคนอื่นอยู่ |
| `HAS_OWN_SERVICES` | สมาชิกพยายามออกจาก space ทั้งที่ยังมี service ที่ตัวเองสร้างค้างอยู่ (ออกไปแล้วจะลบเองไม่ได้อีก) |
| `NAME_TAKEN` | ชื่อ space ซ้ำ — ซ้ำใน DB หรือมี namespace ชื่อนี้อยู่บนคลัสเตอร์แล้ว ต้องเปลี่ยนชื่อ |
| `NAMESPACE_TERMINATING` | namespace ชื่อเดิมยังถูกลบไม่เสร็จ (k8s ลบแบบ async) รอสักครู่แล้วลองใหม่ ไม่ต้องเปลี่ยนชื่อ |
| `INVALID_IMAGE` | รูปแบบ image ไม่ถูกต้องตามไวยากรณ์ของ Docker (ชื่อต้องเป็นตัวพิมพ์เล็ก, ห้ามมีช่องว่าง, ห้ามใส่ http://) |
| `IMAGE_NOT_ALLOWED` | image ที่ขอไม่อยู่ใน `ALLOWED_IMAGE_REGISTRIES` |
| `NOT_CONTRIBUTOR` | เฉพาะเจ้าของ space เท่านั้นที่เชิญ/ดูคำเชิญที่ส่งไปได้ |
| `INVITE_SELF` | เชิญ student_id ของตัวเอง |
| `INVITE_ALREADY_PENDING` | เชิญคนเดิมซ้ำทั้งที่มีคำเชิญ pending อยู่แล้วใน space นี้ |
| `INVITE_NOT_PENDING` | คำเชิญถูก accept/decline/cancel ไปแล้ว ตอบซ้ำไม่ได้ |

---

## ยังไม่ได้ทำ

เรียงตามความสำคัญ ไม่มีข้อไหนบล็อกการใช้งานปกติ แต่ควรรู้ก่อนเปิดให้นักศึกษาใช้จริง

1. 🔴 **ลบบัญชีทดสอบใน `cmd/seed`** — ยังสร้างผู้ใช้ `B6618452` รหัสผ่าน `Banana1234`
   และรายชื่อ eligible ปลอม `B6600001`–`B6600010` ไว้ ต้องเอาออกก่อนเปิดใช้จริง
2. 🟠 **status ไม่ sync กับของจริง** — DB เขียน `running` ตอน deploy สำเร็จครั้งเดียว
   ถ้า pod พังทีหลัง DB ยังบอก `running` ต้องมี reconcile loop มาอ่านสถานะ pod กลับเข้ามา
   (สิทธิ์อ่าน pod เตรียมไว้ใน `deploy/k8s/caesar-backend-rbac.yaml` ให้แล้ว)
3. 🟠 **persistent storage (volume)** — ยังไม่รองรับ ตอนนี้ ResourceQuota ปิดการขอ PVC ไว้
   เพื่อไม่ให้ผู้ใช้สร้างสิ่งที่ระบบยังจัดการต่อไม่ได้
4. 🟠 **หน้าเว็บยังไม่มีช่องกรอก `container_port`** — API รับแล้ว แต่ฟอร์มยังไม่ส่งมา
   ตอนนี้ผู้ใช้เลี่ยงด้วยการใส่ env var ชื่อ `PORT` ซึ่ง provisioner อ่านให้อยู่แล้ว
5. 🟡 **ยังไม่มี TLS** — รับได้ถ้าเข้าผ่าน ZeroTier อย่างเดียว แต่ถ้าเปิดออกวงองค์กรควรมี
6. 🟡 **rate limit เก็บใน memory** — ใช้ได้กับ backend ตัวเดียว ถ้าขยายเป็นหลาย replica ต้องย้ายไป store ที่แชร์กัน
