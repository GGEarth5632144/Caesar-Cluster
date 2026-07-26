import { useEffect, useState } from "react";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Info,
  HelpCircle,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/lib/utils";
import {
  useActionModalStore,
  type ActionModalItem,
  type AlertModalItem,
  type AlertVariant,
  type ConfirmModalItem,
} from "@/store/actionModalStore";

const ALERT_STYLES: Record<AlertVariant, { Icon: LucideIcon; iconBg: string; iconColor: string }> = {
  success: { Icon: CheckCircle2, iconBg: "bg-emerald-100", iconColor: "text-emerald-600" },
  error: { Icon: XCircle, iconBg: "bg-red-100", iconColor: "text-red-600" },
  warning: { Icon: AlertTriangle, iconBg: "bg-amber-100", iconColor: "text-amber-600" },
  info: { Icon: Info, iconBg: "bg-[#F08B51]/15", iconColor: "text-[#F08B51]" },
};

function isConfirmItem(item: ActionModalItem): item is ConfirmModalItem {
  return item.variant === "confirm";
}

function isAlertItem(item: ActionModalItem): item is AlertModalItem {
  return item.variant !== "confirm";
}

function getVisual(item: ActionModalItem) {
  if (item.variant === "confirm") {
    return item.destructive
      ? { Icon: AlertTriangle, iconBg: "bg-red-100", iconColor: "text-red-600" }
      : { Icon: HelpCircle, iconBg: "bg-[#BB6653]/10", iconColor: "text-[#BB6653]" };
  }
  return ALERT_STYLES[item.variant];
}

/**
 * เมาท์ครั้งเดียวที่ root ของแอป — ดึงคิวการแจ้งเตือน/ยืนยันจาก actionModalStore มาแสดงทีละอัน
 * ใช้งานผ่าน notify.* และ confirmAction() ใน src/lib/modal.ts แทน alert()/window.confirm()
 */
export function ActionModalHost() {
  const queue = useActionModalStore((s) => s.queue);
  const dequeue = useActionModalStore((s) => s.dequeue);
  const current = queue[0] ?? null;
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (current) setOpen(true);
  }, [current]);

  if (!current) return null;

  const { Icon, iconBg, iconColor } = getVisual(current);
  const confirmItem = isConfirmItem(current) ? current : null;
  const alertItem = isAlertItem(current) ? current : null;

  const close = () => setOpen(false);

  const handleCancel = () => {
    confirmItem?.resolve(false);
    close();
  };

  const handleConfirm = () => {
    confirmItem?.resolve(true);
    close();
  };

  return (
    <DialogPrimitive.Root
      key={current.id}
      open={open}
      onOpenChange={(next: boolean) => {
        if (!next) {
          confirmItem?.resolve(false);
          setOpen(false);
        }
      }}
      onOpenChangeComplete={(next: boolean) => {
        if (!next) dequeue();
      }}
    >
      <DialogPrimitive.Portal keepMounted>
        <DialogPrimitive.Backdrop
          className={cn(
            "fixed inset-0 z-100 [transform:translateZ(0)] bg-[#211a14]/40 backdrop-blur-sm transition-opacity duration-200 ease-out will-change-[opacity] data-starting-style:opacity-0!",
            open ? "opacity-100" : "opacity-0"
          )}
        />
        <DialogPrimitive.Popup
          className={cn(
            "fixed top-1/2 left-1/2 z-100 w-[calc(100%-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-3xl border border-black/5 bg-[#FFF8E8] p-6 font-mono shadow-2xl outline-none transition-[scale,opacity] duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)] data-starting-style:scale-90! data-starting-style:opacity-0!",
            open ? "scale-100 opacity-100" : "scale-90 opacity-0"
          )}
        >
          <div className="flex flex-col items-center gap-1 text-center">
            <div
              className={cn(
                "mb-3 flex size-14 shrink-0 items-center justify-center rounded-full",
                iconBg
              )}
            >
              <Icon size={28} className={iconColor} strokeWidth={2.25} />
            </div>
            <DialogPrimitive.Title className="text-lg leading-snug font-bold text-[#211a14]">
              {current.title}
            </DialogPrimitive.Title>
            {current.description && (
              <DialogPrimitive.Description className="text-sm leading-relaxed text-[#211a14]/60">
                {current.description}
              </DialogPrimitive.Description>
            )}
          </div>

          <div className="mt-6 flex gap-2.5">
            {confirmItem ? (
              <>
                <button
                  type="button"
                  onClick={handleCancel}
                  className="h-11 flex-1 rounded-xl border border-black/10 bg-white text-sm font-bold text-[#211a14]/70 transition-colors hover:bg-black/[0.03]"
                >
                  {confirmItem.cancelText ?? "ยกเลิก"}
                </button>
                <button
                  type="button"
                  onClick={handleConfirm}
                  autoFocus
                  className={cn(
                    "h-11 flex-1 rounded-xl text-sm font-bold text-white shadow-sm transition-colors",
                    confirmItem.destructive
                      ? "bg-red-600 hover:bg-red-700"
                      : "bg-[#BB6653] hover:bg-[#a65a48]"
                  )}
                >
                  {confirmItem.confirmText ?? "ยืนยัน"}
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={close}
                autoFocus
                className="h-11 w-full rounded-xl bg-[#BB6653] text-sm font-bold text-white shadow-sm transition-colors hover:bg-[#a65a48]"
              >
                {alertItem?.actionText ?? "ตกลง"}
              </button>
            )}
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
