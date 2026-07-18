import {
  User,
  Server,
  Users,
  Sliders,
  Bell,
  FileText,
  ScrollText,
} from "lucide-react";

import type { NavItem } from "./User_Navigate";

export const adminNavItems: NavItem[] = [
  { label: "Profile", icon: User },
  { label: "VM Management", icon: Server },
  { label: "User Management", icon: Users },
  { label: "Make Option", icon: Sliders },
  { label: "Alert", icon: Bell },
  { label: "Request", icon: FileText },
  { label: "Audit Log", icon: ScrollText },
];

export default adminNavItems;
