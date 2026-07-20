import { useState } from "react";
import type { LucideIcon } from "lucide-react";
import { Package, User, Users } from "lucide-react";

import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/authStore";

type VmType = "solo" | "group";

export default function UserDashboard() {
  const user = useAuthStore((state) => state.user);
  const hasVm = Boolean(user?.namespace_id);

  if (hasVm) {
    return <ActiveDashboard userName={user?.nick_name || user?.real_name || "User"} />;
  }

  return <NoVmState />;
}

function NoVmState() {
  const [selected, setSelected] = useState<VmType | null>(null);

  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-4 py-10 text-center">
      <h1 className="text-5xl font-bold text-[#211a14]">Welcome to Caesar Cluster</h1>
      <p className="max-w-2xl text-lg text-[#211a14]/60">
        You don't have any virtual machines yet. Create your first VM to get a
        namespace and start computing.
      </p>

      <div className="mt-8 w-full max-w-3xl rounded-3xl border border-black/5 bg-[#FFFDF6] p-12">
        <div className="flex flex-col items-center gap-3 text-center">
          <div className="flex size-20 items-center justify-center rounded-2xl bg-[#FBDFDA] text-[#BB6653]">
            <Package size={34} />
          </div>
          <h2 className="text-2xl font-semibold text-[#211a14]">Create your first VM</h2>
          <p className="max-w-lg text-base text-[#211a14]/60">
            Choose whether this machine is just for you, or shared with a
            group. This sets up your workspace.
          </p>
        </div>

        <div className="mt-10 grid gap-5 sm:grid-cols-2">
          <VmOptionCard
            icon={User}
            iconBg="bg-[#FBDFDA]"
            iconColor="text-[#BB6653]"
            title="Personal VM"
            description="Private machine only you can access."
            selected={selected === "solo"}
            onClick={() => setSelected("solo")}
          />
          <VmOptionCard
            icon={Users}
            iconBg="bg-[#DCEEDB]"
            iconColor="text-[#5A8F5A]"
            title="Group VM"
            description="Shared machine your team can access."
            selected={selected === "group"}
            onClick={() => setSelected("group")}
          />
        </div>
      </div>
    </div>
  );
}

interface VmOptionCardProps {
  icon: LucideIcon;
  iconBg: string;
  iconColor: string;
  title: string;
  description: string;
  selected: boolean;
  onClick: () => void;
}

function VmOptionCard({
  icon: Icon,
  iconBg,
  iconColor,
  title,
  description,
  selected,
  onClick,
}: VmOptionCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex flex-col items-center gap-2.5 rounded-2xl border px-8 py-10 text-center transition-colors",
        selected
          ? "border-[#BB6653] bg-[#FFF8E8]"
          : "border-black/10 hover:border-black/20 hover:bg-black/[0.02]"
      )}
    >
      <div className={cn("flex size-14 items-center justify-center rounded-xl", iconBg, iconColor)}>
        <Icon size={26} />
      </div>
      <p className="text-lg font-semibold text-[#211a14]">{title}</p>
      <p className="text-base text-[#211a14]/60">{description}</p>
    </button>
  );
}

interface StatCard {
  label: string;
  value: string;
  unit: string;
  percent: number;
}

const stats: StatCard[] = [
  { label: "CPU Usage", value: "3.6", unit: "8 cores", percent: 45 },
  { label: "Memory", value: "9.9", unit: "16 GB", percent: 62 },
  { label: "Storage", value: "60", unit: "200 GB", percent: 30 },
];

interface NotificationItem {
  id: number;
  dotColor: string;
  message: string;
  time: string;
}

const notifications: NotificationItem[] = [
  { id: 1, dotColor: "bg-red-500", message: "CPU usage on kali-lab-01 exceeded 95%", time: "2 mins ago" },
  { id: 2, dotColor: "bg-orange-500", message: "Memory on ubuntu-web reached 80% of quota", time: "10 mins ago" },
  { id: 3, dotColor: "bg-blue-500", message: "kali-lab-01 successfully restarted", time: "1 hour ago" },
];

function statusColor(percent: number) {
  if (percent >= 80) return { text: "text-red-600", bar: "bg-red-500" };
  if (percent >= 50) return { text: "text-orange-600", bar: "bg-orange-500" };
  return { text: "text-green-600", bar: "bg-green-500" };
}

function ActiveDashboard({ userName }: { userName: string }) {
  return (
    <div className="flex flex-col gap-8">
      <h1 className="text-4xl font-bold text-[#211a14]">Welcome back, {userName}</h1>

      <div className="grid gap-6 sm:grid-cols-3">
        {stats.map((stat) => {
          const color = statusColor(stat.percent);
          return (
            <div key={stat.label} className="rounded-2xl bg-[#FFFDF6] p-6">
              <div className="flex items-center justify-between">
                <p className="text-xs font-semibold tracking-wide text-[#211a14]/50 uppercase">
                  {stat.label}
                </p>
                <p className={cn("text-sm font-semibold", color.text)}>{stat.percent}%</p>
              </div>
              <p className="mt-3 text-3xl font-bold text-[#211a14]">
                {stat.value}{" "}
                <span className="text-lg font-medium text-[#211a14]/40">/ {stat.unit}</span>
              </p>
              <div className="mt-4 h-2 w-full overflow-hidden rounded-full bg-black/10">
                <div
                  className={cn("h-full rounded-full", color.bar)}
                  style={{ width: `${stat.percent}%` }}
                />
              </div>
            </div>
          );
        })}
      </div>

      <div className="w-full max-w-3xl rounded-2xl bg-[#FFFDF6] p-6">
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold tracking-wide text-[#BB6653] uppercase">
            Notifications
          </p>
          <button
            type="button"
            className="text-sm font-medium text-[#BB6653] hover:underline"
          >
            View all →
          </button>
        </div>

        <div className="mt-4 flex flex-col gap-3">
          {notifications.map((item) => (
            <div key={item.id} className="flex items-start gap-3 rounded-xl bg-[#FFF8E8] p-4">
              <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", item.dotColor)} />
              <div>
                <p className="text-sm font-medium text-[#211a14]">{item.message}</p>
                <p className="text-xs text-[#211a14]/50">{item.time}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
