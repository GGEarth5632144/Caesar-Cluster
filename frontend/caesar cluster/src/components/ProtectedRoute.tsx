import { Navigate, Outlet } from "react-router-dom";

import { useAuthStore } from "@/store/authStore";
import { PATHS } from "@/config/routes";

export default function ProtectedRoute() {
  const token = useAuthStore((state) => state.token);

  if (!token) {
    return <Navigate to={PATHS.login} replace />;
  }

  return <Outlet />;
}
