import type { LucideIcon } from "lucide-react";
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
  icon: LucideIcon;
  badge?: number;
  /** true = มีหน้าจริงให้กดไปแล้ว, false = ยังเป็นแค่ placeholder ในเมนู */
  active?: boolean;
}

export const userNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, active: true },
  { label: "Profile", icon: User },
  { label: "Settings", icon: Settings },
  { label: "Personal Dashboard", icon: LayoutDashboard },
  { label: "My VMs", icon: Box },
  { label: "Alerts", icon: Bell, badge: 3 },
  { label: "Request Resources", icon: FileText },
];

export default userNavItems;
