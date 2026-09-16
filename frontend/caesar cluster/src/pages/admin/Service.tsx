import { useEffect, useMemo, useState } from "react";
import {
  Box,
  Clock,
  Cpu,
  Database,
  HardDrive,
  Layers,
  Loader2,
  RefreshCw,
  Search,
  Trash2,
  Undo2,
  type LucideIcon,
} from "lucide-react";

import { adminServiceApi, type AdminService, type ServiceStatus } from "@/api/services";
import { getApiErrorMessage } from "@/api/authApi";
import { formatStorage } from "@/config/database";
import { adminServicesScope } from "@/config/searchScopes";
import { cn } from "@/lib/utils";
import { notify } from "@/lib/modal";
import { usePageSearch } from "@/hooks/usePageSearch";
import { AdminModal } from "@/components/ui/admin-modal";
import { TableRowsSkeleton } from "@/components/ui/PageSkeletons";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";

type Tab = "all" | "running" | "problem" | "scheduled";
type Metric = "cpu" | "ram" | "storage";
type DeleteMode = "later" | "now";

const STATUS_BADGE: Record<ServiceStatus, { label: string; className: string }> = {
  running: { label: "Running", className: "bg-green-50 text-green-700" },
  creating: { label: "Deploying", className: "bg-[#FFF8E8] text-[#F08B51]" },
  pending: { label: "รอทรัพยากร", className: "bg-[#FBEFD9] text-[#A96A15]" },
  crashloop: { label: "ตายซ้ำๆ", className: "bg-red-50 text-red-600" },
  failed: { label: "Failed", className: "bg-red-50 text-red-600" },
};

const cores = (milli: number) => (milli / 1000).toFixed(milli % 1000 === 0 ? 0 : 1);
const gigabytes = (mb: number) => (mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1);

const METRIC_FORMAT: Record<Metric, (value: number) => string> = {
  cpu: (v) => `${cores(v)} Core`,
  ram: (v) => `${gigabytes(v)} GB`,
  storage: (v) => formatStorage(v),
};

const METRIC_TABS: { id: Metric; label: string }[] = [
  { id: "cpu", label: "CPU" },
  { id: "ram", label: "RAM" },
  { id: "storage", label: "Disk" },
];

// ยอดที่กินโควตาจริง = สเปกต่อ pod × จำนวน pod (สูตรเดียวกับ QuotaService)
function usageOf(svc: AdminService): Record<Metric, number> {
  return {
    cpu: svc.cpu_milli * svc.replicas,
    ram: svc.ram_mb * svc.replicas,
    storage: svc.storage_mb * svc.replicas,
  };
}

const isProblem = (svc: AdminService) => svc.status === "crashloop" || svc.status === "failed";

function timeLeft(iso: string) {
  const minutes = Math.max(0, Math.round((new Date(iso).getTime() - Date.now()) / 60000));
  if (minutes < 1) return "กำลังจะถูกลบ";
  const hours = Math.floor(minutes / 60);
  return hours > 0 ? `ลบในอีก ${hours} ชม. ${minutes % 60} นาที` : `ลบในอีก ${minutes} นาที`;
}

export default function Service() {
  const [services, setServices] = useState<AdminService[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("all");
  const [metric, setMetric] = useState<Metric>("cpu");
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [actioningId, setActioningId] = useState<number | null>(null);

  const fetchServices = async (silent = false) => {
    try {
      if (silent) setIsRefreshing(true);
      else setIsLoading(true);
      setError(null);
      setServices(await adminServiceApi.listAll());
    } catch (err) {
      console.error("Failed to fetch services:", err);
      setError(getApiErrorMessage(err, "ไม่สามารถดึงรายการ service ได้ โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    fetchServices();
  }, []);

  const totals = useMemo(
    () =>
      services.reduce(
        (sum, svc) => {
          const u = usageOf(svc);
          return { cpu: sum.cpu + u.cpu, ram: sum.ram + u.ram, storage: sum.storage + u.storage };
        },
        { cpu: 0, ram: 0, storage: 0 },
      ),
    [services],
  );

  const problemCount = services.filter(isProblem).length;
  const scheduledCount = services.filter((s) => s.delete_at).length;
  const databaseCount = services.filter((s) => s.is_database).length;

  const topUsage = useMemo(
    () =>
      services
        .filter((s) => usageOf(s)[metric] > 0)
        .sort((a, b) => usageOf(b)[metric] - usageOf(a)[metric])
        .slice(0, 5),
    [services, metric],
  );

  const servicesInTab = useMemo(
    () =>
      services.filter((s) => {
        if (activeTab === "running") return s.status === "running";
        if (activeTab === "problem") return isProblem(s);
        if (activeTab === "scheduled") return s.delete_at !== null;
        return true;
      }),
    [services, activeTab],
  );

  const {
    results: filteredServices,
    isFiltering,
    highlightTerms,
  } = usePageSearch(adminServicesScope, servicesInTab);

  const patchService = (id: number, patch: Partial<AdminService>) =>
    setServices((prev) => prev.map((s) => (s.id === id ? { ...s, ...patch } : s)));

  const handleDelete = async (svc: AdminService, mode: DeleteMode) => {
    setActioningId(svc.id);
    try {
      if (mode === "now") {
        await adminServiceApi.remove(svc.id);
        setServices((prev) => prev.filter((s) => s.id !== svc.id));
        notify.success("ลบ service สำเร็จ", `${svc.name} ถูกถอนออกจากคลัสเตอร์แล้ว`);
      } else {
        const { delete_at } = await adminServiceApi.scheduleDelete(svc.id);
        patchService(svc.id, { delete_at });
        notify.success("ตั้งเวลาลบแล้ว", `${svc.name} จะถูกลบภายใน 24 ชั่วโมง`);
      }
      setDeleteId(null);
    } catch (err) {
      console.error("Failed to delete service:", err);
      notify.error("ดำเนินการไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setActioningId(null);
    }
  };

  const handleCancelSchedule = async (svc: AdminService) => {
    setActioningId(svc.id);
    try {
      await adminServiceApi.cancelScheduledDelete(svc.id);
      patchService(svc.id, { delete_at: null });
      notify.success("ยกเลิกการตั้งเวลาลบแล้ว", svc.name);
    } catch (err) {
      console.error("Failed to cancel scheduled delete:", err);
      notify.error("ยกเลิกไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setActioningId(null);
    }
  };

  const tabs: { id: Tab; label: string }[] = [
    { id: "all", label: "ทั้งหมด" },
    { id: "running", label: "Running" },
    { id: "problem", label: "มีปัญหา" },
    { id: "scheduled", label: "รอลบ" },
  ];

  const deleteTarget = services.find((s) => s.id === deleteId) ?? null;

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6 font-mono">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold text-[#BB6653]">Service Management</h2>
          <p className="mt-1 text-base text-[#211a14]/50">
            บริการทั้งหมดในคลัสเตอร์ — ดูการใช้ทรัพยากร และลบทันทีหรือตั้งเวลาลบภายใน 24 ชม.
          </p>
        </div>

        <button
          onClick={() => fetchServices(true)}
          disabled={isLoading || isRefreshing}
          className="inline-flex items-center gap-2 rounded-xl border-2 border-[#BB6653] bg-transparent px-5 py-2 text-base font-bold text-[#BB6653] shadow-sm transition-colors hover:bg-[#BB6653]/10 disabled:opacity-50"
        >
          <RefreshCw size={18} className={cn(isRefreshing && "animate-spin")} />
          รีเฟรช
        </button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <SummaryCard
          icon={Box}
          label="Services ทั้งหมด"
          value={services.length.toString()}
          hint={`${problemCount} มีปัญหา · ${scheduledCount} รอลบ`}
          hintClassName={problemCount > 0 ? "text-red-600" : undefined}
        />
        <SummaryCard icon={Cpu} label="CPU ที่ใช้รวม" value={METRIC_FORMAT.cpu(totals.cpu)} hint="รวมทุก replica" />
        <SummaryCard icon={Layers} label="RAM ที่ใช้รวม" value={METRIC_FORMAT.ram(totals.ram)} hint="รวมทุก replica" />
        <SummaryCard
          icon={HardDrive}
          label="ดิสก์ที่จองรวม"
          value={METRIC_FORMAT.storage(totals.storage)}
          hint={`จาก database ${databaseCount} ตัว`}
        />
      </div>

      <div className="rounded-3xl bg-[#FFFDF6] p-6 shadow-sm sm:p-8">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">ใช้ทรัพยากรมากที่สุด</p>
          <div className="flex gap-2">
            {METRIC_TABS.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setMetric(tab.id)}
                className={cn(
                  "rounded-lg px-3 py-1.5 text-sm font-bold transition-colors",
                  metric === tab.id
                    ? "bg-[#BB6653] text-white"
                    : "bg-[#FFF8E8] text-[#211a14]/60 hover:bg-[#F08B51]/20",
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>

        {isLoading ? (
          <p className="mt-5 text-base text-[#211a14]/40">กำลังโหลด...</p>
        ) : topUsage.length === 0 ? (
          <p className="mt-5 rounded-xl border border-black/5 bg-white p-4 text-base text-[#211a14]/40">
            ยังไม่มี service ที่ใช้ทรัพยากรนี้
          </p>
        ) : (
          <ol className="mt-5 flex flex-col gap-3">
            {topUsage.map((svc, index) => {
              const value = usageOf(svc)[metric];
              const share = totals[metric] > 0 ? (value / totals[metric]) * 100 : 0;
              return (
                <li key={svc.id} className="flex items-center gap-4 rounded-xl border border-black/5 bg-white px-4 py-3">
                  <span
                    className={cn(
                      "flex size-8 shrink-0 items-center justify-center rounded-lg text-sm font-bold",
                      index === 0 ? "bg-[#BB6653] text-white" : "bg-[#FFF8E8] text-[#BB6653]",
                    )}
                  >
                    {index + 1}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-baseline justify-between gap-3">
                      <p className="truncate font-semibold text-[#211a14]">
                        {svc.name}
                        <span className="ml-2 text-sm font-normal text-[#211a14]/40">{svc.namespace_name}</span>
                      </p>
                      <span className="shrink-0 text-sm font-bold text-[#211a14]">
                        {METRIC_FORMAT[metric](value)}
                        <span className="ml-1.5 font-normal text-[#211a14]/40">{Math.round(share)}%</span>
                      </span>
                    </div>
                    <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-black/5">
                      <div className="h-full rounded-full bg-[#F08B51]" style={{ width: `${share}%` }} />
                    </div>
                  </div>
                </li>
              );
            })}
          </ol>
        )}
      </div>

      <div className="rounded-3xl bg-[#FFFDF6] p-6 shadow-sm sm:p-8">
        <div className="mb-6 flex flex-col gap-4 rounded-2xl border border-black/5 bg-white p-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap gap-2">
            {tabs.map((tab) => (
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
              <col className="w-[30%]" />
              <col className="w-[22%]" />
              <col className="w-[20%]" />
              <col className="w-[16%]" />
              <col className="w-[12%]" />
            </colgroup>
            <thead>
              <tr className="border-b border-black/10 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                <th className="px-6 pb-4 sm:px-3">Service</th>
                <th className="px-3 pb-4">Namespace</th>
                <th className="px-3 pb-4">Resources</th>
                <th className="px-3 pb-4">Status</th>
                <th className="px-6 pb-4 text-center sm:px-3">Action</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <TableRowsSkeleton rows={5} cols={5} />
              ) : error ? (
                <tr>
                  <td colSpan={5} className="py-10">
                    <div className="mx-auto max-w-sm rounded-xl border border-red-100 bg-red-50 p-4 text-center text-base text-red-600">
                      {error}
                    </div>
                  </td>
                </tr>
              ) : filteredServices.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-16 text-center text-neutral-500">
                    <div className="flex flex-col items-center justify-center gap-2">
                      <Search className="size-8 text-[#BB6653]/30" />
                      <p>
                        {isFiltering
                          ? "ไม่มี service ในแท็บนี้ตรงกับคำค้นหา"
                          : services.length === 0
                            ? "ยังไม่มี service ในระบบ"
                            : "ไม่มี service ในหมวดหมู่นี้"}
                      </p>
                    </div>
                  </td>
                </tr>
              ) : (
                filteredServices.map((svc) => {
                  const usage = usageOf(svc);
                  const badge = STATUS_BADGE[svc.status];
                  const busy = actioningId === svc.id;

                  return (
                    <tr
                      key={svc.id}
                      className="border-b border-black/5 transition-colors last:border-0 hover:bg-black/[0.02]"
                    >
                      <td className="px-6 py-4 sm:px-3">
                        <div className="flex items-center gap-3">
                          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-[#F08B51] text-white">
                            {svc.is_database ? <Database size={18} /> : <Box size={18} />}
                          </div>
                          <div className="min-w-0">
                            <div className="truncate font-semibold text-[#211a14]" title={svc.name}>
                              <Highlight text={svc.name} terms={highlightTerms} />
                            </div>
                            <div className="truncate text-sm text-[#211a14]/45" title={svc.image}>
                              <Highlight text={svc.image} terms={highlightTerms} />
                            </div>
                          </div>
                        </div>
                      </td>

                      <td className="px-3 py-4 text-sm">
                        <div className="truncate font-semibold text-[#211a14]/80">
                          <Highlight text={svc.namespace_name} terms={highlightTerms} />
                        </div>
                        <div className="mt-0.5 truncate text-[#211a14]/50">
                          <Highlight text={svc.creator_name || "—"} terms={highlightTerms} />
                          {svc.creator_student_id && ` · ${svc.creator_student_id}`}
                        </div>
                      </td>

                      <td className="px-3 py-4">
                        <div className="flex flex-col gap-1 text-sm text-[#211a14]/70">
                          <span className="flex items-center gap-1.5">
                            <Cpu size={14} className="text-[#BB6653]" />
                            {METRIC_FORMAT.cpu(usage.cpu)}
                            {svc.replicas > 1 && <span className="text-[#211a14]/40">({svc.replicas} pods)</span>}
                          </span>
                          <span className="flex items-center gap-1.5">
                            <Layers size={14} className="text-[#BB6653]" />
                            {METRIC_FORMAT.ram(usage.ram)}
                          </span>
                          {svc.is_database && (
                            <span className="flex items-center gap-1.5">
                              <HardDrive size={14} className="text-[#BB6653]" />
                              {METRIC_FORMAT.storage(usage.storage)}
                            </span>
                          )}
                        </div>
                      </td>

                      <td className="px-3 py-4">
                        <span className={cn("inline-flex rounded-full px-2.5 py-1 text-sm font-bold", badge.className)}>
                          {badge.label}
                        </span>
                        {svc.delete_at && (
                          <div className="mt-1.5 flex items-center gap-1 text-xs font-bold text-red-600">
                            <Clock size={12} className="shrink-0" />
                            {timeLeft(svc.delete_at)}
                          </div>
                        )}
                      </td>

                      <td className="px-6 py-4 text-center sm:px-3">
                        <div className="flex items-center justify-center gap-2">
                          {svc.delete_at && (
                            <button
                              onClick={() => handleCancelSchedule(svc)}
                              disabled={busy}
                              title="ยกเลิกการตั้งเวลาลบ"
                              className="rounded-lg p-1.5 text-[#BB6653] transition-colors hover:bg-black/5 hover:text-[#F08B51] disabled:opacity-50"
                            >
                              <Undo2 size={18} />
                            </button>
                          )}
                          <button
                            onClick={() => setDeleteId(svc.id)}
                            disabled={busy}
                            title="ลบ service"
                            className="rounded-lg p-1.5 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 disabled:opacity-50"
                          >
                            {busy ? <Loader2 size={18} className="animate-spin" /> : <Trash2 size={18} />}
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {deleteTarget && (
        <DeleteServiceModal
          service={deleteTarget}
          isDeleting={actioningId === deleteTarget.id}
          onClose={() => setDeleteId(null)}
          onConfirm={(mode) => handleDelete(deleteTarget, mode)}
        />
      )}
    </div>
  );
}

interface SummaryCardProps {
  icon: LucideIcon;
  label: string;
  value: string;
  hint: string;
  hintClassName?: string;
}

function SummaryCard({ icon: Icon, label, value, hint, hintClassName }: SummaryCardProps) {
  return (
    <div className="rounded-2xl border border-black/5 bg-[#FFFDF6] p-5 shadow-sm">
      <div className="flex items-center gap-1.5 text-sm font-bold uppercase tracking-wider text-[#211a14]/50">
        <Icon size={16} className="text-[#BB6653]" />
        {label}
      </div>
      <p className="mt-3 text-3xl font-bold text-[#211a14]">{value}</p>
      <p className={cn("mt-1 text-sm text-[#211a14]/45", hintClassName)}>{hint}</p>
    </div>
  );
}

interface DeleteServiceModalProps {
  service: AdminService;
  isDeleting: boolean;
  onClose: () => void;
  onConfirm: (mode: DeleteMode) => void;
}

function DeleteServiceModal({ service, isDeleting, onClose, onConfirm }: DeleteServiceModalProps) {
  const alreadyScheduled = service.delete_at !== null;
  const [mode, setMode] = useState<DeleteMode>(alreadyScheduled ? "now" : "later");

  const options: { id: DeleteMode; title: string; description: string; hidden?: boolean }[] = [
    {
      id: "later",
      title: "ตั้งเวลาลบใน 24 ชม.",
      description: "ให้เวลาเจ้าของสำรองข้อมูล ยกเลิกได้ก่อนถึงเวลา",
      hidden: alreadyScheduled,
    },
    {
      id: "now",
      title: "ลบทันที",
      description: "ถอนออกจากคลัสเตอร์ตอนนี้เลย ย้อนกลับไม่ได้",
    },
  ];

  return (
    <AdminModal
      onClose={onClose}
      busy={isDeleting}
      size="sm"
      title={`ลบ service "${service.name}"?`}
      subtitle={`${service.namespace_name} · ${service.creator_name || "ไม่ทราบผู้สร้าง"}`}
      onSubmit={(e) => {
        e.preventDefault();
        onConfirm(mode);
      }}
      footer={(close) => (
        <>
          <button
            type="button"
            onClick={close}
            disabled={isDeleting}
            className="rounded-xl px-5 py-2.5 text-base font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isDeleting}
            className="inline-flex min-w-[140px] items-center justify-center gap-2 rounded-xl bg-red-500 px-5 py-2.5 text-base font-bold text-white transition-colors hover:bg-red-600 disabled:opacity-50"
          >
            {isDeleting ? (
              <Loader2 size={18} className="animate-spin" />
            ) : mode === "now" ? (
              <>
                <Trash2 size={18} />
                ลบทันที
              </>
            ) : (
              <>
                <Clock size={18} />
                ตั้งเวลาลบ
              </>
            )}
          </button>
        </>
      )}
    >
      <div className="flex flex-col gap-3">
        {options
          .filter((option) => !option.hidden)
          .map((option) => (
            <label
              key={option.id}
              className={cn(
                "flex cursor-pointer items-start gap-3 rounded-2xl border bg-white p-4 transition-colors",
                mode === option.id ? "border-[#BB6653] ring-2 ring-[#BB6653]/10" : "border-black/10 hover:border-[#F08B51]/50",
              )}
            >
              <input
                type="radio"
                name="delete-mode"
                checked={mode === option.id}
                onChange={() => setMode(option.id)}
                disabled={isDeleting}
                className="mt-1 size-4 accent-[#BB6653]"
              />
              <span>
                <span className="block font-bold text-[#211a14]">{option.title}</span>
                <span className="block text-sm text-[#211a14]/55">{option.description}</span>
              </span>
            </label>
          ))}

        {alreadyScheduled && service.delete_at && (
          <p className="rounded-xl bg-[#FFF8E8] px-4 py-3 text-sm text-[#211a14]/60">
            ตั้งเวลาลบไว้แล้ว · {timeLeft(service.delete_at)}
          </p>
        )}

        {service.is_database && (
          <p className="rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-600">
            service นี้เป็น database — ข้อมูลในดิสก์ {formatStorage(service.storage_mb)} จะหายไปพร้อมกัน
          </p>
        )}
      </div>
    </AdminModal>
  );
}
