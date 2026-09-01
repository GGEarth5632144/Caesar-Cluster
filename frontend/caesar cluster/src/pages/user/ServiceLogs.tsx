import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  ArrowLeft, Play, Pause, Search, Trash2, Download,
  Loader2, Terminal, X, ArrowDownToLine,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { serviceApi, type AppService } from "@/api/services";
import { streamServiceLogs, type LogLine } from "@/api/logs";
import { PATHS } from "@/config/routes";

// เพดานบรรทัดที่เก็บไว้ในหน่วยความจำของเบราว์เซอร์ — โหมด live tail เปิดค้างไว้เป็นชั่วโมงได้
// ถ้าไม่ตัดทิ้ง tab จะกินแรมขึ้นเรื่อยๆ ไม่มีที่สิ้นสุด (คล้าย log viewer ของ Cloud Run ที่ก็จำกัด
// จำนวนบรรทัดที่ render พร้อมกันเหมือนกัน ไม่ใช่ scroll ย้อนได้ไม่จำกัด)
const MAX_BUFFERED_LINES = 5000;

const TAIL_OPTIONS = [100, 200, 500, 1000] as const;

function formatTimestamp(iso: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("en-GB", { hour12: false }) + "." + String(d.getMilliseconds()).padStart(3, "0");
}

type ConnState = "connecting" | "live" | "idle" | "error";

export default function ServiceLogs() {
  const { serviceId } = useParams<{ serviceId: string }>();
  const navigate = useNavigate();
  const id = Number(serviceId);

  const [service, setService] = useState<AppService | null>(null);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [follow, setFollow] = useState(true);
  const [tail, setTail] = useState<number>(200);
  const [filter, setFilter] = useState("");
  const [connState, setConnState] = useState<ConnState>("connecting");
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  const paneRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const genRef = useRef(0); // กันผลลัพธ์ของ stream เก่าที่ยัง cleanup ไม่ทันมาปนกับ stream ใหม่

  // โหลดข้อมูล service (ชื่อ, image, สถานะ) มาโชว์บนหัวหน้า — เอาจาก list เพราะ backend
  // ยังไม่มี endpoint ดึงทีละตัว และจำนวน service ต่อ space มีไม่มากจนต้องแยก endpoint
  useEffect(() => {
    let cancelled = false;
    serviceApi
      .list()
      .then((all) => {
        if (!cancelled) setService(all.find((s) => s.id === id) ?? null);
      })
      .catch(() => {
        /* หน้ายังใช้งานได้แม้โหลดชื่อ service ไม่สำเร็จ แค่ไม่มี badge สถานะให้โชว์ */
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  // เปิด/ปิด stream ใหม่ทุกครั้งที่ tail หรือ follow เปลี่ยน — ตัด stream เก่าทิ้งก่อนเสมอ
  // ไม่งั้นจะมีหลาย stream ค้างคุยกับ backend พร้อมกันจาก tab เดียว
  const startStream = useCallback(() => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const gen = ++genRef.current;

    setLines([]);
    setErrorMsg(null);
    setConnState("connecting");

    streamServiceLogs(id, {
      tailLines: tail,
      follow,
      signal: controller.signal,
      onLine: (line) => {
        if (gen !== genRef.current) return; // เป็นผลจาก stream รอบก่อนที่ถูกแทนที่ไปแล้ว
        setConnState(follow ? "live" : "idle");
        setLines((prev) => {
          const next = prev.length >= MAX_BUFFERED_LINES ? prev.slice(prev.length - MAX_BUFFERED_LINES + 1) : prev;
          return [...next, line];
        });
      },
    })
      .then(() => {
        if (gen === genRef.current) setConnState("idle");
      })
      .catch((err: unknown) => {
        if (gen !== genRef.current) return;
        setConnState("error");
        setErrorMsg(err instanceof Error ? err.message : "ดึง log ไม่สำเร็จ");
      });
  }, [id, tail, follow]);

  useEffect(() => {
    if (!Number.isFinite(id)) return;
    startStream();
    return () => abortRef.current?.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, tail, follow]);

  // auto-scroll ลงล่างสุดทุกครั้งที่มีบรรทัดใหม่ เว้นแต่ผู้ใช้เลื่อนขึ้นไปดูของเก่าเอง
  useEffect(() => {
    if (autoScroll && paneRef.current) {
      paneRef.current.scrollTop = paneRef.current.scrollHeight;
    }
  }, [lines, autoScroll]);

  const handlePaneScroll = () => {
    const el = paneRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setAutoScroll(atBottom);
  };

  const jumpToLatest = () => {
    setAutoScroll(true);
    if (paneRef.current) paneRef.current.scrollTop = paneRef.current.scrollHeight;
  };

  const filteredLines = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return lines;
    return lines.filter((l) => l.text.toLowerCase().includes(q));
  }, [lines, filter]);

  const handleDownload = () => {
    const content = lines.map((l) => (l.timestamp ? `${l.timestamp} ${l.text}` : l.text)).join("\n");
    const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${service?.name ?? "service"}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const statusDot = {
    connecting: { cls: "bg-yellow-400 animate-pulse", label: "กำลังเชื่อมต่อ" },
    live: { cls: "bg-green-500 animate-pulse", label: "Live" },
    idle: { cls: "bg-white/30", label: "หยุดแล้ว" },
    error: { cls: "bg-red-500", label: "เชื่อมต่อไม่ได้" },
  }[connState];

  return (
    <div className="flex h-full flex-col gap-4">
      {/* ─── Header ─── */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <button
            type="button"
            onClick={() => navigate(`/${PATHS.services}`)}
            className="mt-0.5 rounded-xl p-2 text-[#211a14]/50 transition-colors hover:bg-black/5"
          >
            <ArrowLeft size={18} />
          </button>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Terminal size={16} className="text-[#BB6653] shrink-0" />
              <h1 className="text-lg font-bold text-[#211a14] truncate">
                {service?.name ?? `service #${id}`}
              </h1>
            </div>
            <p className="text-xs text-[#211a14]/45 truncate">{service?.image ?? "กำลังโหลดรายละเอียด..."}</p>
          </div>
        </div>
      </div>

      {/* ─── Toolbar ─── */}
      <div className="flex flex-wrap items-center gap-2 rounded-xl border border-black/5 bg-[#FFFDF6] p-2.5">
        <button
          type="button"
          onClick={() => setFollow((f) => !f)}
          className={cn(
            "inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-bold transition-colors",
            follow ? "bg-[#BB6653] text-white" : "border border-black/10 text-[#211a14]/70 hover:bg-black/[0.03]",
          )}
          title={follow ? "หยุดไหลสด" : "เริ่มไหลสด (live tail)"}
        >
          {follow ? <Pause size={13} /> : <Play size={13} />}
          {follow ? "Streaming" : "Paused"}
        </button>

        <select
          value={tail}
          onChange={(e) => setTail(Number(e.target.value))}
          className="rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-xs font-medium text-[#211a14] outline-none"
          title="จำนวนบรรทัดล่าสุดที่ดึงตอนเปิด"
        >
          {TAIL_OPTIONS.map((n) => (
            <option key={n} value={n}>
              last {n}
            </option>
          ))}
        </select>

        <div className="flex min-w-[160px] flex-1 items-center gap-1.5 rounded-lg border border-black/10 bg-white px-2.5 py-1.5">
          <Search size={13} className="text-[#211a14]/30 shrink-0" />
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="filter log lines..."
            className="w-full bg-transparent text-xs text-[#211a14] placeholder:text-[#211a14]/30 outline-none"
          />
          {filter !== "" && (
            <button type="button" onClick={() => setFilter("")} className="text-[#211a14]/30 hover:text-[#211a14]/60">
              <X size={13} />
            </button>
          )}
        </div>

        <button
          type="button"
          onClick={() => setLines([])}
          className="inline-flex items-center gap-1.5 rounded-lg border border-black/10 px-3 py-1.5 text-xs font-bold text-[#211a14]/60 transition-colors hover:bg-black/[0.03]"
          title="ล้างบรรทัดที่แสดงอยู่ (ไม่หยุด stream)"
        >
          <Trash2 size={13} /> Clear
        </button>

        <button
          type="button"
          onClick={handleDownload}
          disabled={lines.length === 0}
          className="inline-flex items-center gap-1.5 rounded-lg border border-black/10 px-3 py-1.5 text-xs font-bold text-[#211a14]/60 transition-colors hover:bg-black/[0.03] disabled:opacity-40"
          title="ดาวน์โหลด log ที่เห็นอยู่ตอนนี้เป็นไฟล์ .txt"
        >
          <Download size={13} /> Download
        </button>

        <div className="flex items-center gap-1.5 pl-1 text-[11px] font-semibold text-[#211a14]/50">
          <span className={cn("size-2 rounded-full", statusDot.cls)} />
          {statusDot.label}
        </div>
      </div>

      {errorMsg && (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-red-100 bg-red-50 px-4 py-2.5 text-sm text-red-600">
          <span>{errorMsg}</span>
          <button
            type="button"
            onClick={startStream}
            className="shrink-0 rounded-lg bg-red-500 px-3 py-1 text-xs font-bold text-white hover:bg-red-600"
          >
            ลองใหม่
          </button>
        </div>
      )}

      {/* ─── Log pane (terminal look — ธีมเดียวกับ ThinkingBubble ใน AIReviewPage) ─── */}
      <div className="relative flex-1 min-h-0 overflow-hidden rounded-2xl border border-white/8 bg-[#1a1714]">
        <div className="flex items-center gap-2 border-b border-white/8 px-4 py-2.5">
          <span className="size-2.5 rounded-full bg-red-500/70" />
          <span className="size-2.5 rounded-full bg-yellow-500/70" />
          <span className="size-2.5 rounded-full bg-green-500/70" />
          <span className="ml-2 font-mono text-[11px] text-white/30">
            {filter ? `${filteredLines.length} / ${lines.length} lines (filtered)` : `${lines.length} lines`}
          </span>
        </div>

        <div
          ref={paneRef}
          onScroll={handlePaneScroll}
          className="h-full max-h-[calc(100%-2.5rem)] overflow-y-auto px-4 py-3 font-mono text-[12px] leading-relaxed"
        >
          {connState === "connecting" && lines.length === 0 && (
            <p className="flex items-center gap-2 text-white/30">
              <Loader2 size={13} className="animate-spin" /> กำลังเชื่อมต่อ...
            </p>
          )}
          {connState !== "connecting" && filteredLines.length === 0 && (
            <p className="text-white/20">{filter ? "ไม่มีบรรทัดที่ตรงกับตัวกรอง" : "ยังไม่มี log"}</p>
          )}
          {filteredLines.map((line) => (
            <p key={line.id} className="whitespace-pre-wrap break-all text-white/80">
              {line.timestamp && (
                <span className="mr-2 text-white/25 select-none">{formatTimestamp(line.timestamp)}</span>
              )}
              {line.text}
            </p>
          ))}
        </div>

        {!autoScroll && (
          <button
            type="button"
            onClick={jumpToLatest}
            className="absolute bottom-4 right-4 inline-flex items-center gap-1.5 rounded-full bg-[#BB6653] px-3.5 py-2 text-xs font-bold text-white shadow-lg transition-colors hover:bg-[#F08B51]"
          >
            <ArrowDownToLine size={13} /> Jump to latest
          </button>
        )}
      </div>
    </div>
  );
}
