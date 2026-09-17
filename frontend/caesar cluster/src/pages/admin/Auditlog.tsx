import { useEffect, useMemo, useState } from "react";
import { RefreshCw, ScrollText } from "lucide-react";

import { auditLogApi, type AuditEvent, type AuditLog } from "@/api/auditLog";
import { getApiErrorMessage } from "@/api/authApi";
import { auditLogScope } from "@/config/searchScopes";
import { cn } from "@/lib/utils";
import { usePageSearch } from "@/hooks/usePageSearch";
import { TableRowsSkeleton } from "@/components/ui/PageSkeletons";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";

type Tab = "all" | "admin" | "user";

const TABS: { id: Tab; label: string }[] = [
  { id: "all", label: "ทั้งหมด" },
  { id: "admin", label: "ผู้ดูแล" },
  { id: "user", label: "นักศึกษา" },
];

const EVENT_BADGE: Record<AuditEvent, { label: string; className: string }> = {
  CREATE: { label: "สร้าง", className: "bg-green-50 text-green-700" },
  UPDATE: { label: "แก้ไข", className: "bg-[#FFF8E8] text-[#F08B51]" },
  APPROVE: { label: "อนุมัติ", className: "bg-[#FBDFDA] text-[#BB6653]" },
  DELETE: { label: "ลบ", className: "bg-red-50 text-red-600" },
};

const formatTime = (iso: string) =>
  new Date(iso).toLocaleString("th-TH", {
    day: "numeric",
    month: "short",
    year: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

export default function Auditlog() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("all");

  const fetchLogs = async (silent = false) => {
    try {
      if (silent) setIsRefreshing(true);
      else setIsLoading(true);
      setError(null);
      setLogs(await auditLogApi.list());
    } catch (err) {
      console.error("Failed to fetch audit logs:", err);
      setError(getApiErrorMessage(err, "ไม่สามารถดึง audit log ได้ โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    fetchLogs();
  }, []);

  const logsInTab = useMemo(
    () => (activeTab === "all" ? logs : logs.filter((l) => l.actor_role === activeTab)),
    [logs, activeTab],
  );

  const { results, isFiltering, highlightTerms } = usePageSearch(auditLogScope, logsInTab);

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6 font-mono">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold text-[#BB6653]">Audit Log</h2>
          <p className="mt-1 text-base text-[#211a14]/50">
            ประวัติการกระทำในระบบ — ใครทำอะไร เมื่อไหร่ จาก IP ไหน (500 รายการล่าสุด)
          </p>
        </div>

        <button
          onClick={() => fetchLogs(true)}
          disabled={isLoading || isRefreshing}
          className="inline-flex items-center gap-2 rounded-xl border-2 border-[#BB6653] bg-transparent px-5 py-2 text-base font-bold text-[#BB6653] shadow-sm transition-colors hover:bg-[#BB6653]/10 disabled:opacity-50"
        >
          <RefreshCw size={18} className={cn(isRefreshing && "animate-spin")} />
          รีเฟรช
        </button>
      </div>

      <div className="rounded-3xl bg-[#FFFDF6] p-6 shadow-sm sm:p-8">
        <div className="mb-6 flex flex-col gap-4 rounded-2xl border border-black/5 bg-white p-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap gap-2">
            {TABS.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "rounded-xl px-4 py-2 text-base font-bold transition-colors",
                  activeTab === tab.id
                    ? "bg-[#BB6653] text-white"
                    : "bg-[#FFF8E8] text-[#211a14]/60 hover:bg-[#F08B51]/20",
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <SearchStatus className="shrink-0" />
        </div>

        <div className="-mx-6 overflow-x-auto sm:mx-0">
          <table className="w-full min-w-[900px] table-fixed text-left text-base text-[#211a14]">
            <colgroup>
              <col className="w-[17%]" />
              <col className="w-[20%]" />
              <col className="w-[25%]" />
              <col className="w-[26%]" />
              <col className="w-[12%]" />
            </colgroup>
            <thead>
              <tr className="border-b border-black/10 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                <th className="px-6 pb-4 sm:px-3">เวลา</th>
                <th className="px-3 pb-4">ผู้กระทำ</th>
                <th className="px-3 pb-4">การกระทำ</th>
                <th className="px-3 pb-4">รายละเอียด</th>
                <th className="px-6 pb-4 sm:px-3">IP</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <TableRowsSkeleton rows={6} cols={5} />
              ) : error ? (
                <tr>
                  <td colSpan={5} className="py-10">
                    <div className="mx-auto max-w-sm rounded-xl border border-red-100 bg-red-50 p-4 text-center text-base text-red-600">
                      {error}
                    </div>
                  </td>
                </tr>
              ) : results.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-16 text-center text-neutral-500">
                    <div className="flex flex-col items-center justify-center gap-2">
                      <ScrollText className="size-8 text-[#BB6653]/30" />
                      <p>
                        {isFiltering
                          ? "ไม่มีรายการในแท็บนี้ตรงกับคำค้นหา"
                          : logs.length === 0
                            ? "ยังไม่มีการกระทำที่ถูกบันทึก"
                            : "ไม่มีรายการในหมวดหมู่นี้"}
                      </p>
                    </div>
                  </td>
                </tr>
              ) : (
                results.map((log) => {
                  const badge = EVENT_BADGE[log.event_type] ?? { label: log.event_type, className: "bg-black/5" };
                  return (
                    <tr
                      key={log.id}
                      className="border-b border-black/5 align-top transition-colors last:border-0 hover:bg-black/[0.02]"
                    >
                      <td className="px-6 py-4 text-sm text-[#211a14]/70 sm:px-3">{formatTime(log.created_at)}</td>

                      <td className="px-3 py-4">
                        <div className="truncate font-semibold" title={log.actor_name}>
                          <Highlight text={log.actor_name || "—"} terms={highlightTerms} />
                        </div>
                        <div className="text-sm text-[#211a14]/45">
                          {log.actor_role === "admin" ? "ผู้ดูแล" : "นักศึกษา"}
                        </div>
                      </td>

                      <td className="px-3 py-4">
                        <span className={cn("inline-flex rounded-full px-2.5 py-0.5 text-xs font-bold", badge.className)}>
                          {badge.label}
                        </span>
                        <div className="mt-1 text-sm font-semibold">
                          <Highlight text={log.action_title} terms={highlightTerms} />
                        </div>
                      </td>

                      <td className="break-all px-3 py-4 text-sm text-[#211a14]/60">
                        <Highlight text={log.detail} terms={highlightTerms} />
                      </td>

                      <td className="break-all px-6 py-4 text-sm text-[#211a14]/60 sm:px-3">
                        <Highlight text={log.source_ip} terms={highlightTerms} />
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
