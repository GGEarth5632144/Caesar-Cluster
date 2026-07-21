import { useState, useEffect } from "react";
import { Eye, Clock, CheckCircle2, XCircle, Cpu, Layers, HardDrive, X, Loader2 } from "lucide-react";
import { adminVmRequestApi, type AdminVmRequest } from "@/api/requests";
import { getApiErrorMessage } from "@/api/authApi";
import { cn } from "@/lib/utils";

function formatDateTime(dateString: string) {
  return new Date(dateString).toLocaleDateString("th-TH", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function statusBadge(status: AdminVmRequest["status"]) {
  switch (status) {
    case "approved":
      return { label: "approved", icon: CheckCircle2, className: "bg-green-50 text-green-700" };
    case "denied":
      return { label: "denied", icon: XCircle, className: "bg-red-50 text-red-600" };
    default:
      return { label: "pending", icon: Clock, className: "bg-[#FFF8E8] text-[#F08B51]" };
  }
}

export default function AdminRequestQueue() {
  const [requests, setRequests] = useState<AdminVmRequest[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actioningId, setActioningId] = useState<number | null>(null);
  const [detailId, setDetailId] = useState<number | null>(null);

  const fetchRequests = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await adminVmRequestApi.listAll();
      setRequests(data);
    } catch (err) {
      console.error(err);
      setError(getApiErrorMessage(err, "ดึงข้อมูลคำขอไม่สำเร็จ"));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchRequests();
  }, []);

  const handleApprove = async (id: number) => {
    setActioningId(id);
    try {
      await adminVmRequestApi.approve(id);
      await fetchRequests();
      setDetailId(null);
    } catch (err) {
      console.error(err);
      alert(getApiErrorMessage(err, "อนุมัติคำขอไม่สำเร็จ"));
    } finally {
      setActioningId(null);
    }
  };

  const handleDeny = async (id: number) => {
    setActioningId(id);
    try {
      await adminVmRequestApi.deny(id);
      await fetchRequests();
      setDetailId(null);
    } catch (err) {
      console.error(err);
      alert(getApiErrorMessage(err, "ปฏิเสธคำขอไม่สำเร็จ"));
    } finally {
      setActioningId(null);
    }
  };

  const detailRequest = requests.find((r) => r.id === detailId) ?? null;
  const pendingCount = requests.filter((r) => r.status === "pending").length;

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6 font-mono">
      <div className="rounded-3xl bg-[#FFFDF6] p-6 shadow-sm sm:p-8">
        <div className="mb-6 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
          <h2 className="text-2xl font-bold text-[#BB6653]">VM Requests</h2>
          {!isLoading && !error && (
            <p className="text-sm text-[#211a14]/50">
              {requests.length} total · {pendingCount} pending
            </p>
          )}
        </div>

        {isLoading ? (
          <div className="flex flex-col items-center justify-center gap-3 py-16 text-[#BB6653]">
            <Loader2 size={28} className="animate-spin" />
            <p className="text-sm font-medium">กำลังโหลดข้อมูล...</p>
          </div>
        ) : error ? (
          <div className="p-4 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100">{error}</div>
        ) : (
          <div className="-mx-6 overflow-x-auto sm:mx-0">
            <table className="w-full min-w-[720px] table-fixed text-left text-sm text-[#211a14]">
              <colgroup>
                <col className="w-[20%]" />
                <col className="w-[15%]" />
                <col className="w-[15%]" />
                <col className="hidden w-[10%] md:table-column" />
                <col className="w-[20%]" />
              </colgroup>
              <thead>
                <tr className="border-b border-black/10 text-xs font-bold uppercase tracking-wider text-[#BB6653]">
                  <th className="px-6 pb-4 sm:px-3">Requester</th>
                  <th className="px-3 pb-4">Resources</th>
                  <th className="px-3 pb-4 text-center">Status</th>
                  <th className="hidden px-3 pb-4 md:table-cell">Submitted</th>
                  <th className="px-6 pb-4 text-center sm:px-3">Detail</th>
                </tr>
              </thead>
              <tbody>
                {requests.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-10 text-center text-neutral-500">
                      ยังไม่มีคำขอในระบบ
                    </td>
                  </tr>
                ) : (
                  requests.map((req) => {
                    const badge = statusBadge(req.status);
                    const BadgeIcon = badge.icon;
                    const initials = (req.requester_name || `U${req.user_id}`)
                      .split(" ")
                      .map((s) => s[0])
                      .join("")
                      .slice(0, 2)
                      .toUpperCase();

                    return (
                      <tr key={req.id} className="border-b border-black/5 last:border-0 hover:bg-black/[0.02] transition-colors">
                        <td className="px-6 py-5 sm:px-3">
                          <div className="flex items-center gap-3">
                            <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-[#F08B51] text-sm font-bold text-white">
                              {initials}
                            </div>
                            <div className="min-w-0">
                              <div className="truncate font-semibold" title={req.requester_name || undefined}>
                                {req.requester_name || `user #${req.user_id}`}
                              </div>
                              <div className="text-xs text-[#211a14]/50">
                                {req.requester_student_id || "—"} · #REQ-{req.id}
                              </div>
                            </div>
                          </div>
                        </td>
                        <td className="px-3 py-5 text-[#211a14]/70">
                          <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                            <span className="flex items-center gap-1.5">
                              <Cpu size={14} className="text-[#BB6653]" /> {req.cpu_limit_milli / 1000} Core
                            </span>
                            <span className="flex items-center gap-1.5">
                              <Layers size={14} className="text-[#BB6653]" /> {req.ram_limit_mb} MB
                            </span>
                            {req.storage_gb > 0 && (
                              <span className="flex items-center gap-1.5">
                                <HardDrive size={14} className="text-[#BB6653]" /> {req.storage_gb} GB
                              </span>
                            )}
                          </div>
                        </td>
                        <td className="px-3 py-5 text-center">
                          <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-3.5 py-1.5 text-xs font-bold", badge.className)}>
                            <BadgeIcon size={12} />
                            {badge.label}
                          </span>
                        </td>
                        <td className="hidden px-3 py-5 text-[#211a14]/70 whitespace-nowrap md:table-cell">
                          {formatDateTime(req.created_at)}
                        </td>
                        <td className="px-6 py-5 text-center sm:px-3">
                          <button
                            onClick={() => setDetailId(req.id)}
                            className="inline-flex size-9 items-center justify-center rounded-lg bg-[#FBDFDA] text-[#BB6653] hover:bg-[#F08B51] hover:text-white transition-colors"
                            title="View Detail"
                          >
                            <Eye size={17} />
                          </button>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {detailRequest && (
        <RequestDetailModal
          request={detailRequest}
          isActioning={actioningId === detailRequest.id}
          onClose={() => setDetailId(null)}
          onApprove={() => handleApprove(detailRequest.id)}
          onDeny={() => handleDeny(detailRequest.id)}
        />
      )}
    </div>
  );
}

interface RequestDetailModalProps {
  request: AdminVmRequest;
  isActioning: boolean;
  onClose: () => void;
  onApprove: () => void;
  onDeny: () => void;
}

function RequestDetailModal({ request, isActioning, onClose, onApprove, onDeny }: RequestDetailModalProps) {
  const isPending = request.status === "pending";
  const badge = statusBadge(request.status);
  const BadgeIcon = badge.icon;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 font-mono">
      <div className="w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl">
        <div className="flex items-center justify-between px-6 py-5 border-b border-black/5">
          <div>
            <h2 className="text-lg font-bold text-[#211a14]">{request.requester_name || `user #${request.user_id}`}</h2>
            <p className="text-xs text-[#211a14]/50 mt-0.5">
              {request.requester_student_id || "—"} · #REQ-{request.id}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={isActioning}
            className="p-2 rounded-xl text-[#211a14]/50 hover:bg-black/5 transition-colors disabled:opacity-50"
          >
            <X size={18} />
          </button>
        </div>

        <div className="px-6 py-5 flex flex-col gap-5">
          <div className="flex items-center justify-between">
            <span className={cn("inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-bold", badge.className)}>
              <BadgeIcon size={12} />
              {badge.label}
            </span>
            <span className="text-xs text-[#211a14]/50 capitalize">{request.namespace_name} space</span>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">Request</label>
            <p className="text-sm text-[#211a14]/80 italic">
              {request.description || "— no note attached —"}
            </p>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">Time Submitted</label>
            <p className="text-sm text-[#211a14]">{formatDateTime(request.created_at)}</p>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="rounded-xl bg-white border border-black/5 p-3 flex flex-col items-center text-center gap-1">
              <Cpu size={16} className="text-[#BB6653]" />
              <span className="text-sm font-bold text-[#211a14]">{request.cpu_limit_milli / 1000}</span>
              <span className="text-[10px] text-[#211a14]/50 uppercase tracking-wide">Cores</span>
            </div>
            <div className="rounded-xl bg-white border border-black/5 p-3 flex flex-col items-center text-center gap-1">
              <Layers size={16} className="text-[#BB6653]" />
              <span className="text-sm font-bold text-[#211a14]">
                {request.ram_limit_mb >= 1024 ? `${(request.ram_limit_mb / 1024).toFixed(1)}` : request.ram_limit_mb}
              </span>
              <span className="text-[10px] text-[#211a14]/50 uppercase tracking-wide">
                {request.ram_limit_mb >= 1024 ? "GB RAM" : "MB RAM"}
              </span>
            </div>
            <div className="rounded-xl bg-white border border-black/5 p-3 flex flex-col items-center text-center gap-1">
              <HardDrive size={16} className="text-[#BB6653]" />
              <span className="text-sm font-bold text-[#211a14]">{request.storage_gb > 0 ? request.storage_gb : "—"}</span>
              <span className="text-[10px] text-[#211a14]/50 uppercase tracking-wide">Storage GB</span>
            </div>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-black/5">
          {isPending ? (
            <>
              <button
                type="button"
                onClick={onDeny}
                disabled={isActioning}
                className="inline-flex items-center gap-2 rounded-xl border border-red-500 px-5 py-2.5 text-sm font-bold text-red-500 hover:bg-red-50 transition-colors disabled:opacity-50"
              >
                {isActioning && <Loader2 size={14} className="animate-spin" />}
                <X size={15} /> Reject
              </button>
              <button
                type="button"
                onClick={onApprove}
                disabled={isActioning}
                className="inline-flex items-center gap-2 rounded-xl bg-green-600 px-5 py-2.5 text-sm font-bold text-white hover:bg-green-700 transition-colors disabled:opacity-50"
              >
                {isActioning && <Loader2 size={14} className="animate-spin" />}
                <CheckCircle2 size={15} /> Approve request
              </button>
            </>
          ) : (
            <p className="text-sm text-[#211a14]/50">
              This request has already been <span className="font-semibold">{request.status}</span>.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
