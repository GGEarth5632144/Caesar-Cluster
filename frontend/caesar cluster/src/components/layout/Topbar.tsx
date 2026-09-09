import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ArrowRight, CornerDownLeft, History, Search, SlidersHorizontal, X } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { cn, getInitials } from "@/lib/utils";
import { fuzzyScore } from "@/lib/search";
import { PATHS } from "@/config/routes";
import { searchTargetsFor, type SearchTarget } from "@/config/searchTargets";
import { useAuthStore } from "@/store/authStore";
import { readFilterValues, useSearchStore } from "@/store/searchStore";

interface TopbarProps {
  title: string;
  userName: string;
}

// แถวที่กดเลือกได้ด้วยลูกศรขึ้น/ลงในกล่องผลลัพธ์
type PanelRow =
  | { kind: "recent"; value: string }
  | { kind: "target"; target: SearchTarget };

/** จับคู่คำค้นกับหน้าในระบบ — ดูทั้งชื่อหน้า คำพ้อง และคำอธิบาย แล้วเรียงตามความตรง */
function matchTargets(targets: SearchTarget[], query: string, limit: number): SearchTarget[] {
  const trimmed = query.trim();
  if (!trimmed) return targets.slice(0, limit);

  return targets
    .map((target) => {
      const haystacks = [target.label, ...target.keywords, target.description ?? ""];
      // คะแนนของหน้า = คะแนนของคำที่ตรงที่สุด ไม่ใช่ผลรวม
      // (ไม่งั้นหน้าที่ใส่ keyword ไว้เยอะจะชนะเสมอทั้งที่ไม่ได้ตรงกว่า)
      const score = Math.max(...haystacks.map((text) => fuzzyScore(text, trimmed)));
      return { target, score };
    })
    .filter((entry) => entry.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, limit)
    .map((entry) => entry.target);
}

export default function Topbar({ title, userName }: TopbarProps) {
  const initials = getInitials(userName) || "U";
  const navigate = useNavigate();

  const user = useAuthStore((state) => state.user);
  const isAdmin = String(user?.role) === "admin";
  const hasVm = Boolean(user?.namespace_id);

  const scope = useSearchStore((state) => state.scope);
  const query = useSearchStore((state) => state.query);
  const matchCount = useSearchStore((state) => state.matchCount);
  const totalCount = useSearchStore((state) => state.totalCount);
  const disabledFields = useSearchStore((state) => state.disabledFields);
  const recentMap = useSearchStore((state) => state.recent);
  const setQuery = useSearchStore((state) => state.setQuery);
  const clearQuery = useSearchStore((state) => state.clearQuery);
  const toggleField = useSearchStore((state) => state.toggleField);
  const toggleQuickFilter = useSearchStore((state) => state.toggleQuickFilter);
  const commitRecent = useSearchStore((state) => state.commitRecent);
  const removeRecent = useSearchStore((state) => state.removeRecent);
  const focusToken = useSearchStore((state) => state.focusToken);

  const [open, setOpen] = useState(false);
  const [showFilters, setShowFilters] = useState(false);
  const [activeRow, setActiveRow] = useState(-1);
  const [mobileOpen, setMobileOpen] = useState(false);

  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const targets = useMemo(() => searchTargetsFor(isAdmin, hasVm), [isAdmin, hasVm]);
  // ?? [] สร้าง array ใหม่ทุกครั้งที่ render — ถ้าไม่ memo ไว้ useMemo ที่รับไปเป็น dependency
  // จะคิดใหม่ทุกครั้งตามไปด้วย (แม้ประวัติจะไม่ได้เปลี่ยนอะไรเลย)
  const recents = useMemo(
    () => (scope ? (recentMap[scope.id] ?? []) : []),
    [scope, recentMap],
  );
  const disabled = useMemo(
    () => (scope ? (disabledFields[scope.id] ?? []) : []),
    [scope, disabledFields],
  );

  // หน้าไหนไม่ได้ลงทะเบียนว่าค้นอะไรได้ ช่องนี้จะกลายเป็นตัวพาไปหน้าอื่นแทน
  // ดีกว่าปล่อยให้เป็นช่องที่พิมพ์แล้วไม่เกิดอะไรขึ้นเหมือนเดิม
  const isJumpMode = scope === null;

  const jumpMatches = useMemo(
    () => matchTargets(targets, query, isJumpMode ? 8 : 4),
    [targets, query, isJumpMode],
  );

  const rows: PanelRow[] = useMemo(() => {
    if (isJumpMode) return jumpMatches.map((target) => ({ kind: "target" as const, target }));

    const recentRows: PanelRow[] = query.trim()
      ? []
      : recents.map((value) => ({ kind: "recent" as const, value }));
    const targetRows: PanelRow[] = query.trim()
      ? jumpMatches.map((target) => ({ kind: "target" as const, target }))
      : [];
    return [...recentRows, ...targetRows];
  }, [isJumpMode, jumpMatches, recents, query]);

  // เปลี่ยนคำค้นแล้วแถวที่เลือกไว้เดิมไม่ใช่แถวเดิมอีกต่อไป ต้องรีเซ็ต
  useEffect(() => setActiveRow(-1), [query, scope?.id]);

  const closePanel = useCallback(() => {
    setOpen(false);
    setActiveRow(-1);
  }, []);

  // ปิดกล่องเมื่อคลิกที่อื่น — ฟัง mousedown ไม่ใช่ click เพราะปุ่มในกล่องต้องได้ทำงานก่อนปิด
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) closePanel();
    };
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
  }, [open, closePanel]);

  // ทางลัดคีย์บอร์ด: Ctrl/Cmd+K หรือ "/" กระโดดมาที่ช่องค้นหาจากตรงไหนก็ได้
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typingElsewhere =
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable === true;

      const isShortcut = (event.key === "k" || event.key === "K") && (event.metaKey || event.ctrlKey);
      // "/" ใช้ได้เฉพาะตอนที่ยังไม่ได้พิมพ์อยู่ในช่องอื่น ไม่งั้นจะพิมพ์ path ไม่ได้เลยทั้งเว็บ
      const isSlash = event.key === "/" && !typingElsewhere && !event.metaKey && !event.ctrlKey;

      if (!isShortcut && !isSlash) return;
      event.preventDefault();
      setMobileOpen(true);
      setOpen(true);
      inputRef.current?.focus();
      inputRef.current?.select();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  // หน้าข้างในกดปุ่ม "ค้นหา" ของตัวเองแล้วเคอร์เซอร์ต้องเด้งมาที่ช่องนี้
  // ข้าม token แรก (ค่า 0 ตอนเปิดเว็บ) ไม่งั้นช่องค้นหาจะแย่งโฟกัสทุกครั้งที่เปลี่ยนหน้า
  useEffect(() => {
    if (focusToken === 0) return;
    setMobileOpen(true);
    setOpen(true);
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [focusToken]);

  const runRow = (row: PanelRow) => {
    if (row.kind === "recent") {
      setQuery(row.value);
      inputRef.current?.focus();
      return;
    }
    clearQuery();
    closePanel();
    setMobileOpen(false);
    navigate(row.target.path);
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      // Esc ครั้งแรกล้างคำค้น ครั้งที่สอง (ช่องว่างอยู่แล้ว) ถึงจะออกจากช่อง
      if (query) {
        clearQuery();
        return;
      }
      closePanel();
      inputRef.current?.blur();
      return;
    }

    if (event.key === "ArrowDown" && rows.length > 0) {
      event.preventDefault();
      setOpen(true);
      setActiveRow((prev) => (prev + 1) % rows.length);
      return;
    }

    if (event.key === "ArrowUp" && rows.length > 0) {
      event.preventDefault();
      setActiveRow((prev) => (prev <= 0 ? rows.length - 1 : prev - 1));
      return;
    }

    if (event.key === "Enter") {
      event.preventDefault();
      if (activeRow >= 0 && rows[activeRow]) {
        runRow(rows[activeRow]);
        return;
      }
      // ไม่ได้เลือกแถวไหน = ยืนยันคำค้นที่พิมพ์เอง เก็บไว้ในประวัติแล้วปิดกล่อง
      commitRecent();
      closePanel();
    }
  };

  const placeholder = isJumpMode
    ? "ค้นหาหน้าในระบบ… (Ctrl+K)"
    : scope.placeholder;

  const hasActiveFilters = Boolean(
    scope?.quickFilters?.some((filter) => readFilterValues(query, filter.fieldId).length > 0),
  );

  const searchBox = (
    <div ref={containerRef} className="relative w-full">
      <div
        className={cn(
          "flex w-full items-center gap-2.5 rounded-full bg-[#FFF8E8] px-5 py-3 text-[#211a14] transition-shadow",
          open && "shadow-[0_0_0_3px_rgba(240,139,81,0.55)]",
        )}
      >
        <Search size={18} className="shrink-0 text-[#211a14]/60" />

        <input
          ref={inputRef}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onFocus={() => setOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          aria-label={isJumpMode ? "ค้นหาหน้าในระบบ" : `ค้นหา${scope.noun}`}
          className="w-full min-w-0 bg-transparent text-sm outline-none placeholder:text-[#211a14]/50"
        />

        {/* ตัวเลขผลลัพธ์ — โชว์เฉพาะตอนที่กำลังกรองอยู่จริง ไม่งั้นเป็นตัวเลขที่ไม่มีความหมาย */}
        {!isJumpMode && query.trim() !== "" && matchCount !== null && totalCount !== null && (
          <span className="shrink-0 rounded-full bg-[#BB6653]/12 px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap text-[#BB6653]">
            {matchCount}/{totalCount}
          </span>
        )}

        {query !== "" && (
          <button
            type="button"
            onClick={() => {
              clearQuery();
              inputRef.current?.focus();
            }}
            className="shrink-0 rounded-full p-0.5 text-[#211a14]/45 transition-colors hover:bg-black/5 hover:text-[#211a14]"
            aria-label="ล้างคำค้นหา"
            title="ล้างคำค้นหา (Esc)"
          >
            <X size={16} />
          </button>
        )}

        {!isJumpMode && (scope.quickFilters?.length || scope.fields.length > 0) && (
          <button
            type="button"
            onClick={() => {
              setShowFilters((prev) => !prev);
              setOpen(true);
            }}
            className={cn(
              "shrink-0 rounded-full p-1 transition-colors",
              showFilters || hasActiveFilters
                ? "bg-[#BB6653] text-white"
                : "text-[#211a14]/45 hover:bg-black/5 hover:text-[#211a14]",
            )}
            aria-label="ตัวกรองเพิ่มเติม"
            title="ตัวกรองเพิ่มเติม"
          >
            <SlidersHorizontal size={16} />
          </button>
        )}
      </div>

      {open && (
        <SearchPanel
          isJumpMode={isJumpMode}
          scope={scope}
          query={query}
          rows={rows}
          activeRow={activeRow}
          showFilters={showFilters}
          disabledFields={disabled}
          onRun={runRow}
          onHoverRow={setActiveRow}
          onRemoveRecent={removeRecent}
          onToggleField={toggleField}
          onToggleQuickFilter={toggleQuickFilter}
          onPickExample={(example) => {
            setQuery(example);
            inputRef.current?.focus();
          }}
        />
      )}
    </div>
  );

  return (
    <header className="flex h-20 shrink-0 items-center gap-4 bg-[#BB6653] px-4 text-white sm:gap-6 sm:px-8">
      {/* จอเล็กมีที่ไม่พอให้ทั้งชื่อหน้าและช่องค้นหา — เปิดค้นหาเมื่อไหร่ชื่อหน้าหลบให้ */}
      <h1
        className={cn(
          "shrink-0 text-xl font-semibold whitespace-nowrap",
          mobileOpen && "hidden lg:block",
        )}
      >
        {title}
      </h1>

      {/* ช่องค้นหาถูก render ที่เดียวเสมอ (ซ่อน/แสดงด้วย CSS) เพราะถ้าวางสองที่แล้วสลับกันตามขนาดจอ
          จะได้ input สองตัวที่ผูก ref เดียวกัน — ทางลัดคีย์บอร์ดจะไปโฟกัสตัวที่ซ่อนอยู่ */}
      <div
        className={cn(
          "min-w-0 flex-1 justify-center lg:flex",
          mobileOpen ? "flex" : "hidden",
        )}
      >
        <div className="w-full max-w-xl">{searchBox}</div>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-3 sm:gap-5">
        <button
          type="button"
          onClick={() => {
            const next = !mobileOpen;
            setMobileOpen(next);
            // เปิดแถบค้นหาบนจอเล็กแล้วต้องได้พิมพ์ต่อทันที ไม่ต้องแตะช่องอีกรอบ
            if (next) requestAnimationFrame(() => inputRef.current?.focus());
          }}
          className="rounded-full p-2 text-white/85 transition-colors hover:bg-white/10 hover:text-white lg:hidden"
          aria-label={mobileOpen ? "ปิดช่องค้นหา" : "ค้นหา"}
        >
          {mobileOpen ? <X size={20} /> : <Search size={20} />}
        </button>

        <Link to={PATHS.settings} aria-label="ไปหน้าตั้งค่า">
          <Avatar>
            <AvatarFallback className="bg-[#F08B51] text-white">{initials}</AvatarFallback>
          </Avatar>
        </Link>
      </div>
    </header>
  );
}

// ---------------------------------------------------------------------------
// กล่องผลลัพธ์ใต้ช่องค้นหา
// ---------------------------------------------------------------------------

interface SearchPanelProps {
  isJumpMode: boolean;
  scope: ReturnType<typeof useSearchStore.getState>["scope"];
  query: string;
  rows: PanelRow[];
  activeRow: number;
  showFilters: boolean;
  disabledFields: string[];
  onRun: (row: PanelRow) => void;
  onHoverRow: (index: number) => void;
  onRemoveRecent: (query: string) => void;
  onToggleField: (fieldId: string) => void;
  onToggleQuickFilter: ReturnType<typeof useSearchStore.getState>["toggleQuickFilter"];
  onPickExample: (example: string) => void;
}

function SearchPanel({
  isJumpMode,
  scope,
  query,
  rows,
  activeRow,
  showFilters,
  disabledFields,
  onRun,
  onHoverRow,
  onRemoveRecent,
  onToggleField,
  onToggleQuickFilter,
  onPickExample,
}: SearchPanelProps) {
  const recentRows = rows.filter((row) => row.kind === "recent");
  const targetRows = rows.filter((row) => row.kind === "target");
  const freeTextFields = scope?.fields.filter((field) => !field.exactOnly) ?? [];

  return (
    <div className="absolute top-full left-0 z-50 mt-2 w-full overflow-hidden rounded-2xl border border-black/5 bg-[#FFFDF6] text-[#211a14] shadow-xl">
      <div className="max-h-[70vh] overflow-y-auto">
        {/* ตัวกรองสำเร็จรูป — กดแล้วเท่ากับพิมพ์ "status:pending" เองในช่อง */}
        {!isJumpMode && scope?.quickFilters && scope.quickFilters.length > 0 && (
          <section className="border-b border-black/5 px-4 py-3">
            {scope.quickFilters.map((filter) => {
              const selected = readFilterValues(query, filter.fieldId);
              return (
                <div key={filter.fieldId} className="flex flex-wrap items-center gap-2 py-1">
                  <span className="w-20 shrink-0 text-xs font-semibold text-[#211a14]/50">
                    {filter.label}
                  </span>
                  {filter.options.map((option) => {
                    const active = selected.some(
                      (v) => v.toLowerCase() === option.value.toLowerCase(),
                    );
                    return (
                      <button
                        key={option.value}
                        type="button"
                        onClick={() => onToggleQuickFilter(filter, option.value)}
                        className={cn(
                          "rounded-full px-3 py-1 text-xs font-semibold transition-colors",
                          active
                            ? "bg-[#BB6653] text-white"
                            : "bg-[#FFF8E8] text-[#211a14]/70 hover:bg-[#F08B51]/25",
                        )}
                      >
                        {option.label}
                      </button>
                    );
                  })}
                </div>
              );
            })}
          </section>
        )}

        {/* เลือกว่าคำเปล่าๆ จะไปค้นในฟิลด์ไหนบ้าง — ซ่อนไว้หลังปุ่มตัวกรองเพราะเป็นของขั้นสูง */}
        {!isJumpMode && showFilters && freeTextFields.length > 0 && (
          <section className="border-b border-black/5 px-4 py-3">
            <p className="mb-2 text-xs font-semibold text-[#211a14]/50">
              ค้นคำเปล่าในฟิลด์
            </p>
            <div className="flex flex-wrap gap-2">
              {freeTextFields.map((field) => {
                const off = disabledFields.includes(field.id);
                return (
                  <button
                    key={field.id}
                    type="button"
                    onClick={() => onToggleField(field.id)}
                    className={cn(
                      "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
                      off
                        ? "border-black/10 bg-transparent text-[#211a14]/35 line-through"
                        : "border-[#BB6653]/30 bg-[#BB6653]/10 text-[#BB6653]",
                    )}
                    title={off ? `เปิดค้นใน ${field.label}` : `ปิดค้นใน ${field.label}`}
                  >
                    {field.label}
                  </button>
                );
              })}
            </div>
            <p className="mt-2 text-[11px] leading-relaxed text-[#211a14]/45">
              ปิดฟิลด์ไหนไว้ก็ยังเจาะจงค้นได้อยู่ด้วยการพิมพ์ <code>ชื่อฟิลด์:ค่า</code>
            </p>
          </section>
        )}

        {/* ประวัติการค้นหาของหน้านี้ */}
        {recentRows.length > 0 && (
          <section className="border-b border-black/5 py-2">
            <p className="px-4 py-1 text-xs font-semibold text-[#211a14]/50">ค้นหาล่าสุด</p>
            {recentRows.map((row) => {
              const index = rows.indexOf(row);
              return (
                <div
                  key={row.value}
                  onMouseEnter={() => onHoverRow(index)}
                  className={cn(
                    "flex items-center gap-3 px-4 py-2",
                    activeRow === index && "bg-[#F08B51]/15",
                  )}
                >
                  <History size={15} className="shrink-0 text-[#211a14]/40" />
                  <button
                    type="button"
                    onClick={() => onRun(row)}
                    className="min-w-0 flex-1 truncate text-left text-sm"
                  >
                    {row.value}
                  </button>
                  <button
                    type="button"
                    onClick={() => onRemoveRecent(row.value)}
                    className="shrink-0 rounded p-1 text-[#211a14]/35 transition-colors hover:bg-black/5 hover:text-[#211a14]"
                    aria-label={`ลบ "${row.value}" ออกจากประวัติ`}
                  >
                    <X size={13} />
                  </button>
                </div>
              );
            })}
          </section>
        )}

        {/* หน้าที่กระโดดไปได้ */}
        {targetRows.length > 0 && (
          <section className="py-2">
            <p className="px-4 py-1 text-xs font-semibold text-[#211a14]/50">
              {isJumpMode ? "ไปที่หน้า" : "หรือไปที่หน้า"}
            </p>
            {targetRows.map((row) => {
              const index = rows.indexOf(row);
              const Icon = row.target.icon;
              return (
                <button
                  key={row.target.path}
                  type="button"
                  onClick={() => onRun(row)}
                  onMouseEnter={() => onHoverRow(index)}
                  className={cn(
                    "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                    activeRow === index ? "bg-[#F08B51]/15" : "hover:bg-black/[0.03]",
                  )}
                >
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-[#BB6653]/10 text-[#BB6653]">
                    <Icon size={16} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{row.target.label}</span>
                    {row.target.description && (
                      <span className="block truncate text-xs text-[#211a14]/50">
                        {row.target.description}
                      </span>
                    )}
                  </span>
                  <span className="shrink-0 text-[10px] font-semibold tracking-wide text-[#211a14]/35 uppercase">
                    {row.target.group}
                  </span>
                  <ArrowRight size={14} className="shrink-0 text-[#211a14]/30" />
                </button>
              );
            })}
          </section>
        )}

        {/* ตัวอย่างคิวรีของหน้านี้ — โชว์ตอนที่ยังไม่ได้พิมพ์อะไร ทำหน้าที่เป็นคู่มือย่อ */}
        {!isJumpMode && !query.trim() && scope?.examples && scope.examples.length > 0 && (
          <section className="px-4 py-3">
            <p className="mb-2 text-xs font-semibold text-[#211a14]/50">ลองค้นแบบนี้</p>
            <div className="flex flex-wrap gap-2">
              {scope.examples.map((example) => (
                <button
                  key={example}
                  type="button"
                  onClick={() => onPickExample(example)}
                  className="rounded-lg bg-[#FFF8E8] px-2.5 py-1 font-mono text-xs text-[#BB6653] transition-colors hover:bg-[#F08B51]/25"
                >
                  {example}
                </button>
              ))}
            </div>
          </section>
        )}

        {isJumpMode && targetRows.length === 0 && (
          <p className="px-4 py-6 text-center text-sm text-[#211a14]/45">
            ไม่พบหน้าที่ตรงกับ “{query}”
          </p>
        )}
      </div>

      {/* แถบล่าง: บอกกติกาการพิมพ์แบบสั้นที่สุดที่ยังมีประโยชน์ */}
      <div className="flex items-center justify-between gap-3 border-t border-black/5 bg-[#FFF8E8]/60 px-4 py-2 text-[11px] text-[#211a14]/50">
        <span className="truncate">
          {isJumpMode
            ? "หน้านี้ไม่มีข้อมูลให้ค้น — ช่องนี้ใช้ข้ามไปหน้าอื่นแทน"
            : 'ใช้ได้: field:value · -คำที่ไม่เอา · "วลีทั้งวลี" · a|b · cpu:>=500'}
        </span>
        <span className="hidden shrink-0 items-center gap-1 sm:flex">
          <CornerDownLeft size={12} /> เลือก
        </span>
      </div>
    </div>
  );
}
