import { useEffect, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";
import { clearPendingOtp, readPendingOtp, writePendingOtp } from "@/lib/otpSession";
import { PATHS } from "@/config/routes";

const otpSchema = z.object({
  code: z
    .string()
    .regex(/^[0-9]{6}$/, "กรอกรหัสยืนยัน 6 หลักจากอีเมล"),
});

type OtpForm = z.infer<typeof otpSchema>;

/** ต้องรอกี่วินาทีถึงกด "ส่งรหัสใหม่" ได้ — ต้องตรงกับ otpResendCooldown ฝั่ง backend */
const RESEND_COOLDOWN_SECONDS = 60;

/** วินาที → "m:ss" สำหรับนาฬิกาถอยหลังบนหน้าจอ */
function formatCountdown(totalSeconds: number) {
  const safe = Math.max(0, totalSeconds);
  const minutes = Math.floor(safe / 60);
  const seconds = safe % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

/**
 * VerifyOtp = หน้ากรอกรหัส 6 หลักที่ส่งไปทางอีเมล คั่นระหว่าง "รหัสผ่านถูก" กับ "ได้เข้าระบบ"
 *
 * เข้ามาที่นี่ได้สองทาง ทั้งคู่ผ่าน /api/login ที่ตอบ otp_required กลับมา:
 * สมัครสมาชิกเสร็จใหม่ๆ (Register จะ login ต่อให้อัตโนมัติ) หรือล็อกอินจากเครื่องที่ยังไม่เคยยืนยัน
 *
 * ใบยืนยันที่รออยู่ถูกเก็บใน sessionStorage (ดู lib/otpSession) ไม่ได้ส่งผ่าน route state
 * เพราะผู้ใช้กด refresh ระหว่างสลับไปเปิดอีเมลได้ — เข้ามาแล้วไม่เจอใบค้างอยู่ ก็แปลว่า
 * เดินมาผิดทาง (พิมพ์ URL เอง / ใบตายไปแล้ว) เด้งกลับหน้า login ตามปกติ
 */
export default function VerifyOtp() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.setAuth);

  // อ่านครั้งเดียวตอน mount — ไม่อ่านซ้ำทุก render เพราะระหว่างอยู่หน้านี้ค่าไม่เปลี่ยนเอง
  // (เปลี่ยนเฉพาะตอน resend สำเร็จ ซึ่งจัดการผ่าน setPending ด้านล่าง)
  const [pending, setPending] = useState(readPendingOtp);
  const [now, setNow] = useState(() => Date.now());
  const [notice, setNotice] = useState("");
  const [resending, setResending] = useState(false);
  // เริ่มนับ cooldown ทันทีที่เข้าหน้า เพราะเพิ่งมีอีเมลส่งออกไปหนึ่งฉบับก่อนจะมาถึงหน้านี้
  const [resendAt, setResendAt] = useState(() => Date.now() + RESEND_COOLDOWN_SECONDS * 1000);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<OtpForm>({
    resolver: zodResolver(otpSchema),
    defaultValues: { code: "" },
  });

  // นาฬิกาเดินวินาทีละครั้ง ใช้ขับทั้งเวลาหมดอายุของรหัสและ cooldown ของปุ่มส่งใหม่
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  if (!pending) {
    return <Navigate to={PATHS.login} replace />;
  }

  const secondsLeft = Math.ceil((pending.expiresAt - now) / 1000);
  const expired = secondsLeft <= 0;
  const resendIn = Math.ceil((resendAt - now) / 1000);

  const onSubmit = async (values: OtpForm) => {
    try {
      const { token, user } = await authApi.verifyOtp({
        challenge_token: pending.challengeToken,
        code: values.code,
      });
      // เคลียร์ใบที่รออยู่ก่อนพาเข้าระบบ ไม่งั้นกด back กลับมาจะเจอหน้ากรอกรหัสที่ใช้ไม่ได้แล้ว
      clearPendingOtp();
      setAuth(token, user, pending.remember);
      navigate("/", { replace: true });
    } catch (err) {
      setNotice("");
      setError("root", {
        message: getApiErrorMessage(err, "ยืนยันรหัสไม่สำเร็จ กรุณาลองใหม่อีกครั้ง"),
      });
    }
  };

  const onResend = async () => {
    setResending(true);
    setNotice("");
    try {
      const result = await authApi.resendOtp({ challenge_token: pending.challengeToken });
      // เขียนกลับ sessionStorage ด้วย ไม่ใช่แค่ state ในหน่วยความจำ — ถ้าผู้ใช้ refresh
      // หลังกดขอรหัสใหม่ หน้าจะได้ไม่อ่านเวลาหมดอายุอันเก่ามาแล้วขึ้นว่าหมดอายุทั้งที่รหัสยังใช้ได้
      const next = { ...pending, expiresAt: Date.now() + result.expires_in_seconds * 1000 };
      writePendingOtp(next);
      setPending(next);
      setResendAt(Date.now() + RESEND_COOLDOWN_SECONDS * 1000);
      setNotice(result.message);
    } catch (err) {
      setError("root", {
        message: getApiErrorMessage(err, "ส่งรหัสใหม่ไม่สำเร็จ กรุณาลองใหม่อีกครั้ง"),
      });
    } finally {
      setResending(false);
    }
  };

  return (
    <div className="flex min-h-screen w-full flex-col items-center bg-[#FFF8E8] px-4 py-12">
      <h1 className="text-center text-5xl font-bold text-[#211a14]">Caesar Cluster</h1>
      <p className="mt-2 text-center text-lg text-[#211a14]/70">
        {pending.fromRegister ? "ยืนยันอีเมลของคุณ" : "ยืนยันการเข้าสู่ระบบ"}
      </p>

      <div className="mt-10 w-full max-w-xl rounded-[2rem] bg-[#BB6653] p-8 sm:p-10">
        <p className="text-sm leading-relaxed text-white/90">
          {pending.fromRegister
            ? "สมัครสมาชิกเรียบร้อยแล้ว อีกขั้นเดียวเท่านั้น — เราส่งรหัสยืนยัน 6 หลักไปที่ "
            : "เราส่งรหัสยืนยัน 6 หลักไปที่ "}
          <span className="font-semibold text-white">{pending.gmailMasked}</span>{" "}
          กรอกรหัสด้านล่างเพื่อเข้าใช้งาน
        </p>

        <form onSubmit={handleSubmit(onSubmit)} noValidate className="mt-6">
          <Input
            inputMode="numeric"
            maxLength={6}
            autoFocus
            autoComplete="one-time-code"
            placeholder="000000"
            disabled={expired}
            className="h-14 rounded-xl border-none bg-[#F08B51] text-center font-mono text-3xl tracking-[0.5em] text-white placeholder:text-white/50 focus-visible:ring-white/60 disabled:opacity-60"
            {...register("code")}
          />
          {errors.code && (
            <p className="mt-2 text-center text-sm text-white">{errors.code.message}</p>
          )}

          <p className="mt-3 text-center text-sm text-white/80">
            {expired
              ? "รหัสหมดอายุแล้ว กดส่งรหัสใหม่เพื่อรับรหัสอีกครั้ง"
              : `รหัสนี้จะหมดอายุในอีก ${formatCountdown(secondsLeft)} นาที`}
          </p>

          {notice && (
            <p className="mt-4 text-center text-sm font-medium text-white">{notice}</p>
          )}
          {errors.root && (
            <p className="mt-4 text-center text-sm font-medium text-white">
              {errors.root.message}
            </p>
          )}

          <div className="mt-8 flex flex-col items-center gap-3">
            <Button
              type="submit"
              disabled={isSubmitting || expired}
              className="h-11 w-full max-w-sm rounded-full bg-[#FFF8E8] text-base text-[#211a14] hover:bg-[#FFF8E8]/90"
            >
              {isSubmitting ? "กำลังยืนยัน..." : "ยืนยันรหัส"}
            </Button>

            <Button
              type="button"
              variant="ghost"
              onClick={onResend}
              disabled={resending || resendIn > 0}
              className="h-11 w-full max-w-sm rounded-full text-sm text-white hover:bg-white/10 hover:text-white disabled:opacity-60"
            >
              {resendIn > 0 ? `ส่งรหัสใหม่ได้ในอีก ${resendIn} วินาที` : "ส่งรหัสใหม่"}
            </Button>

            <Link
              to={PATHS.login}
              onClick={clearPendingOtp}
              className="text-sm text-white/90 hover:underline"
            >
              กลับไปหน้าเข้าสู่ระบบ
            </Link>
          </div>
        </form>
      </div>

      <p className="mt-6 max-w-xl text-center text-xs text-[#211a14]/60">
        ไม่ได้รับอีเมล? ลองตรวจในกล่องจดหมายขยะ (Spam) ด้วยอีกครั้ง
      </p>
    </div>
  );
}
