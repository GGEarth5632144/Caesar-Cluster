import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { Search, LogOut, Menu } from "lucide-react";

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

  // หุบ/ขยาย sidebar — ใช้ร่วมกันทั้ง user และ admin เพราะทั้งคู่เรียก Sidebar ตัวเดียวกันจาก DashboardLayout
  // เก็บเป็น local state เฉยๆ เพราะ layout ข้างๆ เป็น flex-1 อยู่แล้ว กว้าง/แคบตามที่ sidebar เหลือให้เองโดยอัตโนมัติ
  const [collapsed, setCollapsed] = useState(false);

  return (
    <aside
      className={cn(
        "flex h-screen shrink-0 flex-col bg-[#BB6653] text-white transition-[width] duration-200",
        collapsed ? "w-20" : "w-80"
      )}
    >
      <div
        className={cn(
          "flex items-center border-b border-white/15 py-6",
          collapsed ? "flex-col gap-3 px-3" : "gap-3 px-6"
        )}
      >
        <button
          type="button"
          onClick={() => setCollapsed((prev) => !prev)}
          className="flex size-9 shrink-0 items-center justify-center rounded-lg text-white/85 transition-colors hover:bg-white/10 hover:text-white"
          aria-label={collapsed ? "ขยาย sidebar" : "หุบ sidebar"}
          title={collapsed ? "ขยาย sidebar" : "หุบ sidebar"}
        >
          <Menu size={20} />
        </button>

        {!collapsed && (
          <Link to="/" className="flex items-center gap-3 border-white/15">
            <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-[#FFF8E8] text-base font-bold text-[#BB6653]">
              C
            </div>
            <div className="leading-tight">
              <p className="text-base font-semibold">Caesar Cluster</p>
              <p className="text-xs text-white/70">Cloud for CPE</p>
            </div>
          </Link>
        )}
      </div>

      {!collapsed && (
        <div className="px-5 py-4">
          <div className="flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2.5">
            <Search size={16} className="text-white/70" />
            <input
              placeholder="Search..."
              className="w-full bg-transparent text-sm text-white placeholder:text-white/60 outline-none"
            />
          </div>
        </div>
      )}

      <nav className={cn("flex-1 overflow-y-auto space-y-1.5 py-2", collapsed ? "px-3" : "px-4")}>
        {navItems.map((item) => {
          const Icon = item.icon;

          // เช็คว่า URL ปัจจุบันตรงกับ path ของเมนูนี้หรือไม่
          const isActive = location.pathname === item.path;

          // ลบ cursor-not-allowed ออก และเพิ่ม hover effect เข้าไปแทน
          const itemClassName = cn(
            "relative flex w-full items-center rounded-xl py-3 text-left transition-colors",
            collapsed ? "justify-center px-0" : "gap-3 px-4",
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
              title={collapsed ? item.label : undefined}
            >
              <Icon size={20} className="shrink-0" />
              {collapsed ? (
                item.badge ? (
                  <span className="absolute top-1.5 right-2.5 flex size-2 rounded-full bg-red-500" />
                ) : null
              ) : (
                <>
                  <span className="flex-1 text-base">{item.label}</span>
                  {item.badge ? (
                    <Badge className="h-6 min-w-6 justify-center bg-red-500 px-1.5 text-sm text-white">
                      {item.badge}
                    </Badge>
                  ) : null}
                </>
              )}
            </Link>
          );
        })}
      </nav>

      <div
        className={cn(
          "mt-auto shrink-0 flex items-center border-t border-white/15 py-5",
          collapsed ? "flex-col gap-3 px-3" : "gap-3 px-5"
        )}
      >
        <Avatar>
          <AvatarFallback className="bg-[#F08B51] text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
        {!collapsed && (
          <div className="min-w-0 flex-1 leading-tight">
            <p className="truncate text-base font-medium">{userName}</p>
            <p className="truncate text-xs text-white/70">{studentId}</p>
          </div>
        )}
        <button
          type="button"
          onClick={onLogout}
          className="text-white/80 hover:text-white transition-colors"
          aria-label="ออกจากระบบ"
          title={collapsed ? "ออกจากระบบ" : undefined}
        >
          <LogOut size={20} />
        </button>
      </div>
    </aside>
  );
}
