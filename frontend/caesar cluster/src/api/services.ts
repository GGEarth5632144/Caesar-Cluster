import axiosClient from './axiosClient';

/**
 * สถานะของ service — ต้องตรงกับค่าคงที่ใน backend/internal/entity/service.go
 *
 * running แปลว่าระบบถามคลัสเตอร์แล้วยืนยันว่ามี pod รันอยู่จริง ไม่ใช่แค่สั่ง deploy ผ่าน
 */
export type ServiceStatus = 'creating' | 'pending' | 'running' | 'crashloop' | 'failed';

/** สถานะที่นิ่งแล้ว — หน้าเว็บหยุดดึงข้อมูลซ้ำเมื่อทุกตัวเข้าสถานะกลุ่มนี้ */
export function isSettled(status: ServiceStatus): boolean {
  return status === 'running' || status === 'failed';
}

export interface AppService {
  id: number;
  namespace_id: number;
  name: string;
  created_by: number;
  request_template_id: number | null;
  image: string;
  cpu_milli: number;
  ram_mb: number;
  // container_port = พอร์ตที่ image ฟังอยู่ข้างใน, node_port = ทางเข้าจากนอกคลัสเตอร์ที่ k8s จ่ายให้
  // (30000-32767) — คนละชั้นกัน k8s Service เป็นตัวเชื่อมให้เอง
  container_port: number;
  node_port: number | null;
  // จำนวน Pod ที่รันขนานกัน — หักโควตากลุ่มเป็น cpu_milli x replicas
  replicas: number;
  status: ServiceStatus;

  // รหัสสั้นๆ จากคลัสเตอร์ (CrashLoopBackOff, ImagePullBackOff, ...) ใช้เลือกคำแนะนำที่ตรงกับสาเหตุ
  status_reason: string;
  // คำอธิบาย + log ท้ายๆ ก่อน container ตาย — backend เก็บให้เพราะ log ตัวจริงหายไปกับ pod ที่ถูกสร้างใหม่
  status_message: string;
  // แยก "ช้า" ออกจาก "ตายแล้วเกิดใหม่วนไป"
  restart_count: number;
  // ครั้งล่าสุดที่ระบบถามคลัสเตอร์สำเร็จ (null = ยังไม่เคยถามได้เลย)
  status_checked_at: string | null;

  env_vars: Record<string, string>;

  // สวิตช์ที่ผู้ใช้กดตอนสร้าง (ดู entity.Service ฝั่ง backend) — ที่กระทบหน้าเว็บมากที่สุดคือ
  // database ไม่มี node_port เลย เป็น null ตลอดชีวิต ไม่ใช่ "รออยู่"
  is_database: boolean;
  // ขนาด PVC เป็น MB (0 ถ้าไม่ใช่ database)
  storage_mb: number;
  // จุดที่ดิสก์ถูก mount เข้าไปใน container ('' ถ้าไม่ใช่ database)
  data_path: string;

  created_at: string;
}

export interface CreateServiceDTO {
  name: string;
  image: string;
  request_template_id?: number;
  cpu_milli?: number;
  ram_mb?: number;
  container_port?: number;
  replicas?: number;
  env_vars?: Record<string, string>;
  is_database?: boolean;
  // ส่งเป็น MB เสมอ — หน้าเว็บแปลงจากหน่วยที่ผู้ใช้เลือกให้แล้ว (ดู config/database.ts)
  storage_mb?: number;
  // data_path บังคับส่งทุกครั้งที่ is_database — ระบบไม่เดาจุด mount ให้
  data_path?: string;
}

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const serviceApi = {
  list: async () => {
    const response = await axiosClient.get<ApiResponse<AppService[]>>('/services');
    return response.data.data;
  },

  create: async (payload: CreateServiceDTO) => {
    const response = await axiosClient.post<ApiResponse<AppService>>('/services', payload);
    return response.data.data;
  },

  // ปรับจำนวน Pod ของ service ที่ deploy แล้ว — backend เช็คโควตาให้ก่อน (ไม่พอได้ 409 QUOTA_EXCEEDED)
  scale: async (id: number, replicas: number) => {
    const response = await axiosClient.patch<ApiResponse<AppService>>(`/services/${id}/scale`, {
      replicas,
    });
    return response.data.data;
  },

  remove: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ deleted: number }>>(`/services/${id}`);
    return response.data.data;
  },
};
