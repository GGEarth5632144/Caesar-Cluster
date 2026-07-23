import { useActionModalStore, type AlertVariant } from "@/store/actionModalStore";

function genId() {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2);
}

interface AlertOptions {
  actionText?: string;
}

function pushAlert(variant: AlertVariant, title: string, description?: string, opts?: AlertOptions) {
  useActionModalStore.getState().enqueue({
    id: genId(),
    variant,
    title,
    description,
    actionText: opts?.actionText,
  });
}

/**
 * แทนที่ alert() ทั่วไป — เรียกใช้ได้ทุกที่ ไม่ต้องอยู่ใน component
 * เช่น notify.error("ลบผู้ใช้งานไม่สำเร็จ", err.message)
 */
export const notify = {
  success: (title: string, description?: string, opts?: AlertOptions) =>
    pushAlert("success", title, description, opts),
  error: (title: string, description?: string, opts?: AlertOptions) =>
    pushAlert("error", title, description, opts),
  warning: (title: string, description?: string, opts?: AlertOptions) =>
    pushAlert("warning", title, description, opts),
  info: (title: string, description?: string, opts?: AlertOptions) =>
    pushAlert("info", title, description, opts),
};

interface ConfirmOptions {
  title: string;
  description?: string;
  confirmText?: string;
  cancelText?: string;
  /** โทนสีแดงสำหรับการกระทำที่ย้อนกลับไม่ได้ เช่น ลบข้อมูล */
  destructive?: boolean;
}

/**
 * แทนที่ window.confirm() — คืนค่าเป็น Promise<boolean>
 * เช่น if (!(await confirmAction({ title: "...", destructive: true }))) return;
 */
export function confirmAction(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    useActionModalStore.getState().enqueue({
      id: genId(),
      variant: "confirm",
      title: opts.title,
      description: opts.description,
      confirmText: opts.confirmText,
      cancelText: opts.cancelText,
      destructive: opts.destructive,
      resolve,
    });
  });
}
