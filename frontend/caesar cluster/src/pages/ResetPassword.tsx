import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { PATHS } from "@/config/routes";
const resetSchema = z
  .object({
    new_password: z.string().min(8, "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร"),
    confirm_password: z.string().min(1, "กรุณายืนยันรหัสผ่าน"),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    message: "รหัสผ่านไม่ตรงกัน",
    path: ["confirm_password"],
  });

type ResetForm = z.infer<typeof resetSchema>;

const inputClass =
  "h-11 rounded-xl border-none bg-[#F08B51] px-4 text-white placeholder:text-white/85 focus-visible:ring-white/60";

const labelClass = "text-sm text-white/90";

export default function ResetPassword() {
  const navigate = useNavigate();
  // token มาจาก query string ของลิงก์ในอีเมล (/reset-password?token=...)
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";
  // done = ตั้งรหัสผ่านใหม่สำเร็จ → โชว์ข้อความ + ปุ่มไปหน้า login
  const [done, setDone] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ResetForm>({
    resolver: zodResolver(resetSchema),
    defaultValues: { new_password: "", confirm_password: "" },
  });

  const onSubmit = async (values: ResetForm) => {
    try {
      await authApi.resetPassword({
        token,
        new_password: values.new_password,
      });
      setDone(true);
    } catch (err) {
      setError("root", {
        message: getApiErrorMessage(
          err,
          "ตั้งรหัสผ่านใหม่ไม่สำเร็จ ลิงก์อาจหมดอายุหรือถูกใช้ไปแล้ว",
        ),
      });
    }
  };

  return (
    <div className="flex min-h-screen w-full flex-col items-center bg-[#FFF8E8] px-4 py-12">
      <h1 className="text-center text-5xl font-bold text-[#211a14]">
        Caesar Cluster
      </h1>
      <p className="mt-2 text-center text-lg text-[#211a14]/70">
        ตั้งรหัสผ่านใหม่
      </p>

      <div className="mt-10 w-full max-w-xl rounded-[2rem] bg-[#BB6653] p-8 sm:p-10">
        {!token ? (
          // เปิดหน้านี้โดยไม่มี token ในลิงก์ = ลิงก์ไม่ถูกต้อง
          <div className="flex flex-col items-center gap-6 text-center">
            <p className="text-white">
              ลิงก์รีเซ็ตรหัสผ่านไม่ถูกต้อง กรุณาขอลิงก์ใหม่อีกครั้ง
            </p>
            <Link
              to={PATHS.forgotPassword}
              className="text-sm text-white hover:underline"
            >
              ขอลิงก์รีเซ็ตรหัสผ่านใหม่
            </Link>
          </div>
        ) : done ? (
          <div className="flex flex-col items-center gap-6 text-center">
            <p className="text-white">ตั้งรหัสผ่านใหม่เรียบร้อยแล้ว</p>
            <Button
              onClick={() => navigate(PATHS.login, { replace: true })}
              className="h-11 w-full max-w-sm rounded-full bg-[#FFF8E8] text-base text-[#211a14] hover:bg-[#FFF8E8]/90"
            >
              ไปหน้าเข้าสู่ระบบ
            </Button>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} noValidate>
            <div>
              <label className={labelClass}>รหัสผ่านใหม่</label>
              <Input
                type="password"
                className={`mt-1 ${inputClass}`}
                {...register("new_password")}
              />
              {errors.new_password && (
                <p className="mt-1 text-sm text-white">
                  {errors.new_password.message}
                </p>
              )}
            </div>

            <div className="mt-5">
              <label className={labelClass}>ยืนยันรหัสผ่านใหม่</label>
              <Input
                type="password"
                className={`mt-1 ${inputClass}`}
                {...register("confirm_password")}
              />
              {errors.confirm_password && (
                <p className="mt-1 text-sm text-white">
                  {errors.confirm_password.message}
                </p>
              )}
            </div>

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
                {isSubmitting ? "กำลังบันทึก..." : "ตั้งรหัสผ่านใหม่"}
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
