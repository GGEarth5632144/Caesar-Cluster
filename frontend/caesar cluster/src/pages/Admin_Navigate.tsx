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

export interface NavItem {
  label: string;
  icon: any;
  path: string; // <-- ต้องมีตัวนี้
}

export const adminNavItems: NavItem[] = [
  { label: "General Dashboard", icon: Home, path: "/"},
  { label: "Profile", icon: User, path: "/profile" },
  { label: "VM Management", icon: Server, path: "/vm-management" },
  { label: "User Management", icon: Users, path: "/user-management" },
  { label: "Make Option", icon: Sliders, path: "/make-option" },
  { label: "Alert", icon: Bell, path: "/alert" },
  { label: "Request", icon: FileText, path: "/request" },
  { label: "Audit Log", icon: ScrollText, path: "/audit-log" },
];

export default adminNavItems;
