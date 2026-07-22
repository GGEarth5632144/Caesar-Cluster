import { cn } from "@/lib/utils";

/**
 * Skeleton พื้นฐาน — กล่องสีโทนแบรนด์พร้อมแถบแสงกวาด (shimmer จาก .cc-skeleton ใน index.css)
 * ใช้ประกอบเป็น placeholder ของทุกหน้าในระหว่างรอโหลดข้อมูล
 */
export function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("cc-skeleton rounded-xl", className)} {...props} />
  );
}

export default Skeleton;
