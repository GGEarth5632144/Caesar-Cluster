import {
  Home,
  Box,
  FileText,
  Settings,
} from "lucide-react";

import type { NavItem } from "@/types/nav";
import { PATHS } from "@/config/routes"; // นำเข้า PATHS ที่เราสร้างไว้

export const userNavItems: NavItem[] = [
  // 1. ภาพรวม (Overview)
  { label: "General Dashboard", icon: Home, path: "/" },//กำลังทำ
  
  // 2. การจัดการ Service (Core Features)
  {
    label: "My Services",
    icon: Box,
    path: `/${PATHS.services}`,
    requiresVm: true
  },
  
  // 3. คำขอต่างๆ (Communication & Tracking)
  { 
    label: "My Requests", 
    icon: FileText, 
    path: `/${PATHS.requestResources}` 
  },
  
  // 4. การตั้งค่าบัญชี (System) - ไว้ล่างสุดเสมอ
  { label: "Settings", icon: Settings, path: `/${PATHS.settings}` },//กำลังทำ
];

export default userNavItems;