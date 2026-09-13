import { useState, useEffect } from "react";
import { Cpu, Layers, HardDrive } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { cn } from "@/lib/utils";
import { DashboardStatsSkeleton } from "@/components/ui/PageSkeletons";
import { namespaceApi, namespaceUsagePercent, type NamespaceDetail } from "@/api/namespace";
import { serviceApi, type AppService } from "@/api/services";
import { PATHS } from "@/config/routes";
import { getApiErrorMessage } from "@/api/authApi";

interface ServiceAlert {
  id: number;
  name: string;
  dotColor: string;
  message: string;
  time: string;
}

// คำที่บอกว่าบรรทัดนั้นน่าจะเป็นสาเหตุจริง ใช้คัด log ท้ายๆ ที่ backend แนบมากับสถานะ
// ไม่ได้พยายามเข้าใจ log ของทุก image แค่หยิบบรรทัดที่มีโอกาสอธิบายปัญหามากที่สุดมาแสดง
const ERROR_HINTS =
  /(error|fatal|panic|exception|denied|refused|unable|cannot|missing|not set|no such|exit code|oom)/i;

const MAX_ALERTS = 4; // กันไม่ให้กล่องยาวจนกลบส่วนอื่นของหน้า
const MAX_ALERT_CHARS = 140; // เอาแค่พอเห็นสาเหตุ ที่เหลือกดเข้าไปดูที่หน้า log

/** หยิบบรรทัดที่น่าจะเป็นสาเหตุ ออกจาก log ที่ backend เก็บไว้ตอน container ตาย */
function errorLine(svc: AppService): string {
  const lines = svc.status_message
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  // ไล่จากบรรทัดท้ายขึ้นมา เพราะบรรทัดก่อนตายคือสาเหตุจริง ส่วนต้นๆ มักเป็น log ตอนบูตที่ไม่เกี่ยว
  // ไม่เจอบรรทัดที่เข้าเค้าก็ใช้บรรทัดแรก ซึ่งเป็นคำอธิบายที่ backend เขียนไว้ให้อยู่แล้ว
  const hit = [...lines].reverse().find((line) => ERROR_HINTS.test(line));
  const text = hit ?? lines[0] ?? svc.status_reason;
  return text.length > MAX_ALERT_CHARS ? `${text.slice(0, MAX_ALERT_CHARS)}…` : text;
}

function timeAgo(iso: string | null): string {
  if (!iso) return "";
  const minutes = Math.floor((Date.now() - new Date(iso).getTime()) / 60000);
  if (minutes < 1) return "เมื่อสักครู่";
  if (minutes < 60) return `${minutes} นาทีที่แล้ว`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ชั่วโมงที่แล้ว`;
  return `${Math.floor(hours / 24)} วันที่แล้ว`;
}

/**
 * แจ้งเตือน = service ที่มีปัญหาอยู่ ณ ตอนนี้ ไม่ใช่ประวัติเหตุการณ์ย้อนหลัง
 * ระบบไม่ได้เก็บ event log ไว้ จึงอ่านจากสถานะล่าสุดที่ ServiceHealthMonitor เขียนไว้แทน
 *
 * pending ที่กำลังเตรียม container อยู่ไม่นับว่าเป็นปัญหา เอาเฉพาะตัวที่หา node ลงไม่ได้จริงๆ
 */
function alertsFrom(services: AppService[]): ServiceAlert[] {
  const rank = (s: AppService) => (s.status === "crashloop" ? 0 : s.status === "failed" ? 1 : 2);

  return services
    .filter(
      (s) =>
        s.status === "crashloop" ||
        s.status === "failed" ||
        (s.status === "pending" && s.status_reason === "FailedScheduling"),
    )
    .sort((a, b) => rank(a) - rank(b))
    .slice(0, MAX_ALERTS)
    .map((s) => ({
      id: s.id,
      name: s.name,
      dotColor: s.status === "pending" ? "bg-orange-500" : "bg-red-500",
      message: errorLine(s),
      time: timeAgo(s.status_checked_at),
    }));
}

function statusColor(percent: number) {
  if (percent >= 80) return { text: "text-red-600", bar: "bg-red-500" };
  if (percent >= 50) return { text: "text-orange-600", bar: "bg-orange-500" };
  return { text: "text-green-600", bar: "bg-green-500" };
}

export default function GeneralDashboard({ user }: { user: any }) {
  const navigate = useNavigate();
  const [data, setData] = useState<NamespaceDetail | null>(null);
  const [alerts, setAlerts] = useState<ServiceAlert[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    // ยิงพร้อมกันไม่ให้เป็น waterfall และกลืน error ของรายการ service ไว้ตรงนี้
    // เพราะดึงไม่ได้ควรแปลว่า "ไม่มีแจ้งเตือนให้ดู" ไม่ใช่ทั้งหน้าว่างเปล่า
    Promise.all([namespaceApi.mine(), serviceApi.list().catch(() => [] as AppService[])])
      .then(([detail, services]) => {
        setData(detail);
        setAlerts(alertsFrom(services));
      })
      .catch((err) => {
        console.error(err);
        setError(getApiErrorMessage(err, "ไม่สามารถโหลดข้อมูลสถิติทรัพยากรได้"));
      })
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <DashboardStatsSkeleton />;
  }

  if (error || !data) {
    return (
      <div className="p-6 rounded-2xl bg-red-50 text-red-600 font-mono text-base max-w-xl mx-auto border border-red-100 text-center mt-10">
        {error || "ไม่พบข้อมูล namespace"}
      </div>
    );
  }

  const cpuUsedCores = data.usage.used_cpu_milli / 1000;
  const cpuLimitCores = data.cpu_limit_milli / 1000;
  const ramUsedGB = data.usage.used_ram_mb / 1024;
  const ramLimitGB = data.ram_limit_mb / 1024;

  // ดิสก์คือยอดที่ database ในกลุ่มจองไว้รวมกัน ไม่ใช่พื้นที่ที่เขียนไปจริง — ตัวเลขจึงตรงกับ
  // ขนาดที่ผู้ใช้เลือกตอนสร้างเป๊ะๆ และเป็น 0 ถ้ากลุ่มยังไม่มี database
  const storageUsedGB = data.usage.used_storage_mb / 1024;
  const storageLimitGB = data.storage_limit_mb / 1024;

  // ใช้ตัวคำนวณตัวเดียวกับหน้าแอดมิน เพื่อให้เกณฑ์สีตรงกันและได้การกันหารด้วยศูนย์ไปด้วย
  // (เพดานดิสก์เป็น 0 ได้ ต่างจาก CPU/RAM)
  const usage = namespaceUsagePercent(data);
  const cpuPercent = Math.round(usage.cpu);
  const ramPercent = Math.round(usage.ram);
  const storagePercent = Math.round(usage.storage);

  const userName = user?.real_name || user?.nick_name || "User Name";

  return (
    <div className="flex flex-col gap-10 text-left font-mono animate-in fade-in duration-200">

      <h1 className="text-5xl font-bold text-[#211a14]">Welcome back, {userName}</h1>

      <div className="grid gap-6 sm:grid-cols-3">

        <div className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-sm font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <Cpu size={16} className="text-[#BB6653]" /> CPU Usage
              </span>
              <span className={statusColor(cpuPercent).text}>{cpuPercent}%</span>
            </div>
            <p className="mt-4 text-5xl font-bold text-[#211a14]">
              {cpuUsedCores.toFixed(1)}{" "}
              <span className="text-2xl font-medium text-[#211a14]/40">/ {cpuLimitCores} cores</span>
            </p>
          </div>
          <div className="mt-6 h-2 w-full overflow-hidden rounded-full bg-black/5">
            <div
              className={cn("h-full rounded-full transition-all duration-500", statusColor(cpuPercent).bar)}
              style={{ width: `${Math.min(cpuPercent, 100)}%` }}
            />
          </div>
        </div>

        <div className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-sm font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <Layers size={16} className="text-[#BB6653]" /> Memory
              </span>
              <span className={statusColor(ramPercent).text}>{ramPercent}%</span>
            </div>
            <p className="mt-4 text-5xl font-bold text-[#211a14]">
              {ramUsedGB.toFixed(1)}{" "}
              <span className="text-2xl font-medium text-[#211a14]/40">/ {ramLimitGB} GB</span>
            </p>
          </div>
          <div className="mt-6 h-2 w-full overflow-hidden rounded-full bg-black/5">
            <div
              className={cn("h-full rounded-full transition-all duration-500", statusColor(ramPercent).bar)}
              style={{ width: `${Math.min(ramPercent, 100)}%` }}
            />
          </div>
        </div>

        <div className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-sm font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <HardDrive size={16} className="text-[#BB6653]" /> Storage
              </span>
              <span className={statusColor(storagePercent).text}>{storagePercent}%</span>
            </div>
            <p className="mt-4 text-5xl font-bold text-[#211a14]">
              {storageUsedGB.toFixed(1)}{" "}
              <span className="text-2xl font-medium text-[#211a14]/40">/ {storageLimitGB} GB</span>
            </p>
          </div>
          <div className="mt-6 h-2 w-full overflow-hidden rounded-full bg-black/5">
            <div
              className={cn("h-full rounded-full transition-all duration-500", statusColor(storagePercent).bar)}
              style={{ width: `${Math.min(storagePercent, 100)}%` }}
            />
          </div>
        </div>

      </div>

      <div className="w-full max-w-3xl mx-auto sm:mx-0 rounded-3xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm">
        <div className="flex items-center justify-between pb-2 border-b border-black/5">
          <p className="text-base font-bold tracking-wider text-[#BB6653] uppercase">
            Notifications
          </p>
          <button
            type="button"
            onClick={() => navigate(`/${PATHS.services}`)}
            className="text-sm font-bold text-[#BB6653] hover:text-[#F08B51] hover:underline transition-colors"
          >
            View all →
          </button>
        </div>

        <div className="mt-4 flex flex-col gap-3">
          {alerts.length === 0 ? (
            <p className="rounded-xl border border-black/[0.02] bg-[#FFF8E8]/50 p-4 text-base text-[#211a14]/40">
              ยังไม่มีบริการที่มีปัญหา
            </p>
          ) : (
            alerts.map((item) => (
              // กดแล้วไปหน้า log ของ service นั้นเลย เพราะบรรทัดเดียวที่ตัดมามักไม่พอให้แก้ปัญหา
              <button
                key={item.id}
                type="button"
                onClick={() => navigate(`/${PATHS.serviceLogs}/${item.id}`)}
                className="flex w-full items-start gap-4 rounded-xl border border-black/[0.02] bg-[#FFF8E8]/50 p-4 text-left transition-colors hover:bg-[#FFF8E8]"
              >
                <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", item.dotColor)} />
                <div className="min-w-0 space-y-0.5">
                  <p className="break-words text-base text-[#211a14]">
                    <span className="font-bold">{item.name}</span> — {item.message}
                  </p>
                  <p className="text-base text-[#211a14]/40">{item.time}</p>
                </div>
              </button>
            ))
          )}
        </div>
      </div>

    </div>
  );
}
