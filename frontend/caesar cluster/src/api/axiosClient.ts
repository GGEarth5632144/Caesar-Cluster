import axios from 'axios';

const axiosClient = axios.create({
  // ดึงค่าจากไฟล์ .env มาใช้ผ่านคำสั่ง import.meta.env
  baseURL: import.meta.env.VITE_API_URL, 
  headers: {
    'Content-Type': 'application/json',
  },
});

export default axiosClient;