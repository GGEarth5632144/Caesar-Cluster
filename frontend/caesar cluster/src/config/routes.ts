
import SHA256 from "crypto-js/sha256";

const encodePath = (path: string) => {

  const salt = "sut-cluster-secret-key-2026";
  const hash = SHA256(path + salt).toString();

  return hash.substring(0, 24);
};

// export const PATHS = {
//   login: `/${encodePath("login")}`,
//   register: `/${encodePath("register")}`,
//   forgotPassword: `/${encodePath("forgot-password")}`,
//   resetPassword: `/${encodePath("reset-password")}`,
//   terms: `/${encodePath("terms")}`,

//   settings: encodePath("settings"),
//   services: encodePath("services"),

 
//   requestResources: encodePath("request-resources"),
//   alertuser: encodePath("alertuser"),
//   myService: encodePath("my-service"),
//   createService: encodePath("create-service"),

//   aiReview: encodePath("ai-review"),

//   generalDashboard: encodePath("general-dashboard"),
//   workspaceOnboarding: encodePath("workspace-onboarding"),

//   adminRequest: encodePath("admin-request"),
//   adminApprovals: encodePath("admin-approvals"),
//   userManagement: encodePath("user-management"),
//   alertadmin: encodePath("alertadmin"),
//   ipcManagement: encodePath("ipc-management"),
//   auditLog: encodePath("audit-log"),
//   adminImportStudents: encodePath("admin-import-students"),
// };

export const PATHS = {
  // เส้นทางหลักที่มี / นำหน้า
  login: "/login",
  register: "/register",
  forgotPassword: "/forgot-password",
  resetPassword: "/reset-password",
  terms: "/terms",

  // เส้นทางย่อย (Sub-paths)
  settings: "settings",
  services: "services",

  requestResources: "request-resources",
  alertuser: "alertuser",
  myService: "my-service",
  createService: "create-service",

  aiReview: "ai-review",

  generalDashboard: "general-dashboard",
  workspaceOnboarding: "workspace-onboarding",

  adminRequest: "admin-request",
  adminApprovals: "admin-approvals",
  userManagement: "user-management",
  alertadmin: "alertadmin",
  ipcManagement: "ipc-management",
  auditLog: "audit-log",
  adminImportStudents: "admin-import-students",
};