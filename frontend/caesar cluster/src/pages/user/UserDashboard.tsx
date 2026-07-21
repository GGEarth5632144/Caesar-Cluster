import { useAuthStore } from "@/store/authStore";
import WorkspaceOnboarding from "./WorkspaceOnboarding";
import GeneralDashboard from "./GeneralDashboard";

export default function UserDashboard() {
  const user = useAuthStore((state) => state.user);
  const hasNamespace = Boolean(user?.namespace_id); // เช็คว่ามี Space แล้วหรือยัง

  if (hasNamespace) {
    return <GeneralDashboard user={user} />;
  }

  return <WorkspaceOnboarding />;
}
