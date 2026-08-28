# --- build stage ---
FROM node:22-alpine AS builder
WORKDIR /app

# ลอกเฉพาะ manifest ก่อน เพื่อให้ layer ของ npm ci ถูก cache ไว้ตราบใดที่ dependency ไม่เปลี่ยน
# (สำคัญบน NUC ที่ build ช้า — แก้โค้ดแล้ว build ใหม่จะไม่ต้องลง dependency ซ้ำทุกครั้ง)
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# VITE_API_URL ไม่ต้องส่งเข้ามาตอน build: ค่า default ของ src/config/env.ts เป็น "/api"
# ซึ่งเป็น path บนโดเมนเดียวกัน แล้ว nginx ใน stage ถัดไป proxy /api ต่อไป backend ให้เอง
# ค่าอื่นที่อยากเปลี่ยนทีหลังตั้งผ่าน env ของ container ได้ (ดู docker-entrypoint.d/40-caesar-config.sh)
RUN npm run build

# --- run stage ---
FROM nginx:1.27-alpine

RUN apk add --no-cache tzdata

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

# script นี้ถูก entrypoint ของ image nginx เรียกให้เองตอน start (ทุกไฟล์ใน /docker-entrypoint.d)
# ต้อง chmod ในนี้ ไม่พึ่ง permission ของไฟล์ต้นทาง — บน Windows ไฟล์จะไม่มี executable bit ติดมา
# แล้ว nginx จะข้ามไปเฉยๆ พร้อม log ว่า "not executable, ignoring" ซึ่งหาสาเหตุยากมาก
COPY docker-entrypoint.d/40-caesar-config.sh /docker-entrypoint.d/40-caesar-config.sh
RUN chmod +x /docker-entrypoint.d/40-caesar-config.sh

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1/ || exit 1
