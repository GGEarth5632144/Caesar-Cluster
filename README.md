# Caesar Cluster — Frontend

ระบบ Dashboard สำหรับบริหารจัดการ Kubernetes Cluster ของมหาวิทยาลัย  
สร้างด้วย React 19 + TypeScript + Vite + Tailwind CSS

---

## สถาปัตยกรรมระบบ

```
[Caesar Cluster Frontend]  ←→  [Caesar Cluster API]  ←→  [Kubernetes]
        (React/Vite)                  (Go REST)
             ↑
             └──→  [Cluster AI]  ←→  [Python AI Engine (DeepSeek-R1)]
                   (Go :8090)              (:8000, AGX Orin)
```

ระบบแบ่งเป็น 2 ส่วนหลักที่แยก deploy กัน:
- **Caesar Cluster** — Frontend + Go API (ระบบหลัก จัดการ K8s) รันบน NUC 
- **Cluster AI** — Go server + Python AI Engine (รันบน AGX Orin)

---

## สิ่งที่ทำได้จริงแล้ว

### Authentication & Access Control
- Login / Register / Forgot Password / Reset Password
- Role-based routing: User (role=user) กับ Admin (role=admin) เห็น route ต่างกัน
- Hashed routes ทุก path ด้วย SHA-256 + salt เพื่อป้องกัน enumeration

### User — Service Management
- ดู list services ทั้งหมดของตัวเอง (running / deploying / failed)
- **Direct Deploy** — กรอก Container Image + Resource preset/custom → deploy ไป K8s ทันที
- **Deploy with AI** — ส่ง spec ไป Cluster AI ก่อน แล้ว navigate ไปหน้า AI Review
- ลบ Service

### User — AI Review Page (หน้าใหม่)
- **Full-page pipeline visualization** แสดง 4 stage: Sandbox → Router → Expert → Evaluator
- **Claude-style thinking animation** — typewriter text บอกว่า AI กำลังคิดอะไรอยู่
- **Elapsed timer** MM:SS.d นับตั้งแต่เริ่ม หยุดเมื่อ pipeline จบ
- **Per-stage timing** แสดงเวลาแต่ละ stage เมื่อ done
- **AI recommendation** — แสดง proposed fix + justification + Red-Team insight
- **Accept / Reject** — ผู้ใช้เลือกรับหรือปฏิเสธ AI fix ก่อน deploy จริงไป K8s
- **Circuit Breaker display** — แสดงสถานะ approved / escalated (retry ครบ 3 ครั้ง)
- **Telemetry panel** — latency ต่อ agent, prompt/completion tokens, retry count

### Admin
- Dashboard ภาพรวมระบบ
- จัดการ Request ของ User (อนุมัติ / ปฏิเสธ resource quota)
- User Management
- Audit Log
- IPC Management
- Service overview (admin view)
- Alert admin

### Multi-Agent Pipeline (Cluster AI)
- **Router** — Triage: วิเคราะห์ error log เลือก expert ที่เหมาะสม
- **Expert** — Constitutional AI: Backend / Frontend / Security expert เสนอ fix พร้อม scratchpad
- **Evaluator** — Red-Team review: ตรวจสอบความปลอดภัย ก่อน approve
- **Circuit Breaker** — maxRetries=3 ถ้าเกินจะ escalate ขึ้นให้ admin
- **Post-Mortem RAG** — Vector DB จำประสบการณ์การแก้ปัญหาในอดีต
- **Telemetry** — ติดตาม latency + token usage realtime

### Mock Mode (ทดสอบโดยไม่ต้องรัน backend)
```bash
# ทดสอบ UI ทั้งหมดโดยไม่ต้องรัน Go server หรือ Python AI
VITE_MOCK_AI=true npm run dev

# ทดสอบ Go server โดยไม่ต้องรัน Python AI
set MOCK_AI=true && go run ./cmd/server   # Windows CMD
$env:MOCK_AI="true"; go run ./cmd/server  # PowerShell
```

---

## สิ่งที่ยังรอ / ยังทำไม่ได้

| ส่วน | สถานะ | รอใคร |
|------|--------|--------|
| Dashboard (advanced metrics) | รอ | Infra team วาง K8s metrics endpoint |
| Alert system (real-time) | รอ | Infra team ติดตั้ง alert webhook |
| Cluster AI บน AGX Orin | รอ | Infra team ติดตั้ง NUC Cluster |
| Python AI Engine (DeepSeek-R1) | รอ | Infra team ติดตั้ง model บน AGX Orin |

---

## ขั้นตอนต่อไป

### 1. Deploy แยกกัน
```
Caesar Cluster (Frontend + API)  →  เซิร์ฟเวอร์ปกติ
Cluster AI (Go + Python AI)      →  AGX Orin (NUC Cluster)
```

### 2. เชื่อมต่อ 2 ระบบ
- ตั้งค่า `VITE_AI_API_URL=http://<agx-orin-ip>:8090` ใน production `.env`
- ตรวจสอบ network policy ให้ Caesar Cluster → Cluster AI คุยกันได้
- ทดสอบ CORS / firewall rules ระหว่าง 2 เครื่อง

### 3. ทดสอบกับ AI จริง
- รัน Python AI Engine (DeepSeek-R1) บน AGX Orin
- ปิด `MOCK_AI` ใน Cluster AI
- เปิด Deploy with AI flow บน frontend จริง
- ดู end-to-end workflow: form → AI Review page → pipeline animation → accept/reject → K8s deploy

### 4. เก็บ feedback จาก workflow จริง
- วัด latency จริงของแต่ละ agent
- ปรับ prompt / constitutional AI rules ตาม edge case ที่เจอ
- ตรวจสอบ Circuit Breaker ว่า trigger ถูกเงื่อนไขไหม

---

## การรันโปรเจค

```bash
npm install
npm run dev
```

### Environment Variables

| Variable | Default | ความหมาย |
|----------|---------|-----------|
| `VITE_AI_API_URL` | `http://localhost:8090` | URL ของ Cluster AI Go server |
| `VITE_MOCK_AI` | `false` | `true` = จำลอง AI pipeline ทั้งหมด ไม่ต้องรัน backend |

คัดลอก `.env.local.example` → `.env.local` แล้วแก้ค่าตามต้องการ

---

## Tech Stack

- **React 19** + **TypeScript** + **Vite**
- **Tailwind CSS** v4
- **React Router** v7 (lazy loading + hashed routes)
- **Zustand** — auth state
- **Axios** — HTTP client
- **Lucide React** — icons
- **crypto-js** — SHA-256 route hashing
