import axiosClient from './axiosClient';

// กำหนดหน้าตาข้อมูล (Type)
export interface User {
  id: number;
  name: string;
  email: string;
}

export const userApi = {
  // ดึงข้อมูลทั้งหมด (GET)
  getAllUsers: async () => {
    const response = await axiosClient.get<User[]>('/users');
    return response.data;
  },

  // สร้างข้อมูลใหม่ (POST) - ที่คุณเรียกว่า Push
  createUser: async (userData: { name: string; email: string }) => {
    const response = await axiosClient.post<User>('/users', userData);
    return response.data;
  },

  getme: async () => {
    const response = await axiosClient.get<User>('/me');
    return response.data;
  }
};