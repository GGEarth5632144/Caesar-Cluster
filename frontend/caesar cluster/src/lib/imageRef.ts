// ตรวจรูปแบบ Docker image reference ให้ตรงกับที่ backend เช็คใน internal/controller/helper.go
//
// จงใจทำซ้ำฝั่งหน้าเว็บเพื่อให้ผู้ใช้รู้ตั้งแต่ตอนพิมพ์ ไม่ต้องกด deploy แล้วรอ 400 กลับมา
// ตัวนี้ไม่ใช่ด่านความปลอดภัย — ด่านจริงอยู่ที่ backend เสมอ ตรงนี้แค่ทำให้ใช้งานลื่นขึ้น
//
// ถ้าแก้กฎที่ไหน ต้องแก้อีกที่ให้ตรงกันด้วย ไม่งั้นจะมี image ที่ฟอร์มบอกว่าผ่าน
// แต่ backend ปฏิเสธ (หรือกลับกัน) ซึ่งหาสาเหตุยากมาก

const DOMAIN =
  /^(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))*(?::[0-9]+)?$/;

// ส่วนชื่อต้องเป็นตัวพิมพ์เล็กเท่านั้น เป็นจุดที่คนพลาดบ่อยที่สุด
const PATH = /^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*(?:\/[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*)*$/;

const TAG = /^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$/;

const DIGEST = /^[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}$/;

// ตรงกับ varchar(200) ของคอลัมน์ services.image
export const MAX_IMAGE_LENGTH = 200;

/** เช็คว่า image ที่กรอกเป็น reference ที่ Docker parse ได้จริงหรือไม่ */
export function isValidImageRef(image: string): boolean {
  if (!image || image.length > MAX_IMAGE_LENGTH) return false;

  let rest = image;

  // digest อยู่ท้ายสุดเสมอ ตัดออกก่อน
  const at = rest.indexOf('@');
  if (at >= 0) {
    if (!DIGEST.test(rest.slice(at + 1))) return false;
    rest = rest.slice(0, at);
  }

  // tag คือ ":" ที่อยู่หลัง slash ตัวสุดท้ายเท่านั้น
  // ไม่งั้น localhost:5000/app จะถูกอ่านว่า tag = "5000/app" ทั้งที่เป็น port ของ registry
  const colon = rest.lastIndexOf(':');
  if (colon >= 0 && colon > rest.lastIndexOf('/')) {
    if (!TAG.test(rest.slice(colon + 1))) return false;
    rest = rest.slice(0, colon);
  }

  // ท่อนแรกเป็น registry ก็ต่อเมื่อมีจุดหรือ colon อยู่ในนั้น หรือเป็น localhost
  let name = rest;
  const slash = rest.indexOf('/');
  if (slash > 0) {
    const head = rest.slice(0, slash);
    if (/[.:]/.test(head) || head === 'localhost') {
      if (!DOMAIN.test(head)) return false;
      name = rest.slice(slash + 1);
    }
  }

  return PATH.test(name);
}

/** ข้อความอธิบายว่าผิดตรงไหน คืน null ถ้าไม่มีปัญหา — ใช้โชว์ใต้ช่องกรอก */
export function imageRefError(image: string): string | null {
  const value = image.trim();
  if (!value) return null; // ยังไม่พิมพ์ ไม่ต้องเตือน

  if (value.length > MAX_IMAGE_LENGTH) {
    return `ยาวเกินไป (สูงสุด ${MAX_IMAGE_LENGTH} ตัวอักษร)`;
  }
  if (/\s/.test(value)) {
    return 'ห้ามมีช่องว่าง';
  }
  if (/^[a-z]+:\/\//.test(value)) {
    return 'ไม่ต้องใส่ http:// หรือ https:// — ใส่แค่ชื่อ image เช่น ghcr.io/org/app:v1';
  }
  // เช็คตัวพิมพ์ใหญ่แยกออกมา เพราะเป็นสาเหตุที่เจอบ่อยที่สุดและเดาเองไม่ออก
  if (value !== value.toLowerCase() && isValidImageRef(value.toLowerCase())) {
    return 'ชื่อ image ต้องเป็นตัวพิมพ์เล็กทั้งหมด';
  }
  if (!isValidImageRef(value)) {
    return 'รูปแบบไม่ถูกต้อง — ต้องเป็น [registry/]ชื่อ[:tag] เช่น nginx:1.27-alpine';
  }
  return null;
}
