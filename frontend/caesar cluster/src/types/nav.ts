import type { LucideIcon } from "lucide-react";

// NavItem = ข้อมูลเมนู 1 ช่องใน Sidebar (ใช้ร่วมกันทั้ง Sidebar.tsx, User_Navigate.tsx, Admin_Navigate.tsx
// เดิมแต่ละไฟล์ประกาศ interface นี้แยกกันเอง ทำให้ต้องจำไปแก้หลายที่เวลาเพิ่ม field ใหม่)
export interface NavItem {
  label: string;
  icon: LucideIcon;
  path: string;
  badge?: number;
  // true = แสดงเมนูนี้เฉพาะตอนที่ user สร้าง VM/namespace แล้วเท่านั้น
  requiresVm?: boolean;
}
