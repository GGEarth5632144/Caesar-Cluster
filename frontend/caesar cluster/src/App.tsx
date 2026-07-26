import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route } from "react-router-dom";
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
const MyServices = lazy(() => import("@/pages/user/RequestQuotar"));
const Service = lazy(() => import("@/pages/admin/Service"));
const IPCmanagement = lazy(() => import("@/pages/admin/IPCmanagement"));
const Alertadmin = lazy(() => import("@/pages/admin/Alertadmin"));
const Auditlog = lazy(() => import("@/pages/admin/Auditlog"));
const Createservice = lazy(() => import("@/pages/user/Createservice"));
const Alertuser = lazy(() => import("@/pages/user/Alertuser"));
const Myservice = lazy(() => import("@/pages/user/Myservice"));

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
          <Route path={PATHS.resetPassword} element={<ResetPassword />} />
          <Route path={PATHS.terms} element={<Terms />} />
          
          <Route element={<ProtectedRoute />}>
            <Route path="/" element={<DashboardLayout />}>
              
              {/* ---------------- ROUTE สำหรับ USER (Role 1) ---------------- */}
              {isUser && (
                <>
                  <Route index element={<UserDashboard />} />
                  <Route path={PATHS.settings} element={<Setting />} />
                  <Route path={PATHS.requestResources} element={<RequestResources />} />
                  <Route path={PATHS.services} element={<MyServices />} />
                  <Route path={PATHS.alertuser} element={<Alertuser />} />
                  <Route path={PATHS.myService} element={<Myservice />} />
                  <Route path={PATHS.createService} element={<Createservice />} />
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
                  <Route path={PATHS.alertadmin} element={<Alertadmin />} />
                  <Route path={PATHS.services} element={<Service />} />
                  <Route path={PATHS.ipcManagement} element={<IPCmanagement />} />
                  <Route path={PATHS.auditLog} element={<Auditlog />} />
                  <Route path={PATHS.adminImportStudents} element={<AdminImportStudents />} />
                </>
              )}
            </Route>
          </Route>
        </Routes>
      </Suspense>
      <ActionModalHost />
    </BrowserRouter>
  );
}

export default App;
