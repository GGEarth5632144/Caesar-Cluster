import { useEffect, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";
import {
  Boxes,
  Cpu,
  Crown,
  Layers,
  Loader2,
  RefreshCw,
  Search,
  Server,
  SlidersHorizontal,
  Trash2,
  Users,
  X,
} from "lucide-react";

import {
  adminNamespaceApi,
  namespaceOwner,
  namespaceState,
  namespaceUsagePercent,
  NAMESPACE_FULL_THRESHOLD,
  QUOTA_BOUNDS,
  type NamespaceDetail,
  type NamespaceState,
} from "@/api/namespace";
import { getApiErrorMessage } from "@/api/authApi";
import { cn } from "@/lib/utils";
import { notify, confirmAction } from "@/lib/modal";
import { TableRowsSkeleton } from "@/components/ui/PageSkeletons";
import { usePageSearch } from "@/hooks/usePageSearch";
import { useSearchStore } from "@/store/searchStore";
import { namespacesScope } from "@/config/searchScopes";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";

// สีของแถบ/ตัวเลขตามความตึงของโควตา — เกณฑ์เดียวกับ GeneralDashboard ฝั่งผู้ใช้
// เพื่อให้แอดมินกับเจ้าของ space เห็นสีแดงที่จุดเดียวกัน
function usageTone(percent: number) {
  if (percent >= NAMESPACE_FULL_THRESHOLD) return { text: "text-red-600", bar: "bg-red-500" };
  if (percent >= 50) return { text: "text-orange-600", bar: "bg-orange-500" };
  return { text: "text-green-600", bar: "bg-green-500" };
}

const STATE_BADGE: Record<NamespaceState, { label: string; className: string }> = {
  active: { label: "ใช้งานอยู่", className: "bg-green-50 text-green-700" },
  full: { label: "ใกล้เต็ม", className: "bg-red-50 text-red-600" },
  idle: { label: "ยังไม่มีบริการ", className: "bg-[#FFF8E8] text-[#F08B51]" },
  empty: { label: "ไม่มีสมาชิก", className: "bg-black/5 text-[#211a14]/50" },
};

const cores = (milli: number) => (milli / 1000).toFixed(milli % 1000 === 0 ? 0 : 1);
const gigabytes = (mb: number) => (mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1);

function formatDate(value: string) {
  return new Date(value).toLocaleDateString("th-TH", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

type StateTab = "all" | NamespaceState;

export default function NamespaceManagement() {
  const [namespaces, setNamespaces] = useState<NamespaceDetail[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [activeTab, setActiveTab] = useState<StateTab>("all");
  // เก็บ id ของ space ที่เปิดแผงจัดการอยู่ ไม่ใช่ตัว object — แผงจะได้อ่านข้อมูลล่าสุดเสมอ
  // หลังบันทึก/รีเฟรช (ถ้าเก็บ object ค่าที่โชว์จะค้างอยู่ที่ตอนกดเปิด)
  const [managingId, setManagingId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const fetchNamespaces = async (silent = false) => {
    try {
      if (silent) setIsRefreshing(true);
      else setIsLoading(true);
      setError(null);
      setNamespaces(await adminNamespaceApi.listAll());
    } catch (err) {
      console.error("Failed to fetch namespaces:", err);
      setError(getApiErrorMessage(err, "ไม่สามารถดึงข้อมูลเนมสเปซได้ โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    fetchNamespaces();
  }, []);

  const handleQuotaSaved = (updated: NamespaceDetail) => {
    setNamespaces((prev) => prev.map((ns) => (ns.id === updated.id ? updated : ns)));
    notify.success(
      "ปรับโควตาสำเร็จ",
      `${updated.name} ได้โควตาใหม่เป็น ${cores(updated.cpu_limit_milli)} Core / ${gigabytes(updated.ram_limit_mb)} GB`,
    );
  };

  const handleDelete = async (ns: NamespaceDetail) => {
    const consequences = [
      ns.usage.service_count > 0
        ? `บริการที่รันอยู่ ${ns.usage.service_count} ตัวจะถูกถอนออกจากคลัสเตอร์`
        : null,
      ns.member_count > 0
        ? `สมาชิก ${ns.member_count} คนจะหลุดออกจาก space (บัญชีไม่ถูกลบ)`
        : null,
    ].filter(Boolean);

    const confirmed = await confirmAction({
      title: `ลบเนมสเปซ "${ns.name}"?`,
      description: [...consequences, "การกระทำนี้ย้อนกลับไม่ได้"].join(" · "),
      confirmText: "ลบเนมสเปซ",
      destructive: true,
    });
    if (!confirmed) return;

    setDeletingId(ns.id);
    try {
      await adminNamespaceApi.remove(ns.id);
      setNamespaces((prev) => prev.filter((item) => item.id !== ns.id));
      setManagingId((current) => (current === ns.id ? null : current));
      notify.success("ลบเนมสเปซสำเร็จ", `${ns.name} ถูกถอนออกจากระบบแล้ว`);
    } catch (err) {
      console.error("Failed to delete namespace:", err);
      notify.error("ลบเนมสเปซไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setDeletingId(null);
    }
  };

  // แท็บกรองก่อน แล้วค่อยส่งที่เหลือให้ช่องค้นหาด้านบน — ลำดับเดียวกับหน้า User Management
  // ตัวเลข n/m ที่ Topbar โชว์จึงหมายถึง "เจอกี่ space ในแท็บนี้" ซึ่งตรงกับสิ่งที่ตาเห็น
  const namespacesInTab = useMemo(
    () =>
      activeTab === "all"
        ? namespaces
        : namespaces.filter((ns) => namespaceState(ns) === activeTab),
    [namespaces, activeTab],
  );

  const {
    results: filteredNamespaces,
    isFiltering,
    highlightTerms,
  } = usePageSearch(namespacesScope, namespacesInTab);

  // คำค้นที่หน้าอื่นฝากมาพร้อมการกระโดด (กดชื่อ space ในหน้า User Management แล้วมาโผล่ที่นี่
  // โดยกรองไว้ให้แล้ว) — ต้องตั้งหลัง usePageSearch เพราะ registerScope ล้าง query ทิ้ง
  // ทุกครั้งที่เปลี่ยนหน้า ถ้าตั้งก่อนจะโดนล้างไปพอดี
  const setQuery = useSearchStore((state) => state.setQuery);
  const { state: navState } = useLocation();
  const handoffQuery = (navState as { search?: string } | null)?.search;
  useEffect(() => {
    if (handoffQuery) setQuery(handoffQuery);
  }, [handoffQuery, setQuery]);

  // ยอดรวมทั้งระบบ — นับจากข้อมูลทั้งก้อนเสมอ ไม่ใช่จากแท็บที่เปิดอยู่
  // การ์ดพวกนี้ตอบคำถาม "ตอนนี้คลัสเตอร์ถูกจองไปเท่าไหร่แล้ว" ซึ่งไม่ควรขยับตามตัวกรองที่กด
  const totals = useMemo(
    () => ({
      allocatedCPU: namespaces.reduce((sum, ns) => sum + ns.cpu_limit_milli, 0),
      allocatedRAM: namespaces.reduce((sum, ns) => sum + ns.ram_limit_mb, 0),
      usedCPU: namespaces.reduce((sum, ns) => sum + ns.usage.used_cpu_milli, 0),
      usedRAM: namespaces.reduce((sum, ns) => sum + ns.usage.used_ram_mb, 0),
      members: namespaces.reduce((sum, ns) => sum + ns.member_count, 0),
      services: namespaces.reduce((sum, ns) => sum + ns.usage.service_count, 0),
      needsAttention: namespaces.filter((ns) => {
        const state = namespaceState(ns);
        return state === "full" || state === "empty";
      }).length,
    }),
    [namespaces],
  );

  const tabs: { id: StateTab; label: string }[] = [
    { id: "all", label: "ทั้งหมด" },
    { id: "active", label: "ใช้งานอยู่" },
    { id: "full", label: "ใกล้เต็ม" },
    { id: "idle", label: "ยังไม่มีบริการ" },
    { id: "empty", label: "ไม่มีสมาชิก" },
  ];

  const managingNamespace = namespaces.find((ns) => ns.id === managingId) ?? null;

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6 font-mono">

      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold text-[#BB6653]">Namespace Management</h2>
          <p className="mt-1 text-base text-[#211a14]/50">
            โควตาผูกกับเนมสเปซ ไม่ได้ผูกกับตัวผู้ใช้ — ปรับที่นี่แล้วมีผลกับสมาชิกทุกคนในกลุ่มพร้อมกัน
          </p>
        </div>

        <button
          onClick={() => fetchNamespaces(true)}
          disabled={isLoading || isRefreshing}
          className="inline-flex items-center gap-2 rounded-xl border-2 border-[#BB6653] bg-transparent px-5 py-2 text-base font-bold text-[#BB6653] shadow-sm transition-colors hover:bg-[#BB6653]/10 disabled:opacity-50"
        >
          <RefreshCw size={18} className={cn(isRefreshing && "animate-spin")} />
          รีเฟรชยอดใช้งาน
        </button>
      </div>

      {/* ภาพรวมทรัพยากรที่ถูกจองไปแล้วทั้งคลัสเตอร์ */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <SummaryCard
          icon={Boxes}
          label="เนมสเปซทั้งหมด"
          value={namespaces.length.toString()}
          hint={
            totals.needsAttention > 0
              ? `${totals.needsAttention} อันควรตรวจสอบ`
              : "ไม่มีอันที่ต้องรีบดู"
          }
          hintClassName={totals.needsAttention > 0 ? "text-red-600" : undefined}
        />
        <SummaryCard
          icon={Users}
          label="สมาชิกที่มี space"
          value={totals.members.toString()}
          hint={`${totals.services} บริการที่รันอยู่`}
        />
        <SummaryCard
          icon={Cpu}
          label="CPU ที่จองไปแล้ว"
          value={`${cores(totals.allocatedCPU)} Core`}
          hint={`ใช้จริง ${cores(totals.usedCPU)} Core`}
        />
        <SummaryCard
          icon={Layers}
          label="RAM ที่จองไปแล้ว"
          value={`${gigabytes(totals.allocatedRAM)} GB`}
          hint={`ใช้จริง ${gigabytes(totals.usedRAM)} GB`}
        />
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
              <col className="w-[26%]" />
              <col className="w-[22%]" />
              <col className="w-[30%]" />
              <col className="w-[12%]" />
              <col className="w-[10%]" />
            </colgroup>
            <thead>
              <tr className="border-b border-black/10 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                <th className="px-6 pb-4 sm:px-3">Namespace</th>
                <th className="px-3 pb-4">Members</th>
                <th className="px-3 pb-4">Quota Limit</th>
                <th className="px-3 pb-4 text-center">Services</th>
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
              ) : filteredNamespaces.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-16 text-center text-neutral-500">
                    <div className="flex flex-col items-center justify-center gap-2">
                      <Search className="size-8 text-[#BB6653]/30" />
                      <p>
                        {isFiltering
                          ? "ไม่มีเนมสเปซในแท็บนี้ตรงกับคำค้นหา"
                          : namespaces.length === 0
                            ? "ยังไม่มีใครสร้างเนมสเปซในระบบ"
                            : "ไม่มีเนมสเปซในหมวดหมู่นี้"}
                      </p>
                    </div>
                  </td>
                </tr>
              ) : (
                filteredNamespaces.map((ns) => {
                  const owner = namespaceOwner(ns);
                  const percent = namespaceUsagePercent(ns);
                  const badge = STATE_BADGE[namespaceState(ns)];

                  return (
                    <tr
                      key={ns.id}
                      className="border-b border-black/5 transition-colors last:border-0 hover:bg-black/[0.02]"
                    >
                      <td className="px-6 py-4 sm:px-3">
                        <div className="flex items-center gap-3">
                          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-[#F08B51] text-white">
                            <Boxes size={18} />
                          </div>
                          <div className="min-w-0">
                            <div className="truncate font-semibold text-[#211a14]" title={ns.name}>
                              <Highlight text={ns.name} terms={highlightTerms} />
                            </div>
                            <div className="mt-1 flex items-center gap-2">
                              <span
                                className={cn(
                                  "inline-flex rounded-full px-2 py-0.5 text-xs font-bold",
                                  badge.className,
                                )}
                              >
                                {badge.label}
                              </span>
                              <span className="text-xs text-[#211a14]/40">
                                {formatDate(ns.created_at)}
                              </span>
                            </div>
                          </div>
                        </div>
                      </td>

                      <td className="px-3 py-4">
                        {owner ? (
                          <div className="min-w-0 text-sm">
                            <div className="flex items-center gap-1.5 truncate font-semibold text-[#211a14]/80">
                              <Crown size={14} className="shrink-0 text-[#BB6653]" />
                              <Highlight text={owner.real_name} terms={highlightTerms} />
                            </div>
                            <div className="mt-0.5 text-[#211a14]/50">
                              <Highlight text={owner.student_id} terms={highlightTerms} />
                              {ns.member_count > 1 && ` · อีก ${ns.member_count - 1} คน`}
                            </div>
                          </div>
                        ) : (
                          <span className="text-sm text-[#211a14]/40">
                            {ns.member_count > 0
                              ? `${ns.member_count} คน (ไม่มีเจ้าของ)`
                              : "ไม่เหลือสมาชิก"}
                          </span>
                        )}
                      </td>

                      <td className="px-3 py-4">
                        <div className="flex flex-col gap-2">
                          <UsageBar
                            icon={Cpu}
                            used={cores(ns.usage.used_cpu_milli)}
                            limit={`${cores(ns.cpu_limit_milli)} Core`}
                            percent={percent.cpu}
                          />
                          <UsageBar
                            icon={Layers}
                            used={gigabytes(ns.usage.used_ram_mb)}
                            limit={`${gigabytes(ns.ram_limit_mb)} GB`}
                            percent={percent.ram}
                          />
                        </div>
                      </td>

                      <td className="px-3 py-4 text-center">
                        <span className="inline-flex items-center gap-1.5 rounded-full bg-[#FFF8E8] px-2.5 py-1 text-sm font-bold text-[#BB6653]">
                          <Server size={14} />
                          {ns.usage.service_count}
                        </span>
                      </td>

                      <td className="px-6 py-4 text-center sm:px-3">
                        <div className="flex items-center justify-center gap-2">
                          <button
                            onClick={() => setManagingId(ns.id)}
                            title="จัดการโควตาและสมาชิก"
                            className="rounded-lg p-1.5 text-[#BB6653] transition-colors hover:bg-black/5 hover:text-[#F08B51]"
                          >
                            <SlidersHorizontal size={18} />
                          </button>
                          <button
                            onClick={() => handleDelete(ns)}
                            disabled={deletingId === ns.id}
                            title="ลบเนมสเปซ"
                            className="rounded-lg p-1.5 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 disabled:opacity-50"
                          >
                            {deletingId === ns.id ? (
                              <Loader2 size={18} className="animate-spin" />
                            ) : (
                              <Trash2 size={18} />
                            )}
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

      {managingNamespace && (
        <ManageNamespaceModal
          namespace={managingNamespace}
          onClose={() => setManagingId(null)}
          onSaved={handleQuotaSaved}
          onDelete={() => handleDelete(managingNamespace)}
        />
      )}
    </div>
  );
}

// ==========================================
// การ์ดสรุปด้านบน
// ==========================================
interface SummaryCardProps {
  icon: typeof Cpu;
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

// ==========================================
// แถบยอดใช้งานเทียบเพดานในตาราง
// ==========================================
interface UsageBarProps {
  icon: typeof Cpu;
  used: string;
  limit: string;
  percent: number;
}

function UsageBar({ icon: Icon, used, limit, percent }: UsageBarProps) {
  const tone = usageTone(percent);
  return (
    <div>
      <div className="flex items-center justify-between text-sm">
        <span className="flex items-center gap-1.5 text-[#211a14]/70">
          <Icon size={14} className="text-[#BB6653]" />
          {used} / {limit}
        </span>
        <span className={cn("text-xs font-bold", tone.text)}>{Math.round(percent)}%</span>
      </div>
      <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-black/5">
        <div
          className={cn("h-full rounded-full transition-all duration-500", tone.bar)}
          style={{ width: `${Math.min(percent, 100)}%` }}
        />
      </div>
    </div>
  );
}

// ==========================================
// แผงจัดการ 1 เนมสเปซ — ปรับโควตา + ดูสมาชิก + ลบทิ้ง
// ==========================================
interface ManageNamespaceModalProps {
  namespace: NamespaceDetail;
  onClose: () => void;
  onSaved: (updated: NamespaceDetail) => void;
  onDelete: () => void;
}

// ตัวเลือกสำเร็จรูปที่แอดมินกดบ่อย — 3 Core / 2 GB คือโควตาตั้งต้นของทุก space ที่เพิ่งสร้าง
// ส่วน 8 Core / 8 GB คือเพดานสูงสุดที่ backend ยอมให้ตั้ง (entity.MaxCPULimitMilli / MaxRAMLimitMB)
const CPU_PRESETS = [1000, 2000, 3000, 4000, 8000];
const RAM_PRESETS = [1024, 2048, 4096, 8192];

function ManageNamespaceModal({
  namespace,
  onClose,
  onSaved,
  onDelete,
}: ManageNamespaceModalProps) {
  const [cpuMilli, setCpuMilli] = useState(namespace.cpu_limit_milli);
  const [ramMB, setRamMB] = useState(namespace.ram_limit_mb);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const isDirty = cpuMilli !== namespace.cpu_limit_milli || ramMB !== namespace.ram_limit_mb;

  // ลดโควตาให้ต่ำกว่ายอดที่ใช้อยู่ได้ (backend ยอม เป็นพฤติกรรมเดียวกับ ResourceQuota ของ k8s)
  // แต่ service เดิมจะยังรันต่อและ deploy เพิ่มไม่ได้ — ต้องบอกก่อนกดบันทึก
  // ไม่ใช่ปล่อยให้เจ้าของ space ไปเจอเอาตอน deploy แล้วงงว่าทำไมโดนปฏิเสธ
  const cpuBelowUsage = cpuMilli < namespace.usage.used_cpu_milli;
  const ramBelowUsage = ramMB < namespace.usage.used_ram_mb;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isDirty) return;

    setIsSubmitting(true);
    try {
      const updated = await adminNamespaceApi.setQuota(namespace.id, {
        cpu_limit_milli: cpuMilli,
        ram_limit_mb: ramMB,
      });
      onSaved(updated);
      onClose();
    } catch (err) {
      console.error("Failed to set quota:", err);
      notify.error("ปรับโควตาไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsSubmitting(false);
    }
  };

  const labelClass = "mb-1.5 block text-sm font-bold uppercase tracking-wider text-[#BB6653]";
  const belowUsageWarning =
    "ต่ำกว่ายอดที่ใช้อยู่ — บริการเดิมยังรันต่อ แต่จะ deploy เพิ่มไม่ได้จนกว่าจะลบของเก่าออก";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 font-mono backdrop-blur-sm">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-3xl bg-[#FFF8E8] shadow-2xl">

        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-black/5 bg-[#FFF8E8] px-6 py-5">
          <div className="min-w-0">
            <h2 className="truncate text-xl font-bold text-[#211a14]">{namespace.name}</h2>
            <p className="mt-0.5 text-sm text-[#211a14]/50">
              สร้างเมื่อ {formatDate(namespace.created_at)} · สมาชิก {namespace.member_count} คน ·
              บริการ {namespace.usage.service_count} ตัว
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-xl p-2 text-[#211a14]/50 transition-colors hover:bg-black/5 disabled:opacity-50"
          >
            <X size={20} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6">

          {/* ---------- โควตา ---------- */}
          <div className="flex flex-col gap-6 rounded-2xl border border-black/5 bg-white p-5">
            <QuotaSlider
              label="CPU Limit"
              icon={Cpu}
              value={cpuMilli}
              onChange={setCpuMilli}
              bounds={QUOTA_BOUNDS.cpu}
              display={`${cores(cpuMilli)} Core`}
              rawDisplay={`${cpuMilli}m`}
              usedDisplay={`ใช้อยู่ ${cores(namespace.usage.used_cpu_milli)} Core`}
              maxDisplay={`สูงสุด ${cores(QUOTA_BOUNDS.cpu.max)} Core`}
              presets={CPU_PRESETS.map((v) => ({ value: v, label: `${cores(v)} Core` }))}
              warning={cpuBelowUsage ? belowUsageWarning : null}
              disabled={isSubmitting}
            />

            <div className="h-px bg-black/5" />

            <QuotaSlider
              label="RAM Limit"
              icon={Layers}
              value={ramMB}
              onChange={setRamMB}
              bounds={QUOTA_BOUNDS.ram}
              display={`${gigabytes(ramMB)} GB`}
              rawDisplay={`${ramMB} MB`}
              usedDisplay={`ใช้อยู่ ${gigabytes(namespace.usage.used_ram_mb)} GB`}
              maxDisplay={`สูงสุด ${gigabytes(QUOTA_BOUNDS.ram.max)} GB`}
              presets={RAM_PRESETS.map((v) => ({ value: v, label: `${gigabytes(v)} GB` }))}
              warning={ramBelowUsage ? belowUsageWarning : null}
              disabled={isSubmitting}
            />
          </div>

          {/* ---------- สมาชิก ---------- */}
          <div className="mt-6">
            <span className={labelClass}>สมาชิกในเนมสเปซ</span>
            {namespace.members.length === 0 ? (
              <div className="rounded-xl border border-black/10 bg-black/[0.03] px-4 py-3 text-base text-[#211a14]/50">
                ไม่เหลือสมาชิกใน space นี้แล้ว — โควตาที่ตั้งไว้ยังถูกจองอยู่ ควรพิจารณาลบทิ้ง
              </div>
            ) : (
              <ul className="divide-y divide-black/5 overflow-hidden rounded-xl border border-black/10 bg-white">
                {namespace.members.map((member) => (
                  <li key={member.id} className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="min-w-0">
                      <div className="truncate text-base font-semibold text-[#211a14]">
                        {member.real_name}
                      </div>
                      <div className="text-sm text-[#211a14]/50">{member.student_id}</div>
                    </div>
                    {member.is_contributor && (
                      <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-[#FFF8E8] px-2.5 py-1 text-xs font-bold text-[#BB6653]">
                        <Crown size={12} /> เจ้าของ
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="mt-8 flex flex-col-reverse gap-3 border-t border-black/5 pt-6 sm:flex-row sm:items-center sm:justify-between">
            <button
              type="button"
              onClick={onDelete}
              disabled={isSubmitting}
              className="inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-base font-bold text-red-500 transition-colors hover:bg-red-50 disabled:opacity-50"
            >
              <Trash2 size={18} />
              ลบเนมสเปซนี้
            </button>

            <div className="flex items-center justify-end gap-3">
              <button
                type="button"
                onClick={onClose}
                disabled={isSubmitting}
                className="rounded-xl px-5 py-2.5 text-base font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isSubmitting || !isDirty}
                className="inline-flex min-w-[140px] items-center justify-center rounded-xl bg-green-600 px-5 py-2.5 text-base font-bold text-white transition-colors hover:bg-green-700 disabled:opacity-50"
              >
                {isSubmitting ? <Loader2 size={18} className="animate-spin" /> : "บันทึกโควตา"}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}

// ==========================================
// ตัวปรับโควตา 1 ด้าน (CPU หรือ RAM)
// ==========================================
interface QuotaSliderProps {
  label: string;
  icon: typeof Cpu;
  value: number;
  onChange: (value: number) => void;
  bounds: { readonly min: number; readonly max: number; readonly step: number };
  display: string;
  rawDisplay: string;
  usedDisplay: string;
  maxDisplay: string;
  presets: { value: number; label: string }[];
  warning: string | null;
  disabled: boolean;
}

function QuotaSlider({
  label,
  icon: Icon,
  value,
  onChange,
  bounds,
  display,
  rawDisplay,
  usedDisplay,
  maxDisplay,
  presets,
  warning,
  disabled,
}: QuotaSliderProps) {
  // บีบค่าเข้ากรอบทุกครั้งที่แก้ — ช่องตัวเลขพิมพ์อะไรลงไปก็ได้ ถ้าปล่อยให้เลยกรอบ
  // backend จะตอบ 400 กลับมาแทนที่จะบันทึก (binding min/max ใน dto.SetQuotaRequest)
  const clamp = (next: number) =>
    Number.isFinite(next) ? Math.min(bounds.max, Math.max(bounds.min, next)) : bounds.min;

  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-2">
        <span className="flex items-center gap-1.5 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
          <Icon size={16} />
          {label}
        </span>
        <div className="text-right">
          <span className="text-2xl font-bold text-[#211a14]">{display}</span>
          <span className="ml-2 text-sm text-[#211a14]/40">{rawDisplay}</span>
        </div>
      </div>

      <input
        type="range"
        min={bounds.min}
        max={bounds.max}
        step={bounds.step}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(clamp(Number(e.target.value)))}
        className="mt-3 h-2 w-full cursor-pointer appearance-none rounded-full bg-black/10 accent-[#BB6653] disabled:opacity-50"
      />
      <div className="mt-1 flex justify-between text-xs text-[#211a14]/40">
        <span>{usedDisplay}</span>
        <span>{maxDisplay}</span>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {presets.map((preset) => (
          <button
            key={preset.value}
            type="button"
            disabled={disabled}
            onClick={() => onChange(preset.value)}
            className={cn(
              "rounded-lg px-3 py-1.5 text-sm font-bold transition-colors disabled:opacity-50",
              value === preset.value
                ? "bg-[#BB6653] text-white"
                : "bg-[#FFF8E8] text-[#211a14]/60 hover:bg-[#F08B51]/20",
            )}
          >
            {preset.label}
          </button>
        ))}
        <input
          type="number"
          min={bounds.min}
          max={bounds.max}
          step={bounds.step}
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(clamp(Number(e.target.value)))}
          className="ml-auto w-24 rounded-lg border border-black/10 bg-white px-3 py-1.5 text-right text-sm text-[#211a14] outline-none focus:border-[#BB6653] focus:ring-1 focus:ring-[#BB6653] disabled:opacity-50"
        />
      </div>

      {warning && (
        <p className="mt-2 rounded-lg bg-orange-50 px-3 py-2 text-sm text-orange-700">{warning}</p>
      )}
    </div>
  );
}
