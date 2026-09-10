import axiosClient from './axiosClient';

export interface NamespaceUsage {
  used_cpu_milli: number;
  used_ram_mb: number;
  service_count: number;
}

export interface MemberInfo {
  id: number;
  student_id: string;
  real_name: string;
  is_contributor: boolean;
}

export interface NamespaceDetail {
  id: number;
  name: string;
  contributor_id: number;
  cpu_limit_milli: number;
  ram_limit_mb: number;
  created_at: string;
  usage: NamespaceUsage;
  member_count: number;
  members: MemberInfo[];
}

// body ของ PATCH /admin/namespaces/:id/quota — ตรงกับ dto.SetQuotaRequest ฝั่ง backend
export interface SetQuotaPayload {
  cpu_limit_milli: number;
  ram_limit_mb: number;
}

// เพดาน/พื้นที่ว่างของโควตา — ค่าเดียวกับ entity.MaxCPULimitMilli / MaxRAMLimitMB และ binding
// ของ dto.SetQuotaRequest (min=100/128, max=8000/8192) ประกาศไว้ตรงนี้เพื่อให้ฟอร์มฝั่งหน้าเว็บ
// กันค่าที่เกินไว้ก่อน ไม่ต้องรอ backend ตอบ 400 กลับมา — แก้ที่นี่ต้องแก้ที่ backend ด้วยเสมอ
export const QUOTA_BOUNDS = {
  cpu: { min: 100, max: 8000, step: 100 },
  ram: { min: 128, max: 8192, step: 128 },
} as const;

interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export const namespaceApi = {
  mine: async () => {
    const response = await axiosClient.get<ApiResponse<NamespaceDetail>>('/namespaces/me');
    return response.data.data;
  },
};

/**
 * ฝั่งผู้ดูแล — หน้า Namespace Management ใช้ทั้งสามเส้นนี้
 *
 * ทุกเส้นผ่าน middleware AdminOnly ฝั่ง backend มาแล้ว (ดู router.Setup)
 * โควตาผูกกับ namespace ไม่ใช่ user เพราะฉะนั้นการปรับโควตาให้ใครสักคน
 * คือการปรับโควตาของ space ที่เขาสังกัดอยู่ ซึ่งกระทบสมาชิกทุกคนในนั้นพร้อมกัน
 */
export const adminNamespaceApi = {
  // ทั้งระบบ พร้อมยอดใช้งานและรายชื่อสมาชิกของแต่ละ space
  listAll: async () => {
    const response = await axiosClient.get<ApiResponse<NamespaceDetail[]>>('/admin/namespaces');
    return Array.isArray(response.data.data) ? response.data.data : [];
  },

  // ปรับเพดานทรัพยากรของ space — backend sync ResourceQuota ขึ้นคลัสเตอร์ให้เอง
  setQuota: async (id: number, payload: SetQuotaPayload) => {
    const response = await axiosClient.patch<ApiResponse<NamespaceDetail>>(
      `/admin/namespaces/${id}/quota`,
      payload,
    );
    return response.data.data;
  },

  // ลบทั้ง space — service/รีวิว/คอนเทนเนอร์ในนั้นถูกลบตาม ส่วนสมาชิกแค่หลุดออกจาก space ไม่ถูกลบบัญชี
  remove: async (id: number) => {
    const response = await axiosClient.delete<ApiResponse<{ deleted: number }>>(
      `/admin/namespaces/${id}`,
    );
    return response.data.data;
  },
};

// ---------------------------------------------------------------------------
// ตัวช่วยคำนวณที่หน้า Namespace Management กับ namespacesScope ใช้ร่วมกัน
//
// อยู่ที่นี่เพราะทั้งสองที่ต้องตัดสิน "space นี้อยู่ในสภาพไหน" ด้วยเกณฑ์เดียวกันเป๊ะ —
// ถ้าแยกกันเขียน แท็บที่กดกับผลลัพธ์ที่ค้นด้วย state: จะไม่ตรงกันทันทีที่มีใครแก้เกณฑ์ข้างเดียว
// ---------------------------------------------------------------------------

/** เกณฑ์ที่ถือว่า "ใกล้เต็ม" — ใช้ทั้งสีของแถบและการจัดสถานะ */
export const NAMESPACE_FULL_THRESHOLD = 80;

/** เจ้าของ space (ผู้สร้าง) — คืน undefined ถ้าเจ้าของออกจากกลุ่มไปแล้วแต่ space ยังอยู่ */
export function namespaceOwner(ns: NamespaceDetail): MemberInfo | undefined {
  return ns.members.find((m) => m.is_contributor);
}

/** เปอร์เซ็นต์ที่ใช้ไปของโควตา — peak คือด้านที่ตึงกว่า ใช้ตัดสินสถานะของทั้ง space */
export function namespaceUsagePercent(ns: NamespaceDetail) {
  const cpu = ns.cpu_limit_milli > 0 ? (ns.usage.used_cpu_milli / ns.cpu_limit_milli) * 100 : 0;
  const ram = ns.ram_limit_mb > 0 ? (ns.usage.used_ram_mb / ns.ram_limit_mb) * 100 : 0;
  return { cpu, ram, peak: Math.max(cpu, ram) };
}

export type NamespaceState = 'empty' | 'full' | 'idle' | 'active';

/**
 * สภาพของ space หนึ่งอัน — เรียงตามความเร่งด่วนที่แอดมินต้องเข้าไปดู
 *
 * empty  = ไม่เหลือสมาชิกสักคน (สมาชิกออกหมดแต่ space ยังค้างอยู่ ควรพิจารณาลบ)
 * full   = ใช้โควตาไปแล้ว ≥ 80% ของด้านใดด้านหนึ่ง (จะขอ deploy เพิ่มไม่ได้ในอีกไม่นาน)
 * idle   = มีสมาชิกแต่ยังไม่มี service สักตัว
 * active = ใช้งานอยู่ตามปกติ
 */
export function namespaceState(ns: NamespaceDetail): NamespaceState {
  if (ns.member_count === 0) return 'empty';
  if (namespaceUsagePercent(ns).peak >= NAMESPACE_FULL_THRESHOLD) return 'full';
  if (ns.usage.service_count === 0) return 'idle';
  return 'active';
}
