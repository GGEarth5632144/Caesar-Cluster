import { Search, X } from "lucide-react";

import { cn } from "@/lib/utils";
import { useSearchStore } from "@/store/searchStore";

interface SearchStatusProps {
  className?: string;
}

/**
 * แถบเล็กๆ ที่วางไว้เหนือตารางของแต่ละหน้า แทนช่องค้นหาเดิมที่หน้านั้นเคยมีเอง
 *
 * ทำสองอย่าง: บอกว่าหน้านี้ค้นได้ (พร้อมปุ่มพาไปที่ช่องบน) และตอนกำลังกรองอยู่
 * ก็บอกว่าเหลือกี่รายการพร้อมปุ่มล้าง — เพราะช่องค้นหาอยู่บน Topbar
 * คนที่เลื่อนดูตารางยาวๆ ต้องมีอะไรเตือนว่าที่เห็นอยู่คือผลที่ถูกกรองมาแล้ว
 */
export function SearchStatus({ className }: SearchStatusProps) {
  const scope = useSearchStore((state) => state.scope);
  const query = useSearchStore((state) => state.query);
  const matchCount = useSearchStore((state) => state.matchCount);
  const totalCount = useSearchStore((state) => state.totalCount);
  const clearQuery = useSearchStore((state) => state.clearQuery);
  const requestFocus = useSearchStore((state) => state.requestFocus);

  if (!scope) return null;

  const isFiltering = query.trim() !== "";

  if (!isFiltering) {
    return (
      <button
        type="button"
        onClick={requestFocus}
        className={cn(
          "inline-flex items-center gap-2 rounded-xl border border-black/10 bg-[#FFFDF6] px-3 py-2 text-sm text-[#211a14]/55 transition-colors hover:border-[#BB6653]/40 hover:text-[#BB6653]",
          className,
        )}
      >
        <Search size={15} />
        <span>ค้นหา{scope.noun}</span>
        <kbd className="rounded border border-black/10 bg-white px-1.5 py-0.5 font-mono text-[10px] text-[#211a14]/45">
          Ctrl K
        </kbd>
      </button>
    );
  }

  return (
    <div
      className={cn(
        "inline-flex max-w-full items-center gap-2 rounded-xl border border-[#BB6653]/30 bg-[#BB6653]/10 px-3 py-2 text-sm text-[#BB6653]",
        className,
      )}
    >
      <Search size={15} className="shrink-0" />
      <span className="min-w-0 truncate font-mono text-xs">{query.trim()}</span>
      <span className="shrink-0 font-semibold whitespace-nowrap">
        {matchCount ?? 0}/{totalCount ?? 0} {scope.noun}
      </span>
      <button
        type="button"
        onClick={clearQuery}
        className="shrink-0 rounded-full p-0.5 transition-colors hover:bg-[#BB6653]/20"
        aria-label="ล้างคำค้นหา"
      >
        <X size={14} />
      </button>
    </div>
  );
}

export default SearchStatus;
