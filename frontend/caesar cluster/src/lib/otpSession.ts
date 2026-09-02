/**
 * ที่เก็บสองอย่างของระบบ OTP ฝั่งเบราว์เซอร์ — แยกจาก authStore เพราะทั้งคู่ไม่ใช่ session
 * ที่ล็อกอินอยู่ แต่มีชีวิตอยู่คนละช่วงกับ token
 *
 * 1) device token (localStorage) — ใบผ่านของเครื่องที่เคยกรอก OTP สำเร็จ ต้องรอดจากการปิด
 *    เบราว์เซอร์และการ logout ไม่งั้นไม่มีความหมายว่า "จำเครื่องนี้ไว้"
 *    (logout คือ "ฉันใช้เสร็จแล้ว" ไม่ใช่ "เครื่องนี้เชื่อไม่ได้แล้ว" จึงไม่ล้างตอน logout)
 *
 * 2) pending challenge (sessionStorage) — ใบยืนยันที่รอกรอกรหัสอยู่ ตายพร้อมแท็บโดยตั้งใจ
 *    เพราะการล็อกอินที่ค้างครึ่งทางไม่ควรตกทอดไปแท็บใหม่ ที่ต้องลง storage แทน route state
 *    เพราะผู้ใช้กด refresh ระหว่างสลับไปเปิดอีเมลได้ ถ้าเก็บใน memory อย่างเดียวหน้าจะเด้งกลับ
 *    ไป login แล้วรหัสที่เพิ่งส่งไปเสียเปล่า
 *
 * ทุกฟังก์ชันกลืน exception ของ storage ทิ้ง (โหมดส่วนตัวของบางเบราว์เซอร์ throw ตั้งแต่ตอนอ่าน)
 * ผลที่ได้คือแค่ต้องกรอก OTP ใหม่ ซึ่งดีกว่าหน้าขาวทั้งหน้า
 */

const DEVICE_KEY = 'caesar-cluster-device';
const PENDING_KEY = 'caesar-cluster-otp-pending';

/** ใบยืนยันที่รอกรอกรหัสอยู่ — พอเพียงให้หน้า VerifyOtp ทำงานต่อได้หลัง refresh */
export interface PendingOtp {
  challengeToken: string;
  /** อีเมลที่ backend ปิดบังมาแล้ว เช่น b6****@g.sut.ac.th — ไว้บอกผู้ใช้ว่าไปเปิดกล่องไหน */
  gmailMasked: string;
  /** ติ๊ก Remember For 30 Days มาไหม — ส่งต่อให้ setAuth ตอนกรอกรหัสผ่าน */
  remember: boolean;
  /** เวลาหมดอายุของรหัส (epoch ms) ใช้เดินนาฬิกาถอยหลังบนหน้าจอ */
  expiresAt: number;
  /** มาจากหน้าสมัครสมาชิกหรือหน้าเข้าสู่ระบบ — เปลี่ยนแค่ข้อความบนหน้า ไม่เปลี่ยนตรรกะ */
  fromRegister: boolean;
}

export function getDeviceToken(): string {
  try {
    return localStorage.getItem(DEVICE_KEY) ?? '';
  } catch {
    return '';
  }
}

export function setDeviceToken(token: string) {
  try {
    localStorage.setItem(DEVICE_KEY, token);
  } catch {
    /* เก็บไม่ได้ก็แค่ต้องกรอก OTP ใหม่ครั้งหน้า */
  }
}

export function readPendingOtp(): PendingOtp | null {
  try {
    const raw = sessionStorage.getItem(PENDING_KEY);
    if (!raw) return null;
    return JSON.parse(raw) as PendingOtp;
  } catch {
    return null;
  }
}

export function writePendingOtp(pending: PendingOtp) {
  try {
    sessionStorage.setItem(PENDING_KEY, JSON.stringify(pending));
  } catch {
    /* เก็บไม่ได้ = refresh แล้วต้องล็อกอินใหม่ ยังใช้งานต่อได้ถ้าไม่ refresh */
  }
}

export function clearPendingOtp() {
  try {
    sessionStorage.removeItem(PENDING_KEY);
  } catch {
    /* ลบไม่ได้ก็ปล่อย — ของที่ค้างอยู่หมดอายุเองในไม่กี่นาที */
  }
}
