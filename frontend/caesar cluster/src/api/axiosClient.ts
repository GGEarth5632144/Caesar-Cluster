import axios from 'axios';

import { useAuthStore } from '@/store/authStore';

const axiosClient = axios.create({
  // ดึงค่าจากไฟล์ .env มาใช้ผ่านคำสั่ง import.meta.env
  baseURL: import.meta.env.VITE_API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

axiosClient.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export default axiosClient;