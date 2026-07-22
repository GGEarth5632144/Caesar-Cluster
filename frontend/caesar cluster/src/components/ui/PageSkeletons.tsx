import { Skeleton } from "@/components/ui/skeleton";

/**
 * ชุด Skeleton สำเร็จรูปสำหรับแต่ละ layout ที่ใช้ซ้ำในหลายหน้า
 * ทุกตัว mirror โครงจริงของหน้านั้นๆ เพื่อลดการกระตุกตอนข้อมูลมาถึง
 */

// ---------- ตาราง: แถว skeleton วางใน <tbody> ที่มีอยู่เดิม (header ยังโชว์) ----------
export function TableRowsSkeleton({
  rows = 6,
  cols = 5,
}: {
  rows?: number;
  cols?: number;
}) {
  return (
    <>
      {Array.from({ length: rows }).map((_, r) => (
        <tr key={r} className="border-b border-black/5 last:border-0">
          {Array.from({ length: cols }).map((_, c) => (
            <td key={c} className="px-3 py-4">
              {c === 0 ? (
                <div className="flex items-center gap-3">
                  <Skeleton className="size-10 shrink-0 rounded-full" />
                  <div className="flex flex-1 flex-col gap-1.5">
                    <Skeleton className="h-3.5 w-3/4" />
                    <Skeleton className="h-2.5 w-1/2" />
                  </div>
                </div>
              ) : (
                <Skeleton className="h-3.5 w-[70%]" />
              )}
            </td>
          ))}
        </tr>
      ))}
    </>
  );
}

// ---------- Dashboard: การ์ดสถิติ 3 ใบ + การ์ดแจ้งเตือน (GeneralDashboard) ----------
export function DashboardStatsSkeleton() {
  return (
    <div className="flex flex-col gap-10 font-mono">
      <Skeleton className="h-9 w-72 rounded-lg" />

      <div className="grid gap-6 sm:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="flex flex-col gap-6 rounded-2xl border border-black/5 bg-[#FFFDF6] p-6 shadow-sm"
          >
            <div className="flex items-center justify-between">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-3 w-10" />
            </div>
            <Skeleton className="h-9 w-40" />
            <Skeleton className="h-2 w-full rounded-full" />
          </div>
        ))}
      </div>

      <div className="w-full max-w-3xl rounded-3xl border border-black/5 bg-[#FFFDF6] p-6 shadow-sm">
        <div className="flex items-center justify-between border-b border-black/5 pb-3">
          <Skeleton className="h-3.5 w-32" />
          <Skeleton className="h-3.5 w-16" />
        </div>
        <div className="mt-4 flex flex-col gap-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="flex items-start gap-4 rounded-xl bg-[#FFF8E8]/50 p-4"
            >
              <Skeleton className="mt-1.5 size-2 shrink-0 rounded-full" />
              <div className="flex flex-1 flex-col gap-1.5">
                <Skeleton className="h-3.5 w-[80%]" />
                <Skeleton className="h-2.5 w-20" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ---------- กริดการ์ด service (RequestQuotar / Service) ----------
export function ServiceCardsSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className="flex flex-col gap-4 rounded-2xl border border-black/5 bg-[#FFFDF6] p-6 shadow-sm"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <Skeleton className="size-11 shrink-0 rounded-xl" />
              <div className="flex flex-col gap-1.5">
                <Skeleton className="h-3.5 w-24" />
                <Skeleton className="h-2.5 w-16" />
              </div>
            </div>
            <Skeleton className="h-6 w-20 rounded-full" />
          </div>
          <div className="grid grid-cols-3 gap-2 border-t border-black/5 pt-3">
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-full" />
          </div>
          <Skeleton className="h-8 w-full rounded-xl" />
        </div>
      ))}
    </div>
  );
}

// ---------- รายการคำขอแบบการ์ดเรียงลง (RequestResources) ----------
export function RequestListSkeleton({ count = 3 }: { count?: number }) {
  return (
    <div className="flex max-w-5xl flex-col gap-6">
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className="w-full rounded-3xl border border-l-4 border-black/5 border-l-[#BB6653]/20 bg-[#FFFDF6] p-6 shadow-sm sm:p-8"
        >
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-start gap-4">
              <Skeleton className="size-12 shrink-0 rounded-2xl" />
              <div className="flex flex-col gap-2">
                <Skeleton className="h-5 w-56" />
                <Skeleton className="h-2.5 w-40" />
              </div>
            </div>
            <Skeleton className="h-7 w-40 rounded-full" />
          </div>
          <div className="mt-8 border-b border-black/5 pb-6">
            <Skeleton className="h-3 w-full max-w-xl" />
          </div>
          <div className="mt-6 flex gap-6">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-28" />
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------- กริดการ์ดเทมเพลต/preset (WorkspaceOnboarding, CreateServiceModal) ----------
export function TemplateGridSkeleton({
  count = 4,
  compact = false,
}: {
  count?: number;
  compact?: boolean;
}) {
  return (
    <div className={compact ? "grid grid-cols-2 gap-2.5" : "grid gap-4 sm:grid-cols-2"}>
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className={
            compact
              ? "flex flex-col gap-2 rounded-xl border border-black/10 bg-white p-3"
              : "flex flex-col gap-3 rounded-2xl border border-black/10 bg-[#FFFDF6] p-5"
          }
        >
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-4 w-3/4" />
          {!compact && <Skeleton className="h-2.5 w-full" />}
          <div className="mt-1 flex gap-2 border-t border-black/5 pt-2.5">
            <Skeleton className="h-3 w-14" />
            <Skeleton className="h-3 w-14" />
            {!compact && <Skeleton className="h-3 w-14" />}
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------- ตารางแบบง่าย (list ในโมดัล เช่น รายชื่อผู้มีสิทธิ์) ----------
export function SimpleRowsSkeleton({
  rows = 6,
  cols = 4,
}: {
  rows?: number;
  cols?: number;
}) {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex items-center gap-4 border-b border-black/5 pb-3 last:border-0">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton
              key={c}
              className="h-3.5"
              style={{ width: c === cols - 1 ? "20%" : `${Math.round(80 / cols)}%` }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
