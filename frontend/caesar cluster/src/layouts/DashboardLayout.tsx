import { Suspense, useEffect, useMemo } from "react";
import { Outlet, useNavigate, useLocation } from "react-router-dom";
import { PATHS } from "@/config/routes";
import Sidebar from "@/components/layout/Sidebar";
import Topbar from "@/components/layout/Topbar";
import LogoLoader from "@/components/ui/LogoLoader";
import { useAuthStore } from "@/store/authStore";
import { useAlertStore } from "@/store/alertStore";
import userNavItems from "@/pages/user/User_Navigate";
import adminNavItems from "@/pages/admin/Admin_Navigate";

export default function DashboardLayout() {
  const navigate = useNavigate();
  const location = useLocation();

  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);

  const unreadAlerts = useAlertStore((state) => state.unread);
  const startAlertPolling = useAlertStore((state) => state.startPolling);
  const resetAlerts = useAlertStore((state) => state.reset);

  const isAdmin = user?.role === "admin";
  const hasVm = Boolean(user?.namespace_id);

  // poll จำนวนแจ้งเตือนตามอายุของ layout — วางที่นี่เพราะ Sidebar ต้องรู้ตัวเลขนี้จากทุกหน้า
  // ไม่ใช่เฉพาะตอนเปิดหน้า Alerts อยู่ (ไม่งั้นวงกลมแดงขึ้นก็ต่อเมื่อเข้าไปดูแล้ว ซึ่งไร้ประโยชน์)
  // เฉพาะ user ที่มี namespace: เมนู Alerts ซ่อนอยู่จนกว่าจะมี namespace และ admin ใช้หน้าคนละตัว
  const shouldPollAlerts = !isAdmin && hasVm;
  useEffect(() => {
    if (!shouldPollAlerts) {
      resetAlerts();
      return;
    }
    return startAlertPolling();
  }, [shouldPollAlerts, startAlertPolling, resetAlerts]);

  const navItems = useMemo(
    () =>
      (isAdmin ? adminNavItems : userNavItems)
        .filter((item) => !item.requiresVm || hasVm)
        // เติมตัวเลขวงกลมแดงจากค่าจริง — undefined เมื่อเป็น 0 ให้ Sidebar ไม่ render อะไรเลย
        // (เขียนเงื่อนไขให้ชัดว่าตั้งใจ ไม่พึ่งความ falsy ของ 0)
        .map((item) =>
          item.badgeSource === "alerts"
            ? { ...item, badge: unreadAlerts > 0 ? unreadAlerts : undefined }
            : item,
        ),
    [isAdmin, hasVm, unreadAlerts],
  );

  const currentItem = navItems.find((item) => item.path === location.pathname);
  const pageTitle = currentItem ? currentItem.label : "General Dashboard";

  const handleLogout = () => {
    resetAlerts();
    logout();
    navigate(PATHS.login);
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
          {/* Suspense ระดับหน้า — คงแถบ Sidebar/Topbar ไว้ระหว่างโหลด chunk ของแต่ละหน้า */}
          <Suspense fallback={<LogoLoader label="กำลังโหลดหน้า..." />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  );
}