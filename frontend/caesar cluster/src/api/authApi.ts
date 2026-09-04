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

/** ล็อกอินผ่านครบแล้ว — ได้ token มาใช้งานได้เลย */
export interface SessionResponse {
  token: string;
  user: AuthUser;
}

/**
 * สมัครสำเร็จแล้ว แต่บัญชียังใช้ไม่ได้จนกว่าจะกดลิงก์ยืนยันในอีเมล
 * backend ส่งอีเมลเสร็จจริงก่อนตอบ (อยู่ใน transaction เดียวกับการสร้างบัญชี) หน้าเว็บจึงพูดได้
 * เต็มปากว่า "ส่งไปแล้ว" ไม่ใช่ "กำลังส่ง"
 */
export interface RegisterResponse {
  gmail: string;
  message: string;
}

/**
 * ผลของการกดลิงก์ยืนยัน — กดครั้งแรก (already: false) ได้ session มาเลย เข้า dashboard ได้ทันที
 * กดซ้ำหรือตัวสแกนอีเมลกดให้ก่อน (already: true) ไม่ได้ session เพราะลิงก์ใช้ได้ครั้งเดียวจริงๆ
 */
export type VerifyEmailResponse = ({ already: false } & SessionResponse) | { already: true };

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

/**
 * อ่าน error code จาก response ของ backend (เช่น EMAIL_NOT_VERIFIED, TOKEN_EXPIRED)
 * ใช้ code ไม่ใช่ข้อความ ตอนที่หน้าเว็บต้องทำอะไรต่างออกไป — เทียบข้อความภาษาไทยจะพังทันทีที่มีคนแก้คำ
 */
export function getApiErrorCode(err: unknown): string {
  if (axios.isAxiosError<ApiError>(err) && err.response?.data?.error?.code) {
    return err.response.data.error.code;
  }
  return '';
}

export const authApi = {
  // ดึงสถานะล่าสุดของบัญชี — role/namespace_id อาจเปลี่ยนไปแล้วตั้งแต่ตอน login
  // backend อ่านจาก DB สดทุก request อยู่แล้ว หน้าเว็บต้องซิงก์ตาม ไม่งั้นเมนูไม่ตรงกับสิทธิ์จริง
  me: async () => {
    const response = await axiosClient.get<{ data: AuthUser }>('/me');
    return response.data.data;
  },

  // ล็อกอิน — บัญชีที่ยังไม่ยืนยันอีเมลจะได้ 403 EMAIL_NOT_VERIFIED กลับมา (ไม่ใช่ 200)
  // หน้า Login จับ code นั้นแล้วเสนอช่องขอลิงก์ยืนยันใหม่ให้
  login: async (payload: { student_id: string; password: string; remember: boolean }) => {
    const response = await axiosClient.post<{ data: SessionResponse }>('/login', payload);
    return response.data.data;
  },

  // สมัครสมาชิก — สำเร็จแล้วยังไม่ได้ token: ต้องไปกดลิงก์ในอีเมลก่อน
  // ใช้เวลานานกว่า request อื่นเพราะ backend รอผลการส่งอีเมลจริงก่อนตอบกลับ (สูงสุด ~15 วิ)
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

  // แลก token จากลิงก์ในอีเมลเป็นการเปิดใช้งานบัญชี + session สำหรับเข้าใช้งานต่อ
  verifyEmail: async (payload: { token: string }) => {
    const response = await axiosClient.post<{ data: VerifyEmailResponse }>('/verify-email', payload);
    return response.data.data;
  },

  // ขอลิงก์ยืนยันใบใหม่ — backend ตอบ generic message เสมอ (ไม่บอกว่ามีบัญชีนี้ในระบบไหม)
  resendVerification: async (payload: { gmail: string }) => {
    const response = await axiosClient.post<{ data: { message: string } }>(
      '/resend-verification',
      payload,
    );
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
