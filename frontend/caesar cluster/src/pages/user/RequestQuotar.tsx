import { useState, useEffect, useRef, type ChangeEvent, type ClipboardEvent } from "react";
import {
  Box,
  Cpu,
  Layers,
  Network,
  Loader2,
  Plus,
  X,
  AlertTriangle,
  Trash2,
  Terminal,
  Upload,
} from "lucide-react";
import { useNavigate } from "react-router-dom";

import { cn } from "@/lib/utils";
import { ServiceCardsSkeleton } from "@/components/ui/PageSkeletons";
import GroupMembers from "@/components/GroupMembers";
import { serviceApi, type AppService } from "@/api/services";
import { namespaceApi, type NamespaceDetail } from "@/api/namespace";
import { getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";
import { PATHS } from "@/config/routes";

type EnvPair = { key: string; value: string };

// เพดานของ service ตัวเดียว — ต้องตรงกับ entity.MaxCPUMilliPerService / MaxRAMMBPerService
// และช่วงที่ dto.CreateServiceRequest bind ไว้ (cpu 100-3000m, ram 128-2048MB) ที่ backend บังคับอยู่แล้ว
const MIN_CPU_MILLI = 100;
const MAX_CPU_MILLI = 3000;
const MIN_RAM_MB = 128;
const MAX_RAM_MB = 2048;

// กติกา env var ฝั่ง backend (controller/helper.go) — ยึดตามนี้ตั้งแต่ตอน import เข้าตาราง
// ไม่งั้นค่าที่ paste/อัปโหลดมาจะผ่านหน้าเว็บแต่ไปโดน 400 ทั้งคำขอตอนกด deploy
const ENV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
const MAX_ENV_VARS = 20;

const clamp = (v: number, min: number, max: number) => Math.min(Math.max(v, min), max);

function formatCores(milli: number) {
  return `${(milli / 1000).toFixed(1)} cores`;
}

function formatRam(mb: number) {
  return Math.abs(mb) >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`;
}

// แปลงข้อความสไตล์ .env (KEY=value ทีละบรรทัด) เป็นคู่ key/value
// รองรับกรณีที่เจอบ่อยตอน copy มาวาง: บรรทัดว่าง, คอมเมนต์ #, export นำหน้า, ค่าที่คร่อมด้วย " หรือ '
function parseEnvText(text: string): EnvPair[] {
  const pairs: EnvPair[] = [];

  for (const rawLine of text.split(/\r?\n/)) {
    let line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    if (line.startsWith("export ")) line = line.slice(7).trim();

    const eq = line.indexOf("=");
    if (eq <= 0) continue;

    const key = line.slice(0, eq).trim();
    if (!ENV_KEY_PATTERN.test(key)) continue;

    let value = line.slice(eq + 1).trim();
    const quoted =
      value.length > 1 &&
      ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'")));
    if (quoted) value = value.slice(1, -1);

    pairs.push({ key, value });
  }

  return pairs;
}

// รวมคู่ที่ parse ได้เข้ากับตารางเดิม — key ซ้ำให้ค่าใหม่ทับ ที่เหลือต่อท้าย และทิ้งแถวว่างที่ยังไม่ได้กรอก
function mergeEnv(prev: EnvPair[], incoming: EnvPair[]): EnvPair[] {
  const merged = prev.filter((p) => p.key.trim() || p.value.trim());

  for (const pair of incoming) {
    const at = merged.findIndex((p) => p.key.trim() === pair.key);
    if (at >= 0) merged[at] = pair;
    else merged.push(pair);
  }

  return merged.length > 0 ? merged : [{ key: "", value: "" }];
}

function initialsOf(name: string) {
  const cleaned = name.replace(/[^a-zA-Z0-9]/g, "");
  return (cleaned.slice(0, 2) || "??").toUpperCase();
}

function statusBadge(status: AppService["status"]) {
  switch (status) {
    case "running":
      return { label: "Running", dot: "bg-green-600", text: "text-green-700", bg: "bg-green-50" };
    case "creating":
      return {
        label: "Deploying...",
        dot: "bg-[#F08B51] animate-pulse",
        text: "text-[#F08B51]",
        bg: "bg-[#FFF8E8]",
      };
    case "failed":
    default:
      return { label: "Failed", dot: "bg-red-500", text: "text-red-600", bg: "bg-red-50" };
  }
}

export default function RequestQuotar() {
  const navigate = useNavigate();
  const user = useAuthStore((state) => state.user);
  const [services, setServices] = useState<AppService[]>([]);
  // namespace = โควตาของกลุ่ม + ยอดที่ service เดิมกินไปแล้ว (backend คำนวณสดจากตาราง services)
  // ใช้ 2 ที่: ฟอร์ม deploy เอาไปคิดว่าเหลือให้ขอเท่าไหร่ และการ์ด Group Members ด้านล่าง
  const [namespace, setNamespace] = useState<NamespaceDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  // ดึงใหม่ทุกครั้งที่จำนวน service เปลี่ยน — ยอดคงเหลือจะได้ตรงกับของจริงตอนเปิดฟอร์มรอบถัดไป
  const fetchNamespace = () => {
    namespaceApi
      .mine()
      .then(setNamespace)
      .catch((err) => console.error(err));
  };

  const fetchServices = () => {
    setLoading(true);
    setError(null);
    serviceApi
      .list()
      .then(setServices)
      .catch((err) => {
        console.error(err);
        setError(getApiErrorMessage(err, "ไม่สามารถโหลดรายการ Service ได้"));
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchServices();
    fetchNamespace();
  }, []);

  const runningCount = services.filter((s) => s.status === "running").length;
  const deployingCount = services.filter((s) => s.status === "creating").length;

  const handleDelete = async (id: number) => {
    setDeletingId(id);
    try {
      await serviceApi.remove(id);
      setServices((prev) => prev.filter((s) => s.id !== id));
      fetchNamespace();
    } catch (err) {
      console.error(err);
      setError(getApiErrorMessage(err, "ลบ Service ไม่สำเร็จ"));
    } finally {
      setDeletingId(null);
      setPendingDeleteId(null);
    }
  };

  return (
    <div className="flex flex-col gap-6 text-left font-mono animate-in fade-in duration-200">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-5xl font-bold text-[#211a14]">My Services</h1>
          <p className="text-base text-[#211a14]/50 mt-1">
            {loading
              ? "Loading..."
              : `${services.length} total · ${runningCount} running · ${deployingCount} deploying`}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-2 self-start rounded-xl bg-[#BB6653] px-5 py-3 text-base font-bold text-white shadow-md transition-colors hover:bg-[#F08B51]"
        >
          <Plus size={18} strokeWidth={3} /> New Service
        </button>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-red-50 text-red-600 text-base border border-red-100 max-w-3xl">
          {error}
        </div>
      )}

      {loading ? (
        <ServiceCardsSkeleton />
      ) : (
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {services.map((svc) => {
            const badge = statusBadge(svc.status);
            const isConfirming = pendingDeleteId === svc.id;
            const isDeleting = deletingId === svc.id;

            return (
              <div
                key={svc.id}
                className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col gap-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-[#FBDFDA] text-base font-bold text-[#BB6653]">
                      {initialsOf(svc.name)}
                    </div>
                    <div className="min-w-0">
                      <p className="font-semibold text-[#211a14] truncate">{svc.name}</p>
                      <p className="text-sm text-[#211a14]/45 truncate">{svc.image}</p>
                    </div>
                  </div>
                  <span
                    className={cn(
                      "inline-flex shrink-0 items-center gap-1.5 px-2.5 py-1 rounded-full text-base font-bold whitespace-nowrap",
                      badge.bg,
                      badge.text,
                    )}
                  >
                    <span className={cn("size-1.5 rounded-full", badge.dot)} />
                    {badge.label}
                  </span>
                </div>

                <div className="grid grid-cols-3 gap-2 pt-3 border-t border-black/5 text-sm font-medium text-[#211a14]/70">
                  <div className="flex items-center gap-1.5">
                    <Cpu size={16} className="text-[#BB6653]" />
                    {(svc.cpu_milli / 1000).toFixed(1)} cores
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Layers size={16} className="text-[#BB6653]" />
                    {svc.ram_mb >= 1024
                      ? `${(svc.ram_mb / 1024).toFixed(1)} GB`
                      : `${svc.ram_mb} MB`}
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Network size={16} className="text-[#BB6653]" />
                    {svc.node_port ? `:${svc.node_port}` : "—"}
                  </div>
                </div>

                {isConfirming ? (
                  <div className="flex items-center gap-2 pt-1">
                    <button
                      type="button"
                      disabled={isDeleting}
                      onClick={() => handleDelete(svc.id)}
                      className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-xl bg-red-500 px-3 py-2 text-sm font-bold text-white transition-colors hover:bg-red-600 disabled:opacity-60"
                    >
                      {isDeleting ? <Loader2 size={15} className="animate-spin" /> : "Confirm delete"}
                    </button>
                    <button
                      type="button"
                      disabled={isDeleting}
                      onClick={() => setPendingDeleteId(null)}
                      className="rounded-xl border border-black/10 px-3 py-2 text-sm font-bold text-[#211a14]/60 transition-colors hover:bg-black/[0.03]"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => navigate(`/${PATHS.serviceLogs}/${svc.id}`)}
                      className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-sm font-bold text-[#211a14]/60 transition-colors hover:border-[#BB6653]/30 hover:bg-[#FBDFDA]/40 hover:text-[#BB6653]"
                    >
                      <Terminal size={15} /> Logs
                    </button>
                    <button
                      type="button"
                      onClick={() => setPendingDeleteId(svc.id)}
                      className="inline-flex items-center justify-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-sm font-bold text-[#211a14]/60 transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600"
                    >
                      <X size={15} /> Delete
                    </button>
                  </div>
                )}
              </div>
            );
          })}

          <button
            type="button"
            onClick={() => setShowCreate(true)}
            className="rounded-2xl border-2 border-dashed border-black/10 p-6 flex flex-col items-center justify-center gap-2 text-[#211a14]/40 transition-colors hover:border-[#BB6653]/40 hover:text-[#BB6653] min-h-[168px]"
          >
            <Plus size={28} />
            <span className="text-base font-semibold">Deploy a new service</span>
          </button>
        </div>
      )}

      {/* ย้ายมาจากหน้า General Dashboard — สมาชิกกลุ่มคือคนที่แชร์โควตาก้อนเดียวกับ service ด้านบน */}
      {namespace && (
        <GroupMembers namespace={namespace} isOwner={user?.id === namespace.contributor_id} />
      )}

      {showCreate && (
        <CreateServiceModal
          namespace={namespace}
          onClose={() => setShowCreate(false)}
          onCreated={(svc) => {
            setServices((prev) => [svc, ...prev]);
            fetchNamespace();
            setShowCreate(false);
          }}
        />
      )}
    </div>
  );
}

// ─── Modal ────────────────────────────────────────────────────────────────────

interface CreateServiceModalProps {
  namespace: NamespaceDetail | null;
  onClose: () => void;
  onCreated: (svc: AppService) => void;
}

function CreateServiceModal({ namespace, onClose, onCreated }: CreateServiceModalProps) {
  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  // เลือกระดับที่จะใช้เองได้อิสระ ไม่ผูกกับ preset ตายตัวอีกแล้ว — เก็บเป็นหน่วยเดียวกับที่ backend รับ
  const [cpuMilli, setCpuMilli] = useState(500);
  const [ramMb, setRamMb] = useState(512);
  const [envVars, setEnvVars] = useState<EnvPair[]>([{ key: "", value: "" }]);
  const [envNotice, setEnvNotice] = useState<string | null>(null);
  const envFileRef = useRef<HTMLInputElement>(null);

  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // ── โควตา: "ที่มีอยู่จริง" คือเพดานของกลุ่มหักที่ service เดิมกินไปแล้ว ────────────────
  const cpuLimit = namespace?.cpu_limit_milli ?? 0;
  const ramLimit = namespace?.ram_limit_mb ?? 0;
  const cpuUsed = namespace?.usage.used_cpu_milli ?? 0;
  const ramUsed = namespace?.usage.used_ram_mb ?? 0;

  const cpuAvailable = Math.max(cpuLimit - cpuUsed, 0);
  const ramAvailable = Math.max(ramLimit - ramUsed, 0);

  // ยอดคงเหลือหลังหักตัวที่กำลังจะขอ — ติดลบเมื่อไรคือขอเกิน (backend จะตอบ ErrQuotaExceeded อยู่ดี)
  const cpuRemaining = cpuAvailable - cpuMilli;
  const ramRemaining = ramAvailable - ramMb;
  const overCpu = namespace !== null && cpuRemaining < 0;
  const overRam = namespace !== null && ramRemaining < 0;

  const buildEnvMap = () => {
    const env: Record<string, string> = {};
    envVars.forEach(({ key, value }) => { if (key.trim()) env[key.trim()] = value; });
    return env;
  };

  // K8s-safe name: lowercase letters, numbers, hyphens — must start/end alphanumeric
  const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;
  const nameHasError = name.trim().length > 0 && !NAME_PATTERN.test(name.trim());

  const canSubmit =
    image.trim().length >= 3 &&
    name.trim().length >= 3 &&
    NAME_PATTERN.test(name.trim()) &&
    !overCpu &&
    !overRam;

  const addEnvRow = () => setEnvVars((p) => [...p, { key: "", value: "" }]);
  const removeEnvRow = (i: number) => setEnvVars((p) => p.filter((_, idx) => idx !== i));
  const updateEnvRow = (i: number, field: "key" | "value", val: string) =>
    setEnvVars((p) => { const n = [...p]; n[i] = { ...n[i], [field]: val }; return n; });

  // รวมค่าที่ import เข้ามากับตารางเดิม แล้วรายงานผลให้เห็น — ตัดที่ MAX_ENV_VARS ตั้งแต่ตรงนี้
  const applyParsedEnv = (base: EnvPair[], parsed: EnvPair[], source: string) => {
    const merged = mergeEnv(base, parsed);
    setEnvVars(merged.slice(0, MAX_ENV_VARS));
    setEnvNotice(
      merged.length > MAX_ENV_VARS
        ? `เพิ่มตัวแปรจาก${source}แล้ว — เก็บได้สูงสุด ${MAX_ENV_VARS} ตัว ส่วนที่เกินถูกตัดออก`
        : `เพิ่ม ${parsed.length} ตัวแปรจาก${source}`,
    );
  };

  // วางค่าแบบ Cloud Run: copy ทั้งก้อน KEY=value มาวางในช่อง KEY แล้วแตกเป็นแถวให้เอง
  // ข้อความที่ไม่มี = หรือขึ้นบรรทัดใหม่ ปล่อยให้ paste ตามปกติ (คนตั้งใจพิมพ์ชื่อ key ทีละตัว)
  const handleEnvPaste = (i: number, e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData("text");
    if (!/[\n=]/.test(text)) return;

    const parsed = parseEnvText(text);
    if (parsed.length === 0) return;

    e.preventDefault();
    applyParsedEnv(envVars.filter((_, idx) => idx !== i), parsed, "ที่วางมา");
  };

  // อีกทางเลือกหนึ่ง: หยิบไฟล์ .env มาทั้งไฟล์เลย — parse ด้วยตัวเดียวกับตอน paste
  const handleEnvFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ""; // เคลียร์เพื่อให้เลือกไฟล์เดิมซ้ำแล้วยัง onChange อีก
    if (!file) return;

    const parsed = parseEnvText(await file.text());
    if (parsed.length === 0) {
      setEnvNotice(`อ่านค่าจาก ${file.name} ไม่ได้ — ไฟล์ต้องอยู่ในรูปแบบ KEY=value`);
      return;
    }

    applyParsedEnv(envVars, parsed, ` ${file.name}`);
  };

  // ── Deploy — ตัดเส้นทาง AI review ออกแล้ว เหลือ path เดียวตรงไป provisioner ─────
  const handleDeploy = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const svc = await serviceApi.create({
        name: name.trim(),
        image: image.trim(),
        env_vars: buildEnvMap(),
        cpu_milli: cpuMilli,
        ram_mb: ramMb,
      });
      onCreated(svc);
    } catch (err) {
      setError(getApiErrorMessage(err, "Deploy ไม่สำเร็จ"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 font-mono">
      <div className="w-full max-w-3xl max-h-[90vh] overflow-y-auto rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl">

        {/* Header */}
        <div className="flex items-center justify-between px-8 py-6 border-b border-black/5">
          <div>
            <h2 className="text-2xl font-bold text-[#211a14]">Deploy a new service</h2>
            <p className="text-base text-[#211a14]/50 mt-0.5">Point us at a container image — we handle the rest.</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="p-2.5 rounded-xl text-[#211a14]/50 hover:bg-black/5 transition-colors disabled:opacity-30"
          >
            <X size={22} />
          </button>
        </div>

        {/* Form body */}
        <div className="px-8 py-6 flex flex-col gap-6">
          {error && (
            <div className="flex items-start gap-2 p-3.5 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100">
              <AlertTriangle size={16} className="shrink-0 mt-0.5" /> {error}
            </div>
          )}

          {/* Container Image */}
          <div className="flex flex-col gap-2">
            <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
              Container Image
            </label>
            <div className="flex items-center gap-2 rounded-xl border border-black/10 bg-white px-4 py-3">
              <Box size={18} className="text-[#211a14]/30 shrink-0" />
              <input
                value={image}
                onChange={(e) => setImage(e.target.value)}
                disabled={submitting}
                placeholder="nginx:latest, ghcr.io/you/app:tag"
                className="w-full bg-transparent text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
              />
            </div>
          </div>

          {/* Service Name */}
          <div className="flex flex-col gap-2">
            <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
              Service Name
            </label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={submitting}
              placeholder="my-web-app"
              className={cn(
                "w-full rounded-xl border bg-white px-4 py-3 text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60",
                nameHasError ? "border-red-300 focus:border-red-400" : "border-black/10",
              )}
            />
            <p className={cn("text-sm", nameHasError ? "text-red-500" : "text-[#211a14]/40")}>
              {nameHasError
                ? "Lowercase letters, numbers and hyphens only — start and end with a letter or number"
                : "lowercase letters, numbers and hyphens only"}
            </p>
          </div>

          {/* Resource for this service — เลือกระดับเองได้ทั้ง CPU/RAM ภายในเพดานของ service 1 ตัว
              แล้วสรุปให้เห็นว่าหักกับโควตาที่กลุ่มเหลืออยู่จริงแล้วยังเหลือเท่าไหร่ */}
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                Resource for this service
              </label>
              {namespace && (
                <span className="text-sm text-[#211a14]/40">
                  group quota {formatCores(cpuLimit)} · {formatRam(ramLimit)}
                </span>
              )}
            </div>

            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                  <Cpu size={14} className="text-[#BB6653]" /> CPU
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="number"
                    step="0.1"
                    min={MIN_CPU_MILLI / 1000}
                    max={MAX_CPU_MILLI / 1000}
                    value={(cpuMilli / 1000).toFixed(1)}
                    disabled={submitting}
                    onChange={(e) => {
                      const cores = Number(e.target.value);
                      if (!Number.isFinite(cores)) return;
                      setCpuMilli(clamp(Math.round(cores * 1000), MIN_CPU_MILLI, MAX_CPU_MILLI));
                    }}
                    className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                  <span className="text-sm text-[#211a14]/40">cores</span>
                </div>
              </div>
              <input
                type="range"
                min={MIN_CPU_MILLI}
                max={MAX_CPU_MILLI}
                step={100}
                value={cpuMilli}
                disabled={submitting}
                onChange={(e) => setCpuMilli(Number(e.target.value))}
                className="w-full accent-[#BB6653] disabled:opacity-50"
              />
            </div>

            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                  <Layers size={14} className="text-[#BB6653]" /> Memory
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="number"
                    step="128"
                    min={MIN_RAM_MB}
                    max={MAX_RAM_MB}
                    value={ramMb}
                    disabled={submitting}
                    onChange={(e) => {
                      const mb = Number(e.target.value);
                      if (!Number.isFinite(mb)) return;
                      setRamMb(clamp(Math.round(mb), MIN_RAM_MB, MAX_RAM_MB));
                    }}
                    className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                  <span className="text-sm text-[#211a14]/40">MB</span>
                </div>
              </div>
              <input
                type="range"
                min={MIN_RAM_MB}
                max={MAX_RAM_MB}
                step={128}
                value={ramMb}
                disabled={submitting}
                onChange={(e) => setRamMb(Number(e.target.value))}
                className="w-full accent-[#BB6653] disabled:opacity-50"
              />
            </div>

            <div className="rounded-xl border border-black/8 bg-white/60 p-4">
              {namespace ? (
                <div className="grid grid-cols-[1fr_auto_auto] gap-x-6 gap-y-2 text-sm">
                  <span className="font-bold uppercase tracking-wider text-[#211a14]/35">Summary</span>
                  <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">CPU</span>
                  <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">Memory</span>

                  <span className="text-[#211a14]/55">Group quota</span>
                  <span className="text-right text-[#211a14]/70">{formatCores(cpuLimit)}</span>
                  <span className="text-right text-[#211a14]/70">{formatRam(ramLimit)}</span>

                  <span className="text-[#211a14]/55">
                    In use ({namespace.usage.service_count}{" "}
                    {namespace.usage.service_count === 1 ? "service" : "services"})
                  </span>
                  <span className="text-right text-[#211a14]/70">- {formatCores(cpuUsed)}</span>
                  <span className="text-right text-[#211a14]/70">- {formatRam(ramUsed)}</span>

                  <span className="text-[#211a14]/55">This service</span>
                  <span className="text-right text-[#BB6653]">- {formatCores(cpuMilli)}</span>
                  <span className="text-right text-[#BB6653]">- {formatRam(ramMb)}</span>

                  <span className="border-t border-black/5 pt-2 font-bold text-[#211a14]">
                    Remaining
                  </span>
                  <span
                    className={cn(
                      "border-t border-black/5 pt-2 text-right font-bold",
                      overCpu ? "text-red-600" : "text-green-700",
                    )}
                  >
                    {formatCores(cpuRemaining)}
                  </span>
                  <span
                    className={cn(
                      "border-t border-black/5 pt-2 text-right font-bold",
                      overRam ? "text-red-600" : "text-green-700",
                    )}
                  >
                    {formatRam(ramRemaining)}
                  </span>
                </div>
              ) : (
                <p className="text-sm text-[#211a14]/40">กำลังโหลดโควตาของกลุ่ม...</p>
              )}
            </div>

            {(overCpu || overRam) && (
              <p className="flex items-start gap-1.5 text-sm text-red-600">
                <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                เกินโควตาที่กลุ่มเหลืออยู่ — ลดขนาดลง หรือลบ service ที่ไม่ได้ใช้ออกก่อน
              </p>
            )}
          </div>

          {/* Environment Variables — ตัวเลือกทั้งสองทาง (วางทับ / อัปโหลด .env) อยู่ระดับเดียวกับหัวข้อ */}
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                Environment Variables
              </label>
              <div className="flex items-center gap-4">
                <button
                  type="button" onClick={() => envFileRef.current?.click()} disabled={submitting}
                  className="inline-flex items-center gap-1.5 text-sm text-[#211a14]/50 hover:text-[#211a14] transition-colors disabled:opacity-40"
                >
                  <Upload size={14} /> Upload .env
                </button>
                <button
                  type="button" onClick={addEnvRow} disabled={submitting}
                  className="inline-flex items-center gap-1.5 text-sm text-[#211a14]/50 hover:text-[#211a14] transition-colors disabled:opacity-40"
                >
                  <Plus size={14} /> Add variable
                </button>
              </div>
              <input
                ref={envFileRef}
                type="file"
                accept=".env,.txt,text/plain"
                className="hidden"
                onChange={handleEnvFile}
              />
            </div>
            <div className="flex flex-col gap-2 rounded-xl border border-black/8 bg-white/60 p-3">
              {envVars.map((pair, i) => (
                <div key={i} className="flex items-center gap-2">
                  <input
                    placeholder="KEY"
                    value={pair.key}
                    onChange={(e) => updateEnvRow(i, "key", e.target.value)}
                    onPaste={(e) => handleEnvPaste(i, e)}
                    disabled={submitting}
                    className="flex-1 rounded-lg border border-black/8 bg-white px-3 py-2 text-sm font-mono uppercase tracking-wide text-[#211a14] placeholder:text-[#211a14]/25 outline-none disabled:opacity-50"
                  />
                  <span className="text-[#211a14]/25 text-sm select-none">=</span>
                  <input
                    placeholder="value"
                    value={pair.value}
                    onChange={(e) => updateEnvRow(i, "value", e.target.value)}
                    disabled={submitting}
                    className="flex-[2] rounded-lg border border-black/8 bg-white px-3 py-2 text-sm font-mono text-[#211a14] placeholder:text-[#211a14]/25 outline-none disabled:opacity-50"
                  />
                  <button
                    type="button"
                    onClick={() => removeEnvRow(i)}
                    disabled={submitting || envVars.length === 1}
                    className="p-1 rounded-lg text-[#211a14]/25 hover:text-red-500 transition-colors disabled:opacity-30"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
            {envNotice && <p className="text-sm text-[#BB6653]">{envNotice}</p>}
            <p className="text-xs text-[#211a14]/35">
              วางข้อความ KEY=value หลายบรรทัดลงในช่อง KEY แล้วระบบจะแตกเป็นแถวให้เอง หรือกด Upload .env
              เพื่อดึงทั้งไฟล์ — ค่าเหล่านี้จะถูกใส่ให้ service ตอน deploy
            </p>
          </div>
        </div>

        {/* Footer — เหลือแค่ทางเดียวตรงไป provisioner แล้ว ไม่มี AI review อีกต่อไป */}
        <div className="flex items-center justify-between gap-2 px-8 py-5 border-t border-black/5">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-xl px-5 py-3 text-base font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
          >
            Cancel
          </button>

          <button
            type="button"
            disabled={!canSubmit || submitting}
            onClick={handleDeploy}
            className={cn(
              "inline-flex items-center gap-2 rounded-xl px-6 py-3 text-base font-bold text-white shadow-md transition-all",
              canSubmit && !submitting
                ? "bg-[#BB6653] hover:bg-[#F08B51]"
                : "bg-[#211a14]/20 cursor-not-allowed shadow-none",
            )}
          >
            {submitting && <Loader2 size={16} className="animate-spin" />}
            {submitting ? "Deploying..." : "Deploy"}
          </button>
        </div>

      </div>
    </div>
  );
}
