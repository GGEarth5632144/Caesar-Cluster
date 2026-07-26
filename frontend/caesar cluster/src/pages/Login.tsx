import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Eye, EyeOff } from "lucide-react";
import { PATHS } from "@/config/routes";

import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";
import { cn } from "@/lib/utils";

const loginSchema = z.object({
  student_id: z.string().min(1, "กรุณากรอกรหัสนักศึกษา"),
  password: z.string().min(1, "กรุณากรอกรหัสผ่าน"),
  remember: z.boolean(),
});

type LoginForm = z.infer<typeof loginSchema>;

const inputClass =
  "h-11 rounded-xl border-none bg-[#F08B51] px-4 text-white placeholder:text-white/85 focus-visible:ring-white/60";

export default function Login() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.setAuth);
  const [showPassword, setShowPassword] = useState(false);

  const {
    register,
    handleSubmit,
    control,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: { student_id: "", password: "", remember: false },
  });

  const onSubmit = async (values: LoginForm) => {
    try {
      const { token, user } = await authApi.login({
        student_id: values.student_id,
        password: values.password,
        remember: values.remember,
      });
      setAuth(token, user, values.remember);
      navigate("/", { replace: true });
    } catch (err) {
      setError("root", {
        message: getApiErrorMessage(err, "เข้าสู่ระบบไม่สำเร็จ"),
      });
    }
  };

  return (
    <div className="flex min-h-screen w-full flex-col items-center bg-[#FFF8E8] px-4 py-12">
      <h1 className="text-center text-5xl font-bold text-[#211a14]">
        Caesar Cluster
      </h1>
      <p className="mt-2 text-center text-lg text-[#211a14]/70">
        Cloud for CPE Students
      </p>

      <div className="mt-10 flex w-full max-w-xl items-center gap-4 text-[#211a14]">
        <span className="h-px flex-1 bg-[#211a14]/40" />
        <span className="text-lg">Join</span>
        <span className="h-px flex-1 bg-[#211a14]/40" />
      </div>

      <form
        onSubmit={handleSubmit(onSubmit)}
        noValidate
        className="mt-6 w-full max-w-xl rounded-[2rem] bg-[#BB6653] p-8 sm:p-10"
      >
        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <Input
              placeholder="Student Number"
              className={inputClass}
              {...register("student_id")}
            />
            {errors.student_id && (
              <p className="mt-1 text-sm text-white">
                {errors.student_id.message}
              </p>
            )}
          </div>

          <div>
            <div className="relative">
              <Input
                type={showPassword ? "text" : "password"}
                placeholder="Password"
                className={`${inputClass} pr-11`}
                {...register("password")}
              />
              <button
                type="button"
                onClick={() => setShowPassword((prev) => !prev)}
                className="absolute top-1/2 right-3 -translate-y-1/2 text-white/85 hover:text-white"
                aria-label={showPassword ? "ซ่อนรหัสผ่าน" : "แสดงรหัสผ่าน"}
              >
                {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
              </button>
            </div>
            {errors.password && (
              <p className="mt-1 text-sm text-white">
                {errors.password.message}
              </p>
            )}
          </div>
        </div>

        <div className="mt-3 flex flex-col gap-2 text-sm text-white/90 sm:flex-row sm:items-center sm:justify-between">
          <label className="flex items-center gap-2">
            <Controller
              name="remember"
              control={control}
              render={({ field }) => (
                <Checkbox
                  checked={field.value}
                  onCheckedChange={(checked) => field.onChange(checked === true)}
                  className="border-white/70 bg-white/10 data-checked:bg-white data-checked:text-[#BB6653]"
                />
              )}
            />
            Remember For 30 Days
          </label>
          <Link
            to={PATHS.forgotPassword}
            className="text-left hover:underline sm:text-right"
          >
            Forgot Password
          </Link>
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
            {isSubmitting ? "กำลังเข้าสู่ระบบ..." : "Login"}
          </Button>
          <Link
            to={PATHS.register}
            className={cn(
              buttonVariants({ variant: "secondary" }),
              "h-11 w-full max-w-sm rounded-full bg-[#FBE3E6] text-base text-[#211a14] hover:bg-[#FBE3E6]/90"
            )}
          >
            Create new Account
          </Link>
        </div>
      </form>
    </div>
  );
}
