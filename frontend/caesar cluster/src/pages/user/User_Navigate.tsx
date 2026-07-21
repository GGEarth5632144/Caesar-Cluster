import {
  Home,
  PlusCircle,    // เพิ่มไอคอนสำหรับ Create Service
  Box,
  FileText,
  Bell,
  Settings,
} from "lucide-react";

import type { NavItem } from "@/types/nav";

export const userNavItems: NavItem[] = [
  // 1. ภาพรวม (Overview)
  { label: "General Dashboard", icon: Home, path: "/" },//กำลังทำ
  
  // 2. การจัดการ Service (Core Features) - เอาไว้หมวดเดียวกัน
  { 
    label: "Create Service", 
    icon: PlusCircle, 
    path: "/create-service", 
    requiresVm: true 
  },//กำลังทำ
  { 
    label: "My Services", 
    icon: Box, 
    path: "/services", 
    requiresVm: true 
  },//กำลังทำ
  
  // 3. การแจ้งเตือน & คำขอต่างๆ (Communication & Tracking)
  { 
    label: "My Requests", 
    icon: FileText, 
    path: "/request-resources" 
  },
  { 
    label: "Alerts", 
    icon: Bell, 
    badge: 3, 
    path: "/alertuser", 
    requiresVm: true 
  },//กำลังทำ
  
  // 4. การตั้งค่าบัญชี (System) - ไว้ล่างสุดเสมอ
  { label: "Settings", icon: Settings, path: "/settings" },//กำลังทำ
  
];

export default userNavItems;