import { useState, useEffect } from "react";
import { Clock, CheckCircle2, XCircle, Cpu, Layers, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { vmRequestApi, type VmRequest } from "@/api/requests";
import { getApiErrorMessage } from "@/api/authApi";

export default function RequestResources() {
  const [requests, setRequests] = useState<VmRequest[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);

    vmRequestApi
      .listMine()
      .then((data) => setRequests(data))
      .catch((err) => {
        console.error(err);
        setError(getApiErrorMessage(err, "ไม่สามารถดึงข้อมูลประวัติคำขอทรัพยากรได้"));
      })
      .finally(() => setLoading(false));
  }, []);

  const formatTimeAgo = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleDateString("th-TH", {
      day: "numeric",
      month: "short",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  if (loading) {
    return (
      <div className="flex h-full min-h-[400px] flex-col items-center justify-center gap-3 text-[#BB6653] font-mono">
        <Loader2 size={36} className="animate-spin" />
        <p className="text-sm font-medium">Loading your requests...</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 text-left font-mono animate-in fade-in duration-200">
      <div>
        <h1 className="text-4xl font-bold text-[#211a14]">Requests</h1>
        <p className="max-w-2xl text-base text-[#211a14]/60 mt-1">
          Track your VM and quota requests, and their approval status.
        </p>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100 max-w-3xl">
          {error}
        </div>
      )}

      {!loading && !error && requests.length === 0 ? (
        <div className="w-full max-w-4xl rounded-3xl border border-black/5 bg-[#FFFDF6] p-12 text-center text-gray-400">
          คุณยังไม่เคยยื่นคำขอสร้างหรือปรับโควตาทรัพยากรเข้ามาในระบบ
        </div>
      ) : (
        <div className="flex flex-col gap-6 max-w-5xl">
          {requests.map((req) => {
            const coreDisplay = req.cpu_limit_milli / 1000;
            const ramDisplay = req.ram_limit_mb >= 1024
              ? `${(req.ram_limit_mb / 1024).toFixed(0)} GB`
              : `${req.ram_limit_mb} MB`;

            const isPending = req.status === "pending";
            const isApproved = req.status === "approved";
            const isDenied = req.status === "denied";

            return (
              <div
                key={req.id}
                className={cn(
                  "w-full rounded-3xl border-l-4 bg-[#FFFDF6] p-6 sm:p-8 shadow-sm border border-black/5 transition-all",
                  isPending && "border-l-[#F08B51]",
                  isApproved && "border-l-green-600",
                  isDenied && "border-l-red-500"
                )}
              >
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div className="flex items-start gap-4">
                    <div className={cn(
                      "flex size-12 shrink-0 items-center justify-center rounded-2xl",
                      isPending && "bg-[#FFF8E8] text-[#F08B51]",
                      isApproved && "bg-green-50 text-green-600",
                      isDenied && "bg-red-50 text-red-500"
                    )}>
                      {isPending && <Clock size={24} />}
                      {isApproved && <CheckCircle2 size={24} />}
                      {isDenied && <XCircle size={24} />}
                    </div>
                    <div>
                      <h3 className="text-xl font-bold text-[#211a14]">
                        New VM request submitted
                      </h3>
                      <p className="text-xs text-[#211a14]/40 mt-0.5">
                        #REQ-{req.id} • submitted {formatTimeAgo(req.created_at)}
                      </p>
                    </div>
                  </div>

                  <div className={cn(
                    "inline-flex items-center gap-1.5 self-start sm:self-center px-3 py-1.5 rounded-full text-xs font-bold",
                    isPending && "bg-[#FFF8E8] text-[#F08B51]",
                    isApproved && "bg-green-50 text-green-700",
                    isDenied && "bg-red-50 text-red-600"
                  )}>
                    <span className={cn(
                      "size-2 rounded-full",
                      isPending && "bg-[#F08B51]",
                      isApproved && "bg-green-600",
                      isDenied && "bg-red-500"
                    )} />
                    {isPending && "Pending admin approval"}
                    {isApproved && "Approved / Provisioned"}
                    {isDenied && "Request Denied"}
                  </div>
                </div>

                <div className="mt-8 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 border-b border-black/5 pb-6">

                  <div className="flex items-center gap-2.5">
                    <div className="flex size-6 items-center justify-center rounded-full bg-green-600 text-white text-xs font-bold">
                      ✓
                    </div>
                    <span className="text-sm font-semibold text-[#211a14]">Submitted</span>
                  </div>

                  <div className={cn(
                    "hidden sm:block h-0.5 flex-1 mx-2 bg-black/10",
                    (isApproved || isDenied) && "bg-green-600"
                  )} />

                  <div className="flex items-center gap-2.5">
                    <div className={cn(
                      "flex size-6 items-center justify-center rounded-full text-xs font-bold",
                      isPending && "bg-[#F08B51] text-white",
                      isApproved && "bg-green-600 text-white",
                      isDenied && "bg-red-500 text-white",
                    )}>
                      {isApproved ? "✓" : isDenied ? "✕" : "2"}
                    </div>
                    <span className={cn("text-sm font-semibold", !isPending && "text-[#211a14]/50", isPending && "text-[#211a14]")}>
                      Admin review
                    </span>
                  </div>

                  <div className={cn(
                    "hidden sm:block h-0.5 flex-1 mx-2 bg-black/10",
                    isApproved && "bg-green-600"
                  )} />

                  <div className="flex items-center gap-2.5">
                    <div className={cn(
                      "flex size-6 items-center justify-center rounded-full text-xs font-bold bg-black/10 text-black/40",
                      isApproved && "bg-green-600 text-white"
                    )}>
                      {isApproved ? "✓" : "3"}
                    </div>
                    <span className={cn("text-sm font-semibold text-[#211a14]/40", isApproved && "text-[#211a14]")}>
                      Provisioned
                    </span>
                  </div>

                </div>

                <div className="mt-6 rounded-xl bg-[#FFF8E8]/40 px-5 py-4 text-sm font-medium text-[#211a14]/80 flex flex-wrap items-center gap-x-6 gap-y-2">
                  <div className="capitalize font-bold text-[#BB6653]">
                    {req.namespace_name} Space
                  </div>
                  <div className="hidden sm:block size-1 bg-black/20 rounded-full" />
                  <div className="flex items-center gap-1">
                    <Cpu size={14} className="text-[#BB6653]" />
                    <span>{coreDisplay} cores</span>
                  </div>
                  <div className="hidden sm:block size-1 bg-black/20 rounded-full" />
                  <div className="flex items-center gap-1">
                    <Layers size={14} className="text-[#BB6653]" />
                    <span>{ramDisplay} RAM</span>
                  </div>
                  {req.description && (
                    <>
                      <div className="hidden sm:block size-1 bg-black/20 rounded-full" />
                      <div className="text-xs text-[#211a14]/50 italic truncate max-w-xs">
                        Note: "{req.description}"
                      </div>
                    </>
                  )}
                </div>

              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
