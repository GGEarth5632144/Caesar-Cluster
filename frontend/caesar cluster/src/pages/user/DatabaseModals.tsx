import { useEffect, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  Check,
  Copy,
  Cpu,
  Database,
  Eye,
  EyeOff,
  HardDrive,
  KeyRound,
  Layers,
  Loader2,
  Lock,
  RefreshCw,
  X,
} from "lucide-react";

import { cn } from "@/lib/utils";
import {
  serviceApi,
  type AppService,
  type DatabaseConnection,
  type DatabaseEngineName,
  type DatabaseTemplate,
} from "@/api/services";
import { type NamespaceDetail } from "@/api/namespace";
import { getApiErrorMessage } from "@/api/authApi";
import { formatStorage, STORAGE_BOUNDS } from "@/config/database";

// database จาก template (docs 029): ผู้ใช้เลือก engine/version แล้วกรอกแค่ username/password/ชื่อ database
// image, พอร์ต, จุด mount และ env มาจาก catalog ฝั่ง backend ทั้งหมด — ตั้งค่าผิดจนข้อมูลหายไม่ได้อีก

// ต้องตรงกับเพดานของ service 1 ตัวฝั่ง backend (dto.CreateDatabaseRequest)
const MIN_CPU_MILLI = 100;
const MAX_CPU_MILLI = 3000;
const MAX_RAM_MB = 2048;

// ── กติกาเดียวกับ services.validateDatabaseCredentials ฝั่ง Go (ที่นี่แค่บอกก่อนกดส่ง) ─────────
const SERVICE_NAME_PATTERN = /^[a-z]([a-z0-9-]*[a-z0-9])?$/;
const USERNAME_PATTERN = /^[a-z_][a-z0-9_]{2,31}$/;
const DB_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,62}$/;
const RESERVED_USERNAMES = ["root", "mysql", "mariadb", "information_schema", "performance_schema", "sys"];
const RESERVED_DB_NAMES: Record<DatabaseEngineName, string[]> = {
  postgresql: ["template0", "template1"],
  mysql: ["mysql", "information_schema", "performance_schema", "sys"],
  mariadb: ["mysql", "information_schema", "performance_schema", "sys"],
};

function usernameError(engine: DatabaseEngineName, u: string): string {
  if (!USERNAME_PATTERN.test(u)) return "a-z, 0-9, _ ยาว 3–32 ตัว และไม่ขึ้นต้นด้วยตัวเลข";
  if (RESERVED_USERNAMES.includes(u)) return `'${u}' เป็นชื่อที่ระบบใช้อยู่แล้ว`;
  if (engine === "postgresql" && u.startsWith("pg_")) return "PostgreSQL ห้ามขึ้นต้นด้วย pg_";
  return "";
}

function passwordError(p: string): string {
  if (p.length < 8 || p.length > 64) return "ยาว 8–64 ตัวอักษร";
  // ห้ามช่องว่าง ' " ` \ — entrypoint ของ image เอาไปประกอบ SQL และ driver หลายตัว parse ต่างกัน
  if (/[^\x21-\x7e]|['"`\\]/.test(p)) return "ใช้ได้เฉพาะตัวอักษรอังกฤษ ตัวเลข และสัญลักษณ์ (ห้ามช่องว่าง ' \" ` \\)";
  return "";
}

function databaseNameError(engine: DatabaseEngineName, d: string): string {
  if (!DB_NAME_PATTERN.test(d)) return "A-Z, a-z, 0-9, _ ยาวไม่เกิน 63 ตัว และไม่ขึ้นต้นด้วยตัวเลข";
  if (RESERVED_DB_NAMES[engine].includes(d.toLowerCase())) return `'${d}' เป็นชื่อของระบบ`;
  return "";
}

// สุ่มรหัสจากชุดอักขระที่ไม่ต้อง escape ใน SQL/shell — สัญลักษณ์ที่มีถูก encode ใน URL ให้อยู่แล้ว
const PASSWORD_CHARS = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_!@#%";
function generatePassword(length = 20): string {
  const bytes = crypto.getRandomValues(new Uint32Array(length));
  return Array.from(bytes, (b) => PASSWORD_CHARS[b % PASSWORD_CHARS.length]).join("");
}

const clamp = (v: number, min: number, max: number) => Math.min(Math.max(v, min), max);
const floorTo = (v: number, step: number) => Math.floor(v / step) * step;
const formatCores = (milli: number) => `${(milli / 1000).toFixed(1)} cores`;
const formatRam = (mb: number) => (Math.abs(mb) >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`);

// version ที่เหลืออายุ support ไม่ถึง 90 วัน — เตือนให้เห็นก่อนเลือก (ยังเลือกได้จนถึงวัน EOL)
function eolSoon(eol: string): boolean {
  return new Date(eol).getTime() - Date.now() < 90 * 24 * 60 * 60 * 1000;
}

function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">{label}</label>
      {children}
      {(error || hint) && (
        <p className={cn("text-sm", error ? "text-red-500" : "text-[#211a14]/40")}>{error || hint}</p>
      )}
    </div>
  );
}

const inputClass = (bad: boolean) =>
  cn(
    "w-full rounded-xl border bg-white px-4 py-3 font-mono text-base text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60",
    bad ? "border-red-300 focus:border-red-400" : "border-black/10",
  );

// ─── Deploy Database ─────────────────────────────────────────────────────────

interface DeployDatabaseModalProps {
  namespace: NamespaceDetail | null;
  onClose: () => void;
  onCreated: (svc: AppService) => void;
}

export function DeployDatabaseModal({ namespace, onClose, onCreated }: DeployDatabaseModalProps) {
  const [templates, setTemplates] = useState<DatabaseTemplate[] | null>(null);
  const [engine, setEngine] = useState<DatabaseEngineName>("postgresql");
  const [version, setVersion] = useState("");
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [database, setDatabase] = useState("");
  const [cpuMilli, setCpuMilli] = useState(500);
  const [ramMb, setRamMb] = useState(512);
  const [storageMb, setStorageMb] = useState<number>(STORAGE_BOUNDS.defaultMB);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    serviceApi
      .databaseTemplates()
      .then(setTemplates)
      .catch((err) => setError(getApiErrorMessage(err, "โหลดรายการ database ไม่สำเร็จ")));
  }, []);

  const template = templates?.find((t) => t.engine === engine) ?? null;
  const minRam = template?.min_ram_mb ?? 256;

  // เปลี่ยน engine = กลับไปใช้ version default ของ engine นั้น และดัน RAM ขึ้นถึงขั้นต่ำ
  useEffect(() => {
    if (!template) return;
    setVersion(template.versions.find((v) => v.default)?.version ?? template.versions[0]?.version ?? "");
    setRamMb((v) => Math.max(v, template.min_ram_mb));
  }, [template]);

  // ── โควตา: ที่กลุ่มเหลือจริง (database = 1 pod เสมอ) ─────────────────────────────
  const cpuAvailable = Math.max((namespace?.cpu_limit_milli ?? 0) - (namespace?.usage.used_cpu_milli ?? 0), 0);
  const ramAvailable = Math.max((namespace?.ram_limit_mb ?? 0) - (namespace?.usage.used_ram_mb ?? 0), 0);
  const storageAvailable = Math.max(
    (namespace?.storage_limit_mb ?? 0) - (namespace?.usage.used_storage_mb ?? 0),
    0,
  );
  const cpuMax = namespace ? clamp(floorTo(cpuAvailable, 100), MIN_CPU_MILLI, MAX_CPU_MILLI) : MAX_CPU_MILLI;
  const ramMax = namespace ? clamp(floorTo(ramAvailable, 128), minRam, MAX_RAM_MB) : MAX_RAM_MB;
  const storageMax = namespace
    ? clamp(floorTo(storageAvailable, STORAGE_BOUNDS.stepMB), STORAGE_BOUNDS.minMB, STORAGE_BOUNDS.maxMB)
    : STORAGE_BOUNDS.maxMB;
  const overCpu = namespace !== null && cpuMilli > cpuAvailable;
  const overRam = namespace !== null && ramMb > ramAvailable;
  const overStorage = namespace !== null && storageMb > storageAvailable;

  useEffect(() => setCpuMilli((v) => Math.min(v, cpuMax)), [cpuMax]);
  useEffect(() => setRamMb((v) => Math.min(v, ramMax)), [ramMax]);
  useEffect(() => setStorageMb((v) => Math.min(v, storageMax)), [storageMax]);

  const nameErr = name && !SERVICE_NAME_PATTERN.test(name) ? "ขึ้นต้นด้วย a-z ตามด้วยตัวพิมพ์เล็ก/ตัวเลข/ขีดกลาง" : "";
  const userErr = username ? usernameError(engine, username) : "";
  const passErr = password ? passwordError(password) : "";
  const dbErr = database ? databaseNameError(engine, database) : "";

  const canSubmit =
    template !== null &&
    version !== "" &&
    name.length >= 3 &&
    !nameErr &&
    username !== "" &&
    !userErr &&
    password !== "" &&
    !passErr &&
    database !== "" &&
    !dbErr &&
    ramMb >= minRam &&
    !overCpu &&
    !overRam &&
    !overStorage;

  const handleDeploy = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const svc = await serviceApi.createDatabase({
        engine,
        version,
        name,
        username,
        password,
        database,
        storage_mb: storageMb,
        cpu_milli: cpuMilli,
        ram_mb: ramMb,
      });
      onCreated(svc);
    } catch (err) {
      setError(getApiErrorMessage(err, "Deploy database ไม่สำเร็จ"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 font-mono">
      <div className="w-full max-w-3xl max-h-[90vh] overflow-y-auto rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl">
        <div className="flex items-center justify-between px-8 py-6 border-b border-black/5">
          <div>
            <h2 className="text-2xl font-bold text-[#211a14]">Deploy a database</h2>
            <p className="text-base text-[#211a14]/50 mt-0.5">
              เลือก engine กับ version แล้วตั้งชื่อผู้ใช้/รหัสผ่าน — ระบบตั้งค่าที่เหลือให้
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

        <div className="px-8 py-6 flex flex-col gap-6">
          {error && (
            <div className="flex items-start gap-2 p-3.5 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100">
              <AlertTriangle size={16} className="shrink-0 mt-0.5" /> {error}
            </div>
          )}

          {/* engine */}
          <Field label="Engine">
            {templates === null ? (
              <p className="flex items-center gap-2 text-sm text-[#211a14]/40">
                <Loader2 size={14} className="animate-spin" /> กำลังโหลด...
              </p>
            ) : (
              <div className="grid gap-3 sm:grid-cols-3">
                {templates.map((t) => (
                  <button
                    key={t.engine}
                    type="button"
                    disabled={submitting}
                    onClick={() => setEngine(t.engine)}
                    className={cn(
                      "flex flex-col items-start gap-1 rounded-xl border-2 px-4 py-3 text-left transition-colors disabled:opacity-60",
                      engine === t.engine
                        ? "border-[#BB6653] bg-[#FBDFDA]"
                        : "border-black/10 bg-white hover:border-[#BB6653]/30",
                    )}
                  >
                    <span className="flex items-center gap-2 text-base font-bold text-[#211a14]">
                      <Database size={18} className="text-[#BB6653]" /> {t.label}
                    </span>
                    <span className="text-sm text-[#211a14]/50">
                      port {t.port} · RAM ≥ {formatRam(t.min_ram_mb)}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </Field>

          {/* version */}
          {template && (
            <Field
              label="Version"
              hint="แสดงเฉพาะ version ที่ยังได้รับ support · เปลี่ยน version หลัง deploy ไม่ได้"
            >
              <div className="flex flex-wrap gap-2">
                {template.versions.map((v) => (
                  <button
                    key={v.version}
                    type="button"
                    disabled={submitting}
                    onClick={() => setVersion(v.version)}
                    title={`support ถึง ${v.eol}`}
                    className={cn(
                      "flex flex-col items-start rounded-xl border-2 px-3 py-2 text-left transition-colors disabled:opacity-60",
                      version === v.version
                        ? "border-[#BB6653] bg-[#FBDFDA]"
                        : "border-black/10 bg-white hover:border-[#BB6653]/30",
                    )}
                  >
                    <span className="flex items-center gap-1.5 text-base font-bold text-[#211a14]">
                      {v.version}
                      {v.lts && (
                        <span className="rounded bg-green-100 px-1 text-xs font-bold text-green-700">LTS</span>
                      )}
                      {v.default && (
                        <span className="rounded bg-[#FBEFD9] px-1 text-xs font-bold text-[#A96A15]">แนะนำ</span>
                      )}
                    </span>
                    <span className={cn("text-xs", eolSoon(v.eol) ? "text-red-600" : "text-[#211a14]/45")}>
                      {eolSoon(v.eol) ? "⚠ " : ""}support ถึง {v.eol}
                    </span>
                  </button>
                ))}
              </div>
            </Field>
          )}

          <Field
            label="Service Name"
            error={nameErr}
            hint="ใช้เป็น host ตอนเชื่อมต่อจาก service อื่นในกลุ่ม เช่น mydb:5432"
          >
            <input
              value={name}
              onChange={(e) => setName(e.target.value.trim())}
              disabled={submitting}
              placeholder="mydb"
              spellCheck={false}
              className={inputClass(!!nameErr)}
            />
          </Field>

          <div className="grid gap-6 sm:grid-cols-2">
            <Field label="Username" error={userErr}>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value.trim())}
                disabled={submitting}
                placeholder="appuser"
                autoComplete="off"
                spellCheck={false}
                className={inputClass(!!userErr)}
              />
            </Field>
            <Field label="Database Name" error={dbErr}>
              <input
                value={database}
                onChange={(e) => setDatabase(e.target.value.trim())}
                disabled={submitting}
                placeholder="appdb"
                spellCheck={false}
                className={inputClass(!!dbErr)}
              />
            </Field>
          </div>

          <Field
            label="Password"
            error={passErr}
            hint="เก็บใน Kubernetes Secret · ดูได้อีกครั้งที่หน้ารายละเอียด · เปลี่ยนหลัง deploy ไม่ได้"
          >
            <div className="flex items-center gap-2">
              <div
                className={cn(
                  "flex flex-1 items-center gap-2 rounded-xl border bg-white px-4 py-3",
                  passErr ? "border-red-300" : "border-black/10",
                )}
              >
                <KeyRound size={18} className="text-[#211a14]/30 shrink-0" />
                <input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={submitting}
                  autoComplete="new-password"
                  className="w-full bg-transparent font-mono text-base text-[#211a14] outline-none disabled:opacity-60"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((v) => !v)}
                  className="text-[#211a14]/40 hover:text-[#211a14]"
                  title={showPassword ? "ซ่อน" : "แสดง"}
                >
                  {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
              <button
                type="button"
                disabled={submitting}
                onClick={() => {
                  setPassword(generatePassword());
                  setShowPassword(true);
                }}
                className="inline-flex items-center gap-1.5 rounded-xl border border-black/10 bg-white px-3 py-3 text-sm font-bold text-[#211a14]/60 hover:text-[#BB6653] disabled:opacity-50"
              >
                <RefreshCw size={14} /> สุ่ม
              </button>
            </div>
          </Field>

          {/* resource — ขั้นต่ำ RAM ตาม engine (วัดจริงบน worker) */}
          <div className="flex flex-col gap-4">
            <label className="text-sm font-bold uppercase tracking-wider text-[#BB6653]">Resource</label>
            <RangeRow
              icon={<Cpu size={14} className="text-[#BB6653]" />}
              label="CPU"
              value={cpuMilli}
              min={MIN_CPU_MILLI}
              max={cpuMax}
              step={100}
              display={formatCores(cpuMilli)}
              disabled={submitting}
              onChange={setCpuMilli}
            />
            <RangeRow
              icon={<Layers size={14} className="text-[#BB6653]" />}
              label="Memory"
              value={ramMb}
              min={minRam}
              max={ramMax}
              step={128}
              display={formatRam(ramMb)}
              disabled={submitting}
              onChange={setRamMb}
              note={`${template?.label ?? "database นี้"} ต้องใช้อย่างน้อย ${formatRam(minRam)}`}
            />
            <RangeRow
              icon={<HardDrive size={14} className="text-[#BB6653]" />}
              label="Storage"
              value={storageMb}
              min={STORAGE_BOUNDS.minMB}
              max={storageMax}
              step={STORAGE_BOUNDS.stepMB}
              display={formatStorage(storageMb)}
              disabled={submitting}
              onChange={setStorageMb}
              note="ดิสก์ถาวรของ database · เปลี่ยนขนาดหลัง deploy ไม่ได้ · ลบ database = ข้อมูลหาย"
            />
            {namespace && (
              <p className={cn("text-sm", overCpu || overRam || overStorage ? "text-red-600" : "text-[#211a14]/45")}>
                กลุ่มเหลือ {formatCores(cpuAvailable)} · {formatRam(ramAvailable)} · {formatStorage(storageAvailable)}
                {(overCpu || overRam || overStorage) && " — เกินโควตาที่เหลือ"}
              </p>
            )}
          </div>

          <div className="flex flex-col gap-1.5 rounded-xl border border-green-600/20 bg-green-50 p-4">
            <span className="flex items-center gap-1.5 text-sm font-bold text-green-700">
              <Lock size={14} /> เชื่อมต่อได้เฉพาะ service ในกลุ่มของคุณ
            </span>
            <span className="overflow-x-auto whitespace-nowrap font-mono text-sm text-[#211a14]/70">
              {`${template?.engine === "postgresql" ? "postgresql" : "mysql"}://${username || "user"}:••••@${name || "mydb"}:${template?.port ?? 5432}/${database || "db"}`}
            </span>
            <span className="text-sm text-[#211a14]/45">คนนอกกลุ่มและคนนอกระบบเข้าไม่ได้ · 1 container</span>
          </div>
        </div>

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
            {submitting ? "Deploying..." : "Deploy database"}
          </button>
        </div>
      </div>
    </div>
  );
}

function RangeRow({
  icon,
  label,
  value,
  min,
  max,
  step,
  display,
  disabled,
  onChange,
  note,
}: {
  icon: ReactNode;
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  display: string;
  disabled: boolean;
  onChange: (v: number) => void;
  note?: string;
}) {
  const full = max < min;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-1.5 text-sm text-[#211a14]/50">
          {icon} {label}
        </span>
        <span className="text-sm font-bold text-[#211a14]/70">{display}</span>
      </div>
      <input
        type="range"
        min={min}
        max={Math.max(max, min)}
        step={step}
        value={value}
        disabled={disabled || full}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-[#BB6653] disabled:opacity-50"
      />
      {note && <p className="text-sm text-[#211a14]/40">{note}</p>}
    </div>
  );
}

// ─── Edit Database (CPU/RAM เท่านั้น) ─────────────────────────────────────────

interface EditDatabaseModalProps {
  service: AppService;
  namespace: NamespaceDetail | null;
  onClose: () => void;
  onUpdated: (svc: AppService) => void;
}

// database template แก้ได้แค่ CPU/RAM — ชื่อ/image/version/credential ผูกกับข้อมูลที่ image สร้างครั้งแรก
export function EditDatabaseModal({ service, namespace, onClose, onUpdated }: EditDatabaseModalProps) {
  const [minRam, setMinRam] = useState(256);
  const [cpuMilli, setCpuMilli] = useState(service.cpu_milli);
  const [ramMb, setRamMb] = useState(service.ram_mb);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    serviceApi
      .databaseTemplates()
      .then((ts) => {
        const t = ts.find((x) => x.engine === service.database_engine);
        if (t) setMinRam(t.min_ram_mb);
      })
      .catch(() => {});
  }, [service.database_engine]);

  // โควตาที่ใช้ได้ = ที่กลุ่มเหลือ + ที่ database ตัวนี้ถืออยู่เดิม
  const cpuAvailable = Math.max(
    (namespace?.cpu_limit_milli ?? 0) - (namespace?.usage.used_cpu_milli ?? 0) + service.cpu_milli,
    0,
  );
  const ramAvailable = Math.max(
    (namespace?.ram_limit_mb ?? 0) - (namespace?.usage.used_ram_mb ?? 0) + service.ram_mb,
    0,
  );
  const cpuMax = namespace ? clamp(floorTo(cpuAvailable, 100), MIN_CPU_MILLI, MAX_CPU_MILLI) : MAX_CPU_MILLI;
  const ramMax = namespace ? clamp(floorTo(ramAvailable, 128), minRam, MAX_RAM_MB) : MAX_RAM_MB;
  const changed = cpuMilli !== service.cpu_milli || ramMb !== service.ram_mb;
  const canSubmit = changed && ramMb >= minRam && cpuMilli <= cpuAvailable && ramMb <= ramAvailable;

  const handleSave = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      // ค่าอื่นต้องส่งค่าเดิมกลับไปเป๊ะ — backend ตอบ 409 DATABASE_IMMUTABLE ถ้ามีอะไรเปลี่ยน
      const updated = await serviceApi.update(service.id, {
        name: service.name,
        image: service.image,
        cpu_milli: cpuMilli,
        ram_mb: ramMb,
        container_port: service.container_port,
        replicas: 1,
        env_vars: {},
        is_database: true,
        storage_mb: service.storage_mb,
        data_path: service.data_path,
      });
      onUpdated(updated);
    } catch (err) {
      setError(getApiErrorMessage(err, "แก้ไข database ไม่สำเร็จ"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/30 p-4 font-mono">
      <div className="w-full max-w-lg rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl">
        <div className="flex items-center justify-between px-7 py-5 border-b border-black/5">
          <div>
            <h2 className="text-xl font-bold text-[#211a14]">Edit database</h2>
            <p className="text-sm text-[#211a14]/50 mt-0.5">
              {service.name} · {service.database_engine} {service.database_version}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="p-2 rounded-xl text-[#211a14]/50 hover:bg-black/5 disabled:opacity-30"
          >
            <X size={20} />
          </button>
        </div>
        <div className="px-7 py-5 flex flex-col gap-5">
          {error && (
            <div className="flex items-start gap-2 p-3 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100">
              <AlertTriangle size={16} className="shrink-0 mt-0.5" /> {error}
            </div>
          )}
          <p className="text-sm text-[#211a14]/50">
            database แก้ได้เฉพาะ CPU และ RAM — ชื่อ, version, username, password และขนาดดิสก์เปลี่ยนหลัง deploy ไม่ได้
            · container จะ restart หลังบันทึก (ข้อมูลในดิสก์ไม่หาย)
          </p>
          <RangeRow
            icon={<Cpu size={14} className="text-[#BB6653]" />}
            label="CPU"
            value={cpuMilli}
            min={MIN_CPU_MILLI}
            max={cpuMax}
            step={100}
            display={formatCores(cpuMilli)}
            disabled={submitting}
            onChange={setCpuMilli}
          />
          <RangeRow
            icon={<Layers size={14} className="text-[#BB6653]" />}
            label="Memory"
            value={ramMb}
            min={minRam}
            max={ramMax}
            step={128}
            display={formatRam(ramMb)}
            disabled={submitting}
            onChange={setRamMb}
            note={`ขั้นต่ำ ${formatRam(minRam)}`}
          />
        </div>
        <div className="flex items-center justify-between gap-2 px-7 py-4 border-t border-black/5">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-xl px-4 py-2.5 text-base font-bold text-[#211a14]/60 hover:bg-black/5 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!canSubmit || submitting}
            onClick={handleSave}
            className={cn(
              "inline-flex items-center gap-2 rounded-xl px-5 py-2.5 text-base font-bold text-white shadow-md",
              canSubmit && !submitting ? "bg-[#BB6653] hover:bg-[#F08B51]" : "bg-[#211a14]/20 cursor-not-allowed shadow-none",
            )}
          >
            {submitting && <Loader2 size={16} className="animate-spin" />}
            Save
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Connection panel (หน้ารายละเอียด) ────────────────────────────────────────

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
      className="shrink-0 rounded-lg p-1.5 text-[#211a14]/40 hover:bg-[#FBDFDA] hover:text-[#BB6653]"
      title="คัดลอก"
    >
      {copied ? <Check size={14} /> : <Copy size={14} />}
    </button>
  );
}

// ข้อมูลเชื่อมต่อ database template — รหัสผ่านไม่อยู่ในรายการ service ต้องกด "แสดง" ถึงจะดึงจาก Secret
export function DatabaseConnectionPanel({ service }: { service: AppService }) {
  const [conn, setConn] = useState<DatabaseConnection | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const port = service.container_port;
  const scheme = service.database_engine === "postgresql" ? "postgresql" : "mysql";
  const masked = `${scheme}://${service.db_username}:••••••@${service.name}:${port}/${service.db_name}`;

  const reveal = async () => {
    setLoading(true);
    setError(null);
    try {
      setConn(await serviceApi.connection(service.id));
    } catch (err) {
      setError(getApiErrorMessage(err, "อ่านข้อมูลการเชื่อมต่อไม่สำเร็จ"));
    } finally {
      setLoading(false);
    }
  };

  const rows: [string, string][] = [
    ["Host", service.name],
    ["Port", String(port)],
    ["Username", service.db_username],
    ["Database", service.db_name],
  ];

  return (
    <div className="bg-white p-4 rounded-2xl border border-black/5 flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] font-bold text-gray-400 uppercase tracking-widest">Connection</p>
        {conn ? (
          <button
            type="button"
            onClick={() => setConn(null)}
            className="inline-flex items-center gap-1 text-xs font-bold text-[#211a14]/50 hover:text-[#BB6653]"
          >
            <EyeOff size={14} /> ซ่อน
          </button>
        ) : (
          <button
            type="button"
            disabled={loading}
            onClick={reveal}
            className="inline-flex items-center gap-1 text-xs font-bold text-[#211a14]/50 hover:text-[#BB6653] disabled:opacity-50"
          >
            {loading ? <Loader2 size={14} className="animate-spin" /> : <Eye size={14} />} แสดงรหัสผ่าน
          </button>
        )}
      </div>

      <div className="flex items-center gap-2 rounded-xl bg-black/[0.03] px-3 py-2">
        <span className="flex-1 break-all font-mono text-sm text-[#211a14]/75">{conn ? conn.url : masked}</span>
        {conn && <CopyButton value={conn.url} />}
      </div>

      <div className="grid grid-cols-2 gap-2">
        {rows.map(([k, v]) => (
          <div key={k} className="flex items-center justify-between gap-1 rounded-lg border border-black/5 px-2.5 py-1.5">
            <span className="min-w-0">
              <span className="block text-[10px] font-bold uppercase tracking-widest text-gray-400">{k}</span>
              <span className="block truncate font-mono text-sm">{v}</span>
            </span>
            <CopyButton value={v} />
          </div>
        ))}
        {conn && (
          <div className="col-span-2 flex items-center justify-between gap-1 rounded-lg border border-black/5 px-2.5 py-1.5">
            <span className="min-w-0">
              <span className="block text-[10px] font-bold uppercase tracking-widest text-gray-400">Password</span>
              <span className="block break-all font-mono text-sm">{conn.password}</span>
            </span>
            <CopyButton value={conn.password} />
          </div>
        )}
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <p className="flex items-start gap-1.5 text-xs leading-relaxed text-[#211a14]/45">
        <Lock size={12} className="mt-0.5 shrink-0" />
        ใช้ได้เฉพาะ service ในกลุ่มเดียวกัน (ชื่อเต็ม {service.name}.ns-{service.namespace_id}.svc.cluster.local) · ปิด SSL ไว้
        {service.database_engine === "postgresql"
          ? " (sslmode=disable)"
          : " (ssl=false)"}
      </p>
      {service.database_engine === "mysql" && (
        <p className="text-xs leading-relaxed text-[#A96A15]">
          MySQL แบบไม่ใช้ SSL: บาง driver ต้องเปิดการขอ public key เอง เช่น JDBC ใส่ allowPublicKeyRetrieval=true,
          mysql CLI ใส่ --get-server-public-key (Node mysql2 ไม่ต้องตั้ง)
        </p>
      )}
    </div>
  );
}
