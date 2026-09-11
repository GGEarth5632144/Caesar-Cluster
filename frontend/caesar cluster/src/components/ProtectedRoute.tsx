import { useEffect, useRef, useState } from "react";
import { Navigate, Outlet } from "react-router-dom";

import { useAuthStore } from "@/store/authStore";
import { authApi } from "@/api/authApi";
import LogoLoader from "@/components/ui/LogoLoader";
import { PATHS } from "@/config/routes";

/**
 * ProtectedRoute กันหน้าที่ต้อง login และซิงก์สิทธิ์กับ backend หนึ่งครั้งตอนเปิดเว็บ
 *
 * ต้องถาม /me ซ้ำทั้งที่มี token อยู่แล้ว เพราะ token อยู่ได้ถึง 30 วัน ระหว่างนั้น role/namespace
 * อาจเปลี่ยนไปแล้ว ถ้าหน้าเว็บใช้ค่าเก่าผู้ใช้จะเห็นเมนูที่กดแล้วเจอ 403 ทุกปุ่ม
 *
 * แต่ไม่บล็อกจอไว้รอ: ทุกหน้าเป็น lazy ถ้าไม่ render ตัว import() ก็ไม่ถูกเรียก chunk จึงยังไม่
 * ถูกดาวน์โหลด การรอ /me ก่อนจึงทำให้ "รอ /me → โหลด chunk → หน้าดึงข้อมูลตัวเอง" เรียงต่อกัน
 * เป็นสามต่อ render จากค่าที่ cache ไว้เลยทำให้สองขั้นหลังเดินคู่ไปกับ /me โดยความปลอดภัย
 * ไม่ลดลง เพราะคนบังคับสิทธิ์จริงคือ backend ไม่ใช่เมนูฝั่งนี้
 *
 * ยังบล็อกกรณีเดียว: มี token แต่ไม่มี user ใน storage = ยังไม่รู้ role จึงไม่มี route ไหน match
 * (ดู App.tsx ที่สร้าง route ตาม role) ถ้า /me ตอบว่า role เปลี่ยนจริง store จะอัปเดตแล้ว
 * App.tsx สร้าง route tree ใหม่ ส่วน URL ที่ไม่มีใน tree ใหม่ตกไปที่ NotFoundRedirect
 *
 * 401 (token หมดอายุ / ACCOUNT_GONE): axiosClient เคลียร์ session แล้วพากลับ login ให้เอง
 * error อื่น (เน็ตหลุด, backend รีสตาร์ท): ใช้ค่าเดิมต่อ — เตะผู้ใช้ออกเพราะเน็ตกระตุกแย่กว่า
 */
export default function ProtectedRoute() {
  const token = useAuthStore((state) => state.token);
  const user = useAuthStore((state) => state.user);
  const refreshUser = useAuthStore((state) => state.refreshUser);

  const [resolved, setResolved] = useState(false);
  const startedRef = useRef(false); // ยิงครั้งเดียวต่อการเปิดเว็บ ไม่ใช่ทุกครั้งที่เปลี่ยนหน้า

  useEffect(() => {
    if (!token || startedRef.current) return;
    startedRef.current = true;

    // ไม่มี cancel flag โดยตั้งใจ: StrictMode ตอน dev รัน effect → cleanup → effect ซ้ำบน instance เดิม
    // ถ้า cleanup ตั้งธง cancel ไว้ รอบสองจะถูก startedRef กันไม่ให้ยิงใหม่ แล้วผลของรอบแรกก็ถูกทิ้ง
    // = resolved ค้าง false ตลอดกาล (setState หลัง unmount ใน React 18+ เป็น no-op เงียบๆ อยู่แล้ว)
    authApi
      .me()
      .then(refreshUser)
      .catch(() => {
        /* 401 → interceptor จัดการแล้ว ; error อื่น → ใช้ค่าเดิมต่อ */
      })
      .finally(() => setResolved(true));
  }, [token, refreshUser]);

  if (!token) {
    return <Navigate to={PATHS.login} replace />;
  }
  if (!user && !resolved) {
    return <LogoLoader fullScreen label="กำลังตรวจสอบสิทธิ์..." />;
  }

  return <Outlet />;
}
