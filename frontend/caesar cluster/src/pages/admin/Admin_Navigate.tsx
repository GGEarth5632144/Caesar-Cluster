import {
  Home,
  User,
  Server,
  Users,
  Sliders,
  Bell,
  FileText,
  ScrollText,
} from "lucide-react";

import type { NavItem } from "@/types/nav";

export const adminNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, path: "/"},
  { label: "VM Management", icon: Server, path: "/vm-management" },
  { label: "User Management", icon: Users, path: "/user-management" },
  { label: "Alert", icon: Bell, path: "/alert" },
  { label: "Request", icon: FileText, path: "/admin-request" },
  { label: "Quota", icon: Sliders, path: "/admin-approvals" },
  { label: "Audit Log", icon: ScrollText, path: "/audit-log" },
  { label: "Settings", icon: User, path: "/settings" },
];

export default adminNavItems;
