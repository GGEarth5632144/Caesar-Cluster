
import SHA256 from "crypto-js/sha256";

const encodePath = (path: string) => {

  const salt = "sut-cluster-secret-key-2026";
  const hash = SHA256(path + salt).toString();

  return hash.substring(0, 24);
};

export const PATHS = {
  login: `/${encodePath("login")}`,
  register: `/${encodePath("register")}`,
  forgotPassword: `/${encodePath("forgot-password")}`,
  resetPassword: `/${encodePath("reset-password")}`,

  settings: encodePath("settings"),
  services: encodePath("services"),

 
  requestResources: encodePath("request-resources"),
  alertuser: encodePath("alertuser"),
  myService: encodePath("my-service"),
  createService: encodePath("create-service"),

  adminRequest: encodePath("admin-request"),
  adminApprovals: encodePath("admin-approvals"),
  userManagement: encodePath("user-management"),
  alertadmin: encodePath("alertadmin"),
  ipcManagement: encodePath("ipc-management"),
  auditLog: encodePath("audit-log"),
  adminImportStudents: encodePath("admin-import-students"),
};