# Caesar-Caster — Backend

ระบบให้นักศึกษา (เฉพาะสาขาที่กำหนด) ขอทรัพยากรไปรัน service ของตัวเองได้ฟรี
แนวคิดคล้าย [PebbleHost](https://pebblehost.com/) แต่**ปิดกลุ่ม ไม่มีค่าใช้จ่าย และมีโควตาจำกัด**

ผู้ใช้เลือกได้ว่าจะเอา CPU/RAM เท่าไหร่ (ภายในเพดาน) แล้วระบบจะไป deploy ให้บน Kubernetes

---

## ⚠️ สถานะปัจจุบัน — อ่านก่อน

**การสร้างของจริงบน cluster ยังเป็น MOCK ทั้งหมด** อย่าเข้าใจผิดว่าระบบนี้ deploy ขึ้น k8s ได้แล้ว

| ส่วน | สถานะ |
|---|---|
| Database, schema, migration | ✅ ของจริง |
| Auth (bcrypt + JWT), middleware | ✅ ของจริง |
| **การบังคับโควตา** (transaction + row lock) | ✅ ของจริง กัน overcommit ได้จริง |
| กติกาทั้งหมด (eligible gate, 1 คน 1 space, limits) | ✅ ของจริง |
| **สร้าง namespace / deploy container จริง** | ❌ **MOCK — แค่ log ออกมา ไม่มีอะไรเกิดขึ้นจริง** |

พูดอีกแบบ: ตอนนี้มี **control plane ที่ทำงานจริง** (จองโควตา จดบัญชี ตรวจสิทธิ์)
แต่ยัง **ไม่มีมือที่ไปสร้างของจริง** — `KubernetesProvisioner` ยังเป็น stub ทุก method

ถ้าตั้ง `PROVISIONER=kubernetes` ตอนนี้ ทุก request จะ error ทันที (ตั้งใจให้ fail แบบปิดประตู)

---

## เริ่มใช้งาน

```bash
cd backend
cp .env.example .env
docker compose up -d      # postgres (port 5433)

go run ./cmd/seed         # ** จำเป็น ** สร้าง roles + admin + plans ตั้งต้น
go run ./cmd/server       # http://localhost:8080
```

> **ห้ามข้าม `cmd/seed`** — ถ้าไม่รัน จะไม่มี role `user` ในตาราง `roles`
> ทำให้ **สมัครสมาชิกไม่ได้เลย** (Register หา role ไม่เจอ → 500)

admin ตั้งต้น: `student_id=admin` / `password=changeme123` → **เปลี่ยนทันทีหลัง login ครั้งแรก**

seed รันซ้ำได้ ไม่พัง ไม่เกิดข้อมูลซ้ำ (idempotent)

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

**4. สมัครได้เฉพาะคนที่อยู่ในรายชื่อ**

`eligible_students` (คือตาราง `match` ใน ERD) — admin import รายชื่อเข้ามาก่อน ใครไม่อยู่ในนั้นสมัครไม่ได้
บังคับ 2 ชั้น: เช็คในโค้ด + FK `users.student_id → eligible_students.student_id` ที่ระดับ DB

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
   namespaces  ◄── owner_id ── users      << หน่วยที่ถือโควตา
        │            type = solo | group
        │            cpu_limit_milli / ram_limit_mb / max_services
        ▼
    services ──────► plans         ("choices" ที่ admin สร้างไว้ให้เลือก)
      cpu_milli / ram_mb = snapshot ที่ก๊อปมาจาก plan ตอนสร้าง
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
  seed/           ยัดข้อมูลตั้งต้น (roles, admin, plans) — idempotent
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
| `provisioner_k8s.go` | **ยังเป็น stub** — ของจริงต้องเขียนที่นี่ |

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
| POST | `/api/register` | ต้องอยู่ใน `eligible_students` ไม่งั้น 403 |
| POST | `/api/login` | คืน JWT (อายุ 24 ชม.) |

### ต้อง login (`Authorization: Bearer <token>`)

| Method | Path | หมายเหตุ |
|---|---|---|
| GET | `/api/me` | ดู `namespace_id` ว่ามี space แล้วยัง |
| GET | `/api/plans` | choices ที่ admin เปิดไว้ |
| POST | `/api/namespaces` | สร้าง space (`type`: `solo` \| `group`) |
| POST | `/api/namespaces/join` | เข้าร่วม space แบบ `group` |
| GET | `/api/namespaces/me` | space ของฉัน + ยอดใช้งาน + จำนวนสมาชิก |
| GET | `/api/services` | service ทั้งหมดใน space |
| POST | `/api/services` | deploy (เลือก `plan_id` หรือกรอก `cpu_milli`/`ram_mb` เอง) |
| DELETE | `/api/services/:id` | ลบ → **คืนโควตาทันที** |

### Admin เท่านั้น

| Method | Path | หมายเหตุ |
|---|---|---|
| POST | `/api/admin/eligible-students` | import รายชื่อ นศ. (ทีละหลายคนได้) |
| POST | `/api/admin/plans` | สร้าง choice ใหม่ |
| GET | `/api/admin/namespaces` | ภาพรวมทุก space + ยอดใช้งาน |
| PATCH | `/api/admin/namespaces/:id/quota` | ปรับโควตา (group ≤ 8 core) |

### ลำดับที่ผู้ใช้ต้องเดิน

```
admin import รายชื่อ → user register → login
   → สร้าง namespace (หรือ join กลุ่ม)     ← ข้ามไม่ได้ ไม่งั้น deploy จะได้ 409 NO_NAMESPACE
   → deploy service
```

### error code ที่เจอบ่อย

| code | ความหมาย |
|---|---|
| `NOT_ELIGIBLE` | รหัส นศ. ไม่อยู่ในรายชื่อที่อนุญาต |
| `NO_NAMESPACE` | ยังไม่มี space ต้องสร้าง/เข้ากลุ่มก่อน |
| `QUOTA_EXCEEDED` | ทรัพยากรที่ขอเกินโควตาที่เหลือ |
| `SERVICE_LIMIT` | จำนวน service ใน space เต็มแล้ว |
| `ALREADY_IN_NAMESPACE` | มี space อยู่แล้ว (1 คน = 1 space) |

---

## ยังไม่ได้ทำ (ถ้าจะเอาไปใช้กับเครื่องจริง ต้องทำก่อน)

เรียงตามความสำคัญ:

1. 🔴 **จำกัด image ที่ user รันได้** — ตอนนี้ field `image` รับ string อะไรก็ได้
   พอต่อ k8s จริง = user รันอะไรก็ได้บนเครื่อง (เช่น ขุดเหรียญ) **ต้องมี allowlist หรือบังคับ registry ของเรา**
2. 🔴 **Pod security** — บังคับ `runAsNonRoot`, ห้าม privileged, drop capabilities
3. 🔴 **เขียน `KubernetesProvisioner` จริง** — Namespace + ResourceQuota + LimitRange + NetworkPolicy (default-deny กัน traffic ข้าม namespace) + Deployment
4. 🟠 **ผู้ใช้เข้าถึง service ตัวเองยังไง** — ยังไม่มี port / Service / Ingress ในโมเดลเลย
5. 🟠 **status ไม่ sync กับของจริง** — DB เขียน `running` ตอน deploy สำเร็จครั้งเดียว ถ้า pod พังทีหลัง DB ยังบอก `running` ต้องมี reconcile loop
6. 🟠 **persistent storage** (volume) — ยังไม่มี
7. 🟡 ลบ namespace / ออกจากกลุ่ม
8. 🟡 production hardening — `JWT_SECRET` ยังเป็น `dev-secret`, ยังไม่มี TLS / rate limit
