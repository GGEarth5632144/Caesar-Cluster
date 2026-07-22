import { useState } from "react";
import { Link } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { PATHS } from "@/config/routes";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";

const forgotSchema = z.object({
  gmail: z.string().email("อีเมลไม่ถูกต้อง"),
});

type ForgotForm = z.infer<typeof forgotSchema>;

const inputClass =
  "h-11 rounded-xl border-none bg-[#F08B51] px-4 text-white placeholder:text-white/85 focus-visible:ring-white/60";

export default function ForgotPassword() {
  // sent = ส่งคำขอสำเร็จแล้ว → สลับไปโชว์ข้อความ inline แทนฟอร์ม (ไม่ redirect ไปไหน)
  const [sent, setSent] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ForgotForm>({
    resolver: zodResolver(forgotSchema),
    defaultValues: { gmail: "" },
  });

  const onSubmit = async (values: ForgotForm) => {
    try {
      await authApi.forgotPassword({ gmail: values.gmail });
      setSent(true);
    } catch (err) {
      setError("root", {
        message: getApiErrorMessage(err, "ส่งคำขอไม่สำเร็จ กรุณาลองใหม่อีกครั้ง"),
      });
    }
  };

  return (
    <div className="flex min-h-screen w-full flex-col items-center bg-[#FFF8E8] px-4 py-12">
      <h1 className="text-center text-5xl font-bold text-[#211a14]">
        Caesar Cluster
      </h1>
      <p className="mt-2 text-center text-lg text-[#211a14]/70">ลืมรหัสผ่าน?</p>

      <div className="mt-10 w-full max-w-xl rounded-[2rem] bg-[#BB6653] p-8 sm:p-10">
        {sent ? (
          <div className="flex flex-col items-center gap-6 text-center">
            <p className="text-white">
              ถ้ามีบัญชีที่ใช้อีเมลนี้ เราได้ส่งลิงก์รีเซ็ตรหัสผ่านไปให้แล้ว
              กรุณาตรวจสอบกล่องอีเมลของคุณ
            </p>
            <Link to={PATHS.login} className="text-sm text-white hover:underline">
              กลับไปหน้าเข้าสู่ระบบ
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} noValidate>
            <p className="mb-5 text-sm text-white/90">
              กรอกอีเมลที่ใช้สมัคร เราจะส่งลิงก์สำหรับตั้งรหัสผ่านใหม่ไปให้
            </p>

            <Input
              type="email"
              placeholder="Gmail"
              className={inputClass}
              {...register("gmail")}
            />
            {errors.gmail && (
              <p className="mt-1 text-sm text-white">{errors.gmail.message}</p>
            )}

            {errors.root && (
              <p className="mt-4 text-center text-sm font-medium text-white">
                {errors.root.message}
              </p>
            )}

            <div className="mt-8 flex flex-col items-center gap-3">
              <Button
                type="submit"
                disabled={isSubmitting}
                className="h-11 w-full max-w-sm rounded-full bg-[#FFF8E8] text-base text-[#211a14] hover:bg-[#FFF8E8]/90"
              >
                {isSubmitting ? "กำลังส่ง..." : "ส่งลิงก์รีเซ็ตรหัสผ่าน"}
              </Button>
              <Link
                to={PATHS.login}
                className="text-sm text-white/90 hover:underline"
              >
                กลับไปหน้าเข้าสู่ระบบ
              </Link>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
