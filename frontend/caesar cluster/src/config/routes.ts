// ฟังก์ชันเข้ารหัส Base64 (สามารถเปลี่ยนไปใช้ไลบรารีอื่นเข้ารหัสให้ซับซ้อนกว่านี้ได้ในอนาคต)
const encodePath = (path: string) => btoa(path);

export const PATHS = {
  // ==========================================
  // Public Routes (หน้าทั่วไป ต้องมี / นำหน้า)
  // ==========================================
  login: `/${encodePath("login")}`,
  register: `/${encodePath("register")}`,
  forgotPassword: `/${encodePath("forgot-password")}`,
  resetPassword: `/${encodePath("reset-password")}`,

  // ==========================================
  // Protected Routes (หน้าข้างใน ไม่ต้องมี / นำหน้า)
  // ==========================================
  
  // Shared (หน้าที่แอดมินและผู้ใช้มีชื่อ URL เหมือนกัน)
  settings: encodePath("settings"),
  services: encodePath("services"),

  // User (Role 1)
  requestResources: encodePath("request-resources"),
  alertuser: encodePath("alertuser"),
  myService: encodePath("my-service"),
  createService: encodePath("create-service"),

  // Admin (Role 2)
  adminRequest: encodePath("admin-request"),
  adminApprovals: encodePath("admin-approvals"),
  userManagement: encodePath("user-management"),
  alertadmin: encodePath("alertadmin"),
  ipcManagement: encodePath("ipc-management"),
  auditLog: encodePath("audit-log"),
  adminImportStudents: encodePath("admin-import-students"),
};