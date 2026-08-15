import axiosClient from './axiosClient';

export type InviteStatus = 'pending' | 'accepted' | 'declined';

export interface InviteDetail {
  id: number;
  namespace_id: number;
  invited_student_id: string;
  invited_by: number;
  status: InviteStatus;
  created_at: string;
  namespace_name: string;
  invited_by_name: string;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const inviteApi = {
  // เชิญ student_id คนหนึ่งเข้ากลุ่มของตัวเอง — ต้องเป็นเจ้าของ (contributor) เท่านั้นถึงเรียกได้
  create: async (studentId: string) => {
    const response = await axiosClient.post<ApiResponse<InviteDetail>>('/namespaces/invites', {
      student_id: studentId,
    });
    return response.data.data;
  },

  // คำเชิญที่ pending อยู่ ส่งถึงตัวเอง
  mine: async () => {
    const response = await axiosClient.get<ApiResponse<InviteDetail[]>>('/namespaces/invites/mine');
    return response.data.data;
  },

  // คำเชิญทั้งหมด (ทุกสถานะ) ที่ตัวเองเคยส่งจากกลุ่มของตัวเอง
  sent: async () => {
    const response = await axiosClient.get<ApiResponse<InviteDetail[]>>('/namespaces/invites/sent');
    return response.data.data;
  },

  accept: async (inviteId: number) => {
    const response = await axiosClient.patch<ApiResponse<unknown>>(`/namespaces/invites/${inviteId}/accept`);
    return response.data.data;
  },

  decline: async (inviteId: number) => {
    await axiosClient.patch(`/namespaces/invites/${inviteId}/decline`);
  },

  cancel: async (inviteId: number) => {
    await axiosClient.delete(`/namespaces/invites/${inviteId}`);
  },
};
