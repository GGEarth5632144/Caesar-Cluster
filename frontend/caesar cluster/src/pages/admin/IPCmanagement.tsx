import { useEffect, useState, type ReactNode } from "react";
import {
  Server,
  AlertTriangle,
  RefreshCw,
  HardDrive,
  Clock,
  Zap,
  PlugZap,
  ZapOff,
  PowerOff,
  Cpu, // เพิ่ม Icon CPU
  Thermometer, // เพิ่ม Icon อุณหภูมิ
} from "lucide-react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Area,
  AreaChart,
  BarChart,
  Bar,
  ComposedChart,
  Legend,
} from "recharts";

// นำเข้า API และ Interface ที่อัปเดตแล้ว
import {
  nodetelemetry,
  type NodeTelemetry,
  type ClusterHistoryData,
  type PowerNode,
  type PowerHistoryData,
} from "@/api/mornitorequest";
import { usePageSearch } from "@/hooks/usePageSearch";
import { nodesScope } from "@/config/searchScopes";
import { SearchStatus } from "@/components/ui/search-status";

// --- Constants ---
const REFRESH_INTERVALS = [
  { label: "Off", value: 0 },
  { label: "5s", value: 5000 },
  { label: "10s", value: 10000 },
  { label: "30s", value: 30000 },
  { label: "1m", value: 60000 },
  { label: "2m", value: 120000 },
  { label: "5m", value: 300000 },
  { label: "15m", value: 900000 },
  { label: "30m", value: 1800000 },
  { label: "1h", value: 3600000 },
];

const TIME_RANGES = [
  { label: "Last 1 hour", value: "1h" },
  { label: "Last 6 hours", value: "6h" },
  { label: "Last 24 hours", value: "24h" },
  { label: "Last 7 days", value: "7d" },
  { label: "Last 30 days", value: "30d" },
];

// --- Sub-Components ---
const StatCard = ({ title, value, unit, data, color, type = "line" }: any) => {
  return (
    <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-5 flex flex-col justify-between">
      <div className="flex justify-between items-start mb-2">
        <span className="text-xs font-bold text-gray-500 uppercase tracking-wider">
          {title}
        </span>
      </div>
      <div className="flex items-baseline gap-1 mb-3">
        <span className="text-3xl font-extrabold text-gray-800">{value}</span>
        <span className="text-sm font-medium text-gray-500">{unit}</span>
      </div>
      <div className="h-14 w-full">
        <ResponsiveContainer width="100%" height="100%">
          {type === "area" ? (
            <AreaChart data={data}>
              <defs>
                <linearGradient
                  id={`color-${title.replace(/\s+/g, "-")}`}
                  x1="0"
                  y1="0"
                  x2="0"
                  y2="1"
                >
                  <stop offset="5%" stopColor={color} stopOpacity={0.2} />
                  <stop offset="95%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area
                type="monotone"
                dataKey="value"
                stroke={color}
                fillOpacity={1}
                fill={`url(#color-${title.replace(/\s+/g, "-")})`}
                strokeWidth={2}
                isAnimationActive={false}
              />
            </AreaChart>
          ) : type === "bar" ? (
            <BarChart data={data}>
              <Bar
                dataKey="value"
                fill={color}
                radius={[2, 2, 0, 0]}
                isAnimationActive={false}
              />
            </BarChart>
          ) : (
            <LineChart data={data}>
              <Line
                type="monotone"
                dataKey="value"
                stroke={color}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            </LineChart>
          )}
        </ResponsiveContainer>
      </div>
    </div>
  );
};

const NodeGridItem = ({ node }: { node: NodeTelemetry }) => {
  const isOffline = node.IsUp === 0;
  const isHot = node.Temperature >= 75;
  const isHighCpu = node.CpuUsage >= 85;
  const statusColor = isOffline ? "bg-red-500" : "bg-emerald-500";
  const rowBg = isOffline
    ? "bg-red-50 border-red-200"
    : "bg-white border-gray-200";

  return (
    <div
      className={`flex flex-col p-4 border rounded-xl ${rowBg} hover:shadow-md transition-all duration-200 gap-3`}
    >
      <div className="flex items-center gap-2 pb-2 border-b border-gray-100">
        <div className="relative flex h-2.5 w-2.5 shrink-0">
          {!isOffline && (
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
          )}
          <span
            className={`relative inline-flex rounded-full h-2.5 w-2.5 ${statusColor}`}
          />
        </div>
        <span
          className={`text-sm font-extrabold truncate ${isOffline ? "text-red-700" : "text-gray-800"}`}
        >
          {node.NodeName}
        </span>
      </div>

      {isOffline ? (
        <div className="flex items-center justify-center py-4">
          <span className="text-xs text-red-500 font-bold uppercase tracking-widest">
            Offline
          </span>
        </div>
      ) : (
        <div className="flex flex-col gap-2 font-mono">
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              CPU
            </span>
            <span
              className={`text-sm font-bold ${isHighCpu ? "text-red-500" : "text-gray-700"}`}
            >
              {(node.CpuUsage || 0).toFixed(1)} %
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              RAM
            </span>
            <span className="text-sm font-bold text-gray-700">
              {(node.RamUsedMB / 1024).toFixed(1)} /{" "}
              {node.RamTotalMB ? (node.RamTotalMB / 1024).toFixed(1) : "0.0"} GB
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              Temp
            </span>
            <span
              className={`text-sm font-bold ${isHot ? "text-red-500 animate-pulse" : "text-orange-500"}`}
            >
              {node.Temperature.toFixed(1)} °C
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

const PowerGridItem = ({ power }: { power: PowerNode }) => {
  const isOffline = power.Status !== "On";
  const isIdle = !isOffline && power.Watt === 0;

  let statusColor = "bg-amber-500";
  let rowBg = "bg-white border-amber-200";
  let textColor = "text-amber-700";
  let icon = <PlugZap className="h-4 w-4" />;

  if (isOffline) {
    statusColor = "bg-gray-400";
    rowBg = "bg-gray-50 border-gray-200 opacity-70";
    textColor = "text-gray-500";
    icon = <ZapOff className="h-4 w-4" />;
  } else if (isIdle) {
    statusColor = "bg-blue-400";
    rowBg = "bg-blue-50/50 border-blue-200";
    textColor = "text-blue-700";
    icon = <PowerOff className="h-4 w-4" />;
  }

  return (
    <div
      className={`flex flex-col p-4 border rounded-xl ${rowBg} hover:shadow-md transition-all duration-200 gap-3`}
    >
      <div className="flex items-center gap-2 pb-2 border-b border-gray-100">
        <div className="relative flex h-2.5 w-2.5 shrink-0">
          {!isOffline && !isIdle && (
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75" />
          )}
          <span
            className={`relative inline-flex rounded-full h-2.5 w-2.5 ${statusColor}`}
          />
        </div>
        <span
          className={`flex items-center gap-1.5 text-sm font-extrabold truncate ${textColor}`}
        >
          {icon} Modbus {power.ModbusID}
        </span>
      </div>

      {isOffline ? (
        <div className="flex items-center justify-center py-4 gap-2 text-gray-400">
          <span className="text-xs font-bold uppercase tracking-widest">
            Power Off
          </span>
        </div>
      ) : (
        <div className="flex flex-col gap-2 font-mono">
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              Power
            </span>
            <span
              className={`text-sm font-bold ${isIdle ? "text-blue-600" : "text-amber-600"}`}
            >
              {power.Watt.toFixed(2)} W
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              Current
            </span>
            <span className="text-sm font-bold text-gray-700">
              {power.Amp.toFixed(3)} A
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">
              Voltage
            </span>
            <span className="text-sm font-bold text-gray-700">
              {power.Volt.toFixed(1)} V
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

// --- Formatters ---
function formatTooltipTime(label: ReactNode) {
  const raw = String(label);
  const date = new Date(raw);
  if (isNaN(date.getTime())) return raw;

  return date.toLocaleString("th-TH", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatXAxisTime(val: any, timeRange: string) {
  const date = new Date(val);
  if (isNaN(date.getTime())) return val;

  if (timeRange === "7d" || timeRange === "30d") {
    return date.toLocaleDateString("th-TH", { day: "numeric", month: "short" });
  }
  return date.toLocaleTimeString("th-TH", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

// --- Main Component ---
export default function IPCmanagement() {
  const [nodes, setNodes] = useState<NodeTelemetry[]>([]);
  const [historyData, setHistoryData] = useState<ClusterHistoryData[]>([]);
  const [powers, setPowers] = useState<PowerNode[]>([]);
  const [powerHistory, setPowerHistory] = useState<PowerHistoryData[]>([]);

  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);

  const [timeRange, setTimeRange] = useState("1h");
  const [refreshInterval, setRefreshInterval] = useState(60000);

  const fetchDashboardData = async () => {
    setLoading(true);
    try {
      const [snapshotData, historyDataRes, powerDataRes, powerHistoryRes] =
        await Promise.all([
          nodetelemetry.getAll(),
          nodetelemetry.getHistory(timeRange),
          nodetelemetry.getAllPower(),
          nodetelemetry.getPowerHistory(timeRange),
        ]);

      setNodes(
        snapshotData.sort((a, b) => a.NodeName.localeCompare(b.NodeName)),
      );
      setHistoryData(historyDataRes);
      setPowers(powerDataRes.sort((a, b) => a.ModbusID - b.ModbusID));
      setPowerHistory(powerHistoryRes);

      setLastUpdate(new Date());
      setError(null);
    } catch (err) {
      console.error("Fetch Data Error:", err);
      setError("ไม่สามารถดึงข้อมูลระบบได้ กรุณาตรวจสอบการเชื่อมต่อ");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDashboardData();
    if (refreshInterval === 0) return;
    const intervalId = setInterval(() => {
      fetchDashboardData();
    }, refreshInterval);
    return () => clearInterval(intervalId);
  }, [refreshInterval, timeRange]);

  // --- Derived Metrics ---
  const totalNodes = nodes.length;
  const onlineNodes = nodes.filter((n) => n.IsUp === 1);
  const avgTemp =
    onlineNodes.length > 0
      ? onlineNodes.reduce((s, n) => s + n.Temperature, 0) / onlineNodes.length
      : 0;

  const controlPlane = nodes.find((n) => n.NodeName === "intelnuc");
  const workerNodes = nodes.filter((n) => n.NodeName !== "intelnuc");

  const { results: visibleNodes, isFiltering } = usePageSearch(
    nodesScope,
    nodes,
  );
  const visibleIds = new Set(visibleNodes.map((n) => n.ID));

  const totalWatt = powers.reduce((sum, p) => sum + p.Watt, 0);
  const avgVolt =
    powers.length > 0
      ? powers.reduce((sum, p) => sum + p.Volt, 0) / powers.length
      : 0;
  const activePowerNodes = powers.filter((p) => p.Watt > 0).length;

  // Derived History Metrics สำหรับ StatCards
  const latestCpu =
    historyData.length > 0 ? historyData[historyData.length - 1].avgCpu : 0;
  const latestRamUsed =
    historyData.length > 0 ? historyData[historyData.length - 1].totalRam : 0;
  const latestMaxRam =
    historyData.length > 0 ? historyData[historyData.length - 1].maxRam : 0;

  // กรองข้อมูลสำหรับกราฟ Telemetry (ลดความหนาแน่น)
  const filteredHistoryData = historyData.filter((_, idx) =>
    timeRange === "7d"
      ? idx % 2 === 0
      : timeRange === "30d"
        ? idx % 4 === 0
        : true,
  );

  return (
    <div className="min-h-screen bg-transparent text-gray-800 font-sans p-4 md:p-6">
      <div className="max-w-[1400px] mx-auto flex flex-col gap-6">
        {/* --- Top Navbar --- */}
        <div className="flex flex-col md:flex-row items-center justify-between bg-white border border-gray-200 rounded-2xl shadow-sm p-4 gap-4 sticky top-2 z-30 bg-white/80 backdrop-blur-xl">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-indigo-50 rounded-lg">
              <Server className="h-6 w-6 text-indigo-600" />
            </div>
            <div>
              <h1 className="text-xl font-extrabold text-gray-800 tracking-wide">
                Unified Cluster Monitor
              </h1>
              <p className="text-xs font-semibold text-gray-400 mt-0.5">
                {lastUpdate
                  ? `Updated at ${lastUpdate.toLocaleTimeString("th-TH")}`
                  : "Waiting for data..."}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3 text-sm">
            <div className="flex items-center bg-gray-50 border border-gray-200 rounded-xl overflow-hidden px-2 hover:bg-gray-100 transition-colors">
              <Clock className="h-4 w-4 text-gray-400" />
              <select
                value={timeRange}
                onChange={(e) => setTimeRange(e.target.value)}
                className="bg-transparent text-gray-700 font-semibold py-2 px-2 outline-none cursor-pointer"
              >
                {TIME_RANGES.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex items-center bg-gray-50 border border-gray-200 rounded-xl px-1">
              <button
                onClick={fetchDashboardData}
                disabled={loading}
                className="p-2 hover:bg-gray-200 rounded-lg transition-colors"
                title="Refresh now"
              >
                <RefreshCw
                  className={`h-4 w-4 ${loading ? "animate-spin text-indigo-600" : "text-gray-500"}`}
                />
              </button>
              <div className="w-px h-5 bg-gray-300 mx-1"></div>
              <select
                value={refreshInterval}
                onChange={(e) => setRefreshInterval(Number(e.target.value))}
                className="bg-transparent text-gray-700 font-semibold py-2 px-2 outline-none cursor-pointer"
              >
                {REFRESH_INTERVALS.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </div>

        {error && (
          <div className="bg-red-50 border border-red-200 text-red-700 px-5 py-4 rounded-xl flex items-center gap-3 shadow-sm">
            <AlertTriangle className="h-5 w-5 shrink-0" />
            <p className="text-sm font-bold">{error}</p>
          </div>
        )}

        {/* --- Overview Stat Cards (อัปเดตเป็น 8 กล่อง) --- */}
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-4 gap-4">
          <StatCard
            title="Avg CPU Usage"
            value={(latestCpu || 0).toFixed(1)}
            unit="%"
            color="#ef4444"
            data={historyData.map((d) => ({ value: d.avgCpu }))}
            type="area"
          />
          <StatCard
            title="Cluster RAM"
            value={(latestRamUsed / 1024).toFixed(1)}
            unit={`/ ${(latestMaxRam / 1024).toFixed(1)} GB`}
            color="#3b82f6"
            data={historyData.map((d) => ({ value: d.totalRam }))}
            type="area"
          />
          <StatCard
            title="Avg Temp"
            value={avgTemp.toFixed(1)}
            unit="°C"
            color={avgTemp > 75 ? "#ef4444" : "#f97316"}
            data={historyData.map((d) => ({ value: d.avgTemp }))}
          />
          <StatCard
            title="Online Nodes"
            value={onlineNodes.length}
            unit={`/ ${totalNodes}`}
            color="#10b981"
            data={historyData.map((d) => ({ value: d.onlineNodes }))}
            type="area"
          />
          <StatCard
            title="Total Power"
            value={totalWatt.toFixed(1)}
            unit="W"
            color="#f59e0b"
            data={powerHistory.map((p) => ({ value: p.totalWatt }))}
            type="area"
          />
          <StatCard
            title="Avg Voltage"
            value={avgVolt.toFixed(1)}
            unit="V"
            color="#8b5cf6"
            data={powerHistory.map((p) => ({ value: p.avgVolt }))}
          />
          <StatCard
            title="Active Load"
            value={activePowerNodes}
            unit={`/ ${powers.length}`}
            color="#06b6d4"
            data={powerHistory.map(() => ({ value: activePowerNodes }))}
            type="bar"
          />
          <StatCard
            title="CP Procs"
            value={controlPlane ? controlPlane.Procs : 0}
            unit="tasks"
            color="#6366f1"
            data={historyData.map(() => ({
              value: controlPlane ? controlPlane.Procs : 0,
            }))}
            type="area"
          />
        </div>

        {/* --- Main Charts Area (Row 1: Telemetry - CPU & Temp) --- */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 h-[400px]">
          {/* CPU Trend */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <Cpu className="h-5 w-5 text-rose-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Avg CPU Usage
                <span className="ml-2 text-sm font-medium text-rose-500 bg-rose-50 px-2 py-0.5 rounded-md">
                  ({TIME_RANGES.find((r) => r.value === timeRange)?.label})
                </span>
              </h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={filteredHistoryData}
                  margin={{ left: -15, right: 10, top: 10, bottom: 0 }}
                >
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="#d1d5db"
                    vertical={false}
                  />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickMargin={10}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={50}
                    tickFormatter={(val) => formatXAxisTime(val, timeRange)}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12, fontWeight: 500 }}
                    domain={[0, 100]}
                    tickFormatter={(value) => `${Math.round(value)}%`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "#ffffff",
                      borderColor: "#e5e7eb",
                      borderRadius: "8px",
                      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)",
                    }}
                    itemStyle={{ color: "#f43f5e", fontWeight: "bold" }}
                    labelFormatter={formatTooltipTime}
                    formatter={(value: any) => [
                      Number(value).toFixed(1) + " %",
                      "Avg CPU",
                    ]}
                  />
                  <Line
                    type="monotone"
                    dataKey="avgCpu"
                    name="Avg CPU (%)"
                    stroke="#f43f5e"
                    strokeWidth={3}
                    dot={false}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Temperature Trend */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <Thermometer className="h-5 w-5 text-orange-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Average Temperature
                <span className="ml-2 text-sm font-medium text-orange-500 bg-orange-50 px-2 py-0.5 rounded-md">
                  ({TIME_RANGES.find((r) => r.value === timeRange)?.label})
                </span>
              </h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={filteredHistoryData}
                  margin={{ left: -15, right: 10, top: 10, bottom: 0 }}
                >
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="#d1d5db"
                    vertical={false}
                  />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickMargin={10}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={50}
                    tickFormatter={(val) => formatXAxisTime(val, timeRange)}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12, fontWeight: 500 }}
                    domain={["dataMin - 2", "dataMax + 2"]}
                    tickFormatter={(value) => `${Math.round(value)}°C`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "#ffffff",
                      borderColor: "#e5e7eb",
                      borderRadius: "8px",
                      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)",
                    }}
                    itemStyle={{ color: "#f97316", fontWeight: "bold" }}
                    labelFormatter={formatTooltipTime}
                    formatter={(value: any) => [
                      Number(value).toFixed(1),
                      "Avg Temp (°C)",
                    ]}
                  />
                  <Line
                    type="monotone"
                    dataKey="avgTemp"
                    name="Avg Temp (°C)"
                    stroke="#f97316"
                    strokeWidth={3}
                    dot={false}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* --- Main Charts Area (Row 1.5: Memory Full Width) --- */}
        <div className="grid grid-cols-1 gap-6 h-[400px]">
          {/* Resource Usage Trend (RAM) */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <HardDrive className="h-5 w-5 text-blue-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Memory Allocation (Cluster Total)
                <span className="ml-2 text-sm font-medium text-blue-500 bg-blue-50 px-2 py-0.5 rounded-md">
                  ({TIME_RANGES.find((r) => r.value === timeRange)?.label})
                </span>
              </h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart
                  data={filteredHistoryData}
                  margin={{ left: -5, right: 10, top: 10, bottom: 0 }}
                >
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="#d1d5db"
                    vertical={false}
                  />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickMargin={10}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={50}
                    tickFormatter={(val) => formatXAxisTime(val, timeRange)}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickFormatter={(val) => `${(val / 1024).toFixed(0)} GB`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "#ffffff",
                      borderColor: "#e5e7eb",
                      borderRadius: "8px",
                      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)",
                    }}
                    itemStyle={{ color: "#3b82f6", fontWeight: "bold" }}
                    labelFormatter={formatTooltipTime}
                    formatter={(value: any, name: any, props: any) => {
                      // ดึง maxRam จาก Payload แล้วโชว์แบบ Used / Total
                      const used = (Number(value) / 1024).toFixed(2);
                      const total = props.payload.maxRam
                        ? (Number(props.payload.maxRam) / 1024).toFixed(2)
                        : "0.00";
                      return [`${used} GB / ${total} GB`, "RAM Used"];
                    }}
                  />
                  <defs>
                    <linearGradient id="colorRam" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.2} />
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <Area
                    type="monotone"
                    dataKey="totalRam"
                    name="RAM Used (GB)"
                    stroke="#3b82f6"
                    strokeWidth={3}
                    fillOpacity={1}
                    fill="url(#colorRam)"
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* --- Main Charts Area (Row 2: Power Metrics History) --- */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 h-[400px]">
          {/* Total Power Trend */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <Zap className="h-5 w-5 text-amber-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Total Power Consumption
                <span className="ml-2 text-sm font-medium text-amber-600 bg-amber-50 border border-amber-100 px-2.5 py-0.5 rounded-md">
                  ({TIME_RANGES.find((r) => r.value === timeRange)?.label})
                </span>
              </h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart
                  data={powerHistory}
                  margin={{ left: -15, right: 10, top: 10, bottom: 0 }}
                >
                  <defs>
                    <linearGradient id="colorPower" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="#e5e7eb"
                    vertical={false}
                  />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickMargin={10}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={40}
                    tickFormatter={(val) => formatXAxisTime(val, timeRange)}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickFormatter={(val) => `${val}W`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      borderRadius: "8px",
                      border: "none",
                      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)",
                    }}
                    itemStyle={{ color: "#b45309", fontWeight: "bold" }}
                    labelFormatter={formatTooltipTime}
                  />
                  <Area
                    type="monotone"
                    dataKey="totalWatt"
                    name="Total Power (W)"
                    stroke="#f59e0b"
                    strokeWidth={3}
                    fillOpacity={1}
                    fill="url(#colorPower)"
                    isAnimationActive={false}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Current & Voltage Aggregate */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <PlugZap className="h-5 w-5 text-indigo-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Current & Avg Voltage
                <span className="ml-2 text-sm font-medium text-indigo-600 bg-indigo-50 border border-indigo-100 px-2.5 py-0.5 rounded-md">
                  ({TIME_RANGES.find((r) => r.value === timeRange)?.label})
                </span>
              </h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <ComposedChart
                  data={powerHistory}
                  margin={{ left: -15, right: 10, top: 10, bottom: 0 }}
                >
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="#e5e7eb"
                    vertical={false}
                  />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickMargin={10}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={40}
                    tickFormatter={(val) => formatXAxisTime(val, timeRange)}
                  />
                  <YAxis
                    yAxisId="left"
                    stroke="#9ca3af"
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickFormatter={(val) => `${val}A`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <YAxis
                    yAxisId="right"
                    orientation="right"
                    stroke="#9ca3af"
                    domain={["dataMin - 1", "dataMax + 1"]}
                    tick={{ fill: "#6b7280", fontSize: 12 }}
                    tickFormatter={(val) => `${val}V`}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      borderRadius: "8px",
                      border: "none",
                      boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)",
                    }}
                    labelFormatter={formatTooltipTime}
                  />
                  <Legend
                    wrapperStyle={{ paddingTop: "20px", fontSize: "13px" }}
                  />
                  <Bar
                    yAxisId="left"
                    dataKey="totalAmp"
                    name="Total Current (A)"
                    fill="#3b82f6"
                    radius={[4, 4, 0, 0]}
                    barSize={20}
                    isAnimationActive={false}
                  />
                  <Line
                    yAxisId="right"
                    type="monotone"
                    dataKey="avgVolt"
                    name="Avg Voltage (V)"
                    stroke="#8b5cf6"
                    strokeWidth={3}
                    dot={false}
                    isAnimationActive={false}
                  />
                </ComposedChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* --- Node Details Grid --- */}
        <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col mt-2">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <Server className="h-5 w-5 text-gray-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Node Cluster Status
              </h2>
            </div>
            <div className="flex items-center gap-3">
              <SearchStatus />
              <span className="text-xs font-bold bg-gray-100 text-gray-600 px-3 py-1 rounded-full">
                {onlineNodes.length} / {totalNodes} Online
              </span>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 max-h-[500px] overflow-y-auto pr-2 custom-scrollbar">
            {controlPlane && visibleIds.has(controlPlane.ID) && (
              <NodeGridItem key={`n-${controlPlane.ID}`} node={controlPlane} />
            )}
            {workerNodes
              .filter((node) => visibleIds.has(node.ID))
              .map((node) => (
                <NodeGridItem key={`n-${node.ID}`} node={node} />
              ))}
            {isFiltering && visibleNodes.length === 0 && (
              <p className="col-span-full py-10 text-center text-sm text-gray-400">
                ไม่มีโหนดที่ตรงกับคำค้นหา
              </p>
            )}
          </div>
        </div>

        {/* --- Power Details Grid --- */}
        <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col mb-10">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <Zap className="h-5 w-5 text-amber-500" />
              <h2 className="text-base font-extrabold text-gray-800">
                Power Relays (KWS-303L)
              </h2>
            </div>
            <span className="text-xs font-bold bg-amber-50 text-amber-600 px-3 py-1 rounded-full border border-amber-200">
              Total Active Load: {totalWatt.toFixed(1)} W
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 max-h-[500px] overflow-y-auto pr-2 custom-scrollbar">
            {powers.map((power) => (
              <PowerGridItem key={`p-${power.ModbusID}`} power={power} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
