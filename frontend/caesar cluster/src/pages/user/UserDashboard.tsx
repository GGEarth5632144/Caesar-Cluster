import { useAuthStore } from "@/store/authStore";
import PendingInvites from "@/components/PendingInvites";
import WorkspaceOnboarding from "./WorkspaceOnboarding";
import GeneralDashboard from "./GeneralDashboard";

export default function UserDashboard() {
  const user = useAuthStore((state) => state.user);
  const hasNamespace = Boolean(user?.namespace_id); // เช็คว่ามี Space แล้วหรือยัง

  // คำเชิญเข้ากลุ่มต้องเห็นได้ไม่ว่าจะมี namespace อยู่แล้วหรือยัง (คนอื่นเชิญเราได้ตลอดเวลา)
  // เลยอยู่เหนือ if/else นี้ ไม่ผูกกับหน้าใดหน้าหนึ่ง
  return (
    <div className="flex flex-col gap-6">
      <PendingInvites />
      {hasNamespace ? <GeneralDashboard user={user} /> : <WorkspaceOnboarding />}
    </div>
  );
}
