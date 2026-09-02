/**
 * ที่เก็บสองอย่างที่ระบบ OTP ต้องใช้ฝั่งเบราว์เซอร์ — แยกออกมาจาก authStore เพราะทั้งคู่
 * "ไม่ใช่ session ที่ล็อกอินอยู่" แต่เป็นสิ่งที่มีชีวิตอยู่คนละช่วงกับ token
 *
 * 1) device token — ใบผ่านของเครื่องนี้ที่เคยกรอก OTP สำเร็จ อยู่ใน localStorage เสมอ
 *    ต้องรอดจากการปิดเบราว์เซอร์และการ logout ให้ได้ ไม่งั้นก็ไม่มีความหมายว่า "จำเครื่องนี้ไว้"
 *    (จงใจไม่ล้างตอน logout — logout คือ "ฉันใช้เสร็จแล้ว" ไม่ใช่ "เครื่องนี้เชื่อไม่ได้แล้ว")
 *
 * 2) pending challenge — ใบยืนยันที่กำลังรอกรอกรหัสอยู่ อยู่ใน sessionStorage
 *    ตายไปพร้อมแท็บโดยตั้งใจ: การล็อกอินที่ค้างครึ่งทางไม่ควรตกทอดไปถึงแท็บใหม่หรือวันถัดไป
 *    ที่ต้องเก็บลง storage แทนที่จะส่งผ่าน route state เฉยๆ ก็เพราะผู้ใช้กด refresh ระหว่างรอ
 *    อีเมลได้ ถ้าเก็บไว้ใน memory อย่างเดียวหน้าจะเด้งกลับไป login แล้วรหัสที่เพิ่งส่งไปก็เสียเปล่า
 *
 * ทุกฟังก์ชันกลืน exception ของ storage ทิ้ง — โหมดส่วนตัวของบางเบราว์เซอร์ throw ตั้งแต่ตอนอ่าน
 * ผลที่ได้คือแค่ "ต้องกรอก OTP ใหม่" ซึ่งยอมรับได้ ดีกว่าหน้าขาวทั้งหน้า
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
