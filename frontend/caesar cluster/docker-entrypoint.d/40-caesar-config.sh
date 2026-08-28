#!/bin/sh
# เขียน /config.js ใหม่ทุกครั้งที่ container start จาก env ของ container
#
# entrypoint ของ image nginx รันทุกไฟล์ใน /docker-entrypoint.d ให้เองก่อนสตาร์ท nginx
# ค่าที่เขียนออกไปถูกอ่านโดย src/config/env.ts และมีความสำคัญเหนือค่าที่ Vite ฝังมาตอน build
# ทำให้เปลี่ยน URL ของ API ได้ด้วยการแก้ env แล้ว restart container โดยไม่ต้อง build ใหม่
#
# ค่าที่ไม่ได้ตั้งจะถูกเขียนเป็น string ว่าง ซึ่งฝั่ง env.ts ตีความว่า "ไม่ได้ตั้ง" แล้วตกไปใช้ค่า default
set -eu

TARGET=/usr/share/nginx/html/config.js

cat > "$TARGET" <<EOF
window.__CAESAR_CONFIG__ = {
  API_URL: "${CAESAR_API_URL:-}",
  AI_API_URL: "${CAESAR_AI_API_URL:-}",
  MOCK_AI: "${CAESAR_MOCK_AI:-}"
};
EOF

echo "caesar: เขียน config.js แล้ว (API_URL='${CAESAR_API_URL:-/api (default)}')"
