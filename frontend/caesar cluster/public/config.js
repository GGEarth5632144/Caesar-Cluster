// ค่า runtime ของหน้าเว็บ — ไฟล์นี้เป็นตัวเปล่าไว้ใช้ตอน dev
// container ของ frontend จะเขียนทับไฟล์นี้ตอน start ด้วยค่าจริงจาก env (ดู docker-entrypoint.d/40-caesar-config.sh)
// อ่านค่าออกไปใช้ที่ src/config/env.ts
window.__CAESAR_CONFIG__ = {};
