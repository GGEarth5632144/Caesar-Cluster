import axiosClient from './axiosClient';

// รูปแบบที่ utils.OK ของ Go ส่งมา
interface ApiResponse<T> {
  success: boolean;
  data: T;
  message?: string;
}

export interface EligibleStudentItem {
  student_id: string;
  real_name: string;
  major: string;
  enrollment_status: number;
}

export interface InvalidEligibleRow {
  row: number;
  reason: string;
}

export interface EligibleImportSummary {
  new: number;
  updated: number;
  unchanged: number;
}

export interface PreviewEligibleStudentsResponse {
  valid: EligibleStudentItem[];
  invalid: InvalidEligibleRow[];
  summary: EligibleImportSummary;
}

export interface ConfirmEligibleStudentsResponse {
  submitted: number;
  upserted: number;
}

// role ที่บัญชีจะได้ตอนเจ้าตัวสมัคร — ตรงกับ entity.RoleUser / RoleAdmin ฝั่ง backend
export type EligibleRole = 'user' | 'admin';

// body ของ POST /admin/eligible-students/single — ตรงกับ dto.AddSingleEligibleStudentRequest
export interface AddEligibleStudentPayload {
  student_id: string;
  major: string;
  enrollment_status: number;
  role: EligibleRole;
}

// EligibleStudent = 1 แถวจากตาราง eligible_students จริง (ตรงกับ entity.EligibleStudent ฝั่ง backend)
export interface EligibleStudent {
  student_id: string;
  real_name: string;
  major: string;
  enrollment_status: number;
  // คนที่สมัครแล้วถูกซิงก์ให้ตรงกับบัญชีจริงเสมอ ส่วนคนที่ยังไม่สมัครคือ role ที่จะได้ตอนสมัคร
  role: EligibleRole;
  year_level: number;
  imported_at: string;
  created_at: string;
}

// เลขสถานภาพ -> ป้ายกำกับที่คนอ่านได้ ตามที่ระบบทะเบียนใช้ (ดู entity.ActiveEnrollmentStatuses ฝั่ง backend
// สำหรับสถานะที่ยังสมัคร/ใช้งานระบบได้ — 10, 11 เท่านั้น)
export function enrollmentStatusLabel(status: number): string {
  switch (status) {
    case 10:
      return "10 · กำลังศึกษา";
    case 11:
      return "11 · รักษาสภาพการเป็นนักศึกษา";
    case 12:
      return "12 · ลาพัก";
    case 13:
      return "13 · ให้พัก";
    case 40:
      return "40 · สำเร็จการศึกษา";
    default:
      if (status >= 60 && status <= 89) return `${status} · สิ้นสุดสถานภาพ`;
      return `${status}`;
  }
}

export const eligibleStudentsApi = {
  // ดึงรายชื่อ นศ. ที่มีสิทธิ์ทั้งหมด (ไม่มี pagination — ให้ frontend กรอง/ค้นหาเอง)
  listAll: async () => {
    const response = await axiosClient.get<ApiResponse<EligibleStudent[]>>('/admin/eligible-students');
    return response.data.data;
  },

  // อัปโหลดไฟล์ .xlsx ไป parse+validate ที่ backend เฉยๆ ยังไม่เขียนอะไรลง DB
  preview: async (file: File) => {
    const formData = new FormData();
    formData.append('file', file);
    // axiosClient ตั้ง Content-Type: application/json เป็นค่า default ของ instance ไว้ — ถ้าไม่ล้างค่านี้
    // ต่อคำขอนี้ axios จะเห็นว่า Content-Type เป็น json อยู่แล้วแล้ว "แปลง" FormData เป็น JSON string
    // ทิ้งไฟล์จริงไปเลย (เหลือแค่ {} ของ Blob) แทนที่จะส่งเป็น multipart — ต้อง set เป็น undefined ให้ axios
    // ล้าง header นี้ทิ้ง แล้วให้ browser ใส่ Content-Type: multipart/form-data; boundary=... ให้เองตอนส่งจริง
    const response = await axiosClient.post<ApiResponse<PreviewEligibleStudentsResponse>>(
      '/admin/eligible-students/preview',
      formData,
      { headers: { 'Content-Type': undefined } },
    );
    return response.data.data;
  },

  // เพิ่ม/อัปเดตผู้มีสิทธิ์ทีละคนพร้อม role — ถ้ารหัสนี้สมัครแล้ว backend เปลี่ยน role ของบัญชีให้ด้วย
  // (user_role_updated = true) ชื่อเดิมที่ import มาจากไฟล์ไม่ถูกแตะ
  addOne: async (payload: AddEligibleStudentPayload) => {
    const response = await axiosClient.post<
      ApiResponse<{ student: EligibleStudent; user_role_updated: boolean }>
    >('/admin/eligible-students/single', payload);
    return response.data.data;
  },

  // ยืนยัน apply รายชื่อที่ผ่าน preview แล้ว (upsert จริงลง eligible_students)
  confirm: async (students: EligibleStudentItem[]) => {
    const response = await axiosClient.post<ApiResponse<ConfirmEligibleStudentsResponse>>(
      '/admin/eligible-students',
      { students },
    );
    return response.data.data;
  },
};
