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

/**
 * service นี้มีดิสก์ถาวรไหม — ต้องตรงกับ entity.Service.HasStorage ฝั่ง backend (storage_mb > 0)
 *
 * มีดิสก์ = รันได้ 1 pod เท่านั้น และลบแล้วข้อมูลหายถาวร ไม่ว่าจะเป็นฐานข้อมูลหรือ web อย่าง Nextcloud
 * ส่วน is_database บอกแค่เรื่องเครือข่าย (เข้าได้เฉพาะในกลุ่ม) — ใช้แทนกันไม่ได้
 */
export function hasStorage(svc: Pick<AppService, 'storage_mb'>): boolean {
  return svc.storage_mb > 0;
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
  // container_port = พอร์ตที่ image ฟังอยู่ข้างใน, node_port = ทางเข้าจากนอกคลัสเตอร์ (20000-32767)
  // จองให้ตอนเปิดฟอร์ม — คนละชั้นกัน k8s Service เป็นตัวเชื่อมให้เอง · เข้าที่ <host ที่เปิดเว็บ>:<node_port>
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

  // is_database = เข้าได้เฉพาะในกลุ่ม — ที่กระทบหน้าเว็บมากที่สุดคือไม่มี node_port เลย
  // เป็น null ตลอดชีวิต ไม่ใช่ "รออยู่" (ส่วน web ที่มีดิสก์ยังได้ node_port ตามปกติ)
  // database ใหม่มาจาก template เท่านั้น (docs 029) — is_database=true แต่ database_engine ว่าง = database ยุคก่อน template
  is_database: boolean;
  // '' = ไม่ใช่ database จาก template · มีค่า = มี connection URL ให้ (ดู isTemplateDatabase)
  database_engine: '' | DatabaseEngineName;
  database_version: string;
  db_username: string;
  db_name: string;
  // ขนาด PVC เป็น MB (0 = ไม่มีดิสก์ถาวร) — เช็คว่ามีดิสก์ด้วย hasStorage()
  storage_mb: number;
  // จุดที่ดิสก์ถูก mount เข้าไปใน container ('' = ไม่มีดิสก์ถาวร)
  data_path: string;
  // null = ไม่ได้ตั้งเวลาลบ
  delete_at: string | null;

  created_at: string;
}

/** database ที่สร้างจาก template — มี credential ใน Secret และ connection URL */
export function isTemplateDatabase(svc: Pick<AppService, 'database_engine'>): boolean {
  return svc.database_engine !== '';
}

export type DatabaseEngineName = 'postgresql' | 'mysql' | 'mariadb';

/** GET /api/database-templates — backend กรอง version ที่หมด support (EOL) ออกให้แล้ว */
export interface DatabaseTemplate {
  engine: DatabaseEngineName;
  label: string;
  port: number;
  min_ram_mb: number;
  versions: { version: string; eol: string; lts: boolean; default: boolean }[];
}

export interface CreateDatabaseDTO {
  engine: DatabaseEngineName;
  version: string;
  name: string;
  username: string;
  password: string;
  database: string;
  storage_mb: number;
  cpu_milli: number;
  ram_mb: number;
}

/** GET /api/services/:id/connection — มีรหัสผ่าน จึงเรียกเฉพาะตอนผู้ใช้กด "แสดง" */
export interface DatabaseConnection {
  engine: DatabaseEngineName;
  version: string;
  host: string;
  fqdn: string;
  port: number;
  username: string;
  password: string;
  database: string;
  url: string;
}

export interface UpdateServiceDTO {
  name?: string;
  image?: string;
  request_template_id?: number;
  cpu_milli?: number;
  ram_mb?: number;
  container_port?: number;
  replicas?: number;
  env_vars?: Record<string, string>;
  is_database?: boolean;
  storage_mb?: number;
  data_path?: string;
}
export interface AdminService extends AppService {
  namespace_name: string;
  creator_name: string;
  creator_student_id: string;
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
  storage_mb?: number;
  data_path?: string;
  // เลขจาก reserveNodePort เท่านั้น — เลขอื่น backend ตอบ 409 NODE_PORT_RESERVATION_EXPIRED
  node_port?: number;
}

/** ใบจอง NodePort ตอนเปิดฟอร์ม New Service (docs 031) */
export interface NodePortReservation {
  node_port: number;
  expires_at: string;
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

  // จองพอร์ตว่างให้ฟอร์ม New Service — เปิดฟอร์มซ้ำระหว่างใบจองยังไม่หมดอายุได้เลขเดิม
  reserveNodePort: async () => {
    const response = await axiosClient.post<ApiResponse<NodePortReservation>>('/services/node-port-reservation');
    return response.data.data;
  },

  databaseTemplates: async () => {
    const response = await axiosClient.get<ApiResponse<DatabaseTemplate[]>>('/database-templates');
    return Array.isArray(response.data.data) ? response.data.data : [];
  },

  createDatabase: async (payload: CreateDatabaseDTO) => {
    const response = await axiosClient.post<ApiResponse<AppService>>('/databases', payload);
    return response.data.data;
  },

  connection: async (id: number) => {
    const response = await axiosClient.get<ApiResponse<DatabaseConnection>>(`/services/${id}/connection`);
    return response.data.data;
  },

  update: async (id: number, payload: UpdateServiceDTO): Promise<AppService> => {
    const response = await axiosClient.patch<ApiResponse<AppService>>(`/services/${id}`, payload);
    return response.data.data; 
  },
};

export const adminServiceApi = {
  listAll: async () => {
    const response = await axiosClient.get<ApiResponse<AdminService[]>>('/admin/services');
    return Array.isArray(response.data.data) ? response.data.data : [];
  },

  // reason บังคับ — backend ส่งทางอีเมลถึงสมาชิกใน namespace
  remove: async (id: number, reason: string) => {
    const response = await axiosClient.delete<ApiResponse<{ deleted: number }>>(`/admin/services/${id}`, {
      data: { reason },
    });
    return response.data.data;
  },

  scheduleDelete: async (id: number, reason: string) => {
    const response = await axiosClient.post<ApiResponse<{ id: number; delete_at: string }>>(
      `/admin/services/${id}/schedule-delete`,
      { reason },
    );
    return response.data.data;
  },

  cancelScheduledDelete: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ id: number; delete_at: null }>>(
      `/admin/services/${id}/schedule-delete`,
    );
    return response.data.data;
  },
};
