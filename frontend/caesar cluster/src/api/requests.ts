import axiosClient from './axiosClient';

export interface VmRequest {
  id: number;
  description: string;
  user_id: number;
  status: 'pending' | 'approved' | 'denied';
  namespace_name: string; // "solo" | "group" — ชนิดของ space ไม่ใช่ชื่อจริง
  request_template_id: number | null;
  cpu_limit_milli: number;
  ram_limit_mb: number;
  storage_gb: number; // snapshot จาก template ตอนยื่นคำขอ — 0 ถ้าไม่ได้อ้างอิง template ไหนเลย
  deny_reason: string; // เหตุผลที่ admin เขียนตอนกด Reject — ว่างถ้ายังไม่ถูกปฏิเสธ
  created_at: string;
}

// AdminVmRequest = VmRequest + ข้อมูลผู้ยื่นแบบย่อ (เฉพาะที่ GET /admin/requests คืนมาให้)
export interface AdminVmRequest extends VmRequest {
  requester_name: string;
  requester_student_id: string;
}

export interface CreateVmRequestDTO {
  description?: string;
  namespace_name: 'solo' | 'group';
  request_template_id?: number;
  cpu_limit_milli: number;
  ram_limit_mb: number;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const vmRequestApi = {
  listMine: async () => {
    const response = await axiosClient.get<ApiResponse<VmRequest[]>>('/requests');
    return response.data.data;
  },

  create: async (payload: CreateVmRequestDTO) => {
    const response = await axiosClient.post<ApiResponse<VmRequest>>('/requests', payload);
    return response.data.data;
  },
};

export const adminVmRequestApi = {
  listAll: async () => {
    const response = await axiosClient.get<ApiResponse<AdminVmRequest[]>>('/admin/requests');
    return response.data.data;
  },

  // คืนแถวคำขอหลังอัปเดตมาด้วย หน้าที่เรียกจึงไม่ต้องดึงลิสต์ใหม่ทั้งก้อนเพียงเพื่อดูผลของแถวเดียว
  // (backend ส่ง namespace ที่เพิ่งสร้างมาด้วย แต่หน้า Request Queue ไม่ได้ใช้ จึงไม่ประกาศไว้ใน type)
  approve: async (id: number) => {
    const response = await axiosClient.patch<ApiResponse<{ request: VmRequest }>>(`/admin/requests/${id}/approve`);
    return response.data.data.request;
  },

  // reason บังคับ — backend เก็บไว้กับคำขอ ให้ผู้ยื่นอ่านเหตุผลได้จากหน้า "คำขอของฉัน"
  //
  // ตอบเฉพาะสามฟิลด์ที่เปลี่ยนจริง ไม่ใช่แถวเต็ม — status ผูกชนิดกับ VmRequest["status"] ไว้
  // เพื่อให้ฝั่งหน้าเอาไปวางทับแถวเดิมได้เลยโดยไม่ต้อง cast
  deny: async (id: number, reason: string) => {
    const response = await axiosClient.patch<
      ApiResponse<{ id: number; status: VmRequest['status']; deny_reason: string }>
    >(`/admin/requests/${id}/deny`, { reason });
    return response.data.data;
  },
};

// ---------------------------------------------------------------------------
// ก้อนสรุปของหน้า AdminDashboard — GET /admin/dashboard/summary
//
// หน้านั้นต้องการแค่ "จำนวน" ผู้ใช้/คำขอแยกตามสถานะ กับรายการคำขอที่ยังค้างอยู่
// เท่านั้น — ไม่ใช่รายชื่อผู้ใช้ทั้งระบบหรือคำขอที่ปิดจบไปแล้ว จึงไม่ต้องดึงสองตารางนั้นมาทั้งก้อนทุก 30 วินาที
// ---------------------------------------------------------------------------

export interface DashboardRequestCounts {
  pending: number;
  approved: number;
  denied: number;
  total: number;
}

export interface DashboardTimelinePoint {
  date: string; // YYYY-MM-DD ตามเขตเวลาของเครื่องที่เปิดหน้านี้
  count: number;
}

export interface AdminDashboardSummary {
  user_count: number;
  request_counts: DashboardRequestCounts;
  // ครบ 7 วันเสมอ เรียงเก่า→ใหม่ วันที่ไม่มีคำขอเลยก็มาเป็น count: 0 (backend เติมให้)
  request_timeline: DashboardTimelinePoint[];
  pending_requests: AdminVmRequest[];
}

export const adminDashboardApi = {
  // ส่ง offset ของเขตเวลาไปด้วย เพราะ created_at เก็บเป็น UTC แต่แท่งกราฟต้องแบ่งวัน
  // ตามเวลาที่คนดูเห็น — getTimezoneOffset() นับ "ช้ากว่า UTC กี่นาที" จึงต้องกลับเครื่องหมาย
  summary: async () => {
    const tzOffset = -new Date().getTimezoneOffset();
    const response = await axiosClient.get<ApiResponse<AdminDashboardSummary>>(
      `/admin/dashboard/summary?tz_offset=${tzOffset}`,
    );
    return response.data.data;
  },
};
