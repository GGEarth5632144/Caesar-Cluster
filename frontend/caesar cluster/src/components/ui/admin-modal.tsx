import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";

import { cn } from "@/lib/utils";

// เปลือก modal กลางของหน้า admin — ยกหน้าตามาจากป๊อปอัปหน้า Namespace Management
// แล้วรวมเป็นคอมโพเนนต์เดียว ทุกหน้าจะได้เหมือนกันเป๊ะและแก้ที่เดียวจบ
//
// สิ่งที่ได้เพิ่มจากตอนที่แต่ละหน้าเขียน <div className="fixed inset-0"> เอง:
//  - กด Esc / คลิกฉากหลังเพื่อปิด (กันไว้ตอน busy)
//  - ล็อกสกรอลล์ของหน้าเบื้องหลังระหว่างเปิด (นับซ้อนกันได้)
//  - render ผ่าน portal ที่ <body> จึงไม่โดน overflow/transform ของ ancestor ตัดขอบ
//  - ใส่ role="dialog" + aria-modal + aria-labelledby/aria-describedby
//  - อนิเมชันเข้า-ออก และย้ายโฟกัสเข้ากล่องตอนเปิด คืนที่เดิมตอนปิด
//
// ตั้งใจ "ไม่" กักโฟกัส (focus trap) เพราะกล่อง notify/confirm (action-modal.tsx, z-100)
// ถูกเรียกจากในนี้บ่อย ถ้าเรากักโฟกัสไว้ ปุ่มในกล่องนั้นจะกดไม่ติด — z ของ modal นี้อยู่ที่ 50
// จึงอยู่ใต้ action-modal เสมอ

type AdminModalSize = "sm" | "md" | "lg" | "xl";

const SIZE_CLASS: Record<AdminModalSize, string> = {
  sm: "max-w-lg",
  md: "max-w-2xl",
  lg: "max-w-4xl",
  xl: "max-w-6xl",
};

// ต้องตรงกับ duration-200 ของทรานสิชันด้านล่าง — ใช้หน่วงก่อนเรียก onClose จริงตอนปิด
const ANIM_MS = 200;

// นับ modal ที่เปิดอยู่พร้อมกัน — ปลดล็อกสกรอลล์เฉพาะตอนตัวสุดท้ายปิด
let scrollLockCount = 0;

function lockScroll() {
  if (scrollLockCount === 0) document.body.style.overflow = "hidden";
  scrollLockCount += 1;
}
function unlockScroll() {
  scrollLockCount = Math.max(0, scrollLockCount - 1);
  if (scrollLockCount === 0) document.body.style.overflow = "";
}

interface AdminModalProps {
  /** ปิด modal — ถูกเรียกหลังอนิเมชันปิดจบ (จาก Esc / กากบาท / ฉากหลัง / ปุ่ม Cancel) */
  onClose: () => void;
  title: ReactNode;
  /** บรรทัดคำอธิบายใต้ชื่อ (ไม่ใส่ก็ได้) */
  subtitle?: ReactNode;
  /** ความกว้างสูงสุด: sm=lg / md=2xl / lg=4xl / xl=6xl (ค่าเริ่มต้น md) */
  size?: AdminModalSize;
  /** true = มี action ค้างอยู่ (เช่นกำลังบันทึก) ปิด modal ไม่ได้ชั่วคราว */
  busy?: boolean;
  /** แถบใต้ header ที่ตรึงอยู่กับที่ ไม่เลื่อนตามเนื้อหา เช่นช่องค้นหาของรายการยาว */
  toolbar?: ReactNode;
  /** ปุ่ม action ท้าย modal — อยู่ในแถบ sticky มีเส้นคั่นบน จัดชิดขวาโดยปริยาย
   *  ปุ่มที่อยากดันไปซ้าย (เช่นปุ่มลบ) ให้ใส่ className="mr-auto"
   *  รับเป็นฟังก์ชันได้ จะได้ close() ที่เล่นอนิเมชันปิดก่อนแล้วค่อยเรียก onClose (ใช้กับปุ่ม Cancel) */
  footer?: ReactNode | ((close: () => void) => ReactNode);
  /** ใส่เมื่อเนื้อหาเป็นฟอร์ม — AdminModal ห่อ <form> ให้ ปุ่ม submit ใน footer จึงยิงฟอร์มนี้ได้ */
  onSubmit?: (e: FormEvent) => void;
  /** override คลาส layout ของแถบ footer (ค่าเริ่มต้นจัดปุ่มเรียงแถวชิดขวา) */
  footerClassName?: string;
  /** คลาสเพิ่มให้ตัวกล่อง */
  className?: string;
  /** override คลาสของโซนเนื้อหาที่เลื่อนได้ (ค่าเริ่มต้น px-6 py-5) */
  bodyClassName?: string;
  children: ReactNode;
}

export function AdminModal({
  onClose,
  title,
  subtitle,
  size = "md",
  busy = false,
  toolbar,
  footer,
  onSubmit,
  footerClassName,
  className,
  bodyClassName,
  children,
}: AdminModalProps) {
  // เฟรมแรก render แบบปิดไว้ก่อน (opacity 0 / scale 95) แล้วเฟรมถัดไปสลับเป็นเปิดให้ทรานสิชันวิ่ง
  const [entered, setEntered] = useState(false);
  const [closing, setClosing] = useState(false);
  const shown = entered && !closing;

  const popupRef = useRef<HTMLDivElement>(null);
  const restoreFocusRef = useRef<Element | null>(null);
  const busyRef = useRef(busy);
  busyRef.current = busy;

  const titleId = useId();
  const descId = useId();

  useEffect(() => {
    restoreFocusRef.current = document.activeElement;
    lockScroll();
    const raf = requestAnimationFrame(() => setEntered(true));
    // ย้ายโฟกัสเข้ากล่องพอให้พิมพ์/กด Tab ได้ทันที แต่ไม่กักไว้ (ดูหมายเหตุหัวไฟล์)
    const focusTimer = window.setTimeout(() => {
      const el = popupRef.current;
      if (!el || el.contains(document.activeElement)) return;
      const target = el.querySelector<HTMLElement>(
        "input, select, textarea, button, [tabindex]:not([tabindex='-1'])",
      );
      (target ?? el).focus();
    }, 50);
    return () => {
      cancelAnimationFrame(raf);
      window.clearTimeout(focusTimer);
      unlockScroll();
      const restore = restoreFocusRef.current;
      if (restore instanceof HTMLElement) restore.focus();
    };
  }, []);

  const requestClose = useCallback(() => {
    if (busyRef.current || closing) return;
    setClosing(true);
    window.setTimeout(onClose, ANIM_MS);
  }, [closing, onClose]);

  const isTopmostDialog = useCallback(() => {
    const dialogs = document.querySelectorAll('[role="dialog"], [role="alertdialog"]');
    return !dialogs.length || dialogs[dialogs.length - 1] === popupRef.current;
  }, []);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        // ปิดด้วย Esc เฉพาะตอนที่กล่องนี้อยู่บนสุดจริงๆ — ถ้ามี dialog อื่น (เช่น confirm/notify
        // จาก action-modal) เปิดทับอยู่ ปล่อยให้ตัวนั้นรับ Esc ไป ไม่งั้นฟอร์มข้างล่างจะปิดตามไปด้วย
        if (isTopmostDialog()) requestClose();
        return;
      }
      // วนโฟกัสอยู่ในกล่อง (soft trap) — ทำงานเฉพาะตอนโฟกัสอยู่ในกล่องนี้อยู่แล้ว
      // ถ้ามี dialog อื่นเปิดทับและกักโฟกัสไว้ เงื่อนไข contains จะเป็น false เอง ไม่ไปยุ่งกับมัน
      if (e.key !== "Tab") return;
      const el = popupRef.current;
      if (!el || !el.contains(document.activeElement)) return;
      const focusables = el.querySelectorAll<HTMLElement>(
        "a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])",
      );
      if (!focusables.length) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [requestClose, isTopmostDialog]);

  const body = (
    <div className={cn("min-h-0 flex-1 overflow-y-auto px-6 py-5", bodyClassName)}>{children}</div>
  );
  const footerContent = typeof footer === "function" ? footer(requestClose) : footer;
  const footerBar = footerContent != null && footerContent !== false && (
    <div
      className={cn(
        "shrink-0 border-t border-black/5 bg-[#FFF8E8] px-6 py-4",
        footerClassName ?? "flex flex-wrap items-center justify-end gap-3",
      )}
    >
      {footerContent}
    </div>
  );

  return createPortal(
    <div
      className={cn(
        "fixed inset-0 z-50 flex items-center justify-center p-4 font-mono transition-opacity duration-200 ease-out",
        "bg-[#211a14]/40 backdrop-blur-sm",
        shown ? "opacity-100" : "opacity-0",
      )}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) requestClose();
      }}
    >
      <div
        ref={popupRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={subtitle ? descId : undefined}
        tabIndex={-1}
        className={cn(
          "flex max-h-[90vh] w-full flex-col overflow-hidden rounded-3xl bg-[#FFF8E8] shadow-2xl outline-none transition-[opacity,scale] duration-200 ease-out",
          SIZE_CLASS[size],
          shown ? "scale-100 opacity-100" : "scale-95 opacity-0",
          className,
        )}
      >
        <div className="flex shrink-0 items-start justify-between gap-3 border-b border-black/5 px-6 py-5">
          <div className="min-w-0">
            <h2 id={titleId} className="truncate text-xl font-bold text-[#211a14]">
              {title}
            </h2>
            {subtitle && (
              <p id={descId} className="mt-0.5 text-sm text-[#211a14]/50">
                {subtitle}
              </p>
            )}
          </div>
          <button
            type="button"
            onClick={requestClose}
            disabled={busy}
            aria-label="ปิด"
            className="shrink-0 rounded-xl p-2 text-[#211a14]/50 transition-colors hover:bg-black/5 disabled:opacity-50"
          >
            <X size={20} />
          </button>
        </div>

        {toolbar != null && (
          <div className="shrink-0 border-b border-black/5 px-6 py-4">{toolbar}</div>
        )}

        {onSubmit ? (
          <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
            {body}
            {footerBar}
          </form>
        ) : (
          <>
            {body}
            {footerBar}
          </>
        )}
      </div>
    </div>,
    document.body,
  );
}
