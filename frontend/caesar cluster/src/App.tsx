import { BrowserRouter, Routes, Route } from "react-router-dom";

import Login from "@/pages/Login";
import Register from "@/pages/Register";
import ProtectedRoute from "@/components/ProtectedRoute";
import DashboardLayout from "@/layouts/DashboardLayout";
import Setting from "@/pages/Setting";
import { useAuthStore } from "@/store/authStore";
import AdminDashboard from "@/pages/admin/AdminDashboard";
import UserDashboard from "@/pages/user/UserDashboard";
import AdminRequest from "@/pages/admin/AdminRequest";
import AdminRequestQueue from "@/pages/admin/AdminRequestQueue";
import RequestResources from "@/pages/user/RequestResources";
import UserManagement from "@/pages/admin/Usermanagement";
import MyServices from "@/pages/user/MyServices";
// import VmManagement from "@/pages/VmManagement"; // หน้าอื่นๆ ของ Admin

function App() {
  // ดึงข้อมูล user จาก Zustand
  const user = useAuthStore((state) => state.user);

  // สร้างเงื่อนไข Role (1 = User, 2 = Admin)
  const isUser = String(user?.role) === "user";
  const isAdmin = String(user?.role) === "admin";

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
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
              </>
            )}
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
