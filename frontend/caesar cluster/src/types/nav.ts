import type { LucideIcon } from "lucide-react";

// NavItem = ข้อมูลเมนู 1 ช่องใน Sidebar (ใช้ร่วมกันทั้ง Sidebar.tsx, User_Navigate.tsx, Admin_Navigate.tsx
// เดิมแต่ละไฟล์ประกาศ interface นี้แยกกันเอง ทำให้ต้องจำไปแก้หลายที่เวลาเพิ่ม field ใหม่)
// badgeSource บอกว่าตัวเลขในวงกลมแดงของเมนูนี้มาจากไหน
// ตอนนี้มีแหล่งเดียวคือจำนวนแจ้งเตือนที่ยังไม่ได้อ่าน (useAlertStore)
export type NavBadgeSource = "alerts";

export interface NavItem {
  label: string;
  icon: LucideIcon;
  path: string;
  // badge เป็นค่าที่ DashboardLayout เติมให้ตอน render ไม่ใช่ค่าที่เขียนตายไว้ในไฟล์เมนู
  // (ดู badgeSource) — 0 หรือ undefined = ไม่มีวงกลมแดง
  badge?: number;
  badgeSource?: NavBadgeSource;
  // true = แสดงเมนูนี้เฉพาะตอนที่ user สร้าง VM/namespace แล้วเท่านั้น
  requiresVm?: boolean;
}
