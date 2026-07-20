import { Link, useLocation } from "react-router-dom";
import { Search, LogOut } from "lucide-react";

import { cn, getInitials } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import type { NavItem } from "@/types/nav";

interface SidebarProps {
  navItems: NavItem[];
  userName: string;
  studentId: string;
  onLogout: () => void;
}

export default function Sidebar({
  navItems,
  userName,
  studentId,
  onLogout,
}: SidebarProps) {
  const initials = getInitials(userName) || "U";

  // เพิ่ม useLocation เพื่อดึง URL ปัจจุบันมาเช็คสถานะ Active
  const location = useLocation();

  return (
    <aside className="flex h-screen w-80 shrink-0 flex-col bg-[#BB6653] text-white">
      <div className="flex items-center gap-3 border-b border-white/15 px-6 py-6">
        <Link
          to="/"
          className="flex items-center gap-3 border-white/15"
        >
          <div className="flex size-11 items-center justify-center rounded-xl bg-[#FFF8E8] text-base font-bold text-[#BB6653]">
            C
          </div>
          <div className="leading-tight">
            <p className="text-base font-semibold">Caesar Cluster</p>
            <p className="text-xs text-white/70">Cloud for CPE</p>
          </div>
        </Link>
      </div>

      <div className="px-5 py-4">
        <div className="flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2.5">
          <Search size={16} className="text-white/70" />
          <input
            placeholder="Search..."
            className="w-full bg-transparent text-sm text-white placeholder:text-white/60 outline-none"
          />
        </div>
      </div>

      <nav className="flex-1 overflow-y-auto space-y-1.5 px-4 py-2">
        {navItems.map((item) => {
          const Icon = item.icon;

          // เช็คว่า URL ปัจจุบันตรงกับ path ของเมนูนี้หรือไม่
          const isActive = location.pathname === item.path;

          // ลบ cursor-not-allowed ออก และเพิ่ม hover effect เข้าไปแทน
          const itemClassName = cn(
            "flex w-full items-center gap-3 rounded-xl px-4 py-3 text-left transition-colors",
            isActive
              ? "bg-[#F08B51] text-white"
              : "text-white/85 hover:bg-white/10 hover:text-white",
          );

          return (
            // เปลี่ยนมาใช้ Link ทั้งหมด และดึง item.path มาใช้
            <Link
              key={item.label}
              to={item.path || "/"}
              className={itemClassName}
            >
              <Icon size={20} className="shrink-0" />
              <span className="flex-1 text-base">{item.label}</span>
              {item.badge ? (
                <Badge className="h-6 min-w-6 justify-center bg-red-500 px-1.5 text-sm text-white">
                  {item.badge}
                </Badge>
              ) : null}
            </Link>
          );
        })}
      </nav>

      <div className="mt-auto shrink-0 flex items-center gap-3 border-t border-white/15 px-5 py-5">
        <Avatar>
          <AvatarFallback className="bg-[#F08B51] text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1 leading-tight">
          <p className="truncate text-base font-medium">{userName}</p>
          <p className="truncate text-xs text-white/70">{studentId}</p>
        </div>
        <button
          type="button"
          onClick={onLogout}
          className="text-white/80 hover:text-white transition-colors"
          aria-label="ออกจากระบบ"
        >
          <LogOut size={20} />
        </button>
      </div>
    </aside>
  );
}
