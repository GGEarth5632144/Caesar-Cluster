import {
  Home,
  User,
  Settings,
  LayoutDashboard,
  Bell,
  FileText,
} from "lucide-react";

import type { NavItem } from "@/types/nav";

export const userNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, path: "/" },
  { label: "Profile", icon: User, path: "/profile" },
  { label: "Settings", icon: Settings, path: "/settings" },
  {
    label: "Personal Dashboard",
    icon: LayoutDashboard,
    path: "/personal-dashboard",
    requiresVm: true,
  },
  { label: "Alerts", icon: Bell, badge: 3, path: "/alerts", requiresVm: true },
  {
    label: "Request Resources",
    icon: FileText,
    path: "/request-resources",
  },
];

export default userNavItems;
