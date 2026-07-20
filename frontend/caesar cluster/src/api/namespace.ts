import axiosClient from './axiosClient';

export interface NamespaceUsage {
  used_cpu_milli: number;
  used_ram_mb: number;
  service_count: number;
}

export interface NamespaceDetail {
  id: number;
  name: string;
  type: 'solo' | 'group';
  contributor_id: number;
  cpu_limit_milli: number;
  ram_limit_mb: number;
  max_services: number;
  created_at: string;
  usage: NamespaceUsage;
  member_count: number;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const namespaceApi = {
  mine: async () => {
    const response = await axiosClient.get<ApiResponse<NamespaceDetail>>('/namespaces/me');
    return response.data.data;
  },
};
