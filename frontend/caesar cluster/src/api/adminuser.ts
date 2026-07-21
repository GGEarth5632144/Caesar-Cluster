import axiosClient from './axiosClient';

// โครงสร้างข้อมูล User ที่จะได้จาก Backend
export interface User {
  id: number;
  student_id: string;
  role_id: number;
  real_name: string;
  nick_name: string;
  namespace_id: number | null;
  gmail: string;
  year: number; // ปีที่เข้าศึกษา (พ.ศ. เช่น 2566) — ค่าดิบ ไม่ใช่ชั้นปีปัจจุบัน อย่าเอาไปโชว์ตรงๆ ใช้ year_level แทน
  year_level: number; // ชั้นปีปัจจุบัน คำนวณสดจาก student_id ทุกครั้งที่เรียก (เช่น 4) — ใช้ตัวนี้โชว์
  // โควตาของ namespace ที่ผู้ใช้สังกัด (โควตาผูกกับ namespace ไม่ใช่ user แล้ว)
  // ผู้ใช้ที่ยังไม่มี space จะได้ 0 ทั้งคู่ — cpu เป็น millicore, ram เป็น MB
  cpu_limit_milli: number;
  ram_limit_mb: number;
  created_at: string;
}

// โครงสร้างข้อมูลสำหรับส่งไปแก้ไข (ตรงกับ UpdateUserRequest DTO)
// ใช้ Partial เพื่อให้รองรับการส่งไปแค่บางฟิลด์ได้
export interface UpdateUserDTO {
  student_id?: string;
  real_name?: string;
  gmail?: string;
  nick_name?: string;
  year?: number;
  role_id?: number;
}

// สร้าง Type มารองรับรูปแบบที่ utils.OK ของ Go ส่งมา
export interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const userManagementApi = {
  // ดึงรายชื่อผู้ใช้งานทั้งหมด
  getAll: async () => {
    const response = await axiosClient.get<ApiResponse<User[]>>('/admin/users');
    // ต้อง .data 2 รอบ: รอบแรกของ axios, รอบสองของ utils.OK ที่ห่อมา
    return response.data.data; 
  },

  // แก้ไขข้อมูลผู้ใช้งาน
  update: async (id: number, payload: UpdateUserDTO) => {
    const response = await axiosClient.patch<ApiResponse<User>>(`/admin/users/${id}`, payload);
    return response.data.data;
  },

  // ลบผู้ใช้งาน
  delete: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ message: string }>>(`/admin/users/${id}`);
    return response.data.data;
  }
};