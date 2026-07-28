import axiosClient from './axiosClient';

// "ใบเสร็จ" ของ deploy request ที่ส่งเข้า Cluster-AI — เก็บไว้ที่ Caesar-Cluster เอง (คนละที่กับ
// aiDeployApi/aiReviewApi ใน aiDeployApi.ts ซึ่งคุยกับ Cluster-AI โดยตรง) เพราะ Cluster-AI เก็บแค่สถานะ
// pipeline ใน memory ไม่ได้เก็บ service_name/image/cpu/ram ที่ submit มาด้วย
//
// ใช้ตอน AIReviewPage.tsx โดน refresh/เปิดลิงก์ตรงแล้ว router state (location.state) หายไป
export interface AIReviewRequestRecord {
  id: number;
  request_id: string;
  namespace_id: number;
  created_by: number;
  service_name: string;
  image: string;
  request_template_id: number | null;
  cpu_milli: number;
  ram_mb: number;
  env_vars: Record<string, string>;
  created_at: string;
}

export interface CreateAIReviewRequestDTO {
  request_id: string;
  service_name: string;
  image: string;
  request_template_id?: number | null;
  cpu_milli: number;
  ram_mb: number;
  env_vars?: Record<string, string>;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const aiReviewRequestApi = {
  create: async (payload: CreateAIReviewRequestDTO) => {
    const response = await axiosClient.post<ApiResponse<AIReviewRequestRecord>>('/ai-review-requests', payload);
    return response.data.data;
  },

  get: async (requestId: string) => {
    const response = await axiosClient.get<ApiResponse<AIReviewRequestRecord>>(`/ai-review-requests/${requestId}`);
    return response.data.data;
  },
};
