export const PATHS = {
  // เส้นทางหลักที่มี / นำหน้า
  login: "/login",
  register: "/register",
  forgotPassword: "/forgot-password",
  verifyEmail: "/verify-email",
  resetPassword: "/reset-password",
  terms: "/terms",

  // เส้นทางย่อย (Sub-paths)
  settings: "settings",
  services: "services",

  requestResources: "request-resources",
  createService: "create-service",

  // ปิด route ไว้ชั่วคราว — หน้า AIReviewPage ยังอยู่ในโปรเจกต์แต่ถอดออกจาก App.tsx แล้ว
  // เก็บค่านี้ไว้เพื่อให้เปิดกลับได้โดยไม่ต้องไปไล่หาว่า path เดิมสะกดว่าอะไร
  aiReview: "ai-review",
  serviceLogs: "service-logs",

  generalDashboard: "general-dashboard",
  workspaceOnboarding: "workspace-onboarding",

  adminRequest: "admin-request",
  adminApprovals: "admin-approvals",
  userManagement: "user-management",
  namespaceManagement: "namespace-management",
  ipcManagement: "ipc-management",
  auditLog: "audit-log",
  adminImportStudents: "admin-import-students",
};