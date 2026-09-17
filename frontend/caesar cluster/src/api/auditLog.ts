import axiosClient from './axiosClient';
import type { ApiResponse } from './adminrequest';

export type AuditEvent = 'CREATE' | 'UPDATE' | 'APPROVE' | 'DELETE';

// แถวของตาราง audit_logs — backend (middlewares.Audit) เขียนให้ทุก POST/PATCH/DELETE ที่สำเร็จ
export interface AuditLog {
  id: number;
  event_type: AuditEvent;
  actor_role: string;
  actor_name: string;
  action_title: string;
  detail: string; // "METHOD /api/path/<id>"
  source_ip: string;
  created_at: string;
}

export const auditLogApi = {
  // 500 แถวล่าสุด ใหม่สุดก่อน
  list: async () => (await axiosClient.get<ApiResponse<AuditLog[]>>('/admin/audit-logs')).data.data,
};
