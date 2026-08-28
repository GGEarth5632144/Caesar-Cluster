import { useEffect, useState, type FormEvent } from "react";
import { Users, UserPlus, X, Loader2, Crown } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { inviteApi, type InviteDetail, type InviteStatus } from "@/api/invite";
import { getApiErrorMessage } from "@/api/authApi";
import type { NamespaceDetail } from "@/api/namespace";

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
        <p className="flex items-center gap-2 text-sm font-bold tracking-wider text-[#BB6653] uppercase">
          <Users size={14} />
          Group Members
        </p>
        <span className="text-sm font-semibold text-[#211a14]/60">
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
              <p className="text-sm font-medium text-[#211a14]">{member.real_name}</p>
              <p className="text-xs text-[#211a14]/50">{member.student_id}</p>
            </div>
            {member.is_contributor && (
              <Badge className="gap-1 bg-[#F08B51]/15 text-[#BB6653]">
                <Crown size={11} />
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
              placeholder="Student ID (e.g. B6600001)"
              disabled={inviting}
              className="flex-1"
            />
            <Button
              type="submit"
              size="sm"
              disabled={inviting || !studentId.trim()}
              className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90"
            >
              {inviting ? <Loader2 size={14} className="animate-spin" /> : <UserPlus size={14} />}
              Invite
            </Button>
          </form>
          {inviteError && <p className="mt-2 text-xs text-red-600">{inviteError}</p>}
          {inviteSuccess && <p className="mt-2 text-xs text-green-600">{inviteSuccess}</p>}

          {!loadingSent && unresolvedInvites.length > 0 && (
            <div className="mt-4 flex flex-col gap-2">
              <p className="text-xs font-semibold text-[#211a14]/40 uppercase tracking-wider">
                Invite Requests
              </p>
              {unresolvedInvites.map((invite) => (
                <div
                  key={invite.id}
                  className="flex items-center justify-between rounded-xl bg-[#FFF8E8]/50 p-3 border border-black/[0.02]"
                >
                  <span className="text-sm text-[#211a14]">{invite.invited_student_id}</span>
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
                        <X size={14} />
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
