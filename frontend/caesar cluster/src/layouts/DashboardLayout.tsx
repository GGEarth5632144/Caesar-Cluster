import { useNavigate } from "react-router-dom";

import Sidebar from "@/components/layout/Sidebar";
import Topbar from "@/components/layout/Topbar";
import { useAuthStore } from "@/store/authStore";
import userNavItems from "@/pages/User_Navigate";
import adminNavItems from "@/pages/Admin_Navigate";
import UserDashboard from "@/pages/UserDashboard";
import AdminDashboard from "@/pages/AdminDashboard";

export default function DashboardLayout() {
  const navigate = useNavigate();
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);

  const isAdmin = user?.role === "admin";
  const navItems = isAdmin ? adminNavItems : userNavItems;

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  return (
    <div className="flex h-screen w-full overflow-hidden bg-[#FFF8E8]">
      <Sidebar
        navItems={navItems}
        userName={user?.nick_name || user?.real_name || "User"}
        studentId={user?.student_id ?? ""}
        onLogout={handleLogout}
      />

      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar title="General Dashboard" userName={user?.real_name ?? "User"} />

        <main className="flex-1 overflow-auto p-6">
          {isAdmin ? <AdminDashboard /> : <UserDashboard />}
        </main>
      </div>
    </div>
  );
}
