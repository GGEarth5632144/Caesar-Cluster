import axios from 'axios';

import axiosClient from './axiosClient';

export interface AuthUser {
  id: number;
  student_id: string;
  real_name: string;
  nick_name: string;
  gmail: string;
  year_level: number;
  role: string;
  namespace_id: number | null;
}

export interface LoginResponse {
  token: string;
  user: AuthUser;
}

export interface RegisterResponse {
  id: number;
  student_id: string;
  real_name: string;
  nick_name: string;
  gmail: string;
  major: string;
}

interface ApiError {
  success: false;
  error: { code: string; message: string };
}

// ทุก endpoint ของ backend ห่อ response เป็น { success, data } หรือ { success: false, error: { code, message } }
export function getApiErrorMessage(err: unknown, fallback: string) {
  if (axios.isAxiosError<ApiError>(err) && err.response?.data?.error?.message) {
    return err.response.data.error.message;
  }
  return fallback;
}

export const authApi = {
  login: async (payload: { student_id: string; password: string }) => {
    const response = await axiosClient.post<{ data: LoginResponse }>('/login', payload);
    return response.data.data;
  },

  register: async (payload: {
    student_id: string;
    real_name: string;
    nick_name: string;
    gmail: string;
    password: string;
  }) => {
    const response = await axiosClient.post<{ data: RegisterResponse }>('/register', payload);
    return response.data.data;
  },

  // ขอลิงก์รีเซ็ตรหัสผ่าน — backend ตอบ generic message เสมอ (ไม่บอกว่ามี email นี้ในระบบไหม)
  forgotPassword: async (payload: { gmail: string }) => {
    const response = await axiosClient.post<{ data: { message: string } }>(
      '/forgot-password',
      payload,
    );
    return response.data.data;
  },

  // ตั้งรหัสผ่านใหม่ด้วย token จากลิงก์ในอีเมล
  resetPassword: async (payload: { token: string; new_password: string }) => {
    const response = await axiosClient.post<{ data: { message: string } }>(
      '/reset-password',
      payload,
    );
    return response.data.data;
  },
};
