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

  approve: async (id: number) => {
    const response = await axiosClient.patch<ApiResponse<{ request: VmRequest }>>(`/admin/requests/${id}/approve`);
    return response.data.data;
  },

  // reason บังคับ — backend เก็บไว้กับคำขอและยิงเป็นแจ้งเตือนไปหาผู้ยื่น (หน้า Alerts ของเขา)
  deny: async (id: number, reason: string) => {
    const response = await axiosClient.patch<ApiResponse<{ id: number; status: string; deny_reason: string }>>(
      `/admin/requests/${id}/deny`,
      { reason },
    );
    return response.data.data;
  },
};
