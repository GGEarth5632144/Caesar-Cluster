import { BrowserRouter, Routes, Route } from "react-router-dom";

import Login from "@/pages/Login";
import Register from "@/pages/Register";
import ProtectedRoute from "@/components/ProtectedRoute";
import DashboardLayout from "@/layouts/DashboardLayout";
import Profile from "@/pages/Profile";

// นำเข้า State และหน้า Dashboard ของแต่ละ Role
import { useAuthStore } from "@/store/authStore";
import AdminDashboard from "@/pages/admin/AdminDashboard";
import UserDashboard from "@/pages/user/UserDashboard";
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
                <Route path="profile" element={<Profile />} /> 
              </>
            )}

            {/* ---------------- ROUTE สำหรับ ADMIN (Role 2) ---------------- */}
            {isAdmin && (
              <>
                <Route index element={<AdminDashboard />} />
                <Route path="profile" element={<Profile />} />
              </>
            )}
            
          </Route>         
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;