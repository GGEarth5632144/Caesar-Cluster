import axiosClient from './axiosClient';

export interface VmRequest {
  id: number;
  description: string;
  user_id: number;
  status: 'pending' | 'approved' | 'denied';
  namespace_name: string; // "solo" | "group" — ชนิดของ space ไม่ใช่ชื่อจริง
  cpu_limit_milli: number;
  ram_limit_mb: number;
  created_at: string;
}

export interface CreateVmRequestDTO {
  description?: string;
  namespace_name: 'solo' | 'group';
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
    const response = await axiosClient.get<ApiResponse<VmRequest[]>>('/admin/requests');
    return response.data.data;
  },

  approve: async (id: number) => {
    const response = await axiosClient.patch<ApiResponse<{ request: VmRequest }>>(`/admin/requests/${id}/approve`);
    return response.data.data;
  },

  deny: async (id: number) => {
    const response = await axiosClient.patch<ApiResponse<{ id: number; status: string }>>(`/admin/requests/${id}/deny`);
    return response.data.data;
  },
};
