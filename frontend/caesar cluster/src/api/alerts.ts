import axiosClient from './axiosClient';

// severity ใช้ชุดเดียวกับ backend (entity.Severity*) — critical คือ error ที่หลุดออกมาจาก log ของ service
export type AlertSeverity = 'critical' | 'warning' | 'info';

// source_type บอกว่าแจ้งเตือนมาจากไหน — 'service_log' คือตัวสแกน log เจอ error
// หน้าเว็บใช้ค่านี้ตัดสินว่าจะโชว์ปุ่ม "ดู log" หรือไม่
export type AlertSourceType = 'service_log' | 'system';

export interface UserAlert {
  id: number;
  user_id: number;
  severity: AlertSeverity;
  title: string;
  message: string;
  source_type: AlertSourceType;
  source_name: string;
  service_id: number | null;
  // จำนวนครั้งที่ error เรื่องเดียวกันนี้เกิดขึ้น (backend ยุบบรรทัดซ้ำให้แล้ว)
  count: number;
  is_read: boolean;
  created_at: string;
  last_seen_at: string;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
}

export const alertApi = {
  list: async (opts?: { unread?: boolean; limit?: number }) => {
    const params = new URLSearchParams();
    if (opts?.unread) params.set('unread', 'true');
    if (opts?.limit) params.set('limit', String(opts.limit));
    const res = await axiosClient.get<ApiResponse<UserAlert[]>>(`/alerts?${params}`);
    // backend ตอบ array ว่างเป็น null ได้เมื่อไม่มีแถวเลย — normalize ให้ฝั่งหน้าเว็บเจอ array เสมอ
    return res.data.data ?? [];
  },

  // ตัวเลขวงกลมแดงบน Sidebar มาจาก endpoint นี้ตัวเดียว — เบาที่สุดเพราะถูกเรียกถี่ที่สุด
  unreadCount: async () => {
    const res = await axiosClient.get<ApiResponse<{ unread: number }>>('/alerts/unread-count');
    return res.data.data.unread;
  },

  markRead: async (ids: number[]) => {
    const res = await axiosClient.patch<ApiResponse<{ updated: number }>>('/alerts/read', { ids });
    return res.data.data.updated;
  },

  markAllRead: async () => {
    const res = await axiosClient.patch<ApiResponse<{ updated: number }>>('/alerts/read', { all: true });
    return res.data.data.updated;
  },

  remove: async (id: number) => {
    const res = await axiosClient.delete<ApiResponse<{ deleted: number }>>(`/alerts/${id}`);
    return res.data.data.deleted;
  },

  clearRead: async () => {
    const res = await axiosClient.delete<ApiResponse<{ deleted: number }>>('/alerts/read');
    return res.data.data.deleted;
  },
};
