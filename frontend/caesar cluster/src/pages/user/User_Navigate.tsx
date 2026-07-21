import {
  Home,
  Settings,
  Box,
  Bell,
  FileText,
} from "lucide-react";

import type { NavItem } from "@/types/nav";

export const userNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, path: "/" },
  {
    label: "My Services",
    icon: Box,
    path: "/services",
    requiresVm: true,
  },
  { label: "Alerts", icon: Bell, badge: 3, path: "/alerts", requiresVm: true },
  {
    label: "My Requests",
    icon: FileText,
    path: "/request-resources",
  },
  { label: "Settings", icon: Settings, path: "/settings" }
];

export default userNavItems;
