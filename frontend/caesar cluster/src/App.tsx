import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { PATHS } from "@/config/routes";

import ProtectedRoute from "@/components/ProtectedRoute";
import DashboardLayout from "@/layouts/DashboardLayout";
import LogoLoader from "@/components/ui/LogoLoader";
import { ActionModalHost } from "@/components/ui/action-modal";
import { useAuthStore } from "@/store/authStore";

// ---------------- Lazy load ทุกหน้า (code-splitting) ----------------
// แต่ละหน้าถูกแยกเป็น chunk ของตัวเอง โหลดเมื่อเข้าถึงเส้นทางนั้นจริง
// ระหว่างที่ chunk กำลังดาวน์โหลดจะโชว์ LogoLoader ผ่าน <Suspense>
const Login = lazy(() => import("@/pages/Login"));
const Register = lazy(() => import("@/pages/Register"));
const ForgotPassword = lazy(() => import("@/pages/ForgotPassword"));
const VerifyEmail = lazy(() => import("@/pages/VerifyEmail"));
const ResetPassword = lazy(() => import("@/pages/ResetPassword"));
const Terms = lazy(() => import("@/pages/Terms"));
const Setting = lazy(() => import("@/pages/Setting"));

const AdminDashboard = lazy(() => import("@/pages/admin/AdminDashboard"));
const UserDashboard = lazy(() => import("@/pages/user/UserDashboard"));
const AdminRequest = lazy(() => import("@/pages/admin/AdminRequest"));
const AdminRequestQueue = lazy(() => import("@/pages/admin/AdminRequestQueue"));
const AdminImportStudents = lazy(() => import("@/pages/admin/AdminImportStudents"));
const RequestResources = lazy(() => import("@/pages/user/RequestResources"));
const UserManagement = lazy(() => import("@/pages/admin/Usermanagement"));
const NamespaceManagement = lazy(() => import("@/pages/admin/NamespaceManagement"));
const MyService = lazy(() => import("@/pages/user/MyService"));
const Service = lazy(() => import("@/pages/admin/Service"));
const IPCmanagement = lazy(() => import("@/pages/admin/IPCmanagement"));
const Auditlog = lazy(() => import("@/pages/admin/Auditlog"));
const Createservice = lazy(() => import("@/pages/user/Createservice"));
const ServiceLogs = lazy(() => import("@/pages/user/ServiceLogs"));
const GeneralDashboard = lazy(() => import("@/pages/user/GeneralDashboard"));
const WorkspaceOnboarding = lazy(() => import("@/pages/user/WorkspaceOnboarding"));

// redirect /reset-password?token=... → hashed reset-password path (preserves query string)
// backend email links point to /reset-password but the frontend serves the page at a hashed path
function ResetPasswordRedirect() {
  const { search } = useLocation();
  return <Navigate to={PATHS.resetPassword + search} replace />;
}

/**
 * ปลายทางของ path ที่ไม่ match route ไหนเลย
 *
 * คนที่ยังไม่ล็อกอิน → หน้า login ตามปกติ
 * คนที่ล็อกอินอยู่แล้ว → หน้าแรกของตัวเอง ไม่ใช่ login (เพราะการเตะคนที่มี session อยู่ดีๆ
 * ออกไปหน้า login คือบอกว่า "คุณหลุดแล้ว" ทั้งที่จริงแค่พิมพ์ URL ผิด)
 *
 * เคสที่ต้องพึ่งตรงนี้จริงๆ คือตอน role เปลี่ยน: route tree ถูกสร้างตาม role (ดูด้านล่าง)
 * แอดมินที่เพิ่งถูกถอนสิทธิ์แล้วเปิด bookmark ของหน้าแอดมินค้างไว้ จะไม่มี route ไหนรับ
 * ถ้าปล่อยให้เด้งไป login เขาจะเห็นหน้า login ทั้งที่ session ยังใช้ได้ดีอยู่
 */
function NotFoundRedirect() {
  const token = useAuthStore((state) => state.token);
  return <Navigate to={token ? "/" : PATHS.login} replace />;
}

function App() {
  // ดึงข้อมูล user จาก Zustand
  const user = useAuthStore((state) => state.user);

  // สร้างเงื่อนไข Role (1 = User, 2 = Admin)
  const isUser = String(user?.role) === "user";
  const isAdmin = String(user?.role) === "admin";

return (
    <BrowserRouter>
      <Suspense fallback={<LogoLoader fullScreen label="กำลังโหลด..." />}>
        <Routes>
          {/* ---------------- Public Routes ---------------- */}
          <Route path={PATHS.login} element={<Login />} />
          <Route path={PATHS.register} element={<Register />} />
          <Route path={PATHS.forgotPassword} element={<ForgotPassword />} />
          {/* ยืนยันอีเมล — public เพราะยังไม่มี token จนกว่าจะยืนยันแล้วไปล็อกอิน
              token อยู่ใน query ของลิงก์ที่ส่งไปทางอีเมล (?token=...) */}
          <Route path={PATHS.verifyEmail} element={<VerifyEmail />} />
          <Route path={PATHS.resetPassword} element={<ResetPassword />} />
          <Route path="/reset-password" element={<ResetPasswordRedirect />} />
          <Route path={PATHS.terms} element={<Terms />} />
          
          <Route element={<ProtectedRoute />}>
            <Route path="/" element={<DashboardLayout />}>
              
              {/* ---------------- ROUTE สำหรับ USER (Role 1) ---------------- */}
              {isUser && (
                <>
                  <Route index element={<UserDashboard />} />
                  <Route path={PATHS.settings} element={<Setting />} />
                  <Route path={PATHS.requestResources} element={<RequestResources />} />
                  <Route path={PATHS.services} element={<MyService />} />
                  <Route path={PATHS.createService} element={<Createservice />} />
                  <Route path={`${PATHS.serviceLogs}/:serviceId`} element={<ServiceLogs />} />
                  <Route path={PATHS.generalDashboard} element={<GeneralDashboard user={user} />} />
                  <Route path={PATHS.workspaceOnboarding} element={<WorkspaceOnboarding />} />
                </>
              )}
              
              {/* ---------------- ROUTE สำหรับ ADMIN (Role 2) ---------------- */}
              {isAdmin && (
                <>
                  <Route index element={<AdminDashboard />} />
                  <Route path={PATHS.settings} element={<Setting />} />
                  <Route path={PATHS.adminRequest} element={<AdminRequest />} />
                  <Route path={PATHS.adminApprovals} element={<AdminRequestQueue />} />
                  <Route path={PATHS.userManagement} element={<UserManagement />} />
                  <Route path={PATHS.namespaceManagement} element={<NamespaceManagement />} />
                  <Route path={PATHS.services} element={<Service />} />
                  <Route path={PATHS.ipcManagement} element={<IPCmanagement />} />
                  <Route path={PATHS.auditLog} element={<Auditlog />} />
                  <Route path={PATHS.adminImportStudents} element={<AdminImportStudents />} />
                </>
              )}
            </Route>
          </Route>
          <Route path="*" element={<NotFoundRedirect />} />
        </Routes>
      </Suspense>
      <ActionModalHost />
    </BrowserRouter>
  );
}

export default App;
