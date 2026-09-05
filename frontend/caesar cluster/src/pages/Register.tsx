import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight, Loader2 } from "lucide-react";

import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { PATHS } from "@/config/routes";
import AuthHeroArt from "@/components/AuthHeroArt";
import ResendVerification from "@/components/ResendVerification";
import { cn } from "@/lib/utils";

const registerSchema = z
  .object({
    student_id: z.string().min(1, "กรุณากรอกรหัสนักศึกษา"),
    first_name: z.string().min(1, "กรุณากรอกชื่อ"),
    last_name: z.string().min(1, "กรุณากรอกนามสกุล"),
    gmail: z.string().email("อีเมลไม่ถูกต้อง"),
    password: z.string().min(8, "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร"),
    confirm_password: z.string().min(1, "กรุณายืนยันรหัสผ่าน"),
  })
  .refine((data) => data.password === data.confirm_password, {
    message: "รหัสผ่านไม่ตรงกัน",
    path: ["confirm_password"],
  });

type RegisterForm = z.infer<typeof registerSchema>;

/** ผลของการสมัครที่สำเร็จแล้ว — มีค่านี้เมื่อไรคือเปลี่ยนไปแสดงหน้า "ไปเช็คอีเมล" แทนฟอร์ม */
interface Submitted {
  gmail: string;
  message: string;
}

const inputClass =
  "h-11 rounded-xl border-none bg-[#F08B51] px-4 text-white placeholder:text-white/85 focus-visible:ring-white/60";

const labelClass = "text-sm text-white/90";

export default function Register() {
  const navigate = useNavigate();
  // สมัครเสร็จแล้วไม่ได้พาเข้า dashboard ทันทีเหมือนเดิม เพราะบัญชียังใช้ไม่ได้จนกว่าจะกดลิงก์ในอีเมล
  const [submitted, setSubmitted] = useState<Submitted | null>(null);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      student_id: "",
      first_name: "",
      last_name: "",
      gmail: "",
      password: "",
      confirm_password: "",
    },
  });

  // ไม่ login ต่อให้อัตโนมัติแล้ว — เดิมที่นี่ยิง /api/login ต่อเพื่อขอ OTP ซึ่งเป็นต้นตอของบั๊ก:
  // ขั้นนั้นพังเมื่อไร ผู้ใช้เห็น "สมัครไม่สำเร็จ" ทั้งที่บัญชีถูกสร้างไปแล้ว
  const onSubmit = async (values: RegisterForm) => {
    try {
      const result = await authApi.register({
        student_id: values.student_id,
        real_name: `${values.first_name} ${values.last_name}`.trim(),
        nick_name: values.last_name,
        gmail: values.gmail,
        password: values.password,
      });
      // ไม่มี popup แล้ว — แผงด้านล่างบอกว่าลิงก์ถูกส่งไปที่ไหน และเป็นที่ที่ผู้ใช้ลงมือต่อได้ถ้าเมลไม่มา
      setSubmitted({ gmail: result.gmail, message: result.message });
    } catch (err) {
      setError("root", {
        message: getApiErrorMessage(err, "สมัครสมาชิกไม่สำเร็จ"),
      });
    }
  };

  return (
    <div className="flex min-h-screen w-full">
      <div className="flex w-full flex-col bg-[#BB6653] px-6 py-10 sm:px-16 sm:py-14 md:w-1/2 lg:w-2/5">
        <h1 className="text-4xl font-bold text-[#FFF8E8]">Caesar Cluster</h1>

        {submitted ? (
          <div className="mt-10 flex flex-col gap-5">
            <h2 className="text-xl font-semibold text-[#FFF8E8]">ตรวจสอบอีเมลของคุณ</h2>
            <p className="text-sm leading-relaxed text-white/90">
              เราส่งลิงก์ยืนยันไปที่{" "}
              <span className="font-semibold text-white">{submitted.gmail}</span> แล้ว
              กดลิงก์ในอีเมลเพื่อเปิดใช้งานบัญชี จากนั้นจึงเข้าสู่ระบบได้
            </p>
            <p className="text-sm text-white/70">
              ไม่พบอีเมล? ลองตรวจในกล่องจดหมายขยะ (Spam) ก่อน แล้วค่อยกดขอลิงก์ใหม่
            </p>

            <Button
              onClick={() => navigate(PATHS.login, { replace: true })}
              className="mt-2 h-12 w-full rounded-full bg-[#FFF8E8] text-[#211a14] hover:bg-[#FFF8E8]/90"
            >
              ไปหน้าเข้าสู่ระบบ
            </Button>

            <ResendVerification gmail={submitted.gmail} />
          </div>
        ) : (
          <form
            onSubmit={handleSubmit(onSubmit)}
            noValidate
            className="mt-10 flex flex-col gap-5"
          >
            <div>
              <label className={labelClass}>Student Number</label>
              <Input className={`mt-1 ${inputClass}`} {...register("student_id")} />
              {errors.student_id && (
                <p className="mt-1 text-sm text-white">{errors.student_id.message}</p>
              )}
            </div>

            <div>
              <label className={labelClass}>First Name</label>
              <Input className={`mt-1 ${inputClass}`} {...register("first_name")} />
              {errors.first_name && (
                <p className="mt-1 text-sm text-white">{errors.first_name.message}</p>
              )}
            </div>

            <div>
              <label className={labelClass}>Last Name</label>
              <Input className={`mt-1 ${inputClass}`} {...register("last_name")} />
              {errors.last_name && (
                <p className="mt-1 text-sm text-white">{errors.last_name.message}</p>
              )}
            </div>

            <div>
              <label className={labelClass}>Gmail</label>
              <Input type="email" className={`mt-1 ${inputClass}`} {...register("gmail")} />
              {errors.gmail && (
                <p className="mt-1 text-sm text-white">{errors.gmail.message}</p>
              )}
            </div>

            <div>
              <label className={labelClass}>Password</label>
              <Input
                type="password"
                className={`mt-1 ${inputClass}`}
                {...register("password")}
              />
              {errors.password && (
                <p className="mt-1 text-sm text-white">{errors.password.message}</p>
              )}
            </div>

            <div>
              <label className={labelClass}>Confirm Password</label>
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
              <p className="text-center text-sm font-medium text-white">
                {errors.root.message}
              </p>
            )}

            <Button
              type="submit"
              disabled={isSubmitting}
              className="mt-4 h-12 w-full rounded-full bg-[#FFF8E8] text-[#211a14] hover:bg-[#FFF8E8]/90"
            >
              {isSubmitting ? (
                <>
                  <Loader2 size={18} className="animate-spin" />
                  กำลังตรวจสอบข้อมูล...
                </>
              ) : (
                <ArrowRight size={20} />
              )}
            </Button>

            {/* บอกไว้ตรงๆ ว่าทำไมช้า: backend รอผลการส่งอีเมลจริงก่อนตอบกลับ (สูงสุด ~15 วิ)
                ไม่งั้นผู้ใช้จะคิดว่าปุ่มค้างแล้วกดซ้ำ */}
            {isSubmitting && (
              <p className="text-center text-xs text-white/70">
                กำลังตรวจสอบสิทธิ์และส่งอีเมลยืนยัน อาจใช้เวลาสักครู่
              </p>
            )}

            {/* ทางกลับหน้า login — คู่กับปุ่ม "Create new Account" ที่หน้า Login ใช้สไตล์เดียวกัน
                เป็น Link ไม่ใช่ Button เพื่อให้คลิกขวา/เปิดแท็บใหม่ได้ตามปกติของลิงก์จริง */}
            <Link
              to={PATHS.login}
              className={cn(
                buttonVariants({ variant: "secondary" }),
                "h-12 w-full rounded-full bg-[#FBE3E6] text-base text-[#211a14] hover:bg-[#FBE3E6]/90",
              )}
            >
              มีบัญชีอยู่แล้ว? เข้าสู่ระบบ
            </Link>

            <p className="text-center text-xs text-white/70">
              การสมัครสมาชิกถือว่าคุณยอมรับ{" "}
              <Link to={PATHS.terms} className="underline hover:text-white">
                ข้อกำหนดการให้บริการ
              </Link>{" "}
              ของ Caesar Cluster
            </p>
          </form>
        )}
      </div>

      <div className="hidden flex-1 md:block">
        <AuthHeroArt />
      </div>
    </div>
  );
}
