# --- build stage ---
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# สร้างทั้ง 3 คำสั่งไว้ใน image เดียว ไม่ใช่แค่ server
#   server      = API ตัวหลัก (ENTRYPOINT)
#   seed        = ยัดข้อมูลตั้งต้น ต้องรันครั้งแรกก่อนใช้งาน ไม่งั้นสมัครสมาชิกไม่ได้
#   export-rbac = generate RBAC manifest จาก DB
# ถ้าไม่ใส่มาด้วย จะรัน seed บนเครื่องจริงไม่ได้เลยถ้าไม่มี Go ติดตั้งอยู่บนเครื่องนั้น
# เรียกใช้ตัวอื่นด้วยการ override entrypoint เช่น
#   docker compose run --rm --entrypoint /app/seed backend
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/export-rbac ./cmd/export-rbac

# --- run stage ---
FROM alpine:3.20

# ca-certificates = ต้องมีเพื่อยิง HTTPS ไป Resend และไป Kubernetes API
# tzdata          = ให้ log เป็นเวลาไทยได้เมื่อตั้ง TZ=Asia/Bangkok
# wget            = ใช้โดย HEALTHCHECK ข้างล่าง (busybox มีมาให้อยู่แล้ว)
RUN apk add --no-cache ca-certificates tzdata

# รันเป็น user ธรรมดา ไม่ใช่ root — ถ้ามีช่องโหว่ใน API ผู้บุกรุกจะไม่ได้ root ใน container ทันที
# uid 10001 ถูกอ้างถึงใน DEPLOY.md ตอนตั้งสิทธิ์ไฟล์ kubeconfig ที่ mount เข้ามา
RUN adduser -D -u 10001 caesar

WORKDIR /app
COPY --from=builder /out/server /out/seed /out/export-rbac ./
USER caesar

EXPOSE 8080

# /health ping DB ให้ในตัว — container ที่ต่อ DB ไม่ได้จะถูกมาร์คเป็น unhealthy
# ใช้ ${PORT:-8080} เพราะ PORT ตั้งผ่าน env ได้
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT:-8080}/health" || exit 1

ENTRYPOINT ["/app/server"]
