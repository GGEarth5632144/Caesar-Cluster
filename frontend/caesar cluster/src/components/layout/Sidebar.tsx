import { Link, useLocation } from "react-router-dom";
import { Search, LogOut } from "lucide-react";

import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
export interface NavItem {
  label: string;
  icon: any;
  path: string; // <-- ต้องมีตัวนี้
  badge?: number; // <-- เพิ่มตัวเลือกสำหรับ badge
}

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
  const initials = userName.trim().slice(0, 2).toUpperCase() || "U";
  
  // เพิ่ม useLocation เพื่อดึง URL ปัจจุบันมาเช็คสถานะ Active
  const location = useLocation();

  return (
    <aside className="flex h-screen w-64 shrink-0 flex-col bg-[#BB6653] text-white">
      <div className="flex items-center gap-2 border-b border-white/15 px-4 py-4">
        <div className="flex size-8 items-center justify-center rounded-lg bg-[#FFF8E8] text-sm font-bold text-[#BB6653]">
          C
        </div>
        <div className="leading-tight">
          <p className="text-sm font-semibold">Caesar Cluster</p>
          <p className="text-[11px] text-white/70">Cloud for CPE</p>
        </div>
      </div>

      <div className="px-4 py-3">
        <div className="flex items-center gap-2 rounded-lg bg-white/10 px-3 py-1.5">
          <Search size={14} className="text-white/70" />
          <input
            placeholder="Search..."
            className="w-full bg-transparent text-sm text-white placeholder:text-white/60 outline-none"
          />
        </div>
      </div>

      <nav className="flex-1 space-y-1 px-3 py-2">
        {navItems.map((item) => {
          const Icon = item.icon;
          
          // เช็คว่า URL ปัจจุบันตรงกับ path ของเมนูนี้หรือไม่
          const isActive = location.pathname === item.path;
          
          // ลบ cursor-not-allowed ออก และเพิ่ม hover effect เข้าไปแทน
          const itemClassName = cn(
            "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors",
            isActive
              ? "bg-[#F08B51] text-white"
              : "text-white/85 hover:bg-white/10 hover:text-white" 
          );

          return (
            // เปลี่ยนมาใช้ Link ทั้งหมด และดึง item.path มาใช้
            <Link key={item.label} to={item.path || "/"} className={itemClassName}>
              <Icon size={16} className="shrink-0" />
              <span className="flex-1 text-sm">{item.label}</span>
              {item.badge ? (
                <Badge className="h-5 min-w-5 justify-center bg-red-500 px-1 text-white">
                  {item.badge}
                </Badge>
              ) : null}
            </Link>
          );
        })}
      </nav>

      <div className="flex items-center gap-2 border-t border-white/15 px-4 py-3">
        <Avatar size="sm">
          <AvatarFallback className="bg-[#F08B51] text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1 leading-tight">
          <p className="truncate text-sm font-medium">{userName}</p>
          <p className="truncate text-[11px] text-white/70">{studentId}</p>
        </div>
        <button
          type="button"
          onClick={onLogout}
          className="text-white/80 hover:text-white transition-colors"
          aria-label="ออกจากระบบ"
        >
          <LogOut size={16} />
        </button>
      </div>
    </aside>
  );
}