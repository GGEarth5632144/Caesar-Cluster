import { useEffect, useRef, useState } from "react";
import { Navigate, Outlet } from "react-router-dom";

import { useAuthStore } from "@/store/authStore";
import { authApi } from "@/api/authApi";
import LogoLoader from "@/components/ui/LogoLoader";
import { PATHS } from "@/config/routes";

/**
 * ProtectedRoute กันหน้าที่ต้อง login และ "ตรวจสอบ session ซ้ำกับ backend" หนึ่งครั้งตอนเปิดเว็บ
 *
 * ทำไมต้องตรวจซ้ำ ทั้งที่มี token อยู่ใน storage แล้ว:
 * token เก็บภาพของบัญชี ณ วินาทีที่ login และอยู่ได้นานถึง 30 วันถ้าติ๊ก Remember
 * ระหว่างนั้น role หรือ namespace อาจเปลี่ยนไปแล้ว (แอดมินเลื่อน/ถอนสิทธิ์, ถูกเชิญเข้ากลุ่ม,
 * space ถูกลบ) หรือบัญชีอาจถูกลบไปเลย
 *
 * ฝั่ง backend อ่านค่าพวกนี้จาก DB สดทุก request อยู่แล้ว (ดู middlewares.Auth) จึงบล็อกได้ถูกต้อง
 * เสมอ แต่ถ้าหน้าเว็บยังใช้ค่าเก่าใน storage ต่อไป ผู้ใช้จะเห็นเมนูแอดมินที่กดแล้วเจอ 403 ทุกปุ่ม
 * ซึ่งดูเหมือนระบบพังมากกว่าดูเหมือนถูกถอนสิทธิ์ — ยิงถาม /me รอบเดียวตอนเปิดเว็บก็ตรงกันทั้งสองฝั่ง
 *
 * กรณีที่ตอบ 401 (token หมดอายุ / บัญชีถูกลบ = ACCOUNT_GONE) axiosClient จะเคลียร์ session
 * แล้วพากลับหน้า login ให้เองผ่าน interceptor ที่นี่ไม่ต้องจัดการซ้ำ
 *
 * ส่วน error อื่น (เน็ตหลุด, backend รีสตาร์ทอยู่) ตั้งใจ "ปล่อยผ่าน" ใช้ค่าเดิมใน storage ต่อ —
 * การเตะผู้ใช้ออกจากระบบเพราะเน็ตกระตุกหนึ่งวินาที แย่กว่าการโชว์เมนูที่อาจเก่าไปชั่วคราว
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

    let cancelled = false;
    authApi
      .me()
      .then((fresh) => {
        if (!cancelled) refreshUser(fresh);
      })
      .catch(() => {
        /* 401 → interceptor จัดการแล้ว ; error อื่น → ใช้ค่าเดิมต่อ (ดูเหตุผลใน doc ข้างบน) */
      })
      .finally(() => {
        if (!cancelled) setChecked(true);
      });

    return () => {
      cancelled = true;
    };
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
