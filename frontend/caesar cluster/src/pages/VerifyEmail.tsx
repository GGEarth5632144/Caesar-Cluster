import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";

import { authApi, getApiErrorCode, getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";
import LogoLoader from "@/components/ui/LogoLoader";
import { notify } from "@/lib/modal";
import { PATHS } from "@/config/routes";

/**
 * VerifyEmail = ปลายทางของลิงก์ยืนยันที่ส่งไปทางอีเมล (/verify-email?token=...)
 *
 * กดครั้งแรก backend คืน session มาเลย หน้านี้จึงเก็บ session แล้วพาเข้า dashboard ตรงๆ
 * ข้อความ "ยืนยันตัวตนสำเร็จ" ขึ้นเป็น popup ทับหน้า dashboard
 *
 * ลิงก์ชี้มาที่หน้านี้ ไม่ใช่ API ตรงๆ เพราะตัวสแกนลิงก์ของ Gmail/Outlook กดให้ก่อนผู้ใช้เสมอ
 * มันโหลดได้แค่ HTML ไม่ยิง POST ต่อ ทางที่สำเร็จจึงพาออกจากหน้านี้ เหลือ render แค่ตอนกำลังตรวจกับตอนพัง
 */
export default function VerifyEmail() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.setAuth);
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const [error, setError] = useState(token ? "" : "ลิงก์ยืนยันไม่ถูกต้อง กรุณาเปิดลิงก์จากอีเมลอีกครั้ง");
  // React StrictMode เรียก effect สองรอบตอน dev — ถ้ายิงซ้ำ รอบที่สองจะได้ already: true
  // มาทับผลรอบแรก แล้วผู้ใช้จะถูกพาไปหน้า login ทั้งที่เพิ่งยืนยันสำเร็จ
  const sent = useRef(false);

  useEffect(() => {
    if (!token || sent.current) return;
    sent.current = true;

    authApi
      .verifyEmail({ token })
      .then((result) => {
        if (result.already) {
          notify.info("อีเมลนี้ยืนยันแล้ว", "เข้าสู่ระบบด้วยรหัสนักศึกษาและรหัสผ่านของคุณได้เลย");
          navigate(PATHS.login, { replace: true });
          return;
        }
        // remember = false ให้ตรงกับ session ที่ backend ออกให้ (ไม่ได้ติ๊ก "จำฉันไว้" ที่ไหน)
        setAuth(result.token, result.user, false);
        notify.success("ยืนยันตัวตนสำเร็จ", "บัญชีของคุณพร้อมใช้งานแล้ว ยินดีต้อนรับสู่ Caesar Cluster");
        navigate("/", { replace: true });
      })
      .catch((err) => {
        setError(
          getApiErrorCode(err) === "TOKEN_EXPIRED"
            ? getApiErrorMessage(err, "ลิงก์ยืนยันหมดอายุแล้ว กรุณากดขอลิงก์ใหม่")
            : getApiErrorMessage(err, "ยืนยันอีเมลไม่สำเร็จ กรุณาลองใหม่อีกครั้ง"),
        );
      });
  }, [token, setAuth, navigate]);

  if (!error) {
    return <LogoLoader fullScreen label="กำลังยืนยันอีเมลของคุณ..." />;
  }

  return (
    <div className="flex min-h-screen w-full flex-col items-center bg-[#FFF8E8] px-4 py-12">
      <h1 className="text-center text-5xl font-bold text-[#211a14]">Caesar Cluster</h1>
      <p className="mt-2 text-center text-lg text-[#211a14]/70">ยืนยันอีเมล</p>

      <div className="mt-10 flex w-full max-w-xl flex-col items-center gap-6 rounded-[2rem] bg-[#BB6653] p-8 text-center sm:p-10">
        <p className="text-white">{error}</p>
        {/* ขอลิงก์ใหม่ต้องรู้ว่าเป็นบัญชีไหน ซึ่งหน้านี้ไม่รู้ (token ที่ใช้ไม่ได้เปิดอ่านไม่ได้)
            ส่งไปหน้า login แทน — ล็อกอินแล้วจะเจอช่องขอลิงก์ใหม่อยู่ตรงนั้น */}
        <Link to={PATHS.login} className="text-sm text-white underline hover:text-white/80">
          ไปหน้าเข้าสู่ระบบเพื่อขอลิงก์ใหม่
        </Link>
      </div>

      <p className="mt-6 max-w-xl text-center text-xs text-[#211a14]/60">
        ถ้ากดปุ่มในอีเมลแล้วไม่มีอะไรเกิดขึ้น ลองคัดลอกลิงก์ทั้งบรรทัดไปวางในเบราว์เซอร์โดยตรง
      </p>
    </div>
  );
}
