import { useState, useEffect } from "react";
import { Check, X, Clock, CheckCircle2, XCircle, Cpu, Layers } from "lucide-react";
import { adminVmRequestApi, type VmRequest } from "@/api/requests";
import { getApiErrorMessage } from "@/api/authApi";
import { cn } from "@/lib/utils";

export default function AdminRequestQueue() {
  const [requests, setRequests] = useState<VmRequest[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actioningId, setActioningId] = useState<number | null>(null);

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
    } catch (err) {
      console.error(err);
      alert(getApiErrorMessage(err, "ปฏิเสธคำขอไม่สำเร็จ"));
    } finally {
      setActioningId(null);
    }
  };

  return (
    <div className="flex flex-col gap-6 w-full max-w-5xl mx-auto">
      <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
        <div className="mb-6 flex items-center justify-between">
          <h2 className="text-xl font-bold text-[#BB6653]">VM Requests</h2>
        </div>

        {isLoading ? (
          <div className="text-center py-10 text-neutral-500">กำลังโหลดข้อมูล...</div>
        ) : error ? (
          <div className="p-4 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100">{error}</div>
        ) : (
          <table className="w-full text-left text-sm text-[#211a14]">
            <thead>
              <tr className="border-b border-black/10 text-[#BB6653]">
                <th className="pb-4 font-semibold">Request</th>
                <th className="pb-4 font-semibold">Type</th>
                <th className="pb-4 font-semibold">Resources</th>
                <th className="pb-4 font-semibold text-center">Status</th>
                <th className="pb-4 font-semibold text-center">Action</th>
              </tr>
            </thead>
            <tbody>
              {requests.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-neutral-500">
                    ยังไม่มีคำขอในระบบ
                  </td>
                </tr>
              ) : (
                requests.map((req) => {
                  const isPending = req.status === "pending";
                  const isApproved = req.status === "approved";
                  const isDenied = req.status === "denied";
                  const isActioning = actioningId === req.id;

                  return (
                    <tr key={req.id} className="border-b border-black/5 last:border-0 hover:bg-black/[0.02]">
                      <td className="py-4">
                        <div className="font-medium">#REQ-{req.id} · user #{req.user_id}</div>
                        {req.description && (
                          <div className="text-xs text-[#211a14]/50 italic truncate max-w-xs">{req.description}</div>
                        )}
                      </td>
                      <td className="py-4 capitalize">{req.namespace_name}</td>
                      <td className="py-4 text-[#211a14]/70">
                        <div className="flex items-center gap-3">
                          <span className="flex items-center gap-1"><Cpu size={13} className="text-[#BB6653]" /> {req.cpu_limit_milli / 1000} Core</span>
                          <span className="flex items-center gap-1"><Layers size={13} className="text-[#BB6653]" /> {req.ram_limit_mb} MB</span>
                        </div>
                      </td>
                      <td className="py-4 text-center">
                        <span className={cn(
                          "inline-flex items-center gap-1 px-3 py-1 rounded-full text-xs font-bold",
                          isPending && "bg-[#FFF8E8] text-[#F08B51]",
                          isApproved && "bg-green-50 text-green-700",
                          isDenied && "bg-red-50 text-red-600"
                        )}>
                          {isPending && <Clock size={12} />}
                          {isApproved && <CheckCircle2 size={12} />}
                          {isDenied && <XCircle size={12} />}
                          {req.status}
                        </span>
                      </td>
                      <td className="py-4 text-center">
                        {isPending ? (
                          <div className="flex justify-center gap-2">
                            <button
                              onClick={() => handleApprove(req.id)}
                              disabled={isActioning}
                              className="flex size-8 items-center justify-center rounded-lg bg-green-600 text-white hover:bg-green-700 transition-colors disabled:opacity-50"
                              title="Approve"
                            >
                              <Check size={16} />
                            </button>
                            <button
                              onClick={() => handleDeny(req.id)}
                              disabled={isActioning}
                              className="flex size-8 items-center justify-center rounded-lg border border-red-500 text-red-500 hover:bg-red-50 transition-colors disabled:opacity-50"
                              title="Deny"
                            >
                              <X size={16} />
                            </button>
                          </div>
                        ) : (
                          <span className="text-[#211a14]/30 text-xs">—</span>
                        )}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
