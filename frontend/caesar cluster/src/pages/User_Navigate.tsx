import {
  Home,
  User,
  Settings,
  LayoutDashboard,
  Box,
  Bell,
  FileText,
} from "lucide-react";

export interface NavItem {
  label: string;
  icon: any;
  path: string; // <-- ต้องมีตัวนี้
  badge?: number; // <-- เพิ่มตัวเลือกสำหรับ badge
}

export const userNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, path: "/" },
  { label: "Profile", icon: User, path: "/profile" },
  { label: "Settings", icon: Settings, path: "/settings" },
  {
    label: "Personal Dashboard",
    icon: LayoutDashboard,
    path: "/personal-dashboard",
  },
  { label: "My VMs", icon: Box, path: "/my-vms" },
  { label: "Alerts", icon: Bell, badge: 3, path: "/alerts" },
  { label: "Request Resources", icon: FileText, path: "/request-resources" },
];

export default userNavItems;
