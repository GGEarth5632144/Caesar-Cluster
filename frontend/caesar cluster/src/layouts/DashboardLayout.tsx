import { Outlet, useNavigate, useLocation } from "react-router-dom";

import Sidebar from "@/components/layout/Sidebar";
import Topbar from "@/components/layout/Topbar";
import { useAuthStore } from "@/store/authStore";
import userNavItems from "@/pages/user/User_Navigate";
import adminNavItems from "@/pages/admin/Admin_Navigate";

export default function DashboardLayout() {
  const navigate = useNavigate();
  const location = useLocation(); 
  
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);

  const isAdmin = user?.role === "admin";
  const hasVm = Boolean(user?.namespace_id);
  const navItems = (isAdmin ? adminNavItems : userNavItems).filter(
    (item) => !item.requiresVm || hasVm
  );
  const currentItem = navItems.find((item) => item.path === location.pathname);
  const pageTitle = currentItem ? currentItem.label : "General Dashboard";

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  return (
    <div className="flex h-screen w-full overflow-hidden bg-[#FFF8E8]">
      <Sidebar
        navItems={navItems}
        userName={user?.real_name || "User"}
        studentId={user?.student_id ?? ""}
        onLogout={handleLogout}
      />

      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar title={pageTitle} userName={user?.real_name ?? "User"} />

        <main className="flex-1 overflow-auto p-10">
          <Outlet />
        </main>
      </div>
    </div>
  );
}