import { useEffect, useRef, useState } from "react";
import { Navigate, Outlet } from "react-router-dom";

import { useAuthStore } from "@/store/authStore";
import { authApi } from "@/api/authApi";
import LogoLoader from "@/components/ui/LogoLoader";
import { PATHS } from "@/config/routes";

/**
 * ProtectedRoute กันหน้าที่ต้อง login และตรวจ session ซ้ำกับ backend หนึ่งครั้งตอนเปิดเว็บ
 *
 * ทำไมต้องตรวจซ้ำทั้งที่มี token ใน storage แล้ว: token เก็บภาพของบัญชี ณ วินาทีที่ login
 * และอยู่ได้นานถึง 30 วัน ระหว่างนั้น role/namespace อาจเปลี่ยนไปแล้ว (ถูกเลื่อน/ถอนสิทธิ์,
 * ถูกเชิญเข้ากลุ่ม, space ถูกลบ) หรือบัญชีถูกลบไปเลย
 *
 * backend อ่านค่าพวกนี้จาก DB สดทุก request อยู่แล้ว (middlewares.Auth) จึงบล็อกถูกเสมอ
 * แต่ถ้าหน้าเว็บยังใช้ค่าเก่า ผู้ใช้จะเห็นเมนูแอดมินที่กดแล้วเจอ 403 ทุกปุ่ม ซึ่งดูเหมือนระบบพัง
 * มากกว่าดูเหมือนถูกถอนสิทธิ์ — ยิงถาม /me รอบเดียวตอนเปิดเว็บก็ตรงกันทั้งสองฝั่ง
 *
 * 401 (token หมดอายุ / ACCOUNT_GONE): axiosClient เคลียร์ session แล้วพากลับ login ให้เอง
 * error อื่น (เน็ตหลุด, backend รีสตาร์ท): ปล่อยผ่าน ใช้ค่าเดิมต่อ — การเตะผู้ใช้ออกเพราะ
 * เน็ตกระตุกหนึ่งวินาที แย่กว่าการโชว์เมนูที่อาจเก่าไปชั่วคราว
 */
export default function ProtectedRoute() {
  const token = useAuthStore((state) => state.token);
  const refreshUser = useAuthStore((state) => state.refreshUser);

  // เริ่มที่ "ยังไม่ตรวจ" เฉพาะตอนที่มี token ให้ตรวจจริงๆ — ไม่มี token ก็เด้งออกไปเลยไม่ต้องรอ
  const [checked, setChecked] = useState(!token);
  const startedRef = useRef(false); // ยิงครั้งเดียวต่อการเปิดเว็บ ไม่ใช่ทุกครั้งที่เปลี่ยนหน้า

  useEffect(() => {
    if (!token || startedRef.current) return;
    startedRef.current = true;

    // ไม่มี cancel flag ตรงนี้โดยตั้งใจ: StrictMode ตอน dev รัน effect → cleanup → effect ซ้ำบน
    // instance เดิม ถ้า cleanup ตั้งธง cancel ไว้ รอบสองจะถูก startedRef กันไม่ให้ยิงใหม่ แล้วผลลัพธ์
    // ของรอบแรกก็ถูกทิ้งเพราะธง — checked ค้าง false ตลอดกาล = ค้างหน้า loader ไม่ยอมไปไหน
    // setState หลัง unmount จริงใน React 18+ เป็น no-op เงียบๆ อยู่แล้ว ไม่ต้องกันเอง
    authApi
      .me()
      .then((fresh) => {
        refreshUser(fresh);
      })
      .catch(() => {
        /* 401 → interceptor จัดการแล้ว ; error อื่น → ใช้ค่าเดิมต่อ (ดูเหตุผลใน doc ข้างบน) */
      })
      .finally(() => {
        setChecked(true);
      });
  }, [token, refreshUser]);

  if (!token) {
    return <Navigate to={PATHS.login} replace />;
  }
  if (!checked) {
    // รอให้รู้สิทธิ์จริงก่อนค่อย render — ไม่งั้นเมนูของ role เก่าจะแวบขึ้นมาให้เห็นก่อนแล้วค่อยสลับ
    return <LogoLoader fullScreen label="กำลังตรวจสอบสิทธิ์..." />;
  }

  return <Outlet />;
}
