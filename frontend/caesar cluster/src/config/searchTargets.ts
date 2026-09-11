import {
  Box,
  Boxes,
  FileText,
  Home,
  Inbox,
  Layers,
  PlusCircle,
  ScrollText,
  Server,
  Settings,
  Sliders,
  Upload,
  Users,
  type LucideIcon,
} from "lucide-react";

import { PATHS } from "@/config/routes";

// รายชื่อ "ที่ที่กระโดดไปได้" ของช่องค้นหา
//
// ไม่ใช้ navItems ตรงๆ เพราะเมนูข้างมีแค่หน้าหลัก แต่ผู้ใช้มักค้นหาหน้าที่ไม่ได้อยู่ในเมนู
// (เช่น Create Service ที่เข้าได้จากปุ่มในหน้า My Services เท่านั้น) และค้นด้วยคำที่ไม่ใช่
// ชื่อเมนูเป๊ะๆ เช่น พิมพ์ "โควตา" หรือ "quota" แล้วอยากเจอหน้า Request

export interface SearchTarget {
  label: string;
  path: string;
  icon: LucideIcon;
  /** หมวดที่โชว์คั่นในรายการผลลัพธ์ */
  group: string;
  /** คำอื่นที่ค้นแล้วต้องเจอหน้านี้ — ใส่ทั้งไทยและอังกฤษ */
  keywords: string[];
  /** คำอธิบายสั้นๆ ใต้ชื่อหน้า */
  description?: string;
  /** true = หน้านี้เปิดได้เฉพาะผู้ใช้ที่มี namespace แล้ว */
  requiresVm?: boolean;
}

export const userSearchTargets: SearchTarget[] = [
  {
    label: "General Dashboard",
    path: "/",
    icon: Home,
    group: "ภาพรวม",
    keywords: ["dashboard", "overview", "home", "หน้าแรก", "ภาพรวม", "แดชบอร์ด"],
    description: "สรุปการใช้ทรัพยากรของกลุ่ม",
  },
  {
    label: "My Services",
    path: `/${PATHS.services}`,
    icon: Box,
    group: "บริการ",
    keywords: ["service", "deploy", "container", "pod", "image", "บริการ", "เซอร์วิส"],
    description: "บริการที่รันอยู่ในเนมสเปซของคุณ",
    requiresVm: true,
  },
  {
    label: "Create Service",
    path: `/${PATHS.createService}`,
    icon: PlusCircle,
    group: "บริการ",
    keywords: ["create", "new", "deploy", "สร้าง", "เพิ่ม", "บริการใหม่"],
    description: "สร้างบริการใหม่จาก image",
    requiresVm: true,
  },
  {
    label: "My Requests",
    path: `/${PATHS.requestResources}`,
    icon: FileText,
    group: "คำขอ",
    keywords: ["request", "quota", "resource", "คำขอ", "โควตา", "ทรัพยากร", "ขอเพิ่ม"],
    description: "สถานะคำขอทรัพยากรที่ยื่นไว้",
  },
  {
    label: "Settings",
    path: `/${PATHS.settings}`,
    icon: Settings,
    group: "ระบบ",
    keywords: ["setting", "profile", "password", "account", "ตั้งค่า", "โปรไฟล์", "รหัสผ่าน"],
    description: "ข้อมูลบัญชีและรหัสผ่าน",
  },
];

export const adminSearchTargets: SearchTarget[] = [
  {
    label: "General Dashboard",
    path: "/",
    icon: Home,
    group: "ภาพรวม",
    keywords: ["dashboard", "overview", "home", "หน้าแรก", "ภาพรวม", "แดชบอร์ด"],
    description: "สถานะคลัสเตอร์และคำขอที่ค้างอยู่",
  },
  {
    label: "Request Resource",
    path: `/${PATHS.adminRequest}`,
    icon: Inbox,
    group: "คำขอ",
    keywords: ["template", "option", "preset", "แม่แบบ", "ตัวเลือก", "คำขอ"],
    description: "แม่แบบทรัพยากรที่ให้ผู้ใช้เลือกตอนยื่นคำขอ",
  },
  {
    label: "Quota",
    path: `/${PATHS.adminApprovals}`,
    icon: Sliders,
    group: "คำขอ",
    keywords: ["approve", "deny", "queue", "pending", "อนุมัติ", "ปฏิเสธ", "คิว", "โควตา"],
    description: "คิวคำขอที่รออนุมัติ",
  },
  {
    label: "User Management",
    path: `/${PATHS.userManagement}`,
    icon: Users,
    group: "ผู้ใช้",
    keywords: ["user", "student", "account", "role", "ผู้ใช้", "นักศึกษา", "บัญชี", "สิทธิ์"],
    description: "รายชื่อผู้ใช้และสิทธิ์",
  },
  {
    label: "Namespace Management",
    path: `/${PATHS.namespaceManagement}`,
    icon: Boxes,
    group: "ผู้ใช้",
    keywords: [
      "namespace",
      "quota",
      "limit",
      "resource",
      "cpu",
      "ram",
      "memory",
      "space",
      "group",
      "เนมสเปซ",
      "โควตา",
      "ทรัพยากร",
      "เพดาน",
      "กลุ่ม",
      "ซีพียู",
      "แรม",
    ],
    description: "โควตาและสมาชิกของแต่ละเนมสเปซ",
  },
  {
    label: "Import Students",
    path: `/${PATHS.adminImportStudents}`,
    icon: Upload,
    group: "ผู้ใช้",
    keywords: ["import", "csv", "upload", "eligible", "นำเข้า", "อัปโหลด", "รายชื่อ"],
    description: "นำเข้ารายชื่อผู้มีสิทธิ์ใช้งาน",
  },
  {
    label: "IPC Management",
    path: `/${PATHS.ipcManagement}`,
    icon: Server,
    group: "คลัสเตอร์",
    keywords: ["node", "cluster", "power", "watt", "telemetry", "โหนด", "เครื่อง", "พลังงาน"],
    description: "สถานะโหนดและการใช้พลังงาน",
  },
  {
    label: "Services",
    path: `/${PATHS.services}`,
    icon: Layers,
    group: "คลัสเตอร์",
    keywords: ["service", "deploy", "container", "บริการ", "เซอร์วิส"],
    description: "บริการทั้งหมดในคลัสเตอร์",
  },
  {
    label: "Audit Log",
    path: `/${PATHS.auditLog}`,
    icon: ScrollText,
    group: "ระบบ",
    keywords: ["audit", "log", "history", "บันทึก", "ประวัติ", "ล็อก"],
    description: "ประวัติการกระทำในระบบ",
  },
  {
    label: "Settings",
    path: `/${PATHS.settings}`,
    icon: Settings,
    group: "ระบบ",
    keywords: ["setting", "profile", "password", "account", "ตั้งค่า", "โปรไฟล์", "รหัสผ่าน"],
    description: "ข้อมูลบัญชีและรหัสผ่าน",
  },
];

export function searchTargetsFor(isAdmin: boolean, hasVm: boolean): SearchTarget[] {
  const targets = isAdmin ? adminSearchTargets : userSearchTargets;
  return targets.filter((t) => !t.requiresVm || hasVm);
}
