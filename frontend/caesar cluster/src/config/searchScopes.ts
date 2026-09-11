import type { User } from "@/api/adminuser";
import type { RequestTemplate } from "@/api/adminrequest";
import type { AdminVmRequest, VmRequest } from "@/api/requests";
import type { AppService } from "@/api/services";
import type { LogLine } from "@/api/logs";
import type { NodeTelemetry } from "@/api/mornitorequest";
import type { EligibleStudentItem } from "@/api/eligibleStudents";
import {
  namespaceOwner,
  namespaceState,
  namespaceUsagePercent,
  type NamespaceDetail,
} from "@/api/namespace";
import type { SearchScope } from "@/store/searchStore";

// นิยาม "หน้านี้ค้นอะไรได้บ้าง" ของทุกหน้า รวมไว้ที่เดียว
//
// อยู่แยกจากตัวหน้าเพราะสองเหตุผล:
//  1. usePageSearch ต้องการ object ที่อ้างอิงเดิมทุก render — ประกาศเป็น const ระดับโมดูล
//     คือวิธีที่ง่ายที่สุดที่รับประกันข้อนี้ (ถ้าไปสร้างในคอมโพเนนต์ต้องห่อ useMemo ทุกที่)
//  2. เห็นภาพรวมได้ในไฟล์เดียวว่าแต่ละหน้าค้นด้วยฟิลด์อะไร ชื่อฟิลด์ที่ผู้ใช้พิมพ์
//     จะได้ไม่ขัดกันเอง (cpu/ram/status ใช้ชื่อเดียวกันหมดทุกหน้า)
//
// กติกาที่ใช้ตลอดไฟล์นี้:
//  - ฟิลด์ที่คนใช้ค้นเป็นคำเปล่าบ่อยๆ (ชื่อ, รหัส, ข้อความ) ไม่ต้องตั้ง exactOnly
//  - ฟิลด์ที่ค่าซ้ำกันทั้งตาราง (status, ตัวเลข, วันที่) ตั้ง exactOnly ไว้
//    ไม่งั้นพิมพ์ "1" ทีเดียวจะไปโดนทุกแถวที่มีเลข 1 อยู่ในโควตา
//  - ทุกฟิลด์มี alias ภาษาไทย เพราะผู้ใช้จริงของระบบนี้พิมพ์ไทย

const roleLabel = (roleId: number) => (roleId === 2 ? "admin" : "user");

// ---------------------------------------------------------------------------
// ผู้ดูแล — จัดการผู้ใช้
// ---------------------------------------------------------------------------

export const usersScope: SearchScope = {
  id: "admin-users",
  noun: "ผู้ใช้",
  placeholder: "ค้นหาผู้ใช้ — ชื่อ, รหัสนักศึกษา, อีเมล หรือ role:admin",
  fields: [
    { id: "name", label: "ชื่อ-นามสกุล", aliases: ["ชื่อ", "realname"], get: (u: User) => u.real_name },
    { id: "nick", label: "ชื่อเล่น", aliases: ["ชื่อเล่น", "nickname"], get: (u: User) => u.nick_name },
    { id: "sid", label: "รหัสนักศึกษา", aliases: ["รหัส", "student", "student_id"], get: (u: User) => u.student_id },
    { id: "email", label: "อีเมล", aliases: ["อีเมล", "gmail", "mail"], get: (u: User) => u.gmail },
    {
      id: "role",
      label: "สิทธิ์",
      type: "enum",
      aliases: ["สิทธิ์", "บทบาท"],
      exactOnly: true,
      options: [
        { value: "user", label: "นักศึกษา" },
        { value: "admin", label: "ผู้ดูแลระบบ" },
      ],
      get: (u: User) => roleLabel(u.role_id),
    },
    {
      id: "year",
      label: "ชั้นปี",
      type: "number",
      aliases: ["ปี", "ชั้นปี"],
      exactOnly: true,
      get: (u: User) => u.year_level,
    },
    {
      id: "space",
      label: "มีเนมสเปซแล้ว",
      type: "enum",
      aliases: ["namespace", "เนมสเปซ", "กลุ่ม"],
      exactOnly: true,
      options: [
        { value: "yes", label: "มีแล้ว" },
        { value: "no", label: "ยังไม่มี" },
      ],
      get: (u: User) => (u.namespace_id ? "yes" : "no"),
    },
    // ไม่มีฟิลด์โควตาที่นี่แล้ว — คอลัมน์ Quota Limit ย้ายไปหน้า Namespace Management
    // ค้นด้วยเลขโควตาในหน้านี้จะได้แถวที่มองไม่เห็นว่าตรงกับตรงไหน ให้ไปค้นที่หน้านั้นแทน
    //
    // alias ห้ามชน "space"/"namespace" ที่ฟิลด์ด้านบนจองไว้แล้ว — ตัวหาฟิลด์เลือกตัวแรกที่ตรง
    // (ดู lib/search.ts) alias ที่ซ้ำจะกลายเป็น alias ตายที่ไม่มีวันถูกเรียกใช้
    {
      id: "nsname",
      label: "ชื่อเนมสเปซ",
      aliases: ["ชื่อกลุ่ม", "ชื่อเนมสเปซ", "ns"],
      get: (u: User) => u.namespace_name,
    },
    {
      id: "created",
      label: "วันที่สมัคร",
      type: "date",
      aliases: ["สมัคร", "วันที่"],
      exactOnly: true,
      get: (u: User) => u.created_at,
    },
  ],
  quickFilters: [
    {
      fieldId: "role",
      label: "สิทธิ์",
      options: [
        { value: "user", label: "นักศึกษา" },
        { value: "admin", label: "ผู้ดูแล" },
      ],
    },
    {
      fieldId: "space",
      label: "เนมสเปซ",
      options: [
        { value: "yes", label: "มีแล้ว" },
        { value: "no", label: "ยังไม่มี" },
      ],
    },
  ],
  examples: ["role:admin", "space:no", "year:4 -role:admin", "nsname:lab"],
};

// ---------------------------------------------------------------------------
// ผู้ดูแล — เนมสเปซและโควตา
// ---------------------------------------------------------------------------

const namespaceStateOptions = [
  { value: "active", label: "ใช้งานอยู่" },
  { value: "full", label: "ใกล้เต็ม" },
  { value: "idle", label: "ยังไม่มีบริการ" },
  { value: "empty", label: "ไม่มีสมาชิก" },
];

export const namespacesScope: SearchScope = {
  id: "admin-namespaces",
  noun: "เนมสเปซ",
  placeholder: "ค้นหาเนมสเปซ — ชื่อ space, ชื่อสมาชิก, state:full หรือ cpu:>=4000",
  fields: [
    { id: "name", label: "ชื่อเนมสเปซ", aliases: ["ชื่อ", "space", "ns"], get: (n: NamespaceDetail) => n.name },
    {
      id: "owner",
      label: "เจ้าของ",
      aliases: ["contributor", "เจ้าของ", "ผู้สร้าง"],
      get: (n: NamespaceDetail) => namespaceOwner(n)?.real_name ?? "",
    },
    // ค้นด้วยชื่อหรือรหัสของสมาชิกคนไหนก็ได้ในกลุ่ม — แอดมินมักเริ่มจาก "นักศึกษาคนนี้อยู่ space ไหน"
    // ไม่ใช่จากชื่อ space ที่ตัวเองไม่เคยเห็นมาก่อน
    {
      id: "member",
      label: "สมาชิกในกลุ่ม",
      aliases: ["สมาชิก", "นักศึกษา", "student", "รหัส"],
      get: (n: NamespaceDetail) => n.members.map((m) => `${m.real_name} ${m.student_id}`).join(" "),
    },
    {
      id: "state",
      label: "สภาพการใช้งาน",
      type: "enum",
      aliases: ["สถานะ", "สภาพ", "status"],
      exactOnly: true,
      options: namespaceStateOptions,
      get: (n: NamespaceDetail) => namespaceState(n),
    },
    { id: "cpu", label: "โควตา CPU", type: "number", unit: "millicore", aliases: ["ซีพียู"], exactOnly: true, get: (n: NamespaceDetail) => n.cpu_limit_milli },
    { id: "ram", label: "โควตา RAM", type: "number", unit: "MB", aliases: ["แรม", "memory"], exactOnly: true, get: (n: NamespaceDetail) => n.ram_limit_mb },
    { id: "usage", label: "สัดส่วนที่ใช้ไป", type: "number", unit: "%", aliases: ["ใช้ไป", "percent", "เปอร์เซ็นต์"], exactOnly: true, get: (n: NamespaceDetail) => Math.round(namespaceUsagePercent(n).peak) },
    { id: "members", label: "จำนวนสมาชิก", type: "number", aliases: ["จำนวนสมาชิก", "คน"], exactOnly: true, get: (n: NamespaceDetail) => n.member_count },
    { id: "services", label: "จำนวนบริการ", type: "number", aliases: ["บริการ", "service"], exactOnly: true, get: (n: NamespaceDetail) => n.usage.service_count },
    { id: "created", label: "วันที่สร้าง", type: "date", aliases: ["วันที่", "สร้าง"], exactOnly: true, get: (n: NamespaceDetail) => n.created_at },
  ],
  quickFilters: [{ fieldId: "state", label: "สภาพ", options: namespaceStateOptions, multi: true }],
  examples: ["state:full", "state:empty", "usage:>=90", "cpu:>=4000", "members:>1"],
};

// ---------------------------------------------------------------------------
// ผู้ดูแล — แม่แบบคำขอทรัพยากร
// ---------------------------------------------------------------------------

export const requestTemplatesScope: SearchScope = {
  id: "admin-request-templates",
  noun: "แม่แบบ",
  placeholder: "ค้นหาแม่แบบ — ชื่อ, หมวดหมู่, วิชา หรือ cpu:>=1000",
  fields: [
    { id: "name", label: "ชื่อตัวเลือก", aliases: ["ชื่อ", "option"], get: (t: RequestTemplate) => t.option_name },
    { id: "category", label: "หมวดหมู่", aliases: ["หมวด", "cat"], get: (t: RequestTemplate) => t.category },
    { id: "subject", label: "วิชาที่เกี่ยวข้อง", aliases: ["วิชา", "subject"], get: (t: RequestTemplate) => t.relate_subject },
    { id: "desc", label: "คำอธิบาย", aliases: ["คำอธิบาย", "description"], get: (t: RequestTemplate) => t.description },
    {
      id: "status",
      label: "สถานะ",
      type: "enum",
      aliases: ["สถานะ", "active"],
      exactOnly: true,
      options: [
        { value: "active", label: "เปิดใช้งาน" },
        { value: "inactive", label: "ปิดใช้งาน" },
      ],
      get: (t: RequestTemplate) => (t.is_active ? "active" : "inactive"),
    },
    { id: "cpu", label: "CPU", type: "number", unit: "millicore", aliases: ["ซีพียู"], exactOnly: true, get: (t: RequestTemplate) => t.cpu_limit_milli },
    { id: "ram", label: "RAM", type: "number", unit: "MB", aliases: ["แรม"], exactOnly: true, get: (t: RequestTemplate) => t.ram_limit_mb },
    { id: "storage", label: "Storage", type: "number", unit: "GB", aliases: ["ดิสก์", "พื้นที่"], exactOnly: true, get: (t: RequestTemplate) => t.storage_gb },
    { id: "created", label: "วันที่สร้าง", type: "date", aliases: ["วันที่"], exactOnly: true, get: (t: RequestTemplate) => t.created_at },
  ],
  quickFilters: [
    {
      fieldId: "status",
      label: "สถานะ",
      options: [
        { value: "active", label: "เปิดใช้งาน" },
        { value: "inactive", label: "ปิดใช้งาน" },
      ],
    },
  ],
  examples: ["status:active", "cpu:>=2000", "category:AI", "-status:inactive"],
};

// ---------------------------------------------------------------------------
// ผู้ดูแล — คิวคำขอโควตา
// ---------------------------------------------------------------------------

const requestStatusOptions = [
  { value: "pending", label: "รออนุมัติ" },
  { value: "approved", label: "อนุมัติแล้ว" },
  { value: "denied", label: "ปฏิเสธ" },
];

const spaceTypeOptions = [
  { value: "solo", label: "เดี่ยว" },
  { value: "group", label: "กลุ่ม" },
];

export const adminRequestQueueScope: SearchScope = {
  id: "admin-request-queue",
  noun: "คำขอ",
  placeholder: "ค้นหาคำขอ — ชื่อผู้ยื่น, รหัสนักศึกษา หรือ status:pending",
  fields: [
    { id: "name", label: "ชื่อผู้ยื่น", aliases: ["ชื่อ", "requester"], get: (r: AdminVmRequest) => r.requester_name },
    { id: "sid", label: "รหัสนักศึกษา", aliases: ["รหัส", "student"], get: (r: AdminVmRequest) => r.requester_student_id },
    { id: "desc", label: "รายละเอียดคำขอ", aliases: ["รายละเอียด", "description"], get: (r: AdminVmRequest) => r.description },
    { id: "reason", label: "เหตุผลที่ปฏิเสธ", aliases: ["เหตุผล", "deny"], get: (r: AdminVmRequest) => r.deny_reason },
    {
      id: "status",
      label: "สถานะ",
      type: "enum",
      aliases: ["สถานะ"],
      exactOnly: true,
      options: requestStatusOptions,
      get: (r: AdminVmRequest) => r.status,
    },
    {
      id: "type",
      label: "ชนิดเนมสเปซ",
      type: "enum",
      aliases: ["ชนิด", "namespace", "space"],
      exactOnly: true,
      options: spaceTypeOptions,
      get: (r: AdminVmRequest) => r.namespace_name,
    },
    { id: "cpu", label: "CPU ที่ขอ", type: "number", unit: "millicore", aliases: ["ซีพียู"], exactOnly: true, get: (r: AdminVmRequest) => r.cpu_limit_milli },
    { id: "ram", label: "RAM ที่ขอ", type: "number", unit: "MB", aliases: ["แรม"], exactOnly: true, get: (r: AdminVmRequest) => r.ram_limit_mb },
    { id: "storage", label: "Storage ที่ขอ", type: "number", unit: "GB", aliases: ["พื้นที่"], exactOnly: true, get: (r: AdminVmRequest) => r.storage_gb },
    { id: "id", label: "เลขคำขอ", type: "number", aliases: ["เลขที่"], exactOnly: true, get: (r: AdminVmRequest) => r.id },
    { id: "created", label: "วันที่ยื่น", type: "date", aliases: ["วันที่", "ยื่น"], exactOnly: true, get: (r: AdminVmRequest) => r.created_at },
  ],
  quickFilters: [
    { fieldId: "status", label: "สถานะ", options: requestStatusOptions, multi: true },
    { fieldId: "type", label: "ชนิด", options: spaceTypeOptions },
  ],
  examples: ["status:pending", "type:group cpu:>=2000", "created:>2025-01-01", "-status:denied"],
};

// ---------------------------------------------------------------------------
// ผู้ดูแล — โหนดในคลัสเตอร์
// ---------------------------------------------------------------------------

export const nodesScope: SearchScope = {
  id: "admin-nodes",
  noun: "โหนด",
  placeholder: "ค้นหาโหนด — ชื่อเครื่อง, state:down หรือ temp:>60",
  fields: [
    { id: "node", label: "ชื่อโหนด", aliases: ["ชื่อ", "name", "เครื่อง"], get: (n: NodeTelemetry) => n.NodeName },
    {
      id: "state",
      label: "สถานะ",
      type: "enum",
      aliases: ["สถานะ", "status"],
      exactOnly: true,
      options: [
        { value: "up", label: "ออนไลน์" },
        { value: "down", label: "ออฟไลน์" },
      ],
      get: (n: NodeTelemetry) => (n.IsUp === 1 ? "up" : "down"),
    },
    { id: "temp", label: "อุณหภูมิ", type: "number", unit: "°C", aliases: ["อุณหภูมิ", "temperature"], exactOnly: true, get: (n: NodeTelemetry) => n.Temperature },
    { id: "ram", label: "RAM ที่ใช้", type: "number", unit: "MB", aliases: ["แรม"], exactOnly: true, get: (n: NodeTelemetry) => n.RamUsedMB },
    { id: "procs", label: "จำนวนโปรเซส", type: "number", aliases: ["โปรเซส"], exactOnly: true, get: (n: NodeTelemetry) => n.Procs },
  ],
  quickFilters: [
    {
      fieldId: "state",
      label: "สถานะ",
      options: [
        { value: "up", label: "ออนไลน์" },
        { value: "down", label: "ออฟไลน์" },
      ],
    },
  ],
  examples: ["state:down", "temp:>60", "procs:>=200", "-node:intelnuc"],
};

// ---------------------------------------------------------------------------
// ผู้ดูแล — นำเข้ารายชื่อผู้มีสิทธิ์
// ---------------------------------------------------------------------------

export const importPreviewScope: SearchScope = {
  id: "admin-import-preview",
  noun: "รายชื่อ",
  placeholder: "ค้นหาในรายชื่อที่อ่านได้ — ชื่อ, รหัส หรือ enroll:10",
  fields: [
    { id: "name", label: "ชื่อ-นามสกุล", aliases: ["ชื่อ"], get: (s: EligibleStudentItem) => s.real_name },
    { id: "sid", label: "รหัสนักศึกษา", aliases: ["รหัส", "student"], get: (s: EligibleStudentItem) => s.student_id },
    { id: "major", label: "สาขา", aliases: ["สาขา"], get: (s: EligibleStudentItem) => s.major },
    {
      id: "enroll",
      label: "สถานภาพ",
      type: "number",
      aliases: ["สถานภาพ", "status"],
      exactOnly: true,
      get: (s: EligibleStudentItem) => s.enrollment_status,
    },
  ],
  examples: ["enroll:10", "-enroll:10", "major:CPE"],
};

// ---------------------------------------------------------------------------
// ผู้ใช้ — บริการของฉัน
// ---------------------------------------------------------------------------

const serviceStatusOptions = [
  { value: "running", label: "กำลังรัน" },
  { value: "creating", label: "กำลังสร้าง" },
  { value: "failed", label: "ล้มเหลว" },
];

export const servicesScope: SearchScope = {
  id: "user-services",
  noun: "บริการ",
  placeholder: "ค้นหาบริการ — ชื่อ, image หรือ status:failed",
  fields: [
    { id: "name", label: "ชื่อบริการ", aliases: ["ชื่อ"], get: (s: AppService) => s.name },
    { id: "image", label: "Image", aliases: ["อิมเมจ", "repo"], get: (s: AppService) => s.image },
    { id: "env", label: "ตัวแปรสภาพแวดล้อม", aliases: ["env_vars", "ตัวแปร"], get: (s: AppService) => s.env_vars },
    {
      id: "status",
      label: "สถานะ",
      type: "enum",
      aliases: ["สถานะ"],
      exactOnly: true,
      options: serviceStatusOptions,
      get: (s: AppService) => s.status,
    },
    { id: "cpu", label: "CPU", type: "number", unit: "millicore", aliases: ["ซีพียู"], exactOnly: true, get: (s: AppService) => s.cpu_milli },
    { id: "ram", label: "RAM", type: "number", unit: "MB", aliases: ["แรม"], exactOnly: true, get: (s: AppService) => s.ram_mb },
    { id: "replicas", label: "จำนวน Pod", type: "number", aliases: ["pod", "จำนวน"], exactOnly: true, get: (s: AppService) => s.replicas },
    { id: "port", label: "Container port", type: "number", aliases: ["พอร์ต"], exactOnly: true, get: (s: AppService) => s.container_port },
    { id: "nodeport", label: "Node port", type: "number", aliases: ["พอร์ตนอก"], exactOnly: true, get: (s: AppService) => s.node_port },
    { id: "created", label: "วันที่สร้าง", type: "date", aliases: ["วันที่"], exactOnly: true, get: (s: AppService) => s.created_at },
  ],
  quickFilters: [{ fieldId: "status", label: "สถานะ", options: serviceStatusOptions, multi: true }],
  examples: ["status:failed", "image:nginx", "replicas:>1", "cpu:>=1000"],
};

// ---------------------------------------------------------------------------
// ผู้ใช้ — คำขอของฉัน
// ---------------------------------------------------------------------------

export const myRequestsScope: SearchScope = {
  id: "user-requests",
  noun: "คำขอ",
  placeholder: "ค้นหาคำขอของคุณ — รายละเอียด หรือ status:pending",
  fields: [
    { id: "desc", label: "รายละเอียด", aliases: ["รายละเอียด"], get: (r: VmRequest) => r.description },
    { id: "reason", label: "เหตุผลที่ปฏิเสธ", aliases: ["เหตุผล"], get: (r: VmRequest) => r.deny_reason },
    {
      id: "status",
      label: "สถานะ",
      type: "enum",
      aliases: ["สถานะ"],
      exactOnly: true,
      options: requestStatusOptions,
      get: (r: VmRequest) => r.status,
    },
    {
      id: "type",
      label: "ชนิดเนมสเปซ",
      type: "enum",
      aliases: ["ชนิด", "space"],
      exactOnly: true,
      options: spaceTypeOptions,
      get: (r: VmRequest) => r.namespace_name,
    },
    { id: "cpu", label: "CPU ที่ขอ", type: "number", unit: "millicore", exactOnly: true, get: (r: VmRequest) => r.cpu_limit_milli },
    { id: "ram", label: "RAM ที่ขอ", type: "number", unit: "MB", exactOnly: true, get: (r: VmRequest) => r.ram_limit_mb },
    { id: "id", label: "เลขคำขอ", type: "number", aliases: ["เลขที่"], exactOnly: true, get: (r: VmRequest) => r.id },
    { id: "created", label: "วันที่ยื่น", type: "date", aliases: ["วันที่"], exactOnly: true, get: (r: VmRequest) => r.created_at },
  ],
  quickFilters: [{ fieldId: "status", label: "สถานะ", options: requestStatusOptions, multi: true }],
  examples: ["status:pending", "status:denied", "created:>2025-01-01"],
};

// ---------------------------------------------------------------------------
// ผู้ใช้ — log ของบริการ
// ---------------------------------------------------------------------------

export const serviceLogsScope: SearchScope = {
  id: "user-service-logs",
  noun: "บรรทัด",
  placeholder: 'กรอง log — พิมพ์คำ, "วลีทั้งวลี" หรือ -healthcheck เพื่อตัดออก',
  fields: [
    { id: "text", label: "เนื้อ log", aliases: ["log", "ข้อความ"], get: (l: LogLine) => l.text },
    { id: "time", label: "เวลา", type: "date", aliases: ["เวลา", "timestamp"], exactOnly: true, get: (l: LogLine) => l.timestamp },
  ],
  examples: ['"connection refused"', "error -healthcheck", "panic|fatal"],
};
