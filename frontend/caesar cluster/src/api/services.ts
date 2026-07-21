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
  node_port: number | null;
  status: ServiceStatus;
  created_at: string;
}

export interface CreateServiceDTO {
  name: string;
  image: string;
  request_template_id?: number;
  cpu_milli?: number;
  ram_mb?: number;
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

  remove: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ deleted: number }>>(`/services/${id}`);
    return response.data.data;
  },
};
