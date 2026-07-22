import { cn } from "@/lib/utils";

interface LogoLoaderProps {
  /** ครอบเต็มหน้าจอ — ใช้กับ Suspense fallback ระดับ route และหน้า auth */
  fullScreen?: boolean;
  /** ข้อความใต้โลโก้ (ค่าเริ่มต้น: กำลังโหลด...) */
  label?: string;
  /** ขนาดโลโก้ */
  size?: "sm" | "md" | "lg";
  className?: string;
}

const SIZE_MAP = {
  sm: { box: "size-12", ring: "size-16", text: "text-lg", rounded: "rounded-xl" },
  md: { box: "size-16", ring: "size-24", text: "text-2xl", rounded: "rounded-2xl" },
  lg: { box: "size-20", ring: "size-32", text: "text-3xl", rounded: "rounded-3xl" },
} as const;

/**
 * โลโก้โหลดของ Caesar Cluster — วงแหวนสีส้มหมุนรอบตัวอักษร "C" พร้อมจุดกระพริบ
 * ใช้เป็น fallback ของ lazy loading และหน้าที่รอโหลดข้อมูลทั้งหน้า
 */
export default function LogoLoader({
  fullScreen = false,
  label = "กำลังโหลด...",
  size = "md",
  className,
}: LogoLoaderProps) {
  const s = SIZE_MAP[size];

  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy="true"
      className={cn(
        "flex flex-col items-center justify-center gap-6 font-mono text-[#BB6653]",
        fullScreen
          ? "fixed inset-0 z-50 bg-[#FFF8E8]"
          : "h-full min-h-[320px] w-full",
        className,
      )}
    >
      <div className={cn("relative flex items-center justify-center", s.ring)}>
        {/* วงแหวนหมุน — เส้นส้มด้านบน ที่เหลือจาง */}
        <span
          className={cn(
            "absolute inset-0 animate-spin rounded-full border-4 border-[#BB6653]/15 border-t-[#F08B51]",
          )}
          style={{ animationDuration: "0.9s" }}
        />
        {/* ตัวโลโก้ C เต้นเบาๆ */}
        <span
          className={cn(
            "cc-logo-pulse flex items-center justify-center bg-[#BB6653] font-bold text-[#FFF8E8] shadow-sm",
            s.box,
            s.rounded,
            s.text,
          )}
        >
          C
        </span>
      </div>

      <div className="flex flex-col items-center gap-2">
        <p className="text-sm font-medium text-[#211a14]/70">{label}</p>
        <div className="flex items-center gap-1.5">
          <span className="cc-logo-dot size-1.5 rounded-full bg-[#F08B51]" style={{ animationDelay: "0s" }} />
          <span className="cc-logo-dot size-1.5 rounded-full bg-[#F08B51]" style={{ animationDelay: "0.2s" }} />
          <span className="cc-logo-dot size-1.5 rounded-full bg-[#F08B51]" style={{ animationDelay: "0.4s" }} />
        </div>
      </div>

      <span className="sr-only">{label}</span>
    </div>
  );
}
