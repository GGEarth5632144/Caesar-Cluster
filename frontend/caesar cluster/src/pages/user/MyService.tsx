import {
  useState,
  useEffect,
  useRef,
  type ChangeEvent,
  type ClipboardEvent,
  type KeyboardEvent,
} from "react";
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
  Copy,
  Database,
  HardDrive,
  Lock,
  Eye,
} from "lucide-react";
import { useNavigate } from "react-router-dom";

import { cn } from "@/lib/utils";
import { ServiceCardsSkeleton } from "@/components/ui/PageSkeletons";
import GroupMembers from "@/components/GroupMembers";
import {
  serviceApi,
  isSettled,
  hasStorage,
  isTemplateDatabase,
  type AppService,
  type UpdateServiceDTO,
} from "@/api/services";
import { namespaceApi, type NamespaceDetail } from "@/api/namespace";
import { getApiErrorCode, getApiErrorMessage } from "@/api/authApi";
import { useAuthStore } from "@/store/authStore";
import { PATHS } from "@/config/routes";
import { usePageSearch } from "@/hooks/usePageSearch";
import { servicesScope } from "@/config/searchScopes";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";
import {
  looksSecret,
  formatStorage,
  validateDataPath,
  STORAGE_BOUNDS,
  UNIT_FACTOR,
  type StorageUnit,
} from "@/config/database";
import {
  DeployDatabaseModal,
  EditDatabaseModal,
  DatabaseConnectionPanel,
} from "./DatabaseModals";

type EnvPair = { key: string; value: string };

// เพดานของ service ตัวเดียว — ต้องตรงกับ entity.MaxCPUMilliPerService / MaxRAMMBPerService
// และช่วงที่ dto.CreateServiceRequest bind ไว้ (cpu 100-3000m, ram 128-2048MB) ที่ backend บังคับอยู่แล้ว
const MIN_CPU_MILLI = 100;
const MAX_CPU_MILLI = 3000;
const MIN_RAM_MB = 128;
const MAX_RAM_MB = 2048;

// พอร์ตที่ image ฟังอยู่ข้างใน คนละชั้นกับ node port ที่ k8s จ่ายให้ — ตรงกับ entity.MinContainerPort/Max
const MIN_CONTAINER_PORT = 1;
const MAX_CONTAINER_PORT = 65535;

// ตรงกับ entity.MaxReplicas ที่ backend บังคับ
const MAX_REPLICAS = 10;
const REPLICA_CHOICES = Array.from({ length: MAX_REPLICAS }, (_, i) => i + 1);

// กติกา env var ฝั่ง backend (controller/helper.go) — ยึดตามนี้ตั้งแต่ตอน import เข้าตาราง
// ไม่งั้นค่าที่ paste/อัปโหลดมาจะผ่านหน้าเว็บแต่ไปโดน 400 ทั้งคำขอตอนกด deploy
const ENV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;
const MAX_ENV_VARS = 20;

const clamp = (v: number, min: number, max: number) =>
  Math.min(Math.max(v, min), max);
const floorTo = (v: number, step: number) => Math.floor(v / step) * step;

// กด Enter ที่ช่อง value → ไปแถวถัดไป (แถวสุดท้ายจะเพิ่มแถวใหม่) แล้ว focus ช่อง key
// ใช้ร่วมกันทั้งฟอร์ม deploy และฟอร์มแก้ไข · ข้าม Enter ระหว่างพิมพ์ด้วย IME (ภาษาไทย/จีน) ที่ยังไม่ commit
function useEnvEnterToNext(rowCount: number, addRow: () => void) {
  const listRef = useRef<HTMLDivElement>(null);
  const pendingFocus = useRef<number | null>(null);

  const focusKey = (i: number) =>
    listRef.current
      ?.querySelectorAll<HTMLInputElement>("input[data-env-key]")
      [i]?.focus();

  // แถวใหม่ยังไม่อยู่ใน DOM ตอนกด Enter — รอ render รอบถัดไปก่อนค่อย focus
  useEffect(() => {
    if (pendingFocus.current === null) return;
    focusKey(pendingFocus.current);
    pendingFocus.current = null;
  }, [rowCount]);

  const onValueKeyDown = (i: number) => (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== "Enter" || e.nativeEvent.isComposing) return;
    e.preventDefault(); // กันฟอร์ม submit
    if (i + 1 < rowCount) {
      focusKey(i + 1);
    } else if (rowCount < MAX_ENV_VARS) {
      pendingFocus.current = i + 1;
      addRow();
    }
  };

  return { listRef, onValueKeyDown };
}

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
      return {
        label: "Running",
        dot: "bg-green-600",
        text: "text-green-700",
        bg: "bg-green-50",
      };
    case "creating":
      return {
        label: "Deploying...",
        dot: "bg-[#F08B51] animate-pulse",
        text: "text-[#F08B51]",
        bg: "bg-[#FFF8E8]",
      };
    // รอที่ว่างบนเครื่อง — ไม่ใช่ error แต่ก็ไม่ใช่ "กำลังทำงาน" ต้องแยกสีให้เห็น
    case "pending":
      return {
        label: "รอทรัพยากร",
        dot: "bg-[#F08B51] animate-pulse",
        text: "text-[#A96A15]",
        bg: "bg-[#FBEFD9]",
      };
    // ตายแล้วเกิดใหม่วนไป — เกือบทุกครั้งคือตั้งค่าผิด ผู้ใช้แก้เองได้ถ้ารู้สาเหตุ
    case "crashloop":
      return {
        label: "ตายซ้ำๆ",
        dot: "bg-red-500 animate-pulse",
        text: "text-red-600",
        bg: "bg-red-50",
      };
    case "failed":
    default:
      return {
        label: "Failed",
        dot: "bg-red-500",
        text: "text-red-600",
        bg: "bg-red-50",
      };
  }
}

/**
 * แปลงรหัสสาเหตุจากคลัสเตอร์ ("CrashLoopBackOff" ไม่ได้บอกว่าต้องทำอะไรต่อ)
 * ให้เป็นประโยคที่บอกขั้นตอนถัดไปจริงๆ
 */
function statusAdvice(svc: AppService): string | null {
  if (svc.status === "crashloop") {
    return "container เริ่มทำงานแล้วปิดตัวเองทันที ส่วนใหญ่มาจาก environment variable ไม่ครบหรือตั้งค่าผิด ดูข้อความด้านล่างแล้วแก้ค่า จากนั้นลบ service นี้แล้วสร้างใหม่";
  }
  if (svc.status === "pending") {
    if (svc.status_reason === "FailedScheduling") {
      return "ยังไม่มีเครื่องไหนเหลือทรัพยากรพอสำหรับสเปกที่ขอ ลองลดขนาดลงหรือรอให้ service อื่นว่าง";
    }
    return "กำลังเตรียม container อยู่ ถ้าค้างอยู่นานผิดปกติ ให้ตรวจสอบว่าชื่อ image ถูกต้อง";
  }
  if (svc.status === "failed") {
    if (
      svc.status_reason === "ImagePullBackOff" ||
      svc.status_reason === "ErrImagePull"
    ) {
      return "ดึง image ไม่ได้ ตรวจสอบว่าชื่อกับ tag ถูกต้อง และ image เป็นแบบสาธารณะ";
    }
    if (svc.status_reason === "NotFound") {
      return "ไม่พบ workload นี้บนคลัสเตอร์แล้ว อาจถูกลบจากนอกระบบ — ลบรายการนี้ทิ้งแล้วสร้างใหม่ได้";
    }
  }
  return null;
}

export default function MyService() {
  const navigate = useNavigate();
  const user = useAuthStore((state) => state.user);
  const [services, setServices] = useState<AppService[]>([]);
  // namespace = โควตาของกลุ่ม + ยอดที่ service เดิมกินไปแล้ว (backend คำนวณสดจากตาราง services)
  // ใช้ 2 ที่: ฟอร์ม deploy เอาไปคิดว่าเหลือให้ขอเท่าไหร่ และการ์ด Group Members ด้านล่าง
  const [namespace, setNamespace] = useState<NamespaceDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  // database สร้างจาก template แยกฟอร์ม (docs 029) — ไม่มีสวิตช์ "ใช้เป็นฐานข้อมูล" ในฟอร์ม service แล้ว
  const [showCreateDatabase, setShowCreateDatabase] = useState(false);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  // service ที่รอผล scale อยู่ — ล็อก dropdown ไม่ให้กดรัวจนคำสั่งซ้อนกัน
  const [scalingId, setScalingId] = useState<number | null>(null);

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

  // เก็บแค่ id แล้วอ่านตัวจริงจาก services — ถ้าเก็บทั้ง object ไว้ หน้าต่างรายละเอียดจะค้างสถานะ
  // ตอนที่กดเปิด ไม่ขยับตามรอบดึงข้อมูลด้านล่าง (ถูกลบไปแล้ว find ไม่เจอ หน้าต่างก็ปิดเอง)
  const [selectedServiceId, setSelectedServiceId] = useState<number | null>(
    null,
  );
  const selectedServiceDetail =
    services.find((s) => s.id === selectedServiceId) ?? null;

  // ── ดึงข้อมูลซ้ำระหว่างที่ยังมี service สถานะไม่นิ่ง หรือเปิดดูรายละเอียดอยู่ ──────────────
  //
  // จำเป็นเพราะ backend ไม่เขียน running ทันทีที่ deploy ผ่าน ต้องรอ ServiceHealthMonitor
  // ยืนยันกับคลัสเตอร์ก่อน ถ้าไม่ดึงซ้ำผู้ใช้จะเห็น "Deploying..." ค้างจนกว่าจะกด refresh เอง
  // ส่วนตอนเปิดดูรายละเอียด ตัวที่ running อยู่ก็อาจกลายเป็น crashloop ได้ จึงดึงต่อจนกว่าจะปิด
  // หยุดเองเมื่อทุกตัวนิ่งและไม่ได้เปิดดูรายละเอียดอยู่
  const hasUnsettled = services.some((s) => !isSettled(s.status));
  const isViewingDetail = selectedServiceId !== null;
  useEffect(() => {
    if (!hasUnsettled && !isViewingDetail) return;
    const timer = setInterval(() => {
      serviceApi
        .list()
        .then(setServices)
        .catch((err) => console.error(err));
    }, 4000);
    return () => clearInterval(timer);
  }, [hasUnsettled, isViewingDetail]);

  const runningCount = services.filter((s) => s.status === "running").length;
  const deployingCount = services.filter((s) => !isSettled(s.status)).length;
  const brokenCount = services.filter(
    (s) => s.status === "crashloop" || s.status === "failed",
  ).length;
  const [editingService, setEditingService] = useState<AppService | null>(null);
  // ช่องค้นหาบน Topbar กรองการ์ดด้านล่าง — ตัวเลขสรุปบรรทัดบนยังนับจาก services ทั้งหมด
  // เพราะเป็นภาพรวมของเนมสเปซ ไม่ใช่ผลของคำค้น
  const {
    results: visibleServices,
    isFiltering,
    highlightTerms,
  } = usePageSearch(servicesScope, services);

  // backend เช็คโควตาให้ก่อน ถ้าไม่พอตอบ 409 แล้วเราคง state เดิมไว้ พร้อมโชว์เหตุผลที่ backend บอกมา
  const handleScale = async (id: number, replicas: number) => {
    setScalingId(id);
    setError(null);
    try {
      const updated = await serviceApi.scale(id, replicas);
      setServices((prev) => prev.map((s) => (s.id === id ? updated : s)));
      fetchNamespace();
    } catch (err) {
      console.error(err);
      setError(getApiErrorMessage(err, "ปรับจำนวน replica ไม่สำเร็จ"));
    } finally {
      setScalingId(null);
    }
  };

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
              : `${services.length} total · ${runningCount} running · ${deployingCount} กำลังเริ่ม` +
                (brokenCount > 0 ? ` · ${brokenCount} มีปัญหา` : "")}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3 self-start">
          {services.length > 0 && <SearchStatus />}
          <button
            type="button"
            onClick={() => setShowCreateDatabase(true)}
            className="inline-flex items-center gap-2 rounded-xl border-2 border-[#BB6653] px-5 py-2.5 text-base font-bold text-[#BB6653] transition-colors hover:bg-[#FBDFDA]"
          >
            <Database size={18} /> New Database
          </button>
          <button
            type="button"
            onClick={() => setShowCreate(true)}
            className="inline-flex items-center gap-2 rounded-xl bg-[#BB6653] px-5 py-3 text-base font-bold text-white shadow-md transition-colors hover:bg-[#F08B51]"
          >
            <Plus size={18} strokeWidth={3} /> New Service
          </button>
        </div>
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
          {visibleServices.map((svc) => {
            const badge = statusBadge(svc.status);
            const isConfirming = pendingDeleteId === svc.id;
            const isDeleting = deletingId === svc.id;
            const isScaling = scalingId === svc.id;
            // ระหว่าง status=creating ตัว provisioner ยังถือสเปกเดิม แทรก scale ตอนนั้นแล้ว
            // คลัสเตอร์จะได้จำนวนเก่าแต่ DB บอกจำนวนใหม่ — backend ปฏิเสธอยู่แล้ว ตรงนี้กันไม่ให้กดไปเจอ error เปล่าๆ
            const canScale = svc.status === "running";
            // มีดิสก์ (รวม database) = 1 pod ตลอดชีวิต และลบแล้วข้อมูลหาย — แยกจาก is_database ที่บอกแค่เรื่องเครือข่าย
            const withDisk = hasStorage(svc);

            return (
              <div
                key={svc.id}
                className="rounded-2xl bg-[#FFFDF6] p-6 border border-black/5 shadow-sm flex flex-col gap-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-[#FBDFDA] text-base font-bold text-[#BB6653]">
                      {svc.is_database ? (
                        <Database size={20} />
                      ) : (
                        initialsOf(svc.name)
                      )}
                    </div>
                    <div className="min-w-0">
                      <p className="font-semibold text-[#211a14] truncate flex items-center gap-1.5">
                        <Highlight text={svc.name} terms={highlightTerms} />
                        {svc.is_database && (
                          <span className="shrink-0 rounded-md bg-[#FBDFDA] px-1.5 py-0.5 text-xs font-bold text-[#BB6653]">
                            {isTemplateDatabase(svc)
                              ? `${svc.database_engine} ${svc.database_version}`
                              : "database"}
                          </span>
                        )}
                      </p>
                      <p className="text-sm text-[#211a14]/45 truncate">
                        <Highlight text={svc.image} terms={highlightTerms} />
                      </p>
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
                <button
                  type="button"
                  onClick={() => setSelectedServiceId(svc.id)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-bold text-[#211a14]/50 hover:text-[#BB6653] hover:bg-[#FBDFDA] rounded-xl transition-colors mt-2"
                  title="ดูข้อมูลฉบับเต็ม"
                >
                  <Eye size={16} />
                  See or change and re-deploy
                </button>
                {(svc.status === "crashloop" ||
                  svc.status === "pending" ||
                  svc.status === "failed") && (
                  <div
                    className={cn(
                      "flex flex-col gap-1.5 rounded-xl border p-3",
                      svc.status === "pending"
                        ? "border-[#A96A15]/20 bg-[#FBEFD9]"
                        : "border-red-100 bg-red-50",
                    )}
                  >
                    <span
                      className={cn(
                        "flex items-center gap-1.5 text-sm font-bold",
                        svc.status === "pending"
                          ? "text-[#A96A15]"
                          : "text-red-600",
                      )}
                    >
                      <AlertTriangle size={14} className="shrink-0" />
                      {svc.status_reason || "ไม่ทราบสาเหตุ"}
                      {svc.restart_count > 0 && (
                        <span className="font-normal opacity-70">
                          · restart {svc.restart_count} ครั้ง
                        </span>
                      )}
                    </span>

                    {statusAdvice(svc) && (
                      <span className="text-sm leading-relaxed text-[#211a14]/60">
                        {statusAdvice(svc)}
                      </span>
                    )}

                    {svc.status_message && (
                      <p
                        className="truncate rounded-lg bg-white/70 p-2 font-mono text-xs text-[#211a14]/70"
                        title={svc.status_message}
                      >
                        {svc.status_message}
                      </p>
                    )}
                  </div>
                )}

                {/* สเปกต่อ 1 Pod — คูณด้วยจำนวน replica ด้านล่างถึงจะเป็นยอดที่กินโควตาจริง */}
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
                  {withDisk ? (
                    <div
                      className="flex items-center gap-1.5"
                      title="ดิสก์ถาวร"
                    >
                      <HardDrive size={16} className="text-[#BB6653]" />
                      {formatStorage(svc.storage_mb)}
                    </div>
                  ) : (
                    <div
                      className="flex items-center gap-1.5"
                      title="container port"
                    >
                      <Box size={16} className="text-[#BB6653]" />
                      {svc.container_port}
                    </div>
                  )}
                </div>

                {/* การเข้าถึง — database ไม่มี node_port เลย (ไม่ใช่ "ยังไม่มา") ต้องแยกข้อความให้ชัด
                    ไม่งั้นการ์ดของ database จะค้างที่ "รอคลัสเตอร์จ่ายพอร์ต..." ไปตลอดกาล */}
                {svc.is_database ? (
                  <div className="flex items-start gap-1.5 text-sm text-green-700">
                    <Lock size={16} className="shrink-0 mt-0.5" />
                    <span
                      className="truncate"
                      title={`เชื่อมต่อจาก service อื่นในกลุ่มที่ ${svc.name}:${svc.container_port}`}
                    >
                      ใช้ได้เฉพาะในกลุ่ม &middot; {svc.name}:
                      {svc.container_port}
                    </span>
                  </div>
                ) : (
                  <div className="flex items-center gap-1.5 text-sm text-[#211a14]/45">
                    <Network size={16} className="text-[#BB6653] shrink-0" />
                    {svc.node_port ? (
                      <span className="truncate">
                        {window.location.hostname}:{svc.node_port} &rarr; :
                        {svc.container_port}
                      </span>
                    ) : (
                      <span>รอคลัสเตอร์จ่ายพอร์ต...</span>
                    )}
                  </div>
                )}

                {/* เพิ่มตัวรับโหลดตอนคนใช้เยอะ — กินสเปกต่อ Pod x จำนวนนี้ ระบบเช็คโควตากลุ่มให้ก่อนทุกครั้ง
                    service ที่มีดิสก์ (รวม database) ปรับไม่ได้เลย จึงเอา dropdown ออกไปพร้อมบอกเหตุผล
                    ไม่ใช่ทิ้งช่องจางๆ ที่กดไม่ได้ไว้ให้คนสงสัยว่าตัวเองทำอะไรผิด */}
                <div className="flex items-center justify-between gap-2 text-sm">
                  <span className="flex items-center gap-1.5 text-[#211a14]/50">
                    <Copy size={16} className="text-[#BB6653]" /> Replicas
                  </span>
                  {withDisk ? (
                    <span
                      className="text-[#211a14]/45"
                      title={
                        svc.is_database
                          ? "ฐานข้อมูลรันได้ครั้งละตัวเดียว เพิ่มจำนวนแล้วข้อมูลจะเสียหาย"
                          : "ดิสก์ถาวรใช้ได้ทีละ container — เพิ่มจำนวนแล้วสอง container จะเขียนดิสก์ก้อนเดียวกันจนข้อมูลพัง"
                      }
                    >
                      1 container &middot; ปรับไม่ได้
                    </span>
                  ) : (
                    <div className="flex items-center gap-1.5">
                      {isScaling && (
                        <Loader2
                          size={14}
                          className="animate-spin text-[#BB6653]"
                        />
                      )}
                      <select
                        value={svc.replicas}
                        disabled={isScaling || isDeleting || !canScale}
                        title={
                          canScale ? undefined : "ปรับได้หลัง deploy เสร็จ"
                        }
                        onChange={(e) =>
                          handleScale(svc.id, Number(e.target.value))
                        }
                        className="rounded-lg border border-black/10 bg-white px-2.5 py-1 text-sm text-[#211a14] outline-none disabled:opacity-50"
                      >
                        {REPLICA_CHOICES.map((n) => (
                          <option key={n} value={n}>
                            {n} {n === 1 ? "container" : "containers"}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                </div>

                {isConfirming ? (
                  <div className="flex flex-col gap-2 pt-1">
                    {withDisk && (
                      <p className="flex items-start gap-1.5 text-sm text-red-600">
                        <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                        {svc.is_database
                          ? "ข้อมูลทั้งหมดในฐานข้อมูลนี้"
                          : "ไฟล์ทั้งหมดในดิสก์ของ service นี้"}{" "}
                        ({formatStorage(svc.storage_mb)}) จะถูกลบถาวร
                        กู้คืนไม่ได้
                      </p>
                    )}
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        disabled={isDeleting}
                        onClick={() => handleDelete(svc.id)}
                        className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-xl bg-red-500 px-3 py-2 text-sm font-bold text-white transition-colors hover:bg-red-600 disabled:opacity-60"
                      >
                        {isDeleting ? (
                          <Loader2 size={15} className="animate-spin" />
                        ) : (
                          "Confirm delete"
                        )}
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
                  </div>
                ) : (
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() =>
                        navigate(`/${PATHS.serviceLogs}/${svc.id}`)
                      }
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
          {isFiltering ? (
            visibleServices.length === 0 && (
              <p className="col-span-full rounded-2xl border-2 border-dashed border-black/10 p-10 text-center text-base text-[#211a14]/45">
                ไม่มีบริการที่ตรงกับคำค้นหา
              </p>
            )
          ) : (
            <button
              type="button"
              onClick={() => setShowCreate(true)}
              className="rounded-2xl border-2 border-dashed border-black/10 p-6 flex flex-col items-center justify-center gap-2 text-[#211a14]/40 transition-colors hover:border-[#BB6653]/40 hover:text-[#BB6653] min-h-[168px]"
            >
              <Plus size={28} />
              <span className="text-base font-semibold">
                Deploy a new service
              </span>
            </button>
          )}
          {selectedServiceDetail && (
            
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 backdrop-blur-sm p-4">
              <div className="bg-[#FFFDF6] w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-3xl px-7 py-5 shadow-2xl border border-black/5 flex flex-col gap-4 animate-in fade-in zoom-in duration-200 custom-scrollbar">
                {/* Header Modal */}
                <div className="flex justify-between items-start border-b border-black/5 pb-4">
                  <div className="flex items-center justify-between gap-4">
                    {/* ส่วนซ้าย: ไอคอน + ชื่อหัวข้อ */}
                    <div className="flex items-center gap-3">
                      <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-[#FBDFDA] text-lg font-bold text-[#BB6653]">
                        {selectedServiceDetail.is_database ? (
                          <Database size={24} />
                        ) : (
                          initialsOf(selectedServiceDetail.name)
                        )}
                      </div>
                      <div>
                        <h3 className="text-xl font-extrabold text-[#211a14]">
                          Service Info
                        </h3>
                        <span className="text-xs font-semibold text-[#BB6653] uppercase tracking-wider">
                          Detailed View
                        </span>
                      </div>
                    </div>
                    <span
                    
                      className={cn(
                        "inline-flex shrink-0 items-center gap-1.5 px-3 py-1 rounded-full text-sm font-bold whitespace-nowrap shadow-sm",
                        statusBadge(selectedServiceDetail.status).bg,
                        statusBadge(selectedServiceDetail.status).text,
                      )}
                    >
                      <span
                        className={cn(
                          "size-2 rounded-full animate-pulse",
                          statusBadge(selectedServiceDetail.status).dot,
                        )}
                      />
                      {statusBadge(selectedServiceDetail.status).label}
                    </span>
                  </div>
                  <button
                    onClick={() => setSelectedServiceId(null)}
                    className="p-2 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-xl transition-colors"
                  >
                    <X size={20} />
                  </button>
                </div>

                {/* Content Modal */}
                <div className="flex flex-col gap-5 text-sm text-[#211a14]/80">
                  <div className="bg-white p-4 rounded-2xl border border-black/5">
                    <p className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-1">
                      Service Name
                    </p>
                    <p className="font-semibold text-base break-words">
                      {selectedServiceDetail.name}
                    </p>
                  </div>

                  <div className="bg-white p-4 rounded-2xl border border-black/5">
                    <p className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-1">
                      Docker Image
                    </p>
                    <p className="font-mono text-sm break-all text-[#BB6653]">
                      {selectedServiceDetail.image}
                    </p>
                  </div>

                  {(selectedServiceDetail.status === "crashloop" ||
                    selectedServiceDetail.status === "pending" ||
                    selectedServiceDetail.status === "failed") && (
                    <div
                      className={cn(
                        "flex flex-col gap-2 rounded-2xl border p-4",
                        selectedServiceDetail.status === "pending"
                          ? "border-[#A96A15]/20 bg-[#FBEFD9]"
                          : "border-red-100 bg-red-50",
                      )}
                    >
                      <span
                        className={cn(
                          "flex items-center gap-1.5 text-sm font-bold",
                          selectedServiceDetail.status === "pending"
                            ? "text-[#A96A15]"
                            : "text-red-600",
                        )}
                      >
                        <AlertTriangle size={16} className="shrink-0" />
                        {selectedServiceDetail.status_reason || "ไม่ทราบสาเหตุ"}
                        {selectedServiceDetail.restart_count > 0 && (
                          <span className="font-normal opacity-70">
                            · restart {selectedServiceDetail.restart_count}{" "}
                            ครั้ง
                          </span>
                        )}
                      </span>

                      {statusAdvice(selectedServiceDetail) && (
                        <span className="text-sm leading-relaxed text-[#211a14]/70">
                          {statusAdvice(selectedServiceDetail)}
                        </span>
                      )}

                      {selectedServiceDetail.status_message && (
                        <pre className="mt-1 max-h-32 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-white/60 p-3 text-xs leading-relaxed text-[#211a14]/70 custom-scrollbar">
                          {selectedServiceDetail.status_message}
                        </pre>
                      )}
                    </div>
                  )}

                  <div className="grid grid-cols-3 gap-3">
                    <div className="bg-white p-3 rounded-2xl border border-black/5 flex flex-col items-center justify-center gap-1">
                      <Cpu size={18} className="text-[#BB6653]" />
                      <p className="text-[10px] font-bold text-gray-400 uppercase tracking-widest">
                        CPU
                      </p>
                      <p className="font-bold">
                        {(selectedServiceDetail.cpu_milli / 1000).toFixed(1)}{" "}
                        <span className="text-xs font-normal">cores</span>
                      </p>
                    </div>
                    <div className="bg-white p-3 rounded-2xl border border-black/5 flex flex-col items-center justify-center gap-1">
                      <Layers size={18} className="text-[#BB6653]" />
                      <p className="text-[10px] font-bold text-gray-400 uppercase tracking-widest">
                        RAM
                      </p>
                      <p className="font-bold">
                        {selectedServiceDetail.ram_mb >= 1024
                          ? `${(selectedServiceDetail.ram_mb / 1024).toFixed(1)} GB`
                          : `${selectedServiceDetail.ram_mb} MB`}
                      </p>
                    </div>
                    <div className="bg-white p-3 rounded-2xl border border-black/5 flex flex-col items-center justify-center gap-1">
                      <Box size={18} className="text-[#BB6653]" />
                      <p className="text-[10px] font-bold text-gray-400 uppercase tracking-widest">
                        Port
                      </p>
                      <p className="font-bold">
                        {selectedServiceDetail.container_port}
                      </p>
                    </div>
                  </div>
                  {isTemplateDatabase(selectedServiceDetail) ? (
                    <DatabaseConnectionPanel
                      key={selectedServiceDetail.id}
                      service={selectedServiceDetail}
                    />
                  ) : (
                  <>
                  <div className="bg-white p-4 rounded-2xl border border-black/5 flex flex-col gap-2">
                    <p className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-1">
                      Environment Variables
                    </p>
                    {selectedServiceDetail.env_vars &&
                    Object.keys(selectedServiceDetail.env_vars).length > 0 ? (
                      <div className="flex flex-col gap-1.5 rounded-xl bg-black/[0.02] border border-black/5 p-3">
                        {Object.entries(selectedServiceDetail.env_vars).map(
                          ([key, value]) => (
                            <div
                              key={key}
                              className="font-mono text-sm break-all flex gap-1.5"
                            >
                              <span className="font-bold text-[#BB6653] shrink-0">
                                {key}
                              </span>
                              <span className="text-[#211a14]/30">=</span>
                              <span className="text-[#211a14]/70">
                                {String(value)}
                              </span>
                            </div>
                          ),
                        )}
                      </div>
                    ) : (
                      <p className="text-sm text-[#211a14]/40">
                        No environment variables
                      </p>
                    )}
                  </div>
                  <div className="bg-white p-4 rounded-2xl border border-black/5 flex flex-col gap-2">
                    <p className="text-[11px] font-bold text-gray-400 uppercase tracking-widest mb-1">
                      Network & Routing
                    </p>
                    <div className="flex items-center gap-2">
                      <Network size={16} className="text-[#BB6653] shrink-0" />
                      {selectedServiceDetail.is_database ? (
                        <span className="break-words text-[#211a14]/60">
                          Internal Only: {selectedServiceDetail.name}:
                          {selectedServiceDetail.container_port}
                        </span>
                      ) : (
                        <span className="break-words font-mono text-[#211a14]/60">
                          {selectedServiceDetail.node_port
                            ? `${window.location.hostname}:${selectedServiceDetail.node_port} → :${selectedServiceDetail.container_port}`
                            : "Waiting for cluster port..."}
                        </span>
                      )}
                    </div>
                  </div>
                  </>
                  )}
                </div>
                <div className="mt-2 flex items-center gap-3 w-full">
                  <button
                    onClick={() => setSelectedServiceId(null)}
                    className="flex-1 py-3 rounded-xl bg-gray-100 hover:bg-gray-200 text-[#211a14] font-bold transition-colors"
                  >
                    Close
                  </button>
                  <button
                    onClick={() => {
                      setEditingService(selectedServiceDetail);
                      setSelectedServiceId(null);
                    }}
                    className="flex-1 py-3 rounded-xl bg-[#FBEFD9] hover:bg-[#f2e0c2] text-[#A96A15] font-bold transition-colors"
                  >
                    Update and Re-deploy
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ย้ายมาจากหน้า General Dashboard — สมาชิกกลุ่มคือคนที่แชร์โควตาก้อนเดียวกับ service ด้านบน */}
      {namespace && (
        <GroupMembers
          namespace={namespace}
          isOwner={user?.id === namespace.contributor_id}
        />
      )}

      {showCreateDatabase && (
        <DeployDatabaseModal
          namespace={namespace}
          onClose={() => setShowCreateDatabase(false)}
          onCreated={(svc) => {
            setServices((prev) => [svc, ...prev]);
            fetchNamespace();
            setShowCreateDatabase(false);
          }}
        />
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

      {editingService && isTemplateDatabase(editingService) && (
        <EditDatabaseModal
          service={editingService}
          namespace={namespace}
          onClose={() => setEditingService(null)}
          onUpdated={(updatedSvc) => {
            setServices((prev) =>
              prev.map((s) => (s.id === updatedSvc.id ? updatedSvc : s)),
            );
            fetchNamespace();
            setEditingService(null);
            setSelectedServiceId(updatedSvc.id);
          }}
        />
      )}

      {editingService && !isTemplateDatabase(editingService) && (
        <EditServiceModal
          service={editingService}
          namespace={namespace}
          onClose={() => setEditingService(null)}
          onUpdated={(updatedSvc) => {
            // อัปเดตรายการลงใน State ทันที
            setServices((prev) =>
              prev.map((s) => (s.id === updatedSvc.id ? updatedSvc : s)),
            );
            fetchNamespace();
            setEditingService(null);
            setSelectedServiceId(updatedSvc.id); // เปิดหน้าต่างรายละเอียดให้ดูสถานะ re-deploy ต่อ
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

function CreateServiceModal({
  namespace,
  onClose,
  onCreated,
}: CreateServiceModalProps) {
  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  // เลือกระดับที่จะใช้เองได้อิสระ ไม่ผูกกับ preset ตายตัวอีกแล้ว — เก็บเป็นหน่วยเดียวกับที่ backend รับ
  const [cpuMilli, setCpuMilli] = useState(500);
  const [ramMb, setRamMb] = useState(512);
  // เก็บเป็น string เพื่อให้ลบจนว่างระหว่างพิมพ์ได้ ไม่เด้งกลับเป็น 0
  const [containerPort, setContainerPort] = useState("8080");
  const [replicas, setReplicas] = useState(1);
  const [envVars, setEnvVars] = useState<EnvPair[]>([{ key: "", value: "" }]);
  const [envNotice, setEnvNotice] = useState<string | null>(null);
  const envFileRef = useRef<HTMLInputElement>(null);

  // ── ดิสก์ถาวร (PVC + 1 pod) สำหรับ web ที่เก็บไฟล์ เช่น Nextcloud (docs 022) ────────────
  // สวิตช์ "ใช้เป็นฐานข้อมูล" ถูกถอดแล้ว — database สร้างจากปุ่ม New Database (template, docs 029)
  const [withStorage, setWithStorage] = useState(false);
  const storageOn = withStorage;
  // เก็บเป็น MB เสมอ ส่วนหน่วยที่โชว์เป็นเรื่องของหน้าจอล้วนๆ ไม่เคยส่งขึ้น API
  const [storageMb, setStorageMb] = useState<number>(STORAGE_BOUNDS.defaultMB);
  const [storageUnit, setStorageUnit] = useState<StorageUnit>("GB");
  // ตำแหน่งที่ image เก็บข้อมูล — ผู้ใช้กรอกเองเสมอ ระบบไม่เดาให้
  const [dataPath, setDataPath] = useState("");

  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // ── NodePort จองให้ตั้งแต่เปิดฟอร์ม (docs 031) ─────────────────────────────────────
  // ผู้ใช้เห็น <host ที่เปิดเว็บ>:<พอร์ต> ก่อนกด deploy และได้เลขนั้นจริง — เลือกเลขเองไม่ได้
  // (ใบจองอายุ 30 นาที เปิดฟอร์มซ้ำได้เลขเดิม · หมดอายุ/ถูกใช้ไปแล้ว backend ตอบ 409 แล้วเราขอใหม่)
  const [nodePort, setNodePort] = useState<number | null>(null);
  const [nodePortError, setNodePortError] = useState<string | null>(null);
  const reserveNodePort = () => {
    setNodePortError(null);
    return serviceApi
      .reserveNodePort()
      .then((r) => setNodePort(r.node_port))
      .catch((err) => {
        setNodePort(null);
        setNodePortError(getApiErrorMessage(err, "จองพอร์ตไม่สำเร็จ"));
      });
  };
  useEffect(() => {
    void reserveNodePort();
  }, []);

  // ตำแหน่งเก็บข้อมูลบังคับกรอกทุกครั้งที่มีดิสก์ ไม่ว่าจะใช้ image อะไร
  const dataPathError = storageOn ? validateDataPath(dataPath) : "";

  // ── โควตา: "ที่มีอยู่จริง" คือเพดานของกลุ่มหักที่ service เดิมกินไปแล้ว ────────────────
  const cpuLimit = namespace?.cpu_limit_milli ?? 0;
  const ramLimit = namespace?.ram_limit_mb ?? 0;
  const cpuUsed = namespace?.usage.used_cpu_milli ?? 0;
  const ramUsed = namespace?.usage.used_ram_mb ?? 0;

  const storageLimit = namespace?.storage_limit_mb ?? 0;
  const storageUsed = namespace?.usage.used_storage_mb ?? 0;

  const cpuAvailable = Math.max(cpuLimit - cpuUsed, 0);
  const ramAvailable = Math.max(ramLimit - ramUsed, 0);
  const storageAvailable = Math.max(storageLimit - storageUsed, 0);

  // มีดิสก์ตรึงที่ 1 pod เสมอ — ค่าที่ใช้คิดโควตาต้องตามนั้น ไม่ใช่ค่าใน dropdown ที่ซ่อนไปแล้ว
  const effectiveReplicas = storageOn ? 1 : replicas;

  // ที่กินจริง = สเปกต่อ Pod x จำนวน Pod (0.5 core x 3 = 1.5 core) ตรงกับที่ backend คิดใน QuotaService
  const cpuTotal = cpuMilli * effectiveReplicas;
  const ramTotal = ramMb * effectiveReplicas;
  const storageTotal = storageOn ? storageMb : 0;

  // ยอดคงเหลือหลังหักตัวที่กำลังจะขอ — ติดลบเมื่อไรคือขอเกิน (backend จะตอบ ErrQuotaExceeded อยู่ดี)
  const cpuRemaining = cpuAvailable - cpuTotal;
  const ramRemaining = ramAvailable - ramTotal;
  const storageRemaining = storageAvailable - storageTotal;
  const overCpu = namespace !== null && cpuRemaining < 0;
  const overRam = namespace !== null && ramRemaining < 0;
  const overStorage = namespace !== null && storageOn && storageRemaining < 0;

  // เพดานแถบเลื่อน = ที่กลุ่มเหลือ ÷ จำนวน pod (ไม่เกินเพดานต่อ service) — ต่ำกว่าค่าขั้นต่ำ = ใช้เต็มแล้ว
  const cpuFit = namespace
    ? floorTo(cpuAvailable / effectiveReplicas, 100)
    : MAX_CPU_MILLI;
  const ramFit = namespace
    ? floorTo(ramAvailable / effectiveReplicas, 128)
    : MAX_RAM_MB;
  const storageFit = namespace
    ? floorTo(storageAvailable, STORAGE_BOUNDS.stepMB)
    : STORAGE_BOUNDS.maxMB;
  const cpuFull = cpuFit < MIN_CPU_MILLI;
  const ramFull = ramFit < MIN_RAM_MB;
  const storageFull = storageFit < STORAGE_BOUNDS.minMB;
  const cpuMax = clamp(cpuFit, MIN_CPU_MILLI, MAX_CPU_MILLI);
  const ramMax = clamp(ramFit, MIN_RAM_MB, MAX_RAM_MB);
  const storageMax = clamp(
    storageFit,
    STORAGE_BOUNDS.minMB,
    STORAGE_BOUNDS.maxMB,
  );

  // เพดานลดลง (โหลดโควตาเสร็จ / เพิ่ม replica) ค่าที่เลือกไว้ต้องลดตาม
  useEffect(() => setCpuMilli((v) => Math.min(v, cpuMax)), [cpuMax]);
  useEffect(() => setRamMb((v) => Math.min(v, ramMax)), [ramMax]);
  useEffect(() => setStorageMb((v) => Math.min(v, storageMax)), [storageMax]);

  const buildEnvMap = () => {
    const env: Record<string, string> = {};
    envVars.forEach(({ key, value }) => {
      if (key.trim()) env[key.trim()] = value;
    });
    return env;
  };

  // ต้องตรงกับ serviceNamePattern ฝั่ง backend: ขึ้นต้นด้วยตัวอักษร — ชื่อ service เป็น host ใน URL
  // ชื่อตัวเลขล้วน (เช่น 123) ถูก client ตีความเป็น IP แล้วต่อไม่ติด (docs 029)
  const NAME_PATTERN = /^[a-z]([a-z0-9-]*[a-z0-9])?$/;
  const nameHasError =
    name.trim().length > 0 && !NAME_PATTERN.test(name.trim());

  // นอกช่วงนี้ backend ตีกลับเป็น 400 ตั้งแต่ binding
  const portNumber = Number(containerPort);
  const portIsValid =
    containerPort.trim() !== "" &&
    Number.isInteger(portNumber) &&
    portNumber >= MIN_CONTAINER_PORT &&
    portNumber <= MAX_CONTAINER_PORT;

  const canSubmit =
    image.trim().length >= 3 &&
    name.trim().length >= 3 &&
    NAME_PATTERN.test(name.trim()) &&
    portIsValid &&
    nodePort !== null &&
    !overCpu &&
    !overRam &&
    !overStorage &&
    // มีดิสก์แล้วต้องบอกตำแหน่งเก็บข้อมูลที่ใช้ได้ก่อน ไม่งั้น backend ตีกลับอยู่ดี
    (!storageOn || dataPathError === "");

  // บอกเหตุผลข้างปุ่มแทนที่จะปล่อยให้ปุ่มเทาเฉยๆ แล้วผู้ใช้เดาเอง
  const blockedReason = !storageOn
    ? null
    : dataPathError
      ? dataPathError
      : overStorage
        ? "พื้นที่เก็บข้อมูลที่ขอเกินโควตาที่กลุ่มเหลืออยู่"
        : null;

  const addEnvRow = () => setEnvVars((p) => [...p, { key: "", value: "" }]);
  const envEnter = useEnvEnterToNext(envVars.length, addEnvRow);
  const removeEnvRow = (i: number) =>
    setEnvVars((p) => p.filter((_, idx) => idx !== i));
  const updateEnvRow = (i: number, field: "key" | "value", val: string) =>
    setEnvVars((p) => {
      const n = [...p];
      n[i] = { ...n[i], [field]: val };
      return n;
    });

  // รวมค่าที่ import เข้ามากับตารางเดิม แล้วรายงานผลให้เห็น — ตัดที่ MAX_ENV_VARS ตั้งแต่ตรงนี้
  const applyParsedEnv = (
    base: EnvPair[],
    parsed: EnvPair[],
    source: string,
  ) => {
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
    applyParsedEnv(
      envVars.filter((_, idx) => idx !== i),
      parsed,
      "ที่วางมา",
    );
  };

  // อีกทางเลือกหนึ่ง: หยิบไฟล์ .env มาทั้งไฟล์เลย — parse ด้วยตัวเดียวกับตอน paste
  const handleEnvFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ""; // เคลียร์เพื่อให้เลือกไฟล์เดิมซ้ำแล้วยัง onChange อีก
    if (!file) return;

    const parsed = parseEnvText(await file.text());
    if (parsed.length === 0) {
      setEnvNotice(
        `อ่านค่าจาก ${file.name} ไม่ได้ — ไฟล์ต้องอยู่ในรูปแบบ KEY=value`,
      );
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
        container_port: portNumber,
        replicas: effectiveReplicas,
        node_port: nodePort ?? undefined,
        // ส่งเฉพาะตอนมีดิสก์ — backend ถือว่าส่ง storage_mb หรือ data_path มาเมื่อไหร่คือขอดิสก์ทันที
        ...(storageOn
          ? { storage_mb: storageMb, data_path: dataPath.trim() }
          : {}),
      });
      onCreated(svc);
    } catch (err) {
      setError(getApiErrorMessage(err, "Deploy ไม่สำเร็จ"));
      // พอร์ตที่จองไว้ใช้ไม่ได้แล้ว — ขอเลขใหม่ให้ทันที ผู้ใช้เห็นเลขใหม่ในการ์ดแล้วกด Deploy อีกครั้งได้เลย
      const code = getApiErrorCode(err);
      if (code === "NODE_PORT_RESERVATION_EXPIRED" || code === "NODE_PORT_TAKEN") {
        void reserveNodePort();
      }
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
            <h2 className="text-2xl font-bold text-[#211a14]">
              Deploy a new service
            </h2>
            <p className="text-base text-[#211a14]/50 mt-0.5">
              Point us at a container image — we handle the rest.
            </p>
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
                nameHasError
                  ? "border-red-300 focus:border-red-400"
                  : "border-black/10",
              )}
            />
            <p
              className={cn(
                "text-sm",
                nameHasError ? "text-red-500" : "text-[#211a14]/40",
              )}
            >
              {nameHasError
                ? "Lowercase letters, numbers and hyphens only — start with a letter, end with a letter or number"
                : "lowercase letters, numbers and hyphens only — start with a letter"}
            </p>
          </div>

          {/* พอร์ตที่ image ฟังอยู่ข้างใน ไม่ใช่พอร์ตที่ใช้เข้าถึงจากข้างนอก (อันนั้นคลัสเตอร์จ่ายให้เองตอน deploy เสร็จ) */}
          <div className="flex flex-col gap-2">
            <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
              Container Port
            </label>
            <div
              className={cn(
                "flex items-center gap-2 rounded-xl border bg-white px-4 py-3",
                containerPort.trim() !== "" && !portIsValid
                  ? "border-red-300"
                  : "border-black/10",
              )}
            >
              <Network size={18} className="text-[#211a14]/30 shrink-0" />
              <input
                type="number"
                min={MIN_CONTAINER_PORT}
                max={MAX_CONTAINER_PORT}
                value={containerPort}
                onChange={(e) => setContainerPort(e.target.value)}
                disabled={submitting}
                placeholder="8080"
                className="w-full bg-transparent text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
              />
            </div>
            <p
              className={cn(
                "text-sm",
                containerPort.trim() !== "" && !portIsValid
                  ? "text-red-500"
                  : "text-[#211a14]/40",
              )}
            >
              {containerPort.trim() !== "" && !portIsValid
                ? `พอร์ตต้องเป็นตัวเลข ${MIN_CONTAINER_PORT}-${MAX_CONTAINER_PORT}`
                : "พอร์ตที่แอปของคุณเปิดรอรับอยู่ข้างใน container (ดูได้จาก EXPOSE ใน Dockerfile) ส่วนพอร์ตที่ใช้เข้าจากข้างนอก ระบบจะจ่ายให้เองหลัง deploy เสร็จ"}
            </p>
          </div>

          {/* database ไม่ได้สร้างจากฟอร์มนี้แล้ว — ชี้ทางไปปุ่ม New Database ให้คนที่มองหาสวิตช์เดิม */}
          <p className="flex items-center gap-2 rounded-xl border border-black/8 bg-white/60 px-4 py-3 text-sm text-[#211a14]/55">
            <Database size={16} className="shrink-0 text-[#BB6653]" />
            ต้องการฐานข้อมูล PostgreSQL / MySQL / MariaDB? ปิดหน้านี้แล้วกดปุ่ม New Database
          </p>

          {/* ── สวิตช์ดิสก์ถาวร ─────────────────────────────────────────────────
              แอปอย่าง Nextcloud/WordPress ต้องเก็บไฟล์ผู้ใช้ถาวรและเปิดให้คนนอกเข้าได้ (docs 022)
              ข้อความตอนปิดเตือนเรื่องที่เจอจริง: image ที่มี VOLUME ดูเหมือนเก็บไฟล์ได้ แต่หายเมื่อ pod ถูกสร้างใหม่ */}
          <button
            type="button"
            role="switch"
            aria-checked={storageOn}
            disabled={submitting}
            onClick={() => setWithStorage((v) => !v)}
            className={cn(
              "flex items-center gap-3 rounded-xl border-2 px-4 py-3 text-left transition-colors",
              storageOn
                ? "border-[#BB6653] bg-[#FBDFDA]"
                : "border-black/10 bg-white hover:border-[#BB6653]/30",
              submitting && "opacity-60",
            )}
          >
            <HardDrive
              size={20}
              className={cn(
                "shrink-0",
                storageOn ? "text-[#BB6653]" : "text-[#211a14]/30",
              )}
            />
            <span className="flex min-w-0 flex-1 flex-col">
              <span
                className={cn(
                  "text-base font-bold",
                  storageOn ? "text-[#BB6653]" : "text-[#211a14]",
                )}
              >
                ดิสก์ถาวร
              </span>
              <span className="text-sm text-[#211a14]/50">
                {storageOn
                  ? "ไฟล์ที่เขียนลงตำแหน่งด้านล่างไม่หายเมื่อ container ถูกสร้างใหม่ · รันได้ครั้งละ 1 container"
                  : "ปิดอยู่ — แอปที่ให้ผู้ใช้อัปโหลดไฟล์และมีการเก็บข้อมูล (ที่ไม่ได้อยู่ใน database) อาจสูญหายได้หากไฟดับหรือ service ล่ม"}
              </span>
            </span>
            <SwitchKnob on={storageOn} />
          </button>

          {/* ── ตำแหน่งเก็บข้อมูล ────────────────────────────────────────────────
              กรอกผิดคือเคสที่อันตรายที่สุดของฟีเจอร์นี้: deploy สำเร็จและดิสก์ถูกจอง แต่ image
              เขียนลงที่อื่น ข้อมูลหายตอน restart แบบไม่มีสัญญาณเตือน คำอธิบายใต้ช่องจึงต้องพูดตรงๆ
              */}
          {storageOn && (
            <div className="flex flex-col gap-2">
              <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                ตำแหน่งที่ image นี้เก็บข้อมูล
              </label>
              <div
                className={cn(
                  "flex items-center gap-2 rounded-xl border bg-white px-4 py-3",
                  dataPath.trim() !== "" && dataPathError
                    ? "border-red-300"
                    : "border-black/10",
                )}
              >
                <HardDrive size={18} className="text-[#211a14]/30 shrink-0" />
                <input
                  value={dataPath}
                  onChange={(e) => setDataPath(e.target.value)}
                  disabled={submitting}
                  placeholder="/var/www/html"
                  spellCheck={false}
                  className="w-full bg-transparent font-mono text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
                />
              </div>
              <p
                className={cn(
                  "text-sm",
                  dataPath.trim() !== "" && dataPathError
                    ? "text-red-500"
                    : "text-[#211a14]/40",
                )}
              >
                {dataPath.trim() !== "" && dataPathError
                  ? dataPathError
                  : "ดูได้จากเอกสารของ image เช่น Nextcloud ใช้ /var/www/html, WordPress ใช้ /var/www/html/wp-content — ถ้ากรอกผิด ระบบจะ deploy สำเร็จแต่ไฟล์จะไม่ถูกเก็บและหายเมื่อ restart"}
              </p>
            </div>
          )}

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
                    max={cpuMax / 1000}
                    value={(cpuMilli / 1000).toFixed(1)}
                    disabled={submitting || cpuFull}
                    onChange={(e) => {
                      const cores = Number(e.target.value);
                      if (!Number.isFinite(cores)) return;
                      setCpuMilli(
                        clamp(Math.round(cores * 1000), MIN_CPU_MILLI, cpuMax),
                      );
                    }}
                    className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                  <span className="text-sm text-[#211a14]/40">cores</span>
                </div>
              </div>
              <input
                type="range"
                min={MIN_CPU_MILLI}
                max={cpuMax}
                step={100}
                value={cpuMilli}
                disabled={submitting || cpuFull}
                onChange={(e) => setCpuMilli(Number(e.target.value))}
                className="w-full accent-[#BB6653] disabled:opacity-50"
              />
              {cpuFull && (
                <p className="text-sm text-red-500">
                  CPU ของกลุ่มถูกใช้เต็มแล้ว
                </p>
              )}
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
                    max={ramMax}
                    value={ramMb}
                    disabled={submitting || ramFull}
                    onChange={(e) => {
                      const mb = Number(e.target.value);
                      if (!Number.isFinite(mb)) return;
                      setRamMb(clamp(Math.round(mb), MIN_RAM_MB, ramMax));
                    }}
                    className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                  <span className="text-sm text-[#211a14]/40">MB</span>
                </div>
              </div>
              <input
                type="range"
                min={MIN_RAM_MB}
                max={ramMax}
                step={128}
                value={ramMb}
                disabled={submitting || ramFull}
                onChange={(e) => setRamMb(Number(e.target.value))}
                className="w-full accent-[#BB6653] disabled:opacity-50"
              />
              {ramFull && (
                <p className="text-sm text-red-500">
                  Memory ของกลุ่มถูกใช้เต็มแล้ว
                </p>
              )}
            </div>

            {/* Storage — อยู่ต่อจาก Memory เพราะหักโควตากลุ่มเหมือนกัน
                ตัวเลือกหน่วยเปลี่ยนแค่ตัวเลขที่โชว์ ไม่ได้เปลี่ยนขนาดจริง (5 GB สลับไป MB ต้องเห็น 5120) */}
            {storageOn && (
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between gap-2">
                  <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                    <HardDrive size={14} className="text-[#BB6653]" /> Storage
                  </label>
                  <div className="flex items-center gap-2">
                    <input
                      type="number"
                      min={STORAGE_BOUNDS.minMB / UNIT_FACTOR[storageUnit]}
                      max={storageMax / UNIT_FACTOR[storageUnit]}
                      step={storageUnit === "GB" ? 1 : STORAGE_BOUNDS.stepMB}
                      value={storageMb / UNIT_FACTOR[storageUnit]}
                      disabled={submitting || storageFull}
                      onChange={(e) => {
                        const n = Number(e.target.value);
                        if (!Number.isFinite(n)) return;
                        setStorageMb(
                          clamp(
                            Math.round(n * UNIT_FACTOR[storageUnit]),
                            STORAGE_BOUNDS.minMB,
                            storageMax,
                          ),
                        );
                      }}
                      className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                    />
                    <select
                      value={storageUnit}
                      disabled={submitting}
                      onChange={(e) =>
                        setStorageUnit(e.target.value as StorageUnit)
                      }
                      className="rounded-lg border border-black/10 bg-white px-2 py-1.5 text-sm text-[#211a14] outline-none disabled:opacity-60"
                    >
                      <option value="GB">GB</option>
                      <option value="MB">MB</option>
                    </select>
                  </div>
                </div>
                <input
                  type="range"
                  min={STORAGE_BOUNDS.minMB}
                  max={storageMax}
                  step={STORAGE_BOUNDS.stepMB}
                  value={storageMb}
                  disabled={submitting || storageFull}
                  onChange={(e) => setStorageMb(Number(e.target.value))}
                  className="w-full accent-[#BB6653] disabled:opacity-50"
                />
                <p
                  className={cn(
                    "text-sm",
                    storageFull ? "text-red-500" : "text-[#211a14]/40",
                  )}
                >
                  {storageFull
                    ? "พื้นที่เก็บข้อมูลของกลุ่มเหลือไม่พอ"
                    : "พื้นที่ดิสก์ถาวรของ service นี้ เปลี่ยนขนาดหลัง deploy ไม่ได้"}
                </p>
              </div>
            )}

            {/* จำนวน Pod ที่รันขนานกัน เอาไว้รองรับโหลด/ทำ HA — ทรัพยากรถูกคูณตามจำนวนนี้
                แต่ไม่กระทบเพดานต่อ service เพราะมันคือการทำซ้ำ Pod
                มีดิสก์ (รวม database) เอา dropdown ออกไปเลยพร้อมบอกเหตุผล ไม่ทิ้งช่องจางๆ ที่กดไม่ได้ไว้
                เพราะช่องที่กดไม่ได้ทำให้คนสงสัยว่าตัวเองทำอะไรผิด */}
            <div className="flex items-center justify-between gap-2">
              <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                <Copy size={14} className="text-[#BB6653]" /> Replicas
              </label>
              {storageOn ? (
                <span className="text-right text-sm text-[#211a14]/45">
                  <span className="font-bold text-[#211a14]/70">1 container</span>
                  <br />
                  ดิสก์ถาวรใช้ได้ทีละ container — สอง container เขียนดิสก์ก้อนเดียวกันแล้วข้อมูลพัง
                </span>
              ) : (
                <select
                  value={replicas}
                  disabled={submitting}
                  onChange={(e) => setReplicas(Number(e.target.value))}
                  className="rounded-lg border border-black/10 bg-white px-3 py-1.5 text-sm text-[#211a14] outline-none disabled:opacity-60"
                >
                  {REPLICA_CHOICES.map((n) => (
                    <option
                      key={n}
                      value={n}
                      // ใช้สเปกต่ำสุดแล้วยังเกินโควตา = เลือกไม่ได้
                      disabled={
                        namespace !== null &&
                        (n * MIN_CPU_MILLI > cpuAvailable ||
                          n * MIN_RAM_MB > ramAvailable)
                      }
                    >
                      {n} {n === 1 ? "container" : "containers"}
                    </option>
                  ))}
                </select>
              )}
            </div>

            <div className="rounded-xl border border-black/8 bg-white/60 p-4">
              {namespace ? (
                <div
                  className={cn(
                    "grid gap-x-5 gap-y-2 text-sm",
                    storageOn
                      ? "grid-cols-[1fr_auto_auto_auto]"
                      : "grid-cols-[1fr_auto_auto]",
                  )}
                >
                  <span className="font-bold uppercase tracking-wider text-[#211a14]/35">
                    Summary
                  </span>
                  <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                    CPU
                  </span>
                  <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                    Memory
                  </span>
                  {storageOn && (
                    <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                      Disk
                    </span>
                  )}

                  <span className="text-[#211a14]/55">Group quota</span>
                  <span className="text-right text-[#211a14]/70">
                    {formatCores(cpuLimit)}
                  </span>
                  <span className="text-right text-[#211a14]/70">
                    {formatRam(ramLimit)}
                  </span>
                  {storageOn && (
                    <span className="text-right text-[#211a14]/70">
                      {formatStorage(storageLimit)}
                    </span>
                  )}

                  <span className="text-[#211a14]/55">
                    In use ({namespace.usage.service_count}{" "}
                    {namespace.usage.service_count === 1
                      ? "service"
                      : "services"}
                    )
                  </span>
                  <span className="text-right text-[#211a14]/70">
                    - {formatCores(cpuUsed)}
                  </span>
                  <span className="text-right text-[#211a14]/70">
                    - {formatRam(ramUsed)}
                  </span>
                  {storageOn && (
                    <span className="text-right text-[#211a14]/70">
                      - {formatStorage(storageUsed)}
                    </span>
                  )}

                  <span className="text-[#211a14]/55">
                    This service
                    {effectiveReplicas > 1 && (
                      <span className="text-[#211a14]/35">
                        {" "}
                        ({formatCores(cpuMilli)} / {formatRam(ramMb)} x{" "}
                        {effectiveReplicas})
                      </span>
                    )}
                  </span>
                  <span className="text-right text-[#BB6653]">
                    - {formatCores(cpuTotal)}
                  </span>
                  <span className="text-right text-[#BB6653]">
                    - {formatRam(ramTotal)}
                  </span>
                  {storageOn && (
                    <span className="text-right text-[#BB6653]">
                      - {formatStorage(storageTotal)}
                    </span>
                  )}

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
                  {storageOn && (
                    <span
                      className={cn(
                        "border-t border-black/5 pt-2 text-right font-bold",
                        overStorage ? "text-red-600" : "text-green-700",
                      )}
                    >
                      {formatStorage(storageRemaining)}
                    </span>
                  )}
                </div>
              ) : (
                <p className="text-sm text-[#211a14]/40">
                  กำลังโหลดโควตาของกลุ่ม...
                </p>
              )}
            </div>

            {(overCpu || overRam || overStorage) && (
              <p className="flex items-start gap-1.5 text-sm text-red-600">
                <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                เกินโควตาที่กลุ่มเหลืออยู่ — ลดขนาดลง หรือลบ service
                ที่ไม่ได้ใช้ออกก่อน
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
                  type="button"
                  onClick={() => envFileRef.current?.click()}
                  disabled={submitting}
                  className="inline-flex items-center gap-1.5 text-sm text-[#211a14]/50 hover:text-[#211a14] transition-colors disabled:opacity-40"
                >
                  <Upload size={14} /> Upload .env
                </button>
                <button
                  type="button"
                  onClick={addEnvRow}
                  disabled={submitting}
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

            <div
              ref={envEnter.listRef}
              className="flex flex-col gap-2 rounded-xl border border-black/8 bg-white/60 p-3"
            >
              {envVars.map((pair, i) => (
                <div key={i} className="flex items-center gap-2">
                  <input
                    placeholder="key"
                    data-env-key
                    value={pair.key}
                    onChange={(e) => updateEnvRow(i, "key", e.target.value)}
                    onPaste={(e) => handleEnvPaste(i, e)}
                    disabled={submitting}
                    className="flex-1 rounded-lg border border-black/8 bg-white px-3 py-2 text-sm font-mono tracking-wide text-[#211a14] placeholder:text-[#211a14]/25 outline-none disabled:opacity-50"
                  />
                  <span className="text-[#211a14]/25 text-sm select-none">
                    =
                  </span>
                  {/* เดาจากชื่อ key ว่าน่าจะเป็นรหัสผ่าน เพื่อไม่ให้ค่าโผล่บนจอให้คนข้างหลังเห็น
                      เดาพลาดไปทางปิดบังเกินดีกว่าเปิดเผยพลาด และไม่กระทบค่าที่ส่งขึ้นระบบ */}
                  <input
                    placeholder="value"
                    type={looksSecret(pair.key) ? "password" : "text"}
                    value={pair.value}
                    onChange={(e) => updateEnvRow(i, "value", e.target.value)}
                    onKeyDown={envEnter.onValueKeyDown(i)}
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
              วางข้อความ key=value หลายบรรทัดลงในช่อง key
              แล้วระบบจะแตกเป็นแถวให้เอง หรือกด Upload .env เพื่อดึงทั้งไฟล์ —
              ค่าเหล่านี้จะถูกใส่ให้ service ตอน deploy (ชื่อ key จะเป็นตัวเล็กหรือตัวใหญ่ก็ได้)
            </p>
          </div>

          {/* ── การเข้าถึง — จุดที่ policy ปรากฏตัวให้ผู้ใช้เห็น ────────────────────
              กล่องนี้เปลี่ยนต่อหน้าตอนกดสวิตช์ ผู้ใช้จึงเข้าใจทันทีว่าแลกอะไรกับอะไร */}
          <div className="flex flex-col gap-2">
            <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
              การเข้าถึง
            </label>
              <div className="flex flex-col gap-1.5 rounded-xl border border-black/8 bg-white/60 p-4">
                <span className="flex items-center gap-1.5 text-sm font-bold text-[#211a14]/60">
                  <Network size={14} className="text-[#BB6653]" />{" "}
                  เข้าถึงได้จากนอกระบบ
                </span>
                <span className="font-mono text-sm text-[#211a14]/70">
                  {window.location.hostname}:
                  {nodePort ?? (nodePortError ? "—" : "กำลังจองพอร์ต...")}{" "}
                  &rarr; :{containerPort || "8080"}
                </span>
                {nodePortError ? (
                  <span className="flex items-center gap-2 text-sm text-red-600">
                    {nodePortError}
                    <button
                      type="button"
                      onClick={() => void reserveNodePort()}
                      className="font-bold underline"
                    >
                      ลองใหม่
                    </button>
                  </span>
                ) : (
                  <span className="text-sm text-[#211a14]/45">
                    พอร์ตนี้จองไว้ให้คุณแล้ว และจะเป็นพอร์ตของ service
                    นี้หลัง deploy · ใครที่อยู่บนเครือข่ายมหาวิทยาลัยและรู้พอร์ตก็เข้าใช้งานได้
                  </span>
                )}
              </div>
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

          <div className="flex items-center gap-3">
            {/* บอกเหตุผลข้างปุ่ม ไม่ปล่อยให้ปุ่มเทาเฉยๆ แล้วผู้ใช้ต้องเดาว่าตกอะไร */}
            {blockedReason && !submitting && (
              <span className="max-w-[16rem] text-right text-sm text-red-600">
                {blockedReason}
              </span>
            )}
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
    </div>
  );
}
// ─── Edit Modal ────────────────────────────────────────────────────────────────
interface EditServiceModalProps {
  service: AppService;
  namespace: NamespaceDetail | null;
  onClose: () => void;
  onUpdated: (svc: AppService) => void;
}

export function EditServiceModal({
  service,
  namespace,
  onClose,
  onUpdated,
}: EditServiceModalProps) {
  // ดึงค่าเดิมจาก service มาใส่เป็นค่าเริ่มต้นทั้งหมด
  const [image, setImage] = useState(service.image || "");
  const [name, setName] = useState(service.name || "");
  const [cpuMilli, setCpuMilli] = useState(service.cpu_milli || 500);
  const [ramMb, setRamMb] = useState(service.ram_mb || 512);
  const [containerPort, setContainerPort] = useState(
    String(service.container_port || "8080"),
  );
  const [replicas, setReplicas] = useState(service.replicas || 1);

  // แปลง JSON Object กลับมาเป็น Array ของ EnvPair เพื่อแสดงในตาราง
  const [envVars, setEnvVars] = useState<EnvPair[]>(() => {
    if (!service.env_vars || Object.keys(service.env_vars).length === 0) {
      return [{ key: "", value: "" }];
    }
    return Object.entries(service.env_vars).map(([key, value]) => ({
      key,
      value: String(value),
    }));
  });

  const [envNotice, setEnvNotice] = useState<string | null>(null);
  const envFileRef = useRef<HTMLInputElement>(null);

  // database ยุคก่อน template คงสถานะเดิม — สลับเป็น/ไม่เป็น database จากฟอร์มนี้ไม่ได้แล้ว (docs 029)
  const isDatabase = service.is_database;
  const [withStorage, setWithStorage] = useState(
    !service.is_database && service.storage_mb > 0,
  );
  const storageOn = isDatabase || withStorage;

  const [storageMb, setStorageMb] = useState<number>(
    service.storage_mb || STORAGE_BOUNDS.defaultMB,
  );
  const [storageUnit, setStorageUnit] = useState<StorageUnit>("GB");
  const [dataPath, setDataPath] = useState(service.data_path || "");

  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // State สำหรับควบคุม 2-Step Verification
  const [showConfirm, setShowConfirm] = useState(false);

  // ผู้ใช้กดยืนยันจากปุ่มด้านล่าง แต่ error อยู่บนสุดของฟอร์ม — เลื่อนกลับขึ้นไปให้เห็นก่อนแก้ไข
  const modalRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (error) modalRef.current?.scrollTo({ top: 0, behavior: "smooth" });
  }, [error]);

  const dataPathError = storageOn ? validateDataPath(dataPath) : "";

  // โควตา: โหมดแก้ไข ต้องเอาที่ service เดิมใช้อยู่ "บวกกลับ" เข้าไปให้ available ก่อน
  const cpuLimit = namespace?.cpu_limit_milli ?? 0;
  const ramLimit = namespace?.ram_limit_mb ?? 0;
  const cpuUsed = namespace?.usage.used_cpu_milli ?? 0;
  const ramUsed = namespace?.usage.used_ram_mb ?? 0;
  const storageLimit = namespace?.storage_limit_mb ?? 0;
  const storageUsed = namespace?.usage.used_storage_mb ?? 0;

  // คืนโควตาที่ service นี้ถืออยู่ ณ ปัจจุบัน
  const originalReplicas =
    service.is_database || service.storage_mb > 0 ? 1 : service.replicas;
  const originalCpuTotal = service.cpu_milli * originalReplicas;
  const originalRamTotal = service.ram_mb * originalReplicas;
  const originalStorageTotal = service.storage_mb || 0;

  // เอาโควตาที่เหลือจริง + โควตาที่ service นี้กอดไว้ = โควตาที่สามารถแก้ไขไปถึงได้
  const cpuAvailable = Math.max(cpuLimit - cpuUsed + originalCpuTotal, 0);
  const ramAvailable = Math.max(ramLimit - ramUsed + originalRamTotal, 0);
  const storageAvailable = Math.max(
    storageLimit - storageUsed + originalStorageTotal,
    0,
  );

  const effectiveReplicas = storageOn ? 1 : replicas;

  const cpuTotal = cpuMilli * effectiveReplicas;
  const ramTotal = ramMb * effectiveReplicas;
  const storageTotal = storageOn ? storageMb : 0;

  const cpuRemaining = cpuAvailable - cpuTotal;
  const ramRemaining = ramAvailable - ramTotal;
  const storageRemaining = storageAvailable - storageTotal;

  const overCpu = namespace !== null && cpuRemaining < 0;
  const overRam = namespace !== null && ramRemaining < 0;
  const overStorage = namespace !== null && storageOn && storageRemaining < 0;

  const cpuFit = namespace
    ? floorTo(cpuAvailable / effectiveReplicas, 100)
    : MAX_CPU_MILLI;
  const ramFit = namespace
    ? floorTo(ramAvailable / effectiveReplicas, 128)
    : MAX_RAM_MB;
  const storageFit = namespace
    ? floorTo(storageAvailable, STORAGE_BOUNDS.stepMB)
    : STORAGE_BOUNDS.maxMB;
  const cpuFull = cpuFit < MIN_CPU_MILLI;
  const ramFull = ramFit < MIN_RAM_MB;
  const storageFull = storageFit < STORAGE_BOUNDS.minMB;
  const cpuMax = clamp(cpuFit, MIN_CPU_MILLI, MAX_CPU_MILLI);
  const ramMax = clamp(ramFit, MIN_RAM_MB, MAX_RAM_MB);
  const storageMax = clamp(
    storageFit,
    STORAGE_BOUNDS.minMB,
    STORAGE_BOUNDS.maxMB,
  );

  useEffect(() => setCpuMilli((v) => Math.min(v, cpuMax)), [cpuMax]);
  useEffect(() => setRamMb((v) => Math.min(v, ramMax)), [ramMax]);
  useEffect(() => setStorageMb((v) => Math.min(v, storageMax)), [storageMax]);

  const buildEnvMap = () => {
    const env: Record<string, string> = {};
    envVars.forEach(({ key, value }) => {
      if (key.trim()) env[key.trim()] = value;
    });
    return env;
  };

  const NAME_PATTERN = /^[a-z]([a-z0-9-]*[a-z0-9])?$/;
  const nameHasError =
    name.trim().length > 0 && !NAME_PATTERN.test(name.trim());

  const portNumber = Number(containerPort);
  const portIsValid =
    containerPort.trim() !== "" &&
    Number.isInteger(portNumber) &&
    portNumber >= MIN_CONTAINER_PORT &&
    portNumber <= MAX_CONTAINER_PORT;

  const canSubmit =
    image.trim().length >= 3 &&
    name.trim().length >= 3 &&
    NAME_PATTERN.test(name.trim()) &&
    portIsValid &&
    !overCpu &&
    !overRam &&
    !overStorage &&
    (!storageOn || dataPathError === "");

  const blockedReason = !storageOn
    ? null
    : dataPathError
      ? dataPathError
      : overStorage
        ? "พื้นที่เก็บข้อมูลที่ขอเกินโควตาที่กลุ่มเหลืออยู่"
        : null;

  const addEnvRow = () => setEnvVars((p) => [...p, { key: "", value: "" }]);
  const envEnter = useEnvEnterToNext(envVars.length, addEnvRow);
  const removeEnvRow = (i: number) =>
    setEnvVars((p) => p.filter((_, idx) => idx !== i));
  const updateEnvRow = (i: number, field: "key" | "value", val: string) =>
    setEnvVars((p) => {
      const n = [...p];
      n[i] = { ...n[i], [field]: val };
      return n;
    });

  const applyParsedEnv = (
    base: EnvPair[],
    parsed: EnvPair[],
    source: string,
  ) => {
    const merged = mergeEnv(base, parsed);
    setEnvVars(merged.slice(0, MAX_ENV_VARS));
    setEnvNotice(
      merged.length > MAX_ENV_VARS
        ? `เพิ่มตัวแปรจาก${source}แล้ว — เก็บได้สูงสุด ${MAX_ENV_VARS} ตัว ส่วนที่เกินถูกตัดออก`
        : `เพิ่ม ${parsed.length} ตัวแปรจาก${source}`,
    );
  };

  const handleEnvPaste = (i: number, e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData("text");
    if (!/[\n=]/.test(text)) return;
    const parsed = parseEnvText(text);
    if (parsed.length === 0) return;
    e.preventDefault();
    applyParsedEnv(
      envVars.filter((_, idx) => idx !== i),
      parsed,
      "ที่วางมา",
    );
  };

  const handleEnvFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    const parsed = parseEnvText(await file.text());
    if (parsed.length === 0) {
      setEnvNotice(
        `อ่านค่าจาก ${file.name} ไม่ได้ — ไฟล์ต้องอยู่ในรูปแบบ KEY=value`,
      );
      return;
    }
    applyParsedEnv(envVars, parsed, ` ${file.name}`);
  };

  // ขั้นตอนที่ 1: ตรวจสอบความถูกต้องและเปิด Modal ยืนยัน
  const handleUpdateClick = () => {
    if (!canSubmit || submitting) return;
    setShowConfirm(true);
  };

  // ขั้นตอนที่ 2: ดำเนินการยิง API เมื่อกดยืนยันแล้ว
  const executeUpdate = async () => {
    setShowConfirm(false);
    setSubmitting(true);
    setError(null);
    try {
      const payload: UpdateServiceDTO = {
        name: name.trim(),
        image: image.trim(),
        env_vars: buildEnvMap(),
        cpu_milli: cpuMilli,
        ram_mb: ramMb,
        container_port: portNumber,
        replicas: effectiveReplicas,
        is_database: isDatabase,
        ...(storageOn
          ? { storage_mb: storageMb, data_path: dataPath.trim() }
          : {}),
      };

      const updatedSvc = await serviceApi.update(service.id, payload);
      onUpdated(updatedSvc);
    } catch (err) {
      setError(getApiErrorMessage(err, "เกิดข้อผิดพลาดที่ไม่ทราบสาเหตุ"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/30 p-4 font-mono">
        <div
          ref={modalRef}
          className="w-full max-w-3xl max-h-[90vh] overflow-y-auto rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl custom-scrollbar animate-in fade-in zoom-in duration-200"
        >
          {/* Header */}
          <div className="flex items-center justify-between px-8 py-6 border-b border-black/5">
            <div>
              <h2 className="text-2xl font-bold text-[#211a14]">
                Edit Service
              </h2>
              <p className="text-base text-[#211a14]/50 mt-0.5">
                Update configuration for{" "}
                <span className="font-bold text-[#BB6653]">{service.name}</span>
                .
              </p>
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
            {/* ส่วนแสดง Error จาก Backend */}
            {error && (
              <div
                role="alert"
                className="flex items-start gap-2 p-3.5 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100"
              >
                <AlertTriangle size={16} className="shrink-0 mt-0.5" />
                <div className="flex flex-col gap-1">
                  <span className="font-bold">Update ไม่สำเร็จ</span>
                  <span className="break-words">{error}</span>
                  <span className="text-red-600/70">
                    แก้ไขข้อมูลด้านล่าง แล้วกด Update and Re-Deploy อีกครั้ง
                  </span>
                </div>
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
                  nameHasError
                    ? "border-red-300 focus:border-red-400"
                    : "border-black/10",
                )}
              />
              <p
                className={cn(
                  "text-sm",
                  nameHasError ? "text-red-500" : "text-[#211a14]/40",
                )}
              >
                {nameHasError
                  ? "Lowercase letters, numbers and hyphens only — start with a letter, end with a letter or number"
                  : "lowercase letters, numbers and hyphens only — start with a letter"}
              </p>
            </div>

            {/* Container Port */}
            <div className="flex flex-col gap-2">
              <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                Container Port
              </label>
              <div
                className={cn(
                  "flex items-center gap-2 rounded-xl border bg-white px-4 py-3",
                  containerPort.trim() !== "" && !portIsValid
                    ? "border-red-300"
                    : "border-black/10",
                )}
              >
                <Network size={18} className="text-[#211a14]/30 shrink-0" />
                <input
                  type="number"
                  min={MIN_CONTAINER_PORT}
                  max={MAX_CONTAINER_PORT}
                  value={containerPort}
                  onChange={(e) => setContainerPort(e.target.value)}
                  disabled={submitting}
                  placeholder="8080"
                  className="w-full bg-transparent text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
                />
              </div>
              <p
                className={cn(
                  "text-sm",
                  containerPort.trim() !== "" && !portIsValid
                    ? "text-red-500"
                    : "text-[#211a14]/40",
                )}
              >
                {containerPort.trim() !== "" && !portIsValid
                  ? `พอร์ตต้องเป็นตัวเลข ${MIN_CONTAINER_PORT}-${MAX_CONTAINER_PORT}`
                  : isDatabase
                    ? "พอร์ตที่ฐานข้อมูลเปิดรอรับอยู่ (เช่น 5432 ของ PostgreSQL, 3306 ของ MySQL)"
                    : "พอร์ตที่แอปของคุณเปิดรอรับอยู่ข้างใน container"}
              </p>
            </div>

            <button
              type="button"
              role="switch"
              aria-checked={storageOn}
              disabled={submitting || isDatabase}
              onClick={() => setWithStorage((v) => !v)}
              className={cn(
                "flex items-center gap-3 rounded-xl border-2 px-4 py-3 text-left transition-colors",
                storageOn
                  ? "border-[#BB6653] bg-[#FBDFDA]"
                  : "border-black/10 bg-white hover:border-[#BB6653]/30",
                isDatabase && "cursor-not-allowed",
                submitting && "opacity-60",
              )}
            >
              <HardDrive
                size={20}
                className={cn(
                  "shrink-0",
                  storageOn ? "text-[#BB6653]" : "text-[#211a14]/30",
                )}
              />
              <span className="flex min-w-0 flex-1 flex-col">
                <span
                  className={cn(
                    "text-base font-bold",
                    storageOn ? "text-[#BB6653]" : "text-[#211a14]",
                  )}
                >
                  ดิสก์ถาวร
                </span>
                <span className="text-sm text-[#211a14]/50">
                  {isDatabase
                    ? "ฐานข้อมูลมีดิสก์ถาวรเสมอ — ปิดสวิตช์นี้ไม่ได้"
                    : storageOn
                      ? "ไฟล์ที่เขียนลงตำแหน่งด้านล่างไม่หายเมื่อ container ถูกสร้างใหม่ · รันได้ครั้งละ 1 container"
                      : "ปิดอยู่ — แอปที่ให้ผู้ใช้อัปโหลดไฟล์และมีการเก็บข้อมูล (ที่ไม่ได้อยู่ใน database) อาจสูญหายได้หากไฟดับหรือ service ล่ม"}
                </span>
              </span>
              {isDatabase && (
                <Lock size={16} className="shrink-0 text-[#BB6653]" />
              )}
              <SwitchKnob on={storageOn} />
            </button>

            {storageOn && (
              <div className="flex flex-col gap-2">
                <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                  ตำแหน่งที่ image นี้เก็บข้อมูล
                </label>
                <div
                  className={cn(
                    "flex items-center gap-2 rounded-xl border bg-white px-4 py-3",
                    dataPath.trim() !== "" && dataPathError
                      ? "border-red-300"
                      : "border-black/10",
                  )}
                >
                  <HardDrive size={18} className="text-[#211a14]/30 shrink-0" />
                  <input
                    value={dataPath}
                    onChange={(e) => setDataPath(e.target.value)}
                    disabled={submitting}
                    placeholder={isDatabase ? "/var/lib/mydb" : "/var/www/html"}
                    spellCheck={false}
                    className="w-full bg-transparent font-mono text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
                  />
                </div>
                <p
                  className={cn(
                    "text-sm",
                    dataPath.trim() !== "" && dataPathError
                      ? "text-red-500"
                      : "text-[#211a14]/40",
                  )}
                >
                  {dataPath.trim() !== "" && dataPathError
                    ? dataPathError
                    : isDatabase
                      ? "ดูได้จากเอกสารของ image เช่น PostgreSQL ใช้ /var/lib/postgresql/data, MySQL ใช้ /var/lib/mysql"
                      : "ดูได้จากเอกสารของ image เช่น Nextcloud ใช้ /var/www/html"}
                </p>
              </div>
            )}

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
                      max={cpuMax / 1000}
                      value={(cpuMilli / 1000).toFixed(1)}
                      disabled={submitting || cpuFull}
                      onChange={(e) => {
                        const cores = Number(e.target.value);
                        if (!Number.isFinite(cores)) return;
                        setCpuMilli(
                          clamp(
                            Math.round(cores * 1000),
                            MIN_CPU_MILLI,
                            cpuMax,
                          ),
                        );
                      }}
                      className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                    />
                    <span className="text-sm text-[#211a14]/40">cores</span>
                  </div>
                </div>
                <input
                  type="range"
                  min={MIN_CPU_MILLI}
                  max={cpuMax}
                  step={100}
                  value={cpuMilli}
                  disabled={submitting || cpuFull}
                  onChange={(e) => setCpuMilli(Number(e.target.value))}
                  className="w-full accent-[#BB6653] disabled:opacity-50"
                />
                {cpuFull && (
                  <p className="text-sm text-red-500">
                    CPU ของกลุ่มถูกใช้เต็มแล้ว
                  </p>
                )}
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
                      max={ramMax}
                      value={ramMb}
                      disabled={submitting || ramFull}
                      onChange={(e) => {
                        const mb = Number(e.target.value);
                        if (!Number.isFinite(mb)) return;
                        setRamMb(clamp(Math.round(mb), MIN_RAM_MB, ramMax));
                      }}
                      className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                    />
                    <span className="text-sm text-[#211a14]/40">MB</span>
                  </div>
                </div>
                <input
                  type="range"
                  min={MIN_RAM_MB}
                  max={ramMax}
                  step={128}
                  value={ramMb}
                  disabled={submitting || ramFull}
                  onChange={(e) => setRamMb(Number(e.target.value))}
                  className="w-full accent-[#BB6653] disabled:opacity-50"
                />
                {ramFull && (
                  <p className="text-sm text-red-500">
                    Memory ของกลุ่มถูกใช้เต็มแล้ว
                  </p>
                )}
              </div>

              {storageOn && (
                <div className="flex flex-col gap-2">
                  <div className="flex items-center justify-between gap-2">
                    <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                      <HardDrive size={14} className="text-[#BB6653]" /> Storage
                    </label>
                    <div className="flex items-center gap-2">
                      <input
                        type="number"
                        min={STORAGE_BOUNDS.minMB / UNIT_FACTOR[storageUnit]}
                        max={storageMax / UNIT_FACTOR[storageUnit]}
                        step={storageUnit === "GB" ? 1 : STORAGE_BOUNDS.stepMB}
                        value={storageMb / UNIT_FACTOR[storageUnit]}
                        disabled={submitting || storageFull}
                        onChange={(e) => {
                          const n = Number(e.target.value);
                          if (!Number.isFinite(n)) return;
                          setStorageMb(
                            clamp(
                              Math.round(n * UNIT_FACTOR[storageUnit]),
                              STORAGE_BOUNDS.minMB,
                              storageMax,
                            ),
                          );
                        }}
                        className="w-20 rounded-lg border border-black/10 bg-white px-2.5 py-1.5 text-right text-sm text-[#211a14] outline-none disabled:opacity-60"
                      />
                      <select
                        value={storageUnit}
                        disabled={submitting}
                        onChange={(e) =>
                          setStorageUnit(e.target.value as StorageUnit)
                        }
                        className="rounded-lg border border-black/10 bg-white px-2 py-1.5 text-sm text-[#211a14] outline-none disabled:opacity-60"
                      >
                        <option value="GB">GB</option>
                        <option value="MB">MB</option>
                      </select>
                    </div>
                  </div>
                  <input
                    type="range"
                    min={STORAGE_BOUNDS.minMB}
                    max={storageMax}
                    step={STORAGE_BOUNDS.stepMB}
                    value={storageMb}
                    disabled={submitting || storageFull}
                    onChange={(e) => setStorageMb(Number(e.target.value))}
                    className="w-full accent-[#BB6653] disabled:opacity-50"
                  />
                </div>
              )}

              <div className="flex items-center justify-between gap-2">
                <label className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
                  <Copy size={14} className="text-[#BB6653]" /> Replicas
                </label>
                {storageOn ? (
                  <span className="text-right text-sm text-[#211a14]/45">
                    <span className="font-bold text-[#211a14]/70">1 container</span>
                  </span>
                ) : (
                  <select
                    value={replicas}
                    disabled={submitting}
                    onChange={(e) => setReplicas(Number(e.target.value))}
                    className="rounded-lg border border-black/10 bg-white px-3 py-1.5 text-sm text-[#211a14] outline-none disabled:opacity-60"
                  >
                    {REPLICA_CHOICES.map((n) => (
                      <option
                        key={n}
                        value={n}
                        disabled={
                          namespace !== null &&
                          (n * MIN_CPU_MILLI > cpuAvailable ||
                            n * MIN_RAM_MB > ramAvailable)
                        }
                      >
                        {n} {n === 1 ? "container" : "containers"}
                      </option>
                    ))}
                  </select>
                )}
              </div>

              <div className="rounded-xl border border-black/8 bg-white/60 p-4">
                {namespace ? (
                  <div
                    className={cn(
                      "grid gap-x-5 gap-y-2 text-sm",
                      storageOn
                        ? "grid-cols-[1fr_auto_auto_auto]"
                        : "grid-cols-[1fr_auto_auto]",
                    )}
                  >
                    <span className="font-bold uppercase tracking-wider text-[#211a14]/35">
                      Summary
                    </span>
                    <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                      CPU
                    </span>
                    <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                      Memory
                    </span>
                    {storageOn && (
                      <span className="text-right font-bold uppercase tracking-wider text-[#211a14]/35">
                        Disk
                      </span>
                    )}

                    <span className="text-[#211a14]/55">Group quota</span>
                    <span className="text-right text-[#211a14]/70">
                      {formatCores(cpuLimit)}
                    </span>
                    <span className="text-right text-[#211a14]/70">
                      {formatRam(ramLimit)}
                    </span>
                    {storageOn && (
                      <span className="text-right text-[#211a14]/70">
                        {formatStorage(storageLimit)}
                      </span>
                    )}

                    <span className="text-[#211a14]/55">
                      In use (excluding this)
                    </span>
                    <span className="text-right text-[#211a14]/70">
                      - {formatCores(cpuUsed - originalCpuTotal)}
                    </span>
                    <span className="text-right text-[#211a14]/70">
                      - {formatRam(ramUsed - originalRamTotal)}
                    </span>
                    {storageOn && (
                      <span className="text-right text-[#211a14]/70">
                        - {formatStorage(storageUsed - originalStorageTotal)}
                      </span>
                    )}

                    <span className="text-[#211a14]/55">
                      This service (Editing)
                    </span>
                    <span className="text-right text-[#BB6653]">
                      - {formatCores(cpuTotal)}
                    </span>
                    <span className="text-right text-[#BB6653]">
                      - {formatRam(ramTotal)}
                    </span>
                    {storageOn && (
                      <span className="text-right text-[#BB6653]">
                        - {formatStorage(storageTotal)}
                      </span>
                    )}

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
                    {storageOn && (
                      <span
                        className={cn(
                          "border-t border-black/5 pt-2 text-right font-bold",
                          overStorage ? "text-red-600" : "text-green-700",
                        )}
                      >
                        {formatStorage(storageRemaining)}
                      </span>
                    )}
                  </div>
                ) : (
                  <p className="text-sm text-[#211a14]/40">
                    กำลังโหลดโควตาของกลุ่ม...
                  </p>
                )}
              </div>

              {(overCpu || overRam || overStorage) && (
                <p className="flex items-start gap-1.5 text-sm text-red-600">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  เกินโควตาที่กลุ่มเหลืออยู่ — ลดขนาดลง หรือลบ service
                  ที่ไม่ได้ใช้ออกก่อน
                </p>
              )}
            </div>

            {/* Environment Variables */}
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                  Environment Variables
                </label>
                <div className="flex items-center gap-4">
                  <button
                    type="button"
                    onClick={() => envFileRef.current?.click()}
                    disabled={submitting}
                    className="inline-flex items-center gap-1.5 text-sm text-[#211a14]/50 hover:text-[#211a14] transition-colors disabled:opacity-40"
                  >
                    <Upload size={14} /> Upload .env
                  </button>
                  <button
                    type="button"
                    onClick={addEnvRow}
                    disabled={submitting}
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

              <div
                ref={envEnter.listRef}
                className="flex flex-col gap-2 rounded-xl border border-black/8 bg-white/60 p-3"
              >
                {envVars.map((pair, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <input
                      placeholder="key"
                      data-env-key
                      value={pair.key}
                      onChange={(e) => updateEnvRow(i, "key", e.target.value)}
                      onPaste={(e) => handleEnvPaste(i, e)}
                      disabled={submitting}
                      className="flex-1 rounded-lg border border-black/8 bg-white px-3 py-2 text-sm font-mono tracking-wide text-[#211a14] placeholder:text-[#211a14]/25 outline-none disabled:opacity-50"
                    />
                    <span className="text-[#211a14]/25 text-sm select-none">
                      =
                    </span>
                    <input
                      placeholder="value"
                      type={looksSecret(pair.key) ? "password" : "text"}
                      value={pair.value}
                      onChange={(e) => updateEnvRow(i, "value", e.target.value)}
                      onKeyDown={envEnter.onValueKeyDown(i)}
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
              {envNotice && (
                <p className="text-sm text-[#BB6653]">{envNotice}</p>
              )}
            </div>

            <div className="flex flex-col gap-2">
              <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                การเข้าถึง
              </label>
              {isDatabase ? (
                <div className="flex flex-col gap-1.5 rounded-xl border border-green-600/20 bg-green-50 p-4">
                  <span className="flex items-center gap-1.5 text-sm font-bold text-green-700">
                    <Lock size={14} /> เฉพาะภายในกลุ่มของคุณ
                  </span>
                  <span className="overflow-x-auto whitespace-nowrap font-mono text-sm text-[#211a14]/70">
                    {`${name.trim() || "ชื่อ-service"}.ns-${namespace?.id ?? "<id>"}.svc.cluster.local:${containerPort || "8080"}`}
                  </span>
                </div>
              ) : (
                <div className="flex flex-col gap-1.5 rounded-xl border border-black/8 bg-white/60 p-4">
                  <span className="flex items-center gap-1.5 text-sm font-bold text-[#211a14]/60">
                    <Network size={14} className="text-[#BB6653]" />{" "}
                    เข้าถึงได้จากนอกระบบ
                  </span>
                  <span className="font-mono text-sm text-[#211a14]/70">
                    {window.location.hostname}:
                    {service.node_port ?? "<พอร์ตที่ระบบจ่ายให้>"} &rarr; :
                    {containerPort || "8080"}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* Footer */}
          <div className="flex items-center justify-between gap-2 px-8 py-5 border-t border-black/5 bg-[#FFF8E8] sticky bottom-0 rounded-b-3xl">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded-xl px-5 py-3 text-base font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
            >
              Cancel
            </button>

            <div className="flex items-center gap-3">
              {blockedReason && !submitting && (
                <span className="max-w-[16rem] text-right text-sm text-red-600">
                  {blockedReason}
                </span>
              )}
              <button
                type="button"
                disabled={!canSubmit || submitting}
                onClick={handleUpdateClick}
                className={cn(
                  "inline-flex items-center gap-2 rounded-xl px-6 py-3 text-base font-bold text-white shadow-md transition-all",
                  canSubmit && !submitting
                    ? "bg-[#BB6653] hover:bg-[#F08B51]"
                    : "bg-[#211a14]/20 cursor-not-allowed shadow-none",
                )}
              >
                {submitting && <Loader2 size={16} className="animate-spin" />}
                {submitting ? "Re-Deploying..." : "Update and Re-Deploy"}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* หน้าต่างยืนยันการทำรายการ (2-Step Verification) */}
      {showConfirm && (
        <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4">
          <div className="bg-white rounded-3xl p-6 w-full max-w-sm shadow-2xl flex flex-col gap-4 animate-in zoom-in-95 duration-200">
            <div className="flex items-center gap-3 text-red-600">
              <div className="p-2 bg-red-50 border border-red-100 rounded-xl">
                <AlertTriangle size={24} />
              </div>
              <h3 className="text-lg font-bold">ยืนยันการดำเนินการ</h3>
            </div>

            <p className="text-[#211a14]/70 text-sm leading-relaxed">
              การแก้ไขข้อมูล จะทำการลบ database เก่าของผู้ใช้งานออกไปทั้งหมด
              คุณจะดำเนินการต่อหรือไม่
            </p>

            <div className="flex items-center gap-3 mt-2">
              <button
                type="button"
                onClick={() => setShowConfirm(false)}
                disabled={submitting}
                className="flex-1 py-2.5 rounded-xl font-bold text-[#211a14]/60 bg-gray-100 hover:bg-gray-200 transition-colors"
              >
                ยกเลิก
              </button>
              <button
                type="button"
                onClick={() => executeUpdate()}
                disabled={submitting}
                className="flex-1 py-2.5 rounded-xl font-bold text-white bg-red-600 hover:bg-red-700 shadow-md transition-colors"
              >
                ดำเนินการต่อ
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// ปุ่มเลื่อนของสวิตช์ในฟอร์ม — ใช้ร่วมกันระหว่างสวิตช์ฐานข้อมูลกับสวิตช์ดิสก์ถาวร
function SwitchKnob({ on }: { on: boolean }) {
  return (
    <span
      className={cn(
        "relative h-7 w-12 shrink-0 rounded-full transition-colors",
        on ? "bg-[#BB6653]" : "bg-black/15",
      )}
    >
      <span
        className={cn(
          "absolute top-1 size-5 rounded-full bg-white shadow transition-all",
          on ? "left-6" : "left-1",
        )}
      />
    </span>
  );
}
