import { useNavigate } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";

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

const inputClass =
  "h-11 rounded-xl border-none bg-[#F08B51] px-4 text-white placeholder:text-white/85 focus-visible:ring-white/60";

const labelClass = "text-sm text-white/90";

export default function Register() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.setAuth);

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

  const onSubmit = async (values: RegisterForm) => {
    try {
      await authApi.register({
        student_id: values.student_id,
        real_name: `${values.first_name} ${values.last_name}`.trim(),
        nick_name: values.last_name,
        gmail: values.gmail,
        password: values.password,
      });

      // /api/register ไม่คืน token มาด้วย เลย login ต่อทันทีเพื่อพาเข้า dashboard เลยโดยไม่ต้องกลับไปหน้า login
      const { token, user } = await authApi.login({
        student_id: values.student_id,
        password: values.password,
      });
      setAuth(token, user, true);
      navigate("/", { replace: true });
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
            <ArrowRight size={20} />
          </Button>
        </form>
      </div>

      <div className="hidden flex-1 bg-[#FFF8E8] md:block" />
    </div>
  );
}
