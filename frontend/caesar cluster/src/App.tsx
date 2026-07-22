import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route } from "react-router-dom";

import ProtectedRoute from "@/components/ProtectedRoute";
import DashboardLayout from "@/layouts/DashboardLayout";
import LogoLoader from "@/components/ui/LogoLoader";
import { useAuthStore } from "@/store/authStore";

// ---------------- Lazy load ทุกหน้า (code-splitting) ----------------
// แต่ละหน้าถูกแยกเป็น chunk ของตัวเอง โหลดเมื่อเข้าถึงเส้นทางนั้นจริง
// ระหว่างที่ chunk กำลังดาวน์โหลดจะโชว์ LogoLoader ผ่าน <Suspense>
const Login = lazy(() => import("@/pages/Login"));
const Register = lazy(() => import("@/pages/Register"));
const ForgotPassword = lazy(() => import("@/pages/ForgotPassword"));
const ResetPassword = lazy(() => import("@/pages/ResetPassword"));
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
      {/* fallback ระดับบนสุด — ครอบหน้า auth และตัว layout ระหว่างโหลด chunk */}
      <Suspense fallback={<LogoLoader fullScreen label="กำลังโหลด..." />}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route path="/forgot-password" element={<ForgotPassword />} />
          <Route path="/reset-password" element={<ResetPassword />} />
          <Route element={<ProtectedRoute />}>
            <Route path="/" element={<DashboardLayout />}>
              {/* ---------------- ROUTE สำหรับ USER (Role 1) ---------------- */}
              {isUser && (
                <>
                  <Route index element={<UserDashboard />} />
                  <Route path="settings" element={<Setting />} />
                  <Route
                    path="request-resources"
                    element={<RequestResources />}
                  />
                  <Route path="services" element={<MyServices />} />
                  <Route path="alertuser" element={<Alertuser />} />
                  <Route path="my-service" element={<Myservice />} />
                  <Route path="create-service" element={<Createservice />} />
                </>
              )}
              {/* ---------------- ROUTE สำหรับ ADMIN (Role 2) ---------------- */}
              {isAdmin && (
                <>
                  <Route index element={<AdminDashboard />} />
                  <Route path="settings" element={<Setting />} />
                  <Route path="admin-request" element={<AdminRequest />} />
                  <Route path="admin-approvals" element={<AdminRequestQueue />} />
                  <Route path="user-management" element={<UserManagement />} />
                  <Route path="alertadmin" element={<Alertadmin />} />
                  <Route path="services" element={<Service />} />
                  <Route path="ipc-management" element={<IPCmanagement />} />
                  <Route path="audit-log" element={<Auditlog />} />
                  <Route path="admin-import-students" element={<AdminImportStudents />} />
                </>
              )}
            </Route>
          </Route>
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}

export default App;
