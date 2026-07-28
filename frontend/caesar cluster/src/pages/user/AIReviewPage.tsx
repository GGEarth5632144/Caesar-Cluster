import { useState, useEffect, useRef, useCallback } from "react";
import { useParams, useLocation, useNavigate } from "react-router-dom";
import {
  CheckCircle2, XCircle, Loader2, Terminal, Network, Cpu, Shield,
  ArrowLeft, Clock, Zap, Activity, ThumbsUp, ThumbsDown, Box,
  AlertTriangle, ChevronDown, ChevronUp, Check,
} from "lucide-react";
import { cn } from "@/lib/utils";
import {
  aiReviewApi as aiReview,
  aiTelemetryApi as aiTelemetry,
  type ReviewResult,
  type TelemetrySnapshot,
} from "@/api/aiDeployApi";
import { serviceApi as svcApi } from "@/api/services";
import { PATHS } from "@/config/routes";
import { getApiErrorMessage } from "@/api/authApi";

// ─── Service info passed from the modal via router state ──────────────────────

interface ReviewPageState {
  serviceName: string;
  image: string;
  mode: "preset" | "custom";
  selectedTemplateId: number | null;
  cpuMilli: number;
  ramMb: number;
  envVars: Record<string, string>;
}

// ─── Stage thinking messages (Claude-style) ───────────────────────────────────

const THINKING: Record<string, string[]> = {
  sandbox: [
    "กำลัง pull container image จาก registry...",
    "สร้าง isolated quarantine network...",
    "เริ่มต้น container ใน sandboxed environment...",
    "ตรวจสอบ runtime behavior และ exit codes...",
  ],
  router: [
    "วิเคราะห์ error pattern จาก log...",
    "จับคู่กับ error signature database...",
    "ประเมิน domain ของปัญหา...",
    "เลือก expert agent ที่เหมาะสมที่สุด...",
  ],
  expert: [
    "อ่าน error log อย่างละเอียด...",
    "โหลด past experience จาก RAG memory...",
    "วิเคราะห์ root cause ของปัญหา...",
    "ตรวจสอบ context map ของ container...",
    "เขียน chain-of-thought scratchpad...",
    "ตรวจสอบ Constitutional AI constraints...",
    "ตรวจสอบว่าไม่มี hardcoded secrets หรือ chmod 777...",
    "เสนอ proposed fix ที่ปลอดภัยและตรง root cause...",
  ],
  evaluator: [
    "เริ่ม Red-Team adversarial review...",
    "ตรวจสอบ security vulnerabilities...",
    "ประเมิน side effects ของ proposed fix...",
    "ตรวจสอบ Constitutional AI violations...",
    "สรุปผลการตรวจสอบ...",
  ],
};

// ─── Typing animation hook ────────────────────────────────────────────────────

function useTypingLines(lines: string[], active: boolean) {
  const [doneLines, setDoneLines] = useState<string[]>([]);
  const [partialText, setPartialText] = useState("");
  const [lineIdx, setLineIdx] = useState(0);
  const [charIdx, setCharIdx] = useState(0);

  // Reset when a new stage becomes active
  const prevActive = useRef(false);
  useEffect(() => {
    if (active && !prevActive.current) {
      setDoneLines([]);
      setPartialText("");
      setLineIdx(0);
      setCharIdx(0);
    }
    prevActive.current = active;
  }, [active]);

  useEffect(() => {
    if (!active || lineIdx >= lines.length) return;
    const line = lines[lineIdx];

    if (charIdx <= line.length) {
      const t = setTimeout(() => {
        setPartialText(line.slice(0, charIdx + 1));
        setCharIdx((c) => c + 1);
      }, 22);
      return () => clearTimeout(t);
    }
    // Line done — pause then advance
    const t = setTimeout(() => {
      setDoneLines((prev) => [...prev, line]);
      setPartialText("");
      setLineIdx((l) => l + 1);
      setCharIdx(0);
    }, 480);
    return () => clearTimeout(t);
  }, [active, lineIdx, charIdx, lines]);

  const allDone = lineIdx >= lines.length;
  return { doneLines, partialText, allDone };
}

// ─── Elapsed timer ────────────────────────────────────────────────────────────

function useElapsedTimer(running: boolean) {
  const [ms, setMs] = useState(0);
  const startRef = useRef(Date.now());

  useEffect(() => {
    if (!running) return;
    startRef.current = Date.now();
    const t = setInterval(() => setMs(Date.now() - startRef.current), 100);
    return () => clearInterval(t);
  }, [running]);

  const total = running ? ms : ms;
  const mins = Math.floor(total / 60000);
  const secs = Math.floor((total % 60000) / 1000);
  const tenths = Math.floor((total % 1000) / 100);
  return `${mins.toString().padStart(2, "0")}:${secs.toString().padStart(2, "0")}.${tenths}`;
}

// ─── Thinking bubble ──────────────────────────────────────────────────────────

function ThinkingBubble({ stageKey }: { stageKey: string }) {
  const lines = THINKING[stageKey] ?? [];
  const { doneLines, partialText, allDone } = useTypingLines(lines, true);

  return (
    <div className="mt-3 rounded-xl bg-[#1a1714] border border-white/8 overflow-hidden">
      {/* Terminal header */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-white/8">
        <div className="flex items-center gap-2">
          <span className="size-2.5 rounded-full bg-red-500/70" />
          <span className="size-2.5 rounded-full bg-yellow-500/70" />
          <span className="size-2.5 rounded-full bg-green-500/70" />
        </div>
        <div className="flex items-center gap-1.5 text-[10px] text-white/30 font-mono">
          <span className="flex gap-0.5">
            {[0, 1, 2].map((i) => (
              <span
                key={i}
                className="size-1.5 rounded-full bg-[#F08B51] animate-bounce"
                style={{ animationDelay: `${i * 150}ms`, animationDuration: "1s" }}
              />
            ))}
          </span>
          กำลังประมวลผล...
        </div>
      </div>

      {/* Thinking lines */}
      <div className="px-4 py-3 flex flex-col gap-1 min-h-[60px]">
        {doneLines.map((line, i) => (
          <p key={i} className="text-[11px] font-mono text-green-400/60 leading-relaxed">
            <span className="text-green-600/50 mr-1">✓</span>
            {line}
          </p>
        ))}

        {!allDone && partialText !== "" && (
          <p className="text-[11px] font-mono text-green-300/90 leading-relaxed">
            <span className="text-[#F08B51]/60 mr-1">→</span>
            {partialText}
            <span className="animate-pulse text-[#F08B51]">▌</span>
          </p>
        )}

        {allDone && (
          <p className="text-[11px] font-mono text-white/20 animate-pulse">
            กำลังรอผลลัพธ์<span className="tracking-widest">...</span>
          </p>
        )}
      </div>
    </div>
  );
}

// ─── Stage types ──────────────────────────────────────────────────────────────

type StageState = "pending" | "running" | "done" | "error";

interface Stage {
  key: string;
  label: string;
  subtitle: string;
  icon: React.ReactNode;
  state: StageState;
  doneText?: string;
}

function stageFromReview(result: ReviewResult | null): Record<string, StageState> {
  if (!result || result.status === "queued") {
    return { sandbox: "running", router: "pending", expert: "pending", evaluator: "pending" };
  }
  if (result.status === "reviewing") {
    if (!result.target_agent) {
      return { sandbox: "running", router: "pending", expert: "pending", evaluator: "pending" };
    }
    if (result.attempt_count === 0) {
      return { sandbox: "done", router: "running", expert: "pending", evaluator: "pending" };
    }
    return { sandbox: "done", router: "done", expert: "running", evaluator: "pending" };
  }
  if (result.status === "needs_confirmation") {
    return { sandbox: "done", router: "done", expert: "done", evaluator: "done" };
  }
  if (result.status === "escalated") {
    return { sandbox: "done", router: "done", expert: "error", evaluator: "error" };
  }
  return { sandbox: "error", router: "pending", expert: "pending", evaluator: "pending" };
}

// ─── Stage card ───────────────────────────────────────────────────────────────

function StageCard({ stage, elapsedTimer, telemetry }: {
  stage: Stage;
  elapsedTimer: string;
  telemetry: TelemetrySnapshot | null;
}) {
  const isRunning = stage.state === "running";
  const isDone = stage.state === "done";
  const isError = stage.state === "error";
  const isPending = stage.state === "pending";

  const latency = telemetry?.latencies_ms[stage.key];

  return (
    <div className={cn(
      "rounded-2xl border p-5 transition-all duration-500",
      isRunning
        ? "bg-[#FFFDF6] border-[#F08B51]/40 ring-2 ring-[#F08B51]/20 shadow-md shadow-[#F08B51]/10"
        : isDone
          ? "bg-[#FFFDF6] border-black/5 shadow-sm"
          : isError
            ? "bg-red-50 border-red-100"
            : "bg-[#FFFDF6]/50 border-black/5 opacity-50",
    )}>
      <div className="flex items-start gap-3">
        {/* Stage icon */}
        <div className={cn(
          "flex size-10 shrink-0 items-center justify-center rounded-xl transition-colors",
          isRunning ? "bg-[#F08B51]/15 text-[#F08B51]" :
          isDone ? "bg-green-100 text-green-700" :
          isError ? "bg-red-100 text-red-600" :
          "bg-[#211a14]/5 text-[#211a14]/25",
        )}>
          {isRunning ? <Loader2 size={18} className="animate-spin" /> :
           isDone ? <CheckCircle2 size={18} /> :
           isError ? <XCircle size={18} /> :
           stage.icon}
        </div>

        {/* Stage info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <p className={cn(
              "text-sm font-bold",
              isPending ? "text-[#211a14]/30" : "text-[#211a14]"
            )}>
              {stage.label}
            </p>

            {/* Right badge: state / duration */}
            <div className="shrink-0">
              {isRunning && (
                <span className="flex items-center gap-1 text-[11px] font-mono text-[#F08B51]">
                  <Clock size={11} /> {elapsedTimer}
                </span>
              )}
              {isDone && (
                <span className="flex items-center gap-1 text-[11px] font-bold text-green-700">
                  {latency ? (
                    <><Clock size={11} /> {(latency / 1000).toFixed(2)}s</>
                  ) : "Done"}
                </span>
              )}
              {isError && (
                <span className="text-[11px] font-bold text-red-600">Error</span>
              )}
              {isPending && (
                <span className="text-[11px] text-[#211a14]/25">Pending</span>
              )}
            </div>
          </div>

          <p className={cn(
            "text-xs mt-0.5",
            isPending ? "text-[#211a14]/20" : "text-[#211a14]/50"
          )}>
            {stage.subtitle}
          </p>

          {/* Done text */}
          {isDone && stage.doneText && (
            <p className="mt-2 text-[11px] font-mono text-[#211a14]/55 bg-black/[0.03] rounded-lg px-2.5 py-1.5 border border-black/5">
              {stage.doneText}
            </p>
          )}

          {/* Thinking bubble when running */}
          {isRunning && <ThinkingBubble stageKey={stage.key} />}
        </div>
      </div>
    </div>
  );
}

// ─── Telemetry panel ──────────────────────────────────────────────────────────

function TelemetryPanel({ tel, attemptCount }: { tel: TelemetrySnapshot; attemptCount: number }) {
  const [open, setOpen] = useState(false);
  const circuitColor =
    tel.circuit_status === "normal" || tel.circuit_status === "approved"
      ? "text-green-700 bg-green-50"
      : tel.circuit_status === "halted" ? "text-red-600 bg-red-50"
      : "text-[#F08B51] bg-[#FFF8E8]";

  return (
    <div className="rounded-2xl border border-black/5 bg-[#FFFDF6] overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between px-5 py-3.5 hover:bg-black/[0.02] transition-colors"
      >
        <span className="flex items-center gap-2 text-sm font-bold text-[#211a14]/70">
          <Activity size={14} className="text-[#BB6653]" />
          Performance Metrics
        </span>
        {open ? <ChevronUp size={14} className="text-[#211a14]/30" /> : <ChevronDown size={14} className="text-[#211a14]/30" />}
      </button>

      {open && (
        <div className="px-5 pb-4 flex flex-col gap-3 border-t border-black/5">
          <div className="grid grid-cols-2 gap-3 pt-3">
            <div className="flex flex-col gap-1">
              <p className="text-[10px] text-[#211a14]/40 uppercase tracking-wider">Circuit Breaker</p>
              <span className={cn("text-xs font-bold px-2.5 py-1 rounded-full w-fit", circuitColor)}>
                {tel.circuit_status}
              </span>
            </div>
            <div className="flex flex-col gap-1">
              <p className="text-[10px] text-[#211a14]/40 uppercase tracking-wider">Attempts</p>
              <p className="text-xs font-bold text-[#211a14]">{attemptCount} / {tel.max_retries}</p>
            </div>
            <div className="flex flex-col gap-1">
              <p className="text-[10px] text-[#211a14]/40 uppercase tracking-wider flex items-center gap-1">
                <Zap size={10} /> Tokens
              </p>
              <p className="text-xs font-mono text-[#211a14]/70">
                {tel.prompt_tokens}↑ / {tel.completion_tokens}↓
              </p>
            </div>
          </div>

          {Object.keys(tel.latencies_ms).length > 0 && (
            <div className="flex flex-col gap-1.5">
              <p className="text-[10px] text-[#211a14]/40 uppercase tracking-wider flex items-center gap-1">
                <Clock size={10} /> Agent Latency
              </p>
              <div className="flex flex-wrap gap-2">
                {Object.entries(tel.latencies_ms).map(([agent, ms]) => (
                  <span key={agent} className="text-[10px] font-mono bg-[#211a14]/5 px-2.5 py-1 rounded-lg text-[#211a14]/60">
                    {agent}: {ms}ms
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function AIReviewPage() {
  const { requestId } = useParams<{ requestId: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const info = location.state as ReviewPageState | null;

  const [reviewResult, setReviewResult] = useState<ReviewResult | null>(null);
  const [telemetry, setTelemetry] = useState<TelemetrySnapshot | null>(null);
  const [finalImage, setFinalImage] = useState(info?.image ?? "");
  const [deploying, setDeploying] = useState(false);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [showTelemetry, setShowTelemetry] = useState(false);

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const timerRunning = !reviewResult || (
    reviewResult.status !== "needs_confirmation" &&
    reviewResult.status !== "escalated" &&
    reviewResult.status !== "failed"
  );
  const elapsedTimer = useElapsedTimer(timerRunning);

  const stopPolling = useCallback(() => {
    if (pollingRef.current) { clearInterval(pollingRef.current); pollingRef.current = null; }
  }, []);

  useEffect(() => {
    if (!requestId) return;
    const terminal = ["needs_confirmation", "escalated", "failed"];

    pollingRef.current = setInterval(async () => {
      try {
        const r = await aiReview.getStatus(requestId);
        setReviewResult(r);
        if (terminal.includes(r.status)) {
          stopPolling();
          aiTelemetry.get().then(setTelemetry).catch(console.error);
        }
      } catch (err) {
        console.error("Poll error:", err);
      }
    }, 2000);

    return () => stopPolling();
  }, [requestId, stopPolling]);

  // Derive stage states
  const stageStates = stageFromReview(reviewResult);

  const stages: Stage[] = [
    {
      key: "sandbox",
      label: "Pre-flight & Sandbox Analysis",
      subtitle: "Pull image → run in quarantine → capture errors",
      icon: <Terminal size={18} />,
      state: stageStates.sandbox,
      doneText: reviewResult?.status !== "needs_confirmation" || reviewResult.attempt_count > 0
        ? `CrashLoopBackOff detected — triggering AI pipeline`
        : `All clear — no runtime errors detected`,
    },
    {
      key: "router",
      label: "Brain 1 — The Router",
      subtitle: "Triage error type → select specialist agent",
      icon: <Network size={18} />,
      state: stageStates.router,
      doneText: reviewResult?.target_agent ? `→ ${reviewResult.target_agent}` : undefined,
    },
    {
      key: "expert",
      label: `Brain 2 — The Expert${reviewResult?.attempt_count ? ` (attempt ${reviewResult.attempt_count})` : ""}`,
      subtitle: "Root-cause analysis + Constitutional AI fix proposal",
      icon: <Cpu size={18} />,
      state: stageStates.expert,
      doneText: reviewResult?.proposed_fix ? "Proposed fix ready" : undefined,
    },
    {
      key: "evaluator",
      label: "Brain 3 — The Evaluator",
      subtitle: "Red-team security review → final approval",
      icon: <Shield size={18} />,
      state: stageStates.evaluator,
      doneText: reviewResult?.feedback_insight,
    },
  ];

  const isTerminal =
    reviewResult?.status === "needs_confirmation" ||
    reviewResult?.status === "escalated" ||
    reviewResult?.status === "failed";

  const hasProposedFix = !!(reviewResult?.proposed_fix?.trim());
  const isClean = reviewResult?.status === "needs_confirmation" && !hasProposedFix;
  const hasSuggestions = reviewResult?.status === "needs_confirmation" && hasProposedFix;
  const isEscalated = reviewResult?.status === "escalated";
  const isFailed = reviewResult?.status === "failed";

  const handleFinalDeploy = async (useAiFix: boolean) => {
    if (!info) return;
    setDeploying(true);
    setDeployError(null);
    const imageToUse = useAiFix ? (finalImage.trim() || info.image) : info.image;
    try {
      await svcApi.create({
        name: info.serviceName,
        image: imageToUse,
        ...(info.mode === "preset" && info.selectedTemplateId !== null
          ? { request_template_id: info.selectedTemplateId }
          : { cpu_milli: info.cpuMilli, ram_mb: info.ramMb }),
      });
      navigate(`/${PATHS.services}`, { replace: true });
    } catch (err) {
      setDeployError(getApiErrorMessage(err, "Deploy ไม่สำเร็จ"));
      setDeploying(false);
    }
  };

  return (
    <div className="flex flex-col gap-6 text-left font-mono animate-in fade-in duration-200 pb-10">

      {/* ── Header ──────────────────────────────────────────────────────────── */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <button
            type="button"
            onClick={() => navigate(`/${PATHS.services}`)}
            className="mt-1 p-2 rounded-xl text-[#211a14]/50 hover:bg-black/5 transition-colors"
          >
            <ArrowLeft size={18} />
          </button>
          <div>
            <h1 className="text-3xl font-bold text-[#211a14]">AI Review</h1>
            {info && (
              <div className="flex flex-wrap items-center gap-2 mt-1.5 text-xs text-[#211a14]/50">
                <span className="flex items-center gap-1">
                  <Box size={11} className="text-[#BB6653]" /> {info.image}
                </span>
                <span className="text-[#211a14]/25">·</span>
                <span>{info.serviceName}</span>
                <span className="text-[#211a14]/25">·</span>
                <span>{(info.cpuMilli / 1000).toFixed(1)} cores</span>
                <span className="text-[#211a14]/25">·</span>
                <span>{info.ramMb >= 1024 ? `${(info.ramMb / 1024).toFixed(1)} GB` : `${info.ramMb} MB`}</span>
              </div>
            )}
          </div>
        </div>

        {/* Elapsed timer */}
        <div className={cn(
          "flex items-center gap-2 px-4 py-2 rounded-xl border font-mono text-sm font-bold transition-colors",
          timerRunning
            ? "bg-[#FFF8E8] border-[#F08B51]/30 text-[#F08B51]"
            : "bg-green-50 border-green-100 text-green-700",
        )}>
          <Clock size={14} className={timerRunning ? "animate-pulse" : ""} />
          {elapsedTimer}
        </div>
      </div>

      {/* ── Status pill ──────────────────────────────────────────────────────── */}
      {reviewResult && (
        <div className={cn(
          "flex items-center gap-2 px-4 py-2.5 rounded-xl text-xs font-bold w-fit",
          reviewResult.status === "queued" ? "bg-[#211a14]/8 text-[#211a14]/60" :
          reviewResult.status === "reviewing" ? "bg-[#FFF8E8] text-[#F08B51] border border-[#F08B51]/20" :
          reviewResult.status === "needs_confirmation" ? "bg-green-50 text-green-700 border border-green-100" :
          reviewResult.status === "escalated" ? "bg-orange-50 text-orange-700 border border-orange-100" :
          "bg-red-50 text-red-600 border border-red-100"
        )}>
          {reviewResult.status === "reviewing" && <Loader2 size={12} className="animate-spin" />}
          {reviewResult.status === "needs_confirmation" && <CheckCircle2 size={12} />}
          {reviewResult.status === "escalated" && <AlertTriangle size={12} />}
          {reviewResult.status === "failed" && <XCircle size={12} />}
          {{
            queued: "Queued — waiting to start",
            reviewing: `Reviewing${reviewResult.attempt_count > 0 ? ` — attempt ${reviewResult.attempt_count}/3` : ""}`,
            needs_confirmation: hasSuggestions ? "AI has suggestions — ready for review" : "All clear — ready to deploy",
            escalated: "Circuit breaker triggered — escalated to human",
            failed: "Pipeline error",
          }[reviewResult.status]}
        </div>
      )}

      {/* ── Pipeline stages ───────────────────────────────────────────────────── */}
      <div className="flex flex-col gap-3">
        {stages.map((stage) => (
          <StageCard
            key={stage.key}
            stage={stage}
            elapsedTimer={elapsedTimer}
            telemetry={telemetry}
          />
        ))}
      </div>

      {/* ── Result section ────────────────────────────────────────────────────── */}
      {isTerminal && (
        <div className="flex flex-col gap-4 pt-2">
          <div className="h-px bg-[#211a14]/8" />

          {/* Case: No issues */}
          {isClean && (
            <div className="flex items-start gap-3 p-5 rounded-2xl bg-green-50 border border-green-100">
              <CheckCircle2 size={20} className="text-green-600 shrink-0 mt-0.5" />
              <div>
                <p className="text-sm font-bold text-green-800">All Clear — No Issues Found</p>
                {reviewResult?.justification && (
                  <p className="text-xs text-green-700/80 mt-1.5 leading-relaxed">
                    {reviewResult.justification}
                  </p>
                )}
              </div>
            </div>
          )}

          {/* Case: Has suggestions */}
          {hasSuggestions && (
            <>
              {/* AI recommendation text */}
              <div className="flex flex-col gap-2">
                <p className="text-xs font-bold uppercase tracking-wider text-[#BB6653] flex items-center gap-1.5">
                  <Cpu size={12} /> AI Recommendation
                </p>
                <div className="p-4 rounded-2xl bg-[#FFFDF6] border border-black/5 text-sm text-[#211a14]/75 leading-relaxed">
                  {reviewResult?.justification}
                  {reviewResult?.feedback_insight && (
                    <>
                      <div className="h-px bg-black/8 my-3" />
                      <p className="text-xs text-[#211a14]/55">{reviewResult.feedback_insight}</p>
                    </>
                  )}
                </div>
              </div>

              {/* Proposed fix code block */}
              <div className="flex flex-col gap-2">
                <p className="text-xs font-bold uppercase tracking-wider text-[#BB6653] flex items-center gap-1.5">
                  <Terminal size={12} /> Proposed Changes
                </p>
                <div className="rounded-2xl bg-[#1a1714] border border-black/20 overflow-hidden">
                  <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/8">
                    <span className="text-[10px] text-white/30 font-mono">proposed_fix</span>
                    <div className="flex gap-1.5">
                      <span className="size-2.5 rounded-full bg-red-500/60" />
                      <span className="size-2.5 rounded-full bg-yellow-500/60" />
                      <span className="size-2.5 rounded-full bg-green-500/60" />
                    </div>
                  </div>
                  <pre className="px-5 py-4 text-xs text-green-300/90 font-mono overflow-x-auto max-h-48 leading-relaxed whitespace-pre-wrap break-all">
                    {reviewResult?.proposed_fix}
                  </pre>
                </div>
              </div>

              {/* Editable final image */}
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653] flex items-center gap-1.5">
                  <Box size={12} /> Container Image for K8s Deploy
                </label>
                <div className="flex items-center gap-2 rounded-xl border-2 border-[#F08B51]/30 bg-white px-4 py-3 focus-within:border-[#F08B51]">
                  <input
                    value={finalImage}
                    onChange={(e) => setFinalImage(e.target.value)}
                    className="w-full bg-transparent text-sm text-[#211a14] outline-none font-mono"
                    placeholder="Edit image URL if AI suggests a patched version..."
                  />
                </div>
                <p className="text-[10px] text-[#211a14]/35">
                  Defaults to original. Edit only if the AI suggests a new image tag or version.
                </p>
              </div>
            </>
          )}

          {/* Case: Escalated */}
          {isEscalated && (
            <div className="flex items-start gap-3 p-5 rounded-2xl bg-orange-50 border border-orange-100">
              <AlertTriangle size={20} className="text-orange-600 shrink-0 mt-0.5" />
              <div>
                <p className="text-sm font-bold text-orange-800">Circuit Breaker Triggered</p>
                <p className="text-xs text-orange-700/80 mt-1">
                  The AI pipeline exhausted {reviewResult?.attempt_count} retry attempts without reaching approval.
                  Manual review is recommended before deploying.
                </p>
                {reviewResult?.feedback_insight && (
                  <p className="text-xs text-orange-600/70 mt-2 font-mono bg-orange-100/60 rounded-lg p-2">
                    {reviewResult.feedback_insight}
                  </p>
                )}
              </div>
            </div>
          )}

          {/* Case: Failed */}
          {isFailed && (
            <div className="flex items-start gap-3 p-5 rounded-2xl bg-red-50 border border-red-100">
              <XCircle size={20} className="text-red-600 shrink-0 mt-0.5" />
              <div>
                <p className="text-sm font-bold text-red-800">AI Pipeline Error</p>
                {reviewResult?.error_message && (
                  <p className="text-xs text-red-700/80 mt-1 font-mono break-all">
                    {reviewResult.error_message}
                  </p>
                )}
              </div>
            </div>
          )}

          {/* Deploy error */}
          {deployError && (
            <div className="flex items-start gap-2 p-3.5 rounded-xl bg-red-50 border border-red-100 text-xs text-red-600">
              <AlertTriangle size={14} className="shrink-0 mt-0.5" /> {deployError}
            </div>
          )}

          {/* Telemetry */}
          {telemetry && (
            <TelemetryPanel tel={telemetry} attemptCount={reviewResult?.attempt_count ?? 0} />
          )}

          {/* Action buttons */}
          {info && (
            <div className="flex items-center justify-between gap-3 pt-2">
              <button
                type="button"
                onClick={() => navigate(`/${PATHS.services}`)}
                className="rounded-xl px-5 py-3 text-sm font-bold text-[#211a14]/60 transition-colors hover:bg-black/5"
              >
                Back to Services
              </button>

              <div className="flex items-center gap-2">
                {hasSuggestions && (
                  <button
                    type="button"
                    disabled={deploying}
                    onClick={() => handleFinalDeploy(false)}
                    className="inline-flex items-center gap-2 rounded-xl border border-black/10 px-5 py-3 text-sm font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
                  >
                    <ThumbsDown size={14} /> Deploy Original
                  </button>
                )}

                {(isClean || hasSuggestions) && (
                  <button
                    type="button"
                    disabled={deploying}
                    onClick={() => handleFinalDeploy(hasSuggestions)}
                    className="inline-flex items-center gap-2 rounded-xl bg-[#BB6653] px-6 py-3 text-sm font-bold text-white shadow-md transition-all hover:bg-[#F08B51] disabled:opacity-60"
                  >
                    {deploying ? <Loader2 size={14} className="animate-spin" /> : <ThumbsUp size={14} />}
                    {deploying ? "Deploying..." : hasSuggestions ? "Apply AI Changes & Deploy" : "Deploy"}
                  </button>
                )}

                {(isEscalated || isFailed) && (
                  <>
                    <button
                      type="button"
                      disabled={deploying}
                      onClick={() => handleFinalDeploy(false)}
                      className="inline-flex items-center gap-2 rounded-xl bg-[#BB6653] px-6 py-3 text-sm font-bold text-white shadow-md transition-all hover:bg-[#F08B51] disabled:opacity-60"
                    >
                      {deploying ? <Loader2 size={14} className="animate-spin" /> : null}
                      {deploying ? "Deploying..." : "Deploy Anyway"}
                    </button>
                  </>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
