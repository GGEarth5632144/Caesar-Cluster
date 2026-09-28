import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { getApiErrorMessage } from "@/api/authApi";

/** ต้องรอกี่วินาทีถึงกดขอลิงก์ใหม่ได้ — ต้องตรงกับ verificationResendCooldown ฝั่ง backend */
const RESEND_COOLDOWN_SECONDS = 60;

interface Props {
  /**
   * ยิงขอลิงก์ใบใหม่แล้วคืนข้อความที่จะโชว์ใต้ปุ่ม — แต่ละหน้าส่งคนละ endpoint:
   * หน้าสมัครส่งด้วยอีเมลที่เพิ่งกรอก, หน้าล็อกอินส่งด้วยรหัสนักศึกษา+รหัสผ่าน (ส่งไปอีเมลที่ใช้สมัครเสมอ)
   */
  send: () => Promise<string>;
  /**
   * เริ่มนับ cooldown ทันทีที่ปุ่มโผล่ไหม — true เมื่อเพิ่งมีอีเมลถูกส่งออกไป (หลังกดสมัคร)
   * หน้าล็อกอินใช้ false: ผู้ใช้อาจสมัครไว้หลายชั่วโมงแล้ว บังคับให้รอ 60 วิ ไม่มีเหตุผล
   */
  startCooldown?: boolean;
  className?: string;
}

/**
 * ResendVerification = ปุ่ม "ส่งลิงก์ยืนยันใหม่" พร้อมนาฬิกา cooldown
 *
 * แยกเป็น component เพราะหน้าสมัครกับหน้าล็อกอินต้องการมันเหมือนกันเป๊ะ ถ้าต่างคนต่างทำ
 * วันหนึ่งจะมีฝั่งที่ลืมนับ cooldown แล้วผู้ใช้กดรัวจนโดน 429 โดยไม่เข้าใจว่าทำไม
 */
export default function ResendVerification({ send, startCooldown = true, className = "" }: Props) {
  const [sending, setSending] = useState(false);
  const [notice, setNotice] = useState("");
  const [readyAt, setReadyAt] = useState(() =>
    startCooldown ? Date.now() + RESEND_COOLDOWN_SECONDS * 1000 : 0,
  );
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const secondsLeft = Math.ceil((readyAt - now) / 1000);

  const onResend = async () => {
    setSending(true);
    setNotice("");
    try {
      setNotice(await send());
    } catch (err) {
      // รวม 429 (กดเร็วกว่า cooldown ฝั่ง backend) — ข้อความจาก backend อธิบายให้อยู่แล้ว
      setNotice(getApiErrorMessage(err, "ส่งลิงก์ใหม่ไม่สำเร็จ กรุณาลองใหม่อีกครั้ง"));
    } finally {
      setReadyAt(Date.now() + RESEND_COOLDOWN_SECONDS * 1000);
      setSending(false);
    }
  };

  return (
    <div className={`flex flex-col items-center gap-2 ${className}`}>
      <Button
        type="button"
        variant="ghost"
        onClick={onResend}
        disabled={sending || secondsLeft > 0}
        className="h-11 w-full max-w-sm rounded-full text-sm text-white hover:bg-white/10 hover:text-white disabled:opacity-60"
      >
        {secondsLeft > 0
          ? `ขอลิงก์ยืนยันใหม่ได้ในอีก ${secondsLeft} วินาที`
          : sending
            ? "กำลังส่ง..."
            : "ส่งลิงก์ยืนยันใหม่"}
      </Button>
      {notice && <p className="text-center text-sm text-white/90">{notice}</p>}
    </div>
  );
}
