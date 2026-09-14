/**
 * database.ts — ค่าคงที่และตัวช่วยของ service ที่เปิดสวิตช์ "เป็นฐานข้อมูล"
 *
 * ระบบไม่แยกชนิดของฐานข้อมูล ใช้ image อะไรก็ได้ — สิ่งที่สวิตช์ให้คือเครือข่ายปิด + ดิสก์ถาวร
 * + ตรึง 1 pod ซึ่งไม่มีข้อไหนต้องรู้ชนิดของฐานข้อมูลเลย
 */

/** โฟลเดอร์ที่ห้าม mount ทับ (ต้องตรงกับ reservedMountRoots ฝั่ง Go) — ทับแล้วไฟล์ของ image ถูกบังหมด */
const RESERVED_MOUNT_ROOTS = [
  '/',
  '/bin',
  '/boot',
  '/dev',
  '/etc',
  '/lib',
  '/lib64',
  '/proc',
  '/root',
  '/sbin',
  '/sys',
  '/usr',
];

/**
 * ตรวจตำแหน่งเก็บข้อมูลที่ผู้ใช้กรอก — สตริงว่าง = ผ่าน
 * ต้องให้ผลตรงกับ services.ValidateDataPath ฝั่ง Go (ที่นี่แค่บอกผู้ใช้ก่อนกดส่ง)
 */
export function validateDataPath(raw: string): string {
  const p = raw.trim();
  if (!p) return 'ต้องระบุตำแหน่งที่ image นี้เก็บข้อมูล';
  if (!p.startsWith('/')) return 'ต้องขึ้นต้นด้วย / (เช่น /var/lib/mydb)';
  if (p.length > 200) return 'ยาวเกินไป (สูงสุด 200 ตัวอักษร)';
  if (p.includes('..')) return 'ห้ามมี .. ในเส้นทาง';
  if (p.includes('//')) return 'ห้ามมี / ติดกันสองตัว';

  const clean = p.replace(/\/+$/, '');
  if (!clean) return 'mount ที่ / ไม่ได้ — จะบังไฟล์ทั้งหมดของ image';
  for (const root of RESERVED_MOUNT_ROOTS) {
    // เทียบแบบมี / ต่อท้าย เพื่อไม่ให้ "/usrdata" โดนบล็อกเพราะขึ้นต้นด้วย "/usr"
    if (clean === root || (root !== '/' && (clean + '/').startsWith(root + '/'))) {
      return `mount ทับโฟลเดอร์ระบบ (${root}) ไม่ได้ — container จะขึ้นไม่ได้`;
    }
  }
  return '';
}

/**
 * เดาว่า env ตัวนี้เป็นความลับไหมจากชื่อ key — มีผลแค่ว่าช่องกรอกแสดงเป็นจุดหรือไม่
 * ไม่ได้เปลี่ยนค่าที่ส่งขึ้นระบบ จึงเดาพลาดไปทางปิดบังเกินไว้ก่อน
 */
const SECRET_KEY_HINTS = ['PASSWORD', 'PASSWD', 'SECRET', 'TOKEN', 'APIKEY', 'API_KEY', 'PRIVATE'];

export function looksSecret(key: string): boolean {
  const upper = key.trim().toUpperCase();
  return SECRET_KEY_HINTS.some((hint) => upper.includes(hint));
}

// ── ขนาดดิสก์ต่อ 1 ฐานข้อมูล — ต้องตรงกับ binding ของ dto.CreateServiceRequest ฝั่ง Go ────────
export const STORAGE_BOUNDS = {
  minMB: 1024, // 1 GB
  maxMB: 20480, // 20 GB
  defaultMB: 5120, // 5 GB
  stepMB: 1024,
} as const;

export type StorageUnit = 'MB' | 'GB';

/** ตัวคูณของแต่ละหน่วย — ค่าที่ส่งขึ้น API เป็น MB จำนวนเต็มเสมอ ไม่เคยส่งหน่วยไปด้วย */
export const UNIT_FACTOR: Record<StorageUnit, number> = { MB: 1, GB: 1024 };

/** แสดงขนาดให้อ่านง่าย — ต่ำกว่า 1 GB โชว์เป็น MB */
export function formatStorage(mb: number): string {
  if (Math.abs(mb) < 1024) return `${mb} MB`;
  const gb = mb / 1024;
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
}
