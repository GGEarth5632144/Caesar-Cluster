import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Search, LogOut, Menu, X } from "lucide-react";

import { cn, getInitials } from "@/lib/utils";
import { fuzzyScore } from "@/lib/search";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Highlight } from "@/components/ui/highlight";
import { adminSearchTargets, userSearchTargets } from "@/config/searchTargets";
import type { NavItem } from "@/types/nav";

interface SidebarProps {
  navItems: NavItem[];
  userName: string;
  studentId: string;
  onLogout: () => void;
}

// คำพ้องของแต่ละหน้า อ้างจากรายการเดียวกับที่ Topbar ใช้ เพื่อให้พิมพ์คำเดียวกันแล้วเจอเหมือนกัน
// ไม่ว่าจะพิมพ์ในช่องบนหรือช่องข้าง (เช่น "โควตา" ต้องเจอ Quota ทั้งสองที่)
const keywordsByPath = new Map<string, string[]>(
  [...userSearchTargets, ...adminSearchTargets].map((t) => [t.path, t.keywords]),
);

export default function Sidebar({ navItems, userName, studentId, onLogout }: SidebarProps) {
  const initials = getInitials(userName) || "U";

  const location = useLocation();
  const navigate = useNavigate();

  // หุบ/ขยาย sidebar — ใช้ร่วมกันทั้ง user และ admin เพราะทั้งคู่เรียก Sidebar ตัวเดียวกันจาก DashboardLayout
  // เก็บเป็น local state เฉยๆ เพราะ layout ข้างๆ เป็น flex-1 อยู่แล้ว กว้าง/แคบตามที่ sidebar เหลือให้เองโดยอัตโนมัติ
  const [collapsed, setCollapsed] = useState(false);

  // ช่องค้นหาของ sidebar ทำหน้าที่เดียว: กรองเมนูให้เหลือเฉพาะที่ตรงคำ แล้วกด Enter เพื่อไป
  // (คนละเรื่องกับช่องบน Topbar ที่ค้น "ข้อมูลในหน้า" — แยกหน้าที่กันชัดเจนจะได้ไม่สับสน)
  const [navQuery, setNavQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const filteredItems = useMemo(() => {
    const trimmed = navQuery.trim();
    if (!trimmed) return navItems;

    return navItems
      .map((item) => {
        const haystacks = [item.label, ...(keywordsByPath.get(item.path) ?? [])];
        // คะแนนของเมนู = คำที่ตรงที่สุด ไม่ใช่ผลรวม เมนูที่มีคำพ้องเยอะจะได้ไม่ชนะฟรีๆ
        const score = Math.max(...haystacks.map((text) => fuzzyScore(text, trimmed)));
        return { item, score };
      })
      .filter((entry) => entry.score > 0)
      .sort((a, b) => b.score - a.score)
      .map((entry) => entry.item);
  }, [navItems, navQuery]);

  // พิมพ์คำใหม่แล้วรายการเปลี่ยน ตัวชี้ต้องกลับไปตัวแรกเสมอ ไม่งั้นจะชี้ค้างเมนูที่หายไปแล้ว
  useEffect(() => setActiveIndex(0), [navQuery]);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      setNavQuery("");
      inputRef.current?.blur();
      return;
    }
    if (event.key === "ArrowDown" && filteredItems.length > 0) {
      event.preventDefault();
      setActiveIndex((prev) => (prev + 1) % filteredItems.length);
      return;
    }
    if (event.key === "ArrowUp" && filteredItems.length > 0) {
      event.preventDefault();
      setActiveIndex((prev) => (prev <= 0 ? filteredItems.length - 1 : prev - 1));
      return;
    }
    if (event.key === "Enter") {
      const target = filteredItems[activeIndex];
      if (!target) return;
      event.preventDefault();
      navigate(target.path || "/");
      setNavQuery("");
      inputRef.current?.blur();
    }
  };

  const isSearching = navQuery.trim() !== "";

  return (
    <aside
      className={cn(
        "flex h-screen shrink-0 flex-col bg-[#BB6653] text-white transition-[width] duration-200",
        collapsed ? "w-20" : "w-80",
      )}
    >
      <div
        className={cn(
          "flex items-center border-b border-white/15 py-6",
          collapsed ? "flex-col gap-3 px-3" : "gap-3 px-6",
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
            <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-[#FFF8E8] p-1.5">
              <img src="/sut_logo.png" alt="Caesar Cluster" className="h-full w-full object-contain" />
            </div>
            <div className="leading-tight">
              <p className="text-base font-semibold">Caesar Cluster</p>
              <p className="text-xs text-white/70">Cloud for CPE</p>
            </div>
          </Link>
        )}
      </div>

      {collapsed ? (
        // หุบอยู่ก็ยังต้องค้นเมนูได้ — กดแว่นขยายแล้วกางออกพร้อมโฟกัสให้พิมพ์ต่อได้เลย
        <div className="px-3 py-4">
          <button
            type="button"
            onClick={() => {
              setCollapsed(false);
              requestAnimationFrame(() => inputRef.current?.focus());
            }}
            className="flex w-full items-center justify-center rounded-xl bg-white/10 py-2.5 text-white/80 transition-colors hover:bg-white/20 hover:text-white"
            aria-label="ค้นหาเมนู"
            title="ค้นหาเมนู"
          >
            <Search size={16} />
          </button>
        </div>
      ) : (
        <div className="px-5 py-4">
          <div className="flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2.5 focus-within:bg-white/15">
            <Search size={16} className="shrink-0 text-white/70" />
            <input
              ref={inputRef}
              value={navQuery}
              onChange={(event) => setNavQuery(event.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="ค้นหาเมนู…"
              aria-label="ค้นหาเมนู"
              className="w-full min-w-0 bg-transparent text-sm text-white outline-none placeholder:text-white/60"
            />
            {isSearching && (
              <button
                type="button"
                onClick={() => {
                  setNavQuery("");
                  inputRef.current?.focus();
                }}
                className="shrink-0 rounded-full p-0.5 text-white/60 transition-colors hover:bg-white/15 hover:text-white"
                aria-label="ล้างคำค้นเมนู"
              >
                <X size={14} />
              </button>
            )}
          </div>
        </div>
      )}

      <nav
        className={cn("flex-1 space-y-1.5 overflow-y-auto py-2", collapsed ? "px-3" : "px-4")}
      >
        {filteredItems.map((item, index) => {
          const Icon = item.icon;

          // เช็คว่า URL ปัจจุบันตรงกับ path ของเมนูนี้หรือไม่
          const isActive = location.pathname === item.path;
          // ระหว่างค้นอยู่ ตัวที่ Enter แล้วจะไปต้องเห็นชัดกว่าตัวที่เปิดอยู่ตอนนี้
          const isCursor = isSearching && index === activeIndex;

          const itemClassName = cn(
            "flex w-full items-center rounded-xl py-3 text-left transition-colors",
            collapsed ? "justify-center px-0" : "gap-3 px-4",
            isActive
              ? "bg-[#F08B51] text-white"
              : "text-white/85 hover:bg-white/10 hover:text-white",
            isCursor && !isActive && "bg-white/15 text-white ring-1 ring-white/40",
          );

          return (
            <Link
              key={item.label}
              to={item.path || "/"}
              onClick={() => setNavQuery("")}
              className={itemClassName}
              title={collapsed ? item.label : undefined}
            >
              <Icon size={20} className="shrink-0" />
              {collapsed ? null : (
                <span className="flex-1 text-base">
                  <Highlight
                    text={item.label}
                    terms={isSearching ? [navQuery.trim()] : []}
                    className="bg-white/30 text-white"
                  />
                </span>
              )}
            </Link>
          );
        })}

        {!collapsed && isSearching && filteredItems.length === 0 && (
          <p className="px-4 py-6 text-center text-sm text-white/60">
            ไม่พบเมนูที่ตรงกับ “{navQuery.trim()}”
          </p>
        )}
      </nav>

      <div
        className={cn(
          "mt-auto flex shrink-0 items-center border-t border-white/15 py-5",
          collapsed ? "flex-col gap-3 px-3" : "gap-3 px-5",
        )}
      >
        <Avatar>
          <AvatarFallback className="bg-[#F08B51] text-white">{initials}</AvatarFallback>
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
          className="text-white/80 transition-colors hover:text-white"
          aria-label="ออกจากระบบ"
          title={collapsed ? "ออกจากระบบ" : undefined}
        >
          <LogOut size={20} />
        </button>
      </div>
    </aside>
  );
}
