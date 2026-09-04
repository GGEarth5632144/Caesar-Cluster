import { create } from 'zustand';

import { alertApi } from '@/api/alerts';

// ถามจำนวนแจ้งเตือนใหม่ทุกกี่มิลลิวินาที
// 60 วินาทีเท่ากับรอบสแกน log ฝั่ง backend (ALERT_SCAN_INTERVAL_SECONDS) — ถี่กว่านี้ไม่มี
// อะไรใหม่ให้เห็น เพราะตัวเลขเปลี่ยนได้เร็วที่สุดก็ต่อเมื่อ backend สแกนเจอรอบใหม่
const POLL_INTERVAL_MS = 60_000;

interface AlertState {
  /** จำนวนแจ้งเตือนที่ยังไม่ได้อ่าน — 0 แปลว่า Sidebar ต้องไม่โชว์วงกลมแดงเลย */
  unread: number;
  /** true ระหว่างที่ยังไม่เคยดึงค่าจริงสำเร็จสักครั้ง — ใช้กันวงกลมแดงกะพริบตอนเพิ่งเปิดเว็บ */
  loading: boolean;
  refresh: () => Promise<void>;
  /** ปรับตัวเลขในเครื่องทันทีหลังผู้ใช้กดอ่าน โดยไม่ต้องรอ round-trip กลับมา */
  decrease: (by: number) => void;
  setUnread: (n: number) => void;
  /** เริ่ม poll — คืนฟังก์ชันสำหรับหยุด (ผูกกับอายุของ DashboardLayout) */
  startPolling: () => () => void;
  reset: () => void;
}

// นับจำนวนผู้ใช้ hook ที่กำลัง poll อยู่ เก็บนอก store เพราะเป็นรายละเอียดภายใน
// ไม่ใช่ state ที่ component ไหนต้อง render ตาม (ถ้าใส่ใน store จะ re-render ทุกครั้งที่ mount/unmount)
let pollTimer: ReturnType<typeof setInterval> | null = null;
let subscribers = 0;

export const useAlertStore = create<AlertState>((set, get) => ({
  unread: 0,
  loading: true,

  refresh: async () => {
    try {
      const n = await alertApi.unreadCount();
      set({ unread: n, loading: false });
    } catch {
      // ดึงไม่สำเร็จ (เน็ตหลุด/token หมดอายุ) — คงตัวเลขเดิมไว้เฉยๆ ดีกว่ารีเซ็ตเป็น 0
      // เพราะการทำให้วงกลมแดงหายไปเองทั้งที่ยังมีแจ้งเตือนค้าง คือการซ่อนปัญหาจากผู้ใช้
      set({ loading: false });
    }
  },

  decrease: (by) => set({ unread: Math.max(0, get().unread - by) }),
  setUnread: (n) => set({ unread: Math.max(0, n), loading: false }),

  startPolling: () => {
    subscribers += 1;
    void get().refresh();

    if (!pollTimer) {
      pollTimer = setInterval(() => void useAlertStore.getState().refresh(), POLL_INTERVAL_MS);
    }

    return () => {
      subscribers -= 1;
      if (subscribers <= 0 && pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
        subscribers = 0;
      }
    };
  },

  reset: () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    subscribers = 0;
    set({ unread: 0, loading: true });
  },
}));
