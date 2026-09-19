import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Users, UserPlus, X, Loader2, Crown, LogOut, AlertTriangle } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { inviteApi, type InviteDetail, type InviteStatus } from "@/api/invite";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { namespaceApi, type NamespaceDetail } from "@/api/namespace";
import { useAuthStore } from "@/store/authStore";

function statusBadgeClass(status: InviteStatus) {
  switch (status) {
    case "accepted":
      return "bg-green-100 text-green-700";
    case "declined":
      return "bg-red-100 text-red-600";
    default:
      return "bg-[#F08B51]/15 text-[#BB6653]";
  }
}

// GroupMembers = การ์ด "ใครอยู่ในกลุ่มนี้บ้าง" (namespace.members — เห็นได้ทุกคนในกลุ่ม)
// + (เฉพาะเจ้าของ) ฟอร์มเชิญคนใหม่ + ลิสต์คำเชิญที่ยังไม่จบ (pending/declined)
//
// ตั้งใจแยก "รายชื่อสมาชิกจริง" ออกจาก "ประวัติคำเชิญ" ให้ชัดเจน — เดิมเอาลิสต์คำเชิญมาโชว์
// แทนรายชื่อสมาชิกไปเลย ทำให้ non-owner ไม่เห็นใครอยู่ในกลุ่มด้วยตัวเองเลย (การ์างว่างเปล่า)
// และคำเชิญที่ accepted แล้วก็ไม่โชว์ซ้ำในลิสต์ประวัติ เพราะมันคือคนในรายชื่อสมาชิกด้านบนอยู่แล้ว
//
// สมาชิกธรรมดา (ไม่ใช่เจ้าของ) เห็นรายชื่อสมาชิกได้ปกติ แต่เชิญคนอื่นไม่ได้ (ตรงกับที่ backend
// บังคับผ่าน ErrNotContributor — ไม่โชว์ฟอร์มที่กดแล้วจะเจอ 403 อยู่ดี)
export default function GroupMembers({
  namespace,
  isOwner,
}: {
  namespace: NamespaceDetail;
  isOwner: boolean;
}) {
  const [studentId, setStudentId] = useState("");
  const [inviting, setInviting] = useState(false);
  const [inviteError, setInviteError] = useState<string | null>(null);
  const [inviteSuccess, setInviteSuccess] = useState<string | null>(null);

  const [sent, setSent] = useState<InviteDetail[]>([]);
  const [loadingSent, setLoadingSent] = useState(isOwner);
  const [cancellingId, setCancellingId] = useState<number | null>(null);

  const navigate = useNavigate();
  const refreshUser = useAuthStore((state) => state.refreshUser);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);

  // หัวหน้ากลุ่มที่เหลือคนเดียว: การออก = ลบพื้นที่ทำงานทั้งก้อน (backend เรียก Delete ต่อให้)
  // หัวหน้าที่ยังมีสมาชิกอยู่: ออกไม่ได้ ต้องให้สมาชิกออกก่อน — บอกไว้ตรงๆ ดีกว่าให้กดแล้วเจอ error
  const isLastOwner = isOwner && namespace.member_count <= 1;
  const ownerBlocked = isOwner && namespace.member_count > 1;

  const handleLeave = async () => {
    setLeaving(true);
    setLeaveError(null);
    try {
      await namespaceApi.leave();
      // ซิงก์ namespace_id ใน store จาก /me — UserDashboard จะสลับไปหน้ายื่นคำขอใหม่ให้เอง
      // (รูปแบบเดียวกับตอนตอบรับคำเชิญใน PendingInvites)
      const me = await authApi.me();
      refreshUser(me);
      navigate("/");
    } catch (err) {
      setLeaveError(getApiErrorMessage(err, "ออกจากกลุ่มไม่สำเร็จ"));
      setLeaving(false);
      setConfirmLeave(false);
    }
  };

  const loadSent = () => {
    if (!isOwner) return;
    setLoadingSent(true);
    inviteApi
      .sent()
      .then(setSent)
      .catch((err) => console.error(err))
      .finally(() => setLoadingSent(false));
  };

  useEffect(() => {
    loadSent();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOwner]);

  const handleInvite = async (e: FormEvent) => {
    e.preventDefault();
    const id = studentId.trim();
    if (!id || inviting) return;

    setInviting(true);
    setInviteError(null);
    setInviteSuccess(null);
    try {
      await inviteApi.create(id);
      setInviteSuccess(`Invite sent to ${id}`);
      setStudentId("");
      loadSent();
    } catch (err) {
      setInviteError(getApiErrorMessage(err, "Failed to send invite"));
    } finally {
      setInviting(false);
    }
  };

  const handleCancel = async (inviteId: number) => {
    setCancellingId(inviteId);
    try {
      await inviteApi.cancel(inviteId);
      setSent((prev) => prev.filter((i) => i.id !== inviteId));
    } catch (err) {
      console.error(err);
    } finally {
      setCancellingId(null);
    }
  };

  // accepted แล้วก็คือสมาชิกที่โชว์อยู่ใน namespace.members อยู่แล้ว ไม่ต้องซ้ำในลิสต์ประวัติ
  const unresolvedInvites = sent.filter((invite) => invite.status !== "accepted");

  return (
    <div className="w-full max-w-3xl mx-auto sm:mx-0 rounded-3xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm font-mono">
      <div className="flex items-center justify-between pb-2 border-b border-black/5">
        <p className="flex items-center gap-2 text-base font-bold tracking-wider text-[#BB6653] uppercase">
          <Users size={16} />
          Group Members
        </p>
        <span className="text-base font-semibold text-[#211a14]/60">
          {namespace.member_count} {namespace.member_count === 1 ? "member" : "members"}
        </span>
      </div>

      <div className="mt-4 flex flex-col gap-2">
        {namespace.members.map((member) => (
          <div
            key={member.id}
            className="flex items-center justify-between rounded-xl bg-[#FFF8E8]/50 p-3 border border-black/[0.02]"
          >
            <div>
              <p className="text-base font-medium text-[#211a14]">{member.real_name}</p>
              <p className="text-sm text-[#211a14]/50">{member.student_id}</p>
            </div>
            {member.is_contributor && (
              <Badge className="gap-1 bg-[#F08B51]/15 text-[#BB6653]">
                <Crown size={13} />
                Owner
              </Badge>
            )}
          </div>
        ))}
      </div>

      {isOwner && (
        <>
          <form onSubmit={handleInvite} className="mt-5 flex items-center gap-2 border-t border-black/5 pt-4">
            <Input
              value={studentId}
              onChange={(e) => setStudentId(e.target.value)}
              placeholder="รหัสประจำตัว"
              disabled={inviting}
              className="flex-1"
            />
            <Button
              type="submit"
              size="sm"
              disabled={inviting || !studentId.trim()}
              className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90"
            >
              {inviting ? <Loader2 size={16} className="animate-spin" /> : <UserPlus size={16} />}
              Invite
            </Button>
          </form>
          {inviteError && <p className="mt-2 text-sm text-red-600">{inviteError}</p>}
          {inviteSuccess && <p className="mt-2 text-sm text-green-600">{inviteSuccess}</p>}

          {!loadingSent && unresolvedInvites.length > 0 && (
            <div className="mt-4 flex flex-col gap-2">
              <p className="text-sm font-semibold text-[#211a14]/40 uppercase tracking-wider">
                Invite Requests
              </p>
              {unresolvedInvites.map((invite) => (
                <div
                  key={invite.id}
                  className="flex items-center justify-between rounded-xl bg-[#FFF8E8]/50 p-3 border border-black/[0.02]"
                >
                  <span className="text-base text-[#211a14]">{invite.invited_student_id}</span>
                  <div className="flex items-center gap-2">
                    <Badge className={cn("capitalize", statusBadgeClass(invite.status))}>
                      {invite.status}
                    </Badge>
                    {invite.status === "pending" && (
                      <button
                        type="button"
                        onClick={() => handleCancel(invite.id)}
                        disabled={cancellingId === invite.id}
                        className="text-[#211a14]/40 hover:text-red-600 transition-colors"
                        aria-label={`Cancel invite to ${invite.invited_student_id}`}
                      >
                        <X size={16} />
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}

      {/* ออกจากกลุ่ม — อยู่ล่างสุดและไม่ใช่ปุ่มเด่น เพราะเป็นทางออก ไม่ใช่งานที่ทำบ่อย */}
      <div className="mt-5 border-t border-black/5 pt-4">
        {ownerBlocked ? (
          <p className="text-sm text-[#211a14]/50">
            คุณเป็นหัวหน้ากลุ่ม จึงออกจากกลุ่มไม่ได้ขณะที่ยังมีสมาชิกคนอื่นอยู่ —
            ให้สมาชิกออกให้หมดก่อน หรือแจ้งผู้ดูแลระบบให้ลบพื้นที่ทำงานนี้
          </p>
        ) : confirmLeave ? (
          <div className="flex flex-col gap-3">
            {isLastOwner ? (
              <p className="flex items-start gap-2 text-sm text-red-600">
                <AlertTriangle size={15} className="mt-0.5 shrink-0" />
                คุณเป็นสมาชิกคนสุดท้าย การออกจากกลุ่มจะ<strong>ลบพื้นที่ทำงาน “{namespace.name}” ทั้งหมด</strong>
                {namespace.usage.service_count > 0 &&
                  ` — service ${namespace.usage.service_count} ตัวและข้อมูลในดิสก์จะถูกลบถาวร กู้คืนไม่ได้`}
              </p>
            ) : (
              <p className="text-sm text-[#211a14]/60">
                ออกจากกลุ่ม “{namespace.name}” ใช่ไหม? คุณจะไม่เห็น service ของกลุ่มนี้อีก
                และต้องให้หัวหน้ากลุ่มเชิญใหม่ถึงจะกลับเข้ามาได้
                {" "}ส่วน service ที่เคยสร้างไว้จะยังอยู่กับกลุ่ม
              </p>
            )}
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={handleLeave}
                disabled={leaving}
                className="inline-flex items-center justify-center gap-1.5 rounded-xl bg-red-500 px-4 py-2 text-sm font-bold text-white transition-colors hover:bg-red-600 disabled:opacity-60"
              >
                {leaving ? (
                  <Loader2 size={15} className="animate-spin" />
                ) : isLastOwner ? (
                  "ยืนยันลบพื้นที่ทำงาน"
                ) : (
                  "ยืนยันออกจากกลุ่ม"
                )}
              </button>
              <button
                type="button"
                onClick={() => setConfirmLeave(false)}
                disabled={leaving}
                className="rounded-xl border border-black/10 px-4 py-2 text-sm font-bold text-[#211a14]/60 transition-colors hover:bg-black/[0.03]"
              >
                ยกเลิก
              </button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => {
              setLeaveError(null);
              setConfirmLeave(true);
            }}
            className="inline-flex items-center gap-1.5 rounded-xl border border-black/10 px-4 py-2 text-sm font-bold text-[#211a14]/60 transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600"
          >
            <LogOut size={15} />
            {isLastOwner ? "ออกจากกลุ่มและลบพื้นที่ทำงาน" : "ออกจากกลุ่ม"}
          </button>
        )}
        {leaveError && <p className="mt-2 text-sm text-red-600">{leaveError}</p>}
      </div>
    </div>
  );
}
