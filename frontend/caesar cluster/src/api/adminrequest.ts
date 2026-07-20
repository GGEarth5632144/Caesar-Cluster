import axiosClient from './axiosClient';

export interface RequestTemplate {
  id: number;
  option_name: string;
  category: string;
  description: string;
  relate_subject: string;
  cpu_limit_milli: number;
  ram_limit_mb: number;
  storage_gb: number;
  is_active: boolean;
  created_at: string;
}

export interface CreateRequestTemplateDTO {
  option_name: string;
  category: string;
  description: string;
  relate_subject: string;
  cpu_limit_milli: number;
  ram_limit_mb: number;
  storage_gb: number;
}

// สร้าง Type มารองรับรูปแบบที่ utils.OK ของ Go ส่งมา
export interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const requestTemplateApi = {
  // ดึงข้อมูลทั้งหมด (เปลี่ยนไปเรียกเส้น admin เพื่อให้เห็นอันที่ is_active=false ด้วย)
  getAll: async () => {
    const response = await axiosClient.get<ApiResponse<RequestTemplate[]>>('/admin/request-templates');
    // ต้อง .data 2 รอบ: รอบแรกของ axios, รอบสองของ utils.OK ที่ห่อมา
    return response.data.data; 
  },

  create: async (payload: CreateRequestTemplateDTO) => {
    const response = await axiosClient.post<ApiResponse<RequestTemplate>>('/admin/request-templates', payload);
    return response.data.data;
  },

  update: async (id: number, payload: Partial<CreateRequestTemplateDTO> & { is_active?: boolean }) => {
    const response = await axiosClient.patch<ApiResponse<RequestTemplate>>(`/admin/request-templates/${id}`, payload);
    return response.data.data;
  },

  delete: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<any>>(`/admin/request-templates/${id}`);
    return response.data.data;
  }
};