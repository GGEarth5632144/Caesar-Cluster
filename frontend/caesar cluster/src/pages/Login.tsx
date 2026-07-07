import { z } from "zod";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// โหลด Field แบบใหม่จาก shadcn
import { Field, FieldLabel, FieldError } from "@/components/ui/field";

// 1. สร้างกฎด้วย Zod เหมือนเดิม
const loginSchema = z.object({
  email: z.string().email({ message: "รูปแบบอีเมลไม่ถูกต้อง" }),
  password: z.string().min(8, { message: "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร" }),
});

type LoginFormValues = z.infer<typeof loginSchema>;

export default function Login() {
  const form = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  const onSubmit = (data: LoginFormValues) => {
    console.log("ข้อมูลพร้อมส่งไป API:", data);
  };

  return (
    <div className="flex justify-center items-center min-h-screen p-4">
      <div className="w-full max-w-md p-8 border rounded-xl bg-card shadow-sm">
        <h1 className="text-2xl font-bold mb-6 text-center">เข้าสู่ระบบ</h1>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
          
          {/* ช่องกรอกอีเมล (แบบใหม่) */}
          <Field data-invalid={!!form.formState.errors.email}>
            <FieldLabel htmlFor="email">อีเมล</FieldLabel>
            <Input 
              id="email" 
              placeholder="name@example.com" 
              aria-invalid={!!form.formState.errors.email}
              {...form.register("email")} // ผูกข้อมูลตรงๆ แบบนี้เลย
            />
            {/* แสดง Error แบบนี้แทน */}
            {form.formState.errors.email && (
              <FieldError>{form.formState.errors.email.message}</FieldError>
            )}
          </Field>

          {/* ช่องกรอกรหัสผ่าน (แบบใหม่) */}
          <Field data-invalid={!!form.formState.errors.password}>
            <FieldLabel htmlFor="password">รหัสผ่าน</FieldLabel>
            <Input 
              id="password" 
              type="password" 
              placeholder="••••••••" 
              aria-invalid={!!form.formState.errors.password}
              {...form.register("password")} 
            />
            {form.formState.errors.password && (
              <FieldError>{form.formState.errors.password.message}</FieldError>
            )}
          </Field>

          <Button type="submit" className="w-full">
            เข้าสู่ระบบ
          </Button>
        </form>
      </div>
    </div>
  );
}

export { Login };