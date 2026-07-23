import axios from 'axios';

import { useAuthStore } from '@/store/authStore';
import { PATHS } from '@/config/routes';

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

// token หมดอายุ/ไม่ถูกต้อง (401) — เคลียร์ session แล้วเด้งกลับไปหน้า login แทนที่จะปล่อยให้ค้าง
// อยู่หน้าที่ดูเหมือน login อยู่แต่ยิง request อะไรก็ไม่ผ่านสักอย่าง
axiosClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401 && useAuthStore.getState().token) {
      useAuthStore.getState().logout();
      window.location.assign(PATHS.login);
    }
    return Promise.reject(error);
  },
);

export default axiosClient;