import axiosClient from './axiosClient';

export type ServiceStatus = 'creating' | 'running' | 'failed';

export interface AppService {
  id: number;
  namespace_id: number;
  name: string;
  created_by: number;
  request_template_id: number | null;
  image: string;
  cpu_milli: number;
  ram_mb: number;
  // container_port = พอร์ตที่ image ฟังอยู่ข้างใน, node_port = ทางเข้าจากนอกคลัสเตอร์ที่ k8s จ่ายให้
  // (30000-32767) — คนละชั้นกัน k8s Service เป็นตัวเชื่อมให้เอง
  container_port: number;
  node_port: number | null;
  // จำนวน Pod ที่รันขนานกัน — หักโควตากลุ่มเป็น cpu_milli x replicas
  replicas: number;
  status: ServiceStatus;
  env_vars: Record<string, string>;
  created_at: string;
}

export interface CreateServiceDTO {
  name: string;
  image: string;
  request_template_id?: number;
  cpu_milli?: number;
  ram_mb?: number;
  container_port?: number;
  replicas?: number;
  env_vars?: Record<string, string>;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const serviceApi = {
  list: async () => {
    const response = await axiosClient.get<ApiResponse<AppService[]>>('/services');
    return response.data.data;
  },

  create: async (payload: CreateServiceDTO) => {
    const response = await axiosClient.post<ApiResponse<AppService>>('/services', payload);
    return response.data.data;
  },

  // ปรับจำนวน Pod ของ service ที่ deploy แล้ว — backend เช็คโควตาให้ก่อน (ไม่พอได้ 409 QUOTA_EXCEEDED)
  scale: async (id: number, replicas: number) => {
    const response = await axiosClient.patch<ApiResponse<AppService>>(`/services/${id}/scale`, {
      replicas,
    });
    return response.data.data;
  },

  remove: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ deleted: number }>>(`/services/${id}`);
    return response.data.data;
  },
};
