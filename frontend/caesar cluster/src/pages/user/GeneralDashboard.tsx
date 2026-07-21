import { useState, useEffect } from "react";
import { Cpu, Layers, HardDrive, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { namespaceApi, type NamespaceDetail } from "@/api/namespace";
import { getApiErrorMessage } from "@/api/authApi";

interface NotificationItem {
  id: number;
  dotColor: string;
  message: string;
  time: string;
}

// จำลองข้อมูลแจ้งเตือนตามรูปดีไซน์ — ยังไม่มี endpoint แจ้งเตือนจริงในระบบ
const MOCK_NOTIFICATIONS: NotificationItem[] = [
  { id: 1, dotColor: "bg-red-500", message: "CPU usage on kali-lab-01 exceeded 95%", time: "2 mins ago" },
  { id: 2, dotColor: "bg-orange-500", message: "Memory on ubuntu-web reached 80% of quota", time: "10 mins ago" },
  { id: 3, dotColor: "bg-blue-500", message: "kali-lab-01 successfully restarted", time: "1 hour ago" },
];

// จำลองข้อมูล Storage — backend ยังไม่มี endpoint สำหรับ storage usage
const MOCK_STORAGE_USED_GB = 60;
const MOCK_STORAGE_LIMIT_GB = 200;

function statusColor(percent: number) {
  if (percent >= 80) return { text: "text-red-600", bar: "bg-red-500" };
  if (percent >= 50) return { text: "text-orange-600", bar: "bg-orange-500" };
  return { text: "text-green-600", bar: "bg-green-500" };
}

export default function GeneralDashboard({ user }: { user: any }) {
  const [data, setData] = useState<NamespaceDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    namespaceApi
      .mine()
      .then((detail) => setData(detail))
      .catch((err) => {
        console.error(err);
        setError(getApiErrorMessage(err, "ไม่สามารถโหลดข้อมูลสถิติทรัพยากรได้"));
      })
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="flex h-full min-h-[400px] flex-col items-center justify-center gap-3 text-[#BB6653]">
        <Loader2 size={36} className="animate-spin" />
        <p className="text-sm font-medium font-mono">Loading cluster metrics...</p>
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="p-6 rounded-2xl bg-red-50 text-red-600 font-mono text-sm max-w-xl mx-auto border border-red-100 text-center mt-10">
        {error || "ไม่พบข้อมูล namespace"}
      </div>
    );
  }

  const cpuUsedCores = data.usage.used_cpu_milli / 1000;
  const cpuLimitCores = data.cpu_limit_milli / 1000;
  const cpuPercent = Math.round((cpuUsedCores / cpuLimitCores) * 100) || 0;

  const ramUsedGB = data.usage.used_ram_mb / 1024;
  const ramLimitGB = data.ram_limit_mb / 1024;
  const ramPercent = Math.round((ramUsedGB / ramLimitGB) * 100) || 0;

  const storagePercent = Math.round((MOCK_STORAGE_USED_GB / MOCK_STORAGE_LIMIT_GB) * 100) || 0;

  const userName = user?.real_name || user?.nick_name || "User Name";

  return (
    <div className="flex flex-col gap-10 text-left font-mono animate-in fade-in duration-200">

      <h1 className="text-4xl font-bold text-[#211a14]">Welcome back, {userName}</h1>

      <div className="grid gap-6 sm:grid-cols-3">

        <div className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-xs font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <Cpu size={14} className="text-[#BB6653]" /> CPU Usage
              </span>
              <span className={statusColor(cpuPercent).text}>{cpuPercent}%</span>
            </div>
            <p className="mt-4 text-4xl font-bold text-[#211a14]">
              {cpuUsedCores.toFixed(1)}{" "}
              <span className="text-xl font-medium text-[#211a14]/40">/ {cpuLimitCores} cores</span>
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
            <div className="flex items-center justify-between text-xs font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <Layers size={14} className="text-[#BB6653]" /> Memory
              </span>
              <span className={statusColor(ramPercent).text}>{ramPercent}%</span>
            </div>
            <p className="mt-4 text-4xl font-bold text-[#211a14]">
              {ramUsedGB.toFixed(1)}{" "}
              <span className="text-xl font-medium text-[#211a14]/40">/ {ramLimitGB} GB</span>
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
            <div className="flex items-center justify-between text-xs font-bold tracking-wider uppercase">
              <span className="text-[#211a14]/50 flex items-center gap-1">
                <HardDrive size={14} className="text-[#BB6653]" /> Storage
              </span>
              <span className={statusColor(storagePercent).text}>{storagePercent}%</span>
            </div>
            <p className="mt-4 text-4xl font-bold text-[#211a14]">
              {MOCK_STORAGE_USED_GB}{" "}
              <span className="text-xl font-medium text-[#211a14]/40">/ {MOCK_STORAGE_LIMIT_GB} GB</span>
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
          <p className="text-sm font-bold tracking-wider text-[#BB6653] uppercase">
            Notifications
          </p>
          <button
            type="button"
            className="text-xs font-bold text-[#BB6653] hover:text-[#F08B51] hover:underline transition-colors"
          >
            View all →
          </button>
        </div>

        <div className="mt-4 flex flex-col gap-3">
          {MOCK_NOTIFICATIONS.map((item) => (
            <div key={item.id} className="flex items-start gap-4 rounded-xl bg-[#FFF8E8]/50 p-4 border border-black/[0.02]">
              <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", item.dotColor)} />
              <div className="space-y-0.5">
                <p className="text-sm font-medium text-[#211a14]">{item.message}</p>
                <p className="text-[11px] text-[#211a14]/40">{item.time}</p>
              </div>
            </div>
          ))}
        </div>
      </div>

    </div>
  );
}
