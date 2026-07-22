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
import { PATHS } from "@/config/routes"; // นำเข้า PATHS

export const adminNavItems: NavItem[] = [
  // 1. ภาพรวม
  { label: "General Dashboard", icon: Home, path: "/" },//กำลังทำ
  
  // 2. สิ่งที่แอดมินต้องจัดการ/ตรวจสอบเป็นอันดับแรก
  { label: "Request", icon: Inbox, path: `/${PATHS.adminRequest}` },
  { label: "Alert", icon: BellRing, path: `/${PATHS.alertadmin}` },//กำลังทำ

  // 3. การจัดการทรัพยากรหลักในระบบ (เรียงจากคน -> เครื่อง -> เซอร์วิส -> โควตา)
  { label: "User Management", icon: Users, path: `/${PATHS.userManagement}` },
  { label: "IPC Management", icon: Server, path: `/${PATHS.ipcManagement}` },//กำลังทำ
  { label: "Services", icon: Layers, path: `/${PATHS.services}` },//กำลังทำ
  { label: "Quota", icon: Sliders, path: `/${PATHS.adminApprovals}` },
  { label: "Import Students", icon: Upload, path: `/${PATHS.adminImportStudents}` },

  // 4. การตรวจสอบย้อนหลัง และตั้งค่าระบบ (มักจะอยู่ล่างสุดเสมอ)
  { label: "Audit Log", icon: ScrollText, path: `/${PATHS.auditLog}` },//กำลังทำ
  { label: "Settings", icon: Settings, path: `/${PATHS.settings}` },//กำลังทำ
];

export default adminNavItems;