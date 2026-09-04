import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  AlertTriangle, BellOff, CheckCheck, Info, Loader2,
  RefreshCw, Terminal, Trash2, X, XCircle,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { alertApi, type AlertSeverity, type UserAlert } from "@/api/alerts";
import { getApiErrorMessage } from "@/api/authApi";
import { useAlertStore } from "@/store/alertStore";
import { PATHS } from "@/config/routes";

type Filter = "all" | "unread" | "critical";

// ดึงมาให้ครบเท่าที่ backend เก็บไว้ต่อคน (maxAlertsPerUser ใน services/alert_manager.go)
// ถ้าดึงน้อยกว่านั้น ตัวเลข "N total / N unread" บนหัวหน้าจะนับได้แค่หน้าที่โหลดมา
// แล้วรายงานตัวเลขที่ต่ำกว่าความจริงโดยไม่มีอะไรบอกผู้ใช้ว่ายังมีที่เหลืออยู่
const ALERT_PAGE_LIMIT = 200;

// หน้าตาของแต่ละระดับความรุนแรง — ใช้โทนเดียวกับการ์ด service ในหน้า My Services
const SEVERITY_STYLE: Record<AlertSeverity, {
  label: string; icon: typeof XCircle; chip: string; iconWrap: string; bar: string;
}> = {
  critical: {
    label: "Error",
    icon: XCircle,
    chip: "bg-red-50 text-red-600 border-red-100",
    iconWrap: "bg-red-50 text-red-500",
    bar: "bg-red-400",
  },
  warning: {
    label: "Warning",
    icon: AlertTriangle,
    chip: "bg-amber-50 text-amber-700 border-amber-100",
    iconWrap: "bg-amber-50 text-amber-500",
    bar: "bg-amber-400",
  },
  info: {
    label: "Info",
    icon: Info,
    chip: "bg-sky-50 text-sky-700 border-sky-100",
    iconWrap: "bg-sky-50 text-sky-500",
    bar: "bg-sky-400",
  },
};

// แสดงเวลาแบบ "เมื่อสักครู่ / 5 นาทีที่แล้ว" เพราะจากหน้านี้ผู้ใช้อยากรู้ว่าเพิ่งเกิดหรือนานแล้ว
// ไม่ใช่เวลาเป๊ะๆ (เวลาเต็มอยู่ใน title ให้ hover ดู)
function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diffSec = Math.round((Date.now() - then) / 1000);
  if (diffSec < 60) return "เมื่อสักครู่";
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)} นาทีที่แล้ว`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} ชั่วโมงที่แล้ว`;
  return `${Math.floor(diffSec / 86400)} วันที่แล้ว`;
}

export default function Alertuser() {
  const navigate = useNavigate();
  const setUnread = useAlertStore((s) => s.setUnread);
  const decreaseUnread = useAlertStore((s) => s.decrease);

  const [alerts, setAlerts] = useState<UserAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [busyId, setBusyId] = useState<number | null>(null);

  const load = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true);
    setError(null);
    try {
      const list = await alertApi.list({ limit: ALERT_PAGE_LIMIT });
      setAlerts(list);
      // นับจากข้อมูลชุดที่เพิ่งได้มา แทนที่จะยิงถาม unread-count อีกรอบ — ประหยัดหนึ่ง request
      // และตัวเลขบน Sidebar ตรงกับรายการที่ผู้ใช้เห็นอยู่ตรงหน้าเสมอ (ดู ALERT_PAGE_LIMIT)
      setUnread(list.filter((a) => !a.is_read).length);
    } catch (err) {
      setError(getApiErrorMessage(err, "โหลดแจ้งเตือนไม่สำเร็จ"));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [setUnread]);

  useEffect(() => {
    void load();
  }, [load]);

  const unreadCount = useMemo(() => alerts.filter((a) => !a.is_read).length, [alerts]);
  const criticalCount = useMemo(
    () => alerts.filter((a) => a.severity === "critical").length,
    [alerts],
  );

  const visible = useMemo(() => {
    if (filter === "unread") return alerts.filter((a) => !a.is_read);
    if (filter === "critical") return alerts.filter((a) => a.severity === "critical");
    return alerts;
  }, [alerts, filter]);

  const handleMarkRead = async (alert: UserAlert) => {
    if (alert.is_read) return;
    // อัปเดตหน้าจอก่อนแล้วค่อยยิง request (optimistic) — ถ้าพลาดค่อยดึงของจริงกลับมาทับ
    setAlerts((prev) => prev.map((a) => (a.id === alert.id ? { ...a, is_read: true } : a)));
    decreaseUnread(1);
    try {
      await alertApi.markRead([alert.id]);
    } catch {
      void load();
    }
  };

  const handleMarkAllRead = async () => {
    if (unreadCount === 0) return;
    setAlerts((prev) => prev.map((a) => ({ ...a, is_read: true })));
    setUnread(0);
    try {
      await alertApi.markAllRead();
    } catch (err) {
      setError(getApiErrorMessage(err, "อัปเดตไม่สำเร็จ"));
      void load();
    }
  };

  const handleDelete = async (alert: UserAlert) => {
    setBusyId(alert.id);
    try {
      await alertApi.remove(alert.id);
      setAlerts((prev) => prev.filter((a) => a.id !== alert.id));
      if (!alert.is_read) decreaseUnread(1);
    } catch (err) {
      setError(getApiErrorMessage(err, "ลบไม่สำเร็จ"));
    } finally {
      setBusyId(null);
    }
  };

  const handleClearRead = async () => {
    const readCount = alerts.length - unreadCount;
    if (readCount === 0) return;
    try {
      await alertApi.clearRead();
      setAlerts((prev) => prev.filter((a) => !a.is_read));
    } catch (err) {
      setError(getApiErrorMessage(err, "ล้างไม่สำเร็จ"));
    }
  };

  return (
    <div className="flex flex-col gap-6 text-left font-mono animate-in fade-in duration-200">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-4xl font-bold text-[#211a14]">Alerts</h1>
          <p className="mt-1 text-sm text-[#211a14]/50">
            {loading
              ? "Loading..."
              : `${alerts.length} total · ${unreadCount} unread · ${criticalCount} error`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void load(true)}
            disabled={refreshing}
            className="inline-flex items-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-xs font-bold text-[#211a14]/60 transition-colors hover:border-[#BB6653]/30 hover:bg-[#FBDFDA]/40 hover:text-[#BB6653] disabled:opacity-60"
          >
            <RefreshCw size={13} className={cn(refreshing && "animate-spin")} /> Refresh
          </button>
          <button
            type="button"
            onClick={handleMarkAllRead}
            disabled={unreadCount === 0}
            className="inline-flex items-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-xs font-bold text-[#211a14]/60 transition-colors hover:border-[#BB6653]/30 hover:bg-[#FBDFDA]/40 hover:text-[#BB6653] disabled:opacity-40 disabled:hover:border-black/10 disabled:hover:bg-transparent disabled:hover:text-[#211a14]/60"
          >
            <CheckCheck size={13} /> Mark all read
          </button>
          <button
            type="button"
            onClick={handleClearRead}
            disabled={alerts.length - unreadCount === 0}
            className="inline-flex items-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-xs font-bold text-[#211a14]/60 transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600 disabled:opacity-40 disabled:hover:border-black/10 disabled:hover:bg-transparent disabled:hover:text-[#211a14]/60"
          >
            <Trash2 size={13} /> Clear read
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {([
          ["all", `All (${alerts.length})`],
          ["unread", `Unread (${unreadCount})`],
          ["critical", `Errors (${criticalCount})`],
        ] as const).map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setFilter(key)}
            className={cn(
              "rounded-full px-4 py-1.5 text-xs font-bold transition-colors",
              filter === key
                ? "bg-[#BB6653] text-white"
                : "border border-black/10 text-[#211a14]/55 hover:bg-black/[0.03]",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {error && (
        <div className="max-w-3xl rounded-xl border border-red-100 bg-red-50 p-4 text-sm text-red-600">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex flex-col gap-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-24 animate-pulse rounded-2xl border border-black/5 bg-[#FFFDF6]" />
          ))}
        </div>
      ) : visible.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-black/5 bg-[#FFFDF6] px-6 py-20 text-center">
          <div className="flex size-14 items-center justify-center rounded-2xl bg-[#FBDFDA] text-[#BB6653]">
            <BellOff size={24} />
          </div>
          <p className="font-semibold text-[#211a14]">
            {filter === "all" ? "ยังไม่มีแจ้งเตือน" : "ไม่มีรายการในตัวกรองนี้"}
          </p>
          <p className="max-w-md text-xs leading-relaxed text-[#211a14]/45">
            ระบบจะอ่าน log ของ service ที่กำลังรันอยู่เป็นรอบๆ ถ้าเจอบรรทัดที่เป็น error
            จะสรุปมาแจ้งที่หน้านี้ให้เอง — ไม่มีอะไรขึ้นแปลว่า service ทำงานปกติดี
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {visible.map((alert) => {
            const style = SEVERITY_STYLE[alert.severity] ?? SEVERITY_STYLE.info;
            const Icon = style.icon;
            const canOpenLogs = alert.source_type === "service_log" && alert.service_id !== null;

            return (
              <div
                key={alert.id}
                className={cn(
                  "relative flex gap-4 overflow-hidden rounded-2xl border bg-[#FFFDF6] p-5 shadow-sm transition-colors",
                  alert.is_read ? "border-black/5" : "border-[#BB6653]/25",
                )}
              >
                {/* แถบสีซ้ายมือโชว์เฉพาะรายการที่ยังไม่อ่าน — เป็นตัวบอก "อันไหนใหม่" ที่กวาดตาเห็นเร็วที่สุด */}
                {!alert.is_read && (
                  <span className={cn("absolute inset-y-0 left-0 w-1", style.bar)} />
                )}

                <div className={cn("flex size-11 shrink-0 items-center justify-center rounded-xl", style.iconWrap)}>
                  <Icon size={20} />
                </div>

                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={cn("rounded-full border px-2.5 py-0.5 text-[11px] font-bold", style.chip)}>
                      {style.label}
                    </span>
                    {alert.count > 1 && (
                      <span
                        className="rounded-full bg-[#211a14]/5 px-2.5 py-0.5 text-[11px] font-bold text-[#211a14]/55"
                        title="จำนวนครั้งที่ error เรื่องเดียวกันนี้เกิดขึ้น"
                      >
                        ×{alert.count}
                      </span>
                    )}
                    {!alert.is_read && (
                      <span className="rounded-full bg-[#BB6653] px-2.5 py-0.5 text-[11px] font-bold text-white">
                        New
                      </span>
                    )}
                    <span
                      className="text-[11px] text-[#211a14]/40"
                      title={new Date(alert.last_seen_at).toLocaleString()}
                    >
                      {relativeTime(alert.last_seen_at)}
                    </span>
                  </div>

                  <p className="truncate font-semibold text-[#211a14]">{alert.title}</p>

                  {/* ข้อความ log ดิบ — โชว์ในกล่องสีเข้มแบบเดียวกับหน้า Logs เพื่อให้เห็นทันทีว่า
                      นี่คือสิ่งที่ container พิมพ์ออกมาจริงๆ ไม่ใช่ข้อความที่ระบบเขียนเอง */}
                  <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-lg bg-[#1a1714] px-3 py-2 text-[11px] leading-relaxed text-[#e6ddd1]">
                    {alert.message}
                  </pre>

                  <div className="flex flex-wrap items-center gap-2 pt-1">
                    {alert.source_name && (
                      <span className="text-[11px] text-[#211a14]/45">
                        service: <span className="font-semibold text-[#211a14]/70">{alert.source_name}</span>
                      </span>
                    )}
                    {canOpenLogs && (
                      <button
                        type="button"
                        onClick={() => navigate(`/${PATHS.serviceLogs}/${alert.service_id}`)}
                        className="inline-flex items-center gap-1.5 rounded-lg border border-black/10 px-2.5 py-1 text-[11px] font-bold text-[#211a14]/60 transition-colors hover:border-[#BB6653]/30 hover:bg-[#FBDFDA]/40 hover:text-[#BB6653]"
                      >
                        <Terminal size={12} /> ดู log
                      </button>
                    )}
                  </div>
                </div>

                <div className="flex shrink-0 flex-col items-end gap-1.5">
                  {!alert.is_read && (
                    <button
                      type="button"
                      onClick={() => void handleMarkRead(alert)}
                      title="ทำเครื่องหมายว่าอ่านแล้ว"
                      className="rounded-lg p-1.5 text-[#211a14]/35 transition-colors hover:bg-black/[0.04] hover:text-[#BB6653]"
                    >
                      <CheckCheck size={16} />
                    </button>
                  )}
                  <button
                    type="button"
                    disabled={busyId === alert.id}
                    onClick={() => void handleDelete(alert)}
                    title="ลบแจ้งเตือนนี้"
                    className="rounded-lg p-1.5 text-[#211a14]/35 transition-colors hover:bg-red-50 hover:text-red-500 disabled:opacity-50"
                  >
                    {busyId === alert.id ? <Loader2 size={16} className="animate-spin" /> : <X size={16} />}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
