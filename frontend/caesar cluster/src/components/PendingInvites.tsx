import { useEffect, useState } from "react";
import { Mail, Check, X, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { inviteApi, type InviteDetail } from "@/api/invite";
import { getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";

// PendingInvites = คำเชิญเข้ากลุ่มที่ยังไม่ตอบรับ ส่งถึงตัวเอง
//
// ต้องแสดงได้ทั้งตอนที่ยังไม่มี namespace (อยู่หน้า WorkspaceOnboarding) และตอนมีแล้ว
// (อยู่หน้า GeneralDashboard) เพราะคนอื่นเชิญเราได้ตลอดเวลาไม่ว่าเราจะอยู่สถานะไหน
// เลยแยกเป็น component อิสระ เรียกจาก UserDashboard.tsx เหนือ if/else ของสองหน้านั้น
//
// ไม่โชว์อะไรเลยถ้าไม่มีคำเชิญค้าง (ไม่ใช่ empty state ที่ต้องบอกผู้ใช้ — ไม่มีก็คือไม่มี)
export default function PendingInvites() {
  const user = useAuthStore((state) => state.user);
  const refreshUser = useAuthStore((state) => state.refreshUser);

  const [invites, setInvites] = useState<InviteDetail[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [rowError, setRowError] = useState<Record<number, string>>({});

  useEffect(() => {
    inviteApi
      .mine()
      .then(setInvites)
      .catch((err) => console.error(err))
      .finally(() => setLoading(false));
  }, []);

  const handleAccept = async (invite: InviteDetail) => {
    setBusyId(invite.id);
    setRowError((prev) => ({ ...prev, [invite.id]: "" }));
    try {
      const ns = await inviteApi.accept(invite.id);
      // namespace_id เปลี่ยนแล้วที่ backend — sync กลับเข้า store โดยไม่ต้อง login ใหม่
      // (accept แตะแค่ namespace_id ฟิลด์เดียว ฟิลด์อื่นของ user เหมือนเดิมทุกอย่าง)
      if (user) {
        refreshUser({ ...user, namespace_id: (ns as { id: number }).id });
      }
      setInvites((prev) => prev.filter((i) => i.id !== invite.id));
    } catch (err) {
      setRowError((prev) => ({
        ...prev,
        [invite.id]: getApiErrorMessage(err, "ตอบรับคำเชิญไม่สำเร็จ"),
      }));
    } finally {
      setBusyId(null);
    }
  };

  const handleDecline = async (invite: InviteDetail) => {
    setBusyId(invite.id);
    setRowError((prev) => ({ ...prev, [invite.id]: "" }));
    try {
      await inviteApi.decline(invite.id);
      setInvites((prev) => prev.filter((i) => i.id !== invite.id));
    } catch (err) {
      setRowError((prev) => ({
        ...prev,
        [invite.id]: getApiErrorMessage(err, "ปฏิเสธคำเชิญไม่สำเร็จ"),
      }));
    } finally {
      setBusyId(null);
    }
  };

  if (loading || invites.length === 0) {
    return null;
  }

  return (
    <div className="w-full rounded-3xl bg-[#FFFDF6] p-6 border border-[#F08B51]/20 shadow-sm font-mono">
      <div className="flex items-center gap-2 pb-2 border-b border-black/5">
        <Mail size={18} className="text-[#F08B51]" />
        <p className="text-base font-bold tracking-wider text-[#BB6653] uppercase">
          Group Invitations
        </p>
      </div>

      <div className="mt-4 flex flex-col gap-3">
        {invites.map((invite) => (
          <div
            key={invite.id}
            className="flex flex-col gap-2 rounded-xl bg-[#FFF8E8]/50 p-4 border border-black/[0.02] sm:flex-row sm:items-center sm:justify-between"
          >
            <p className="text-base text-[#211a14]">
              <span className="font-semibold">{invite.invited_by_name}</span> invited you to join{" "}
              <span className="font-semibold">{invite.namespace_name}</span>
            </p>

            <div className="flex items-center gap-2">
              {rowError[invite.id] && (
                <span className="text-sm text-red-600">{rowError[invite.id]}</span>
              )}
              <Button
                type="button"
                size="sm"
                className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90"
                disabled={busyId === invite.id}
                onClick={() => handleAccept(invite)}
              >
                {busyId === invite.id ? <Loader2 size={16} className="animate-spin" /> : <Check size={16} />}
                Accept
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busyId === invite.id}
                onClick={() => handleDecline(invite)}
              >
                <X size={16} />
                Decline
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
