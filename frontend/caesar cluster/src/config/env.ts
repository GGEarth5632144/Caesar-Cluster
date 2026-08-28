// ค่า runtime ที่ frontend ต้องรู้ — อ่านจาก window ก่อน แล้วค่อยตกไปที่ค่าที่ Vite ฝังไว้ตอน build
//
// ทำไมต้องอ่านจาก window ด้วย: Vite แทนค่า import.meta.env ลงใน bundle ตั้งแต่ตอน build
// ถ้าพึ่งมันอย่างเดียว การเปลี่ยน URL ของ API สักครั้งเท่ากับต้อง build ใหม่ทั้งก้อน
// container ของ frontend เลยเขียนไฟล์ /config.js ให้ตอน start จาก env ของ container เอง
// แล้ว index.html โหลดไฟล์นั้นก่อน bundle — เปลี่ยนค่าแล้ว restart container ก็พอ
//
// ลำดับความสำคัญ: ค่าจาก container (window) → ค่าที่ build ฝังมา (.env) → ค่า default ในไฟล์นี้

declare global {
  interface Window {
    __CAESAR_CONFIG__?: {
      API_URL?: string;
      AI_API_URL?: string;
      MOCK_AI?: string;
    };
  }
}

const runtime = typeof window === 'undefined' ? undefined : window.__CAESAR_CONFIG__;

// ค่าว่างถือว่า "ไม่ได้ตั้ง" ไม่ใช่ "ตั้งเป็นค่าว่าง" — entrypoint ของ container เขียนคีย์มาครบทุกตัว
// เสมอ ตัวที่ไม่ได้ตั้ง env จะมาเป็น string ว่าง ซึ่งต้องตกไปใช้ค่าที่ build ฝังไว้แทน
function pick(...candidates: Array<string | undefined>): string {
  for (const value of candidates) {
    if (typeof value === 'string' && value.trim() !== '') return value.trim();
  }
  return '';
}

// default เป็น path ไม่ใช่ URL เต็ม: บนเครื่องจริง nginx เสิร์ฟหน้าเว็บและ proxy /api ไป backend
// ให้ในตัว ทำให้เป็น same-origin เลยไม่ต้องยุ่งกับ CORS และไม่ต้องเปิดพอร์ต backend ออกข้างนอก
export const API_URL = pick(runtime?.API_URL, import.meta.env.VITE_API_URL, '/api');

export const AI_API_URL = pick(runtime?.AI_API_URL, import.meta.env.VITE_AI_API_URL, 'http://localhost:8090');

export const MOCK_AI = pick(runtime?.MOCK_AI, import.meta.env.VITE_MOCK_AI, 'false') === 'true';
