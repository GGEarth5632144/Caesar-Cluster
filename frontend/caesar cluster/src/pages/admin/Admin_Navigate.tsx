import {
  Home,
  Inbox,          // เปลี่ยนจาก FileText มาใช้ Inbox หรือ ClipboardList เพื่อสื่อถึงคำขอที่รออยู่
  Users,
  Server,
  Layers,         // แนะนำให้ใช้ Layers หรือ ServerCog แทน Service (ไม่มีใน lucide)
  Sliders,
  BellRing,       // ใช้ BellRing ให้ดูเป็นการแจ้งเตือนที่ตื่นตัวขึ้น (หรือใช้ Bell ก็ได้)
  ScrollText,
  Settings,       // เปลี่ยนจาก User เป็น Settings (รูปเฟือง)
  Upload,
} from "lucide-react";

import type { NavItem } from "@/types/nav";

export const adminNavItems: NavItem[] = [
  // 1. ภาพรวม
  { label: "General Dashboard", icon: Home, path: "/" },//กำลังทำ
  
  // 2. สิ่งที่แอดมินต้องจัดการ/ตรวจสอบเป็นอันดับแรก
  { label: "Request", icon: Inbox, path: "/admin-request" },
  { label: "Alert", icon: BellRing, path: "/alertadmin" },//กำลังทำ

  // 3. การจัดการทรัพยากรหลักในระบบ (เรียงจากคน -> เครื่อง -> เซอร์วิส -> โควตา)
  { label: "User Management", icon: Users, path: "/user-management" },
  { label: "IPC Management", icon: Server, path: "/ipc-management" },//กำลังทำ
  { label: "Services", icon: Layers, path: "/services" },//กำลังทำ
  { label: "Quota", icon: Sliders, path: "/admin-approvals" },
  { label: "Import Students", icon: Upload, path: "/admin-import-students" },

  // 4. การตรวจสอบย้อนหลัง และตั้งค่าระบบ (มักจะอยู่ล่างสุดเสมอ)
  { label: "Audit Log", icon: ScrollText, path: "/audit-log" },//กำลังทำ
  { label: "Settings", icon: Settings, path: "/settings" },//กำลังทำ
];

export default adminNavItems;