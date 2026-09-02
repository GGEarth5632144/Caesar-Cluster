import axios from 'axios';

import axiosClient from './axiosClient';
import { getDeviceToken, setDeviceToken } from '@/lib/otpSession';

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
  otp_required: false;
  token: string;
  user: AuthUser;
  /** มีเฉพาะตอนมาจาก /verify-otp — ใบผ่านของเครื่องนี้สำหรับข้าม OTP ครั้งหน้า */
  device_token?: string;
}

/** รหัสผ่านถูกแล้ว แต่ยังต้องกรอก OTP ที่ส่งไปทางอีเมลก่อนถึงจะได้ token */
export interface OtpRequiredResponse {
  otp_required: true;
  challenge_token: string;
  gmail_masked: string;
  expires_in_seconds: number;
}

// /api/login ตอบได้สองแบบด้วย HTTP 200 ทั้งคู่ (ไม่ใช่ error — รหัสผ่านถูกแล้วทั้งคู่)
// แยกด้วย otp_required ซึ่ง backend ส่งมาเสมอ ไม่ต้องเดาจากการมี/ไม่มีของ field อื่น
export type LoginResult = SessionResponse | OtpRequiredResponse;

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
  // ดึงสถานะล่าสุดของบัญชีตัวเอง — role/namespace_id อาจเปลี่ยนไปแล้วตั้งแต่ตอน login
  // backend อ่านค่าพวกนี้จาก DB สดทุก request อยู่แล้ว หน้าเว็บจึงต้องซิงก์ตาม
  // ไม่งั้นเมนูที่โชว์จะไม่ตรงกับสิทธิ์จริง (ดู ProtectedRoute)
  me: async () => {
    const response = await axiosClient.get<{ data: AuthUser }>('/me');
    return response.data.data;
  },

  // แนบ device token ของเครื่องนี้ไปทุกครั้ง — backend รู้จักก็ข้ามขั้น OTP ให้เลย
  // ทำที่ชั้น api ไม่ใช่ที่หน้า login เพราะเป็นเรื่องของ "เครื่อง" ไม่ใช่ของฟอร์ม
  // (หลักการเดียวกับที่ axiosClient แนบ Authorization ให้เองโดยหน้าไม่ต้องรู้)
  login: async (payload: { student_id: string; password: string; remember: boolean }) => {
    const response = await axiosClient.post<{ data: LoginResult }>('/login', {
      ...payload,
      device_token: getDeviceToken(),
    });
    return response.data.data;
  },

  // แลกรหัส 6 หลักเป็น token — สำเร็จแล้วเก็บ device token ที่ backend ออกให้ไว้ในเครื่อง
  // ล็อกอินครั้งถัดไปภายใน 30 วันจะได้ไม่ต้องกรอก OTP อีก
  verifyOtp: async (payload: { challenge_token: string; code: string }) => {
    const response = await axiosClient.post<{ data: SessionResponse }>('/verify-otp', payload);
    const data = response.data.data;
    if (data.device_token) {
      setDeviceToken(data.device_token);
    }
    return data;
  },

  // ขอให้ส่งรหัสใหม่ไปที่อีเมลเดิม — ใช้ใบเดิม challenge_token จึงไม่เปลี่ยน
  resendOtp: async (payload: { challenge_token: string }) => {
    const response = await axiosClient.post<{
      data: { message: string; expires_in_seconds: number };
    }>('/resend-otp', payload);
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
