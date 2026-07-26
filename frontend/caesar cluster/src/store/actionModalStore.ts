import { create } from "zustand";

export type AlertVariant = "success" | "error" | "warning" | "info";

interface ActionModalBase {
  id: string;
  title: string;
  description?: string;
}

export interface AlertModalItem extends ActionModalBase {
  variant: AlertVariant;
  actionText?: string;
}

export interface ConfirmModalItem extends ActionModalBase {
  variant: "confirm";
  confirmText?: string;
  cancelText?: string;
  destructive?: boolean;
  resolve: (confirmed: boolean) => void;
}

export type ActionModalItem = AlertModalItem | ConfirmModalItem;

interface ActionModalState {
  queue: ActionModalItem[];
  enqueue: (item: ActionModalItem) => void;
  dequeue: () => void;
}

// คิวทีละรายการ — ถ้ามีการแจ้งเตือนซ้อนกันจะเข้าคิวแสดงต่อจากอันที่ค้างอยู่ ไม่ถูกทับหรือหายไป
export const useActionModalStore = create<ActionModalState>((set) => ({
  queue: [],
  enqueue: (item) => set((state) => ({ queue: [...state.queue, item] })),
  dequeue: () => set((state) => ({ queue: state.queue.slice(1) })),
}));
