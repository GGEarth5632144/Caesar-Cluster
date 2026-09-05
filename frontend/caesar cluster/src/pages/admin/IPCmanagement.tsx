import React, { useEffect, useState, useMemo } from "react";
import {
  Server,
  ServerCrash,
  Cpu,
  Activity,
  AlertTriangle,
  RefreshCw,
  HardDrive,
  Clock,
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
} from "recharts";

import { nodetelemetry, type NodeTelemetry, type ClusterHistoryData } from "@/api/mornitorequest";

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
        <span className="text-xs font-bold text-gray-500 uppercase tracking-wider">{title}</span>
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
                <linearGradient id={`color-${title}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor={color} stopOpacity={0.2} />
                  <stop offset="95%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area type="monotone" dataKey="value" stroke={color} fillOpacity={1} fill={`url(#color-${title})`} strokeWidth={2} />
            </AreaChart>
          ) : (
            <LineChart data={data}>
              <Line type="monotone" dataKey="value" stroke={color} strokeWidth={2} dot={false} isAnimationActive={false} />
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
  const statusColor = isOffline ? "bg-red-500" : "bg-emerald-500";
  const rowBg = isOffline ? "bg-red-50 border-red-200" : "bg-white border-gray-200";

  return (
    <div className={`flex flex-col p-4 border rounded-xl ${rowBg} hover:shadow-md transition-all duration-200 gap-3`}>
      {/* บรรทัดที่ 1: ไฟสถานะ + ชื่อ Node */}
      <div className="flex items-center gap-2 pb-2 border-b border-gray-100">
        <div className="relative flex h-2.5 w-2.5 shrink-0">
          {!isOffline && <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />}
          <span className={`relative inline-flex rounded-full h-2.5 w-2.5 ${statusColor}`} />
        </div>
        <span className={`text-sm font-extrabold truncate ${isOffline ? "text-red-700" : "text-gray-800"}`}>
          {node.NodeName}
        </span>
      </div>

      {/* บรรทัดต่อๆ มา: ข้อมูลที่เรียงซ้อนกัน */}
      {isOffline ? (
        <div className="flex items-center justify-center py-2">
          <span className="text-xs text-red-500 font-bold uppercase tracking-widest">Offline</span>
        </div>
      ) : (
        <div className="flex flex-col gap-2 font-mono">
          {/* บรรทัดที่ 2: อุณหภูมิ */}
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">Temp</span>
            <span className={`text-sm font-bold ${isHot ? "text-red-500 animate-pulse" : "text-orange-500"}`}>
              {node.Temperature.toFixed(1)} °C
            </span>
          </div>
          
          {/* บรรทัดที่ 3: RAM */}
          <div className="flex items-center justify-between">
            <span className="text-xs font-sans font-semibold text-gray-400">RAM</span>
            <span className="text-sm font-bold text-gray-700">
              {(node.RamUsedMB / 1024).toFixed(1)} GB
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

// --- Main Component ---
export default function IPCmanagement() {
  const [nodes, setNodes] = useState<NodeTelemetry[]>([]);
  const [historyData, setHistoryData] = useState<ClusterHistoryData[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);

  const [timeRange, setTimeRange] = useState("1h");
  const [refreshInterval, setRefreshInterval] = useState(5000);

  const fetchClusterData = async () => {
    setLoading(true);
    try {
      const [snapshotData, historyDataRes] = await Promise.all([
        nodetelemetry.getAll(),
        nodetelemetry.getHistory(timeRange)
      ]);

      setNodes(snapshotData.sort((a, b) => a.NodeName.localeCompare(b.NodeName)));
      setHistoryData(historyDataRes);

      setLastUpdate(new Date());
      setError(null);
    } catch (err) {
      console.error("Fetch Data Error:", err);
      setError("ไม่สามารถดึงข้อมูลคลัสเตอร์ได้"); 
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchClusterData();

    if (refreshInterval === 0) return;

    const intervalId = setInterval(() => {
      fetchClusterData();
    }, refreshInterval);

    return () => clearInterval(intervalId);
  }, [refreshInterval, timeRange]);

  const totalNodes = nodes.length;
  const onlineNodes = nodes.filter((n) => n.IsUp === 1);
  const offlineCount = totalNodes - onlineNodes.length;
  const avgTemp = onlineNodes.length > 0 ? onlineNodes.reduce((s, n) => s + n.Temperature, 0) / onlineNodes.length : 0;
  
  const controlPlane = nodes.find((n) => n.NodeName === "intelnuc");
  const workerNodes = nodes.filter((n) => n.NodeName !== "intelnuc");

  return (
    <div className="min-h-screen bg-transparent text-gray-800 font-sans p-4 md:p-6">
      <div className="max-w-[1400px] mx-auto flex flex-col gap-6">
        
        {/* --- Top Navbar --- */}
        <div className="flex flex-col md:flex-row items-center justify-between bg-white border border-gray-200 rounded-2xl shadow-sm p-4 gap-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-orange-50 rounded-lg">
              <Cpu className="h-6 w-6 text-orange-500" />
            </div>
            <div>
              <h1 className="text-xl font-extrabold text-gray-800 tracking-wide">Cluster Telemetry</h1>
              <p className="text-xs font-semibold text-gray-400 mt-0.5">
                {lastUpdate ? `Updated at ${lastUpdate.toLocaleTimeString("th-TH")}` : "Waiting for data..."}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3 text-sm">
            {/* Time Range Selector */}
            <div className="flex items-center bg-gray-50 border border-gray-200 rounded-xl overflow-hidden px-2 hover:bg-gray-100 transition-colors">
              <Clock className="h-4 w-4 text-gray-400" />
              <select 
                value={timeRange}
                onChange={(e) => setTimeRange(e.target.value)}
                className="bg-transparent text-gray-700 font-semibold py-2 px-2 outline-none cursor-pointer"
              >
                {TIME_RANGES.map(r => <option key={r.value} value={r.value}>{r.label}</option>)}
              </select>
            </div>

            {/* Refresh Interval Selector */}
            <div className="flex items-center bg-gray-50 border border-gray-200 rounded-xl px-1">
              <button 
                onClick={fetchClusterData}
                disabled={loading}
                className="p-2 hover:bg-gray-200 rounded-lg transition-colors"
                title="Refresh now"
              >
                <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin text-orange-500' : 'text-gray-500'}`} />
              </button>
              <div className="w-px h-5 bg-gray-300 mx-1"></div>
              <select 
                value={refreshInterval}
                onChange={(e) => setRefreshInterval(Number(e.target.value))}
                className="bg-transparent text-gray-700 font-semibold py-2 px-2 outline-none cursor-pointer"
              >
                {REFRESH_INTERVALS.map(r => <option key={r.value} value={r.value}>{r.label}</option>)}
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

        {/* --- Overview Stat Cards --- */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
          <StatCard 
            title="Online Nodes" 
            value={onlineNodes.length} 
            unit={`/ ${totalNodes}`}
            color="#10b981"
            data={historyData.map(d => ({ value: d.onlineNodes }))} 
            type="area"
          />
          <StatCard 
            title="Offline Nodes" 
            value={offlineCount} 
            unit="nodes"
            color="#ef4444"
            data={historyData.map(d => ({ value: totalNodes - d.onlineNodes }))} 
          />
          <StatCard 
            title="Cluster Avg Temp" 
            value={avgTemp.toFixed(1)} 
            unit="°C"
            color={avgTemp > 75 ? "#ef4444" : "#f97316"}
            data={historyData.map(d => ({ value: d.avgTemp }))} 
          />
          <StatCard 
            title="Control Plane Load" 
            value={controlPlane ? (controlPlane.Procs).toString() : "N/A"} 
            unit="procs"
            color="#3b82f6"
            data={historyData.map(() => ({ value: controlPlane ? controlPlane.Procs : 0 }))} 
            type="area"
          />
        </div>

        {/* --- Main Charts Area (ขยายความสูงเป็น 450px) --- */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 h-[450px]">
          
          {/* Temperature Trend */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <Activity className="h-5 w-5 text-orange-500" />
              <h2 className="text-base font-extrabold text-gray-800">Average Temperature Trend</h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={historyData} margin={{ left: -15, right: 10, top: 10, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#d1d5db" vertical={false} />
                  <XAxis dataKey="time" stroke="#9ca3af" tick={{ fill: '#6b7280', fontSize: 12 }} tickMargin={10} axisLine={false} tickLine={false} />
                  {/* กำหนด tickFormatter ให้เติมคำว่า °C */}
                  <YAxis 
                    stroke="#9ca3af" 
                    tick={{ fill: '#6b7280', fontSize: 12, fontWeight: 500 }} 
                    domain={['dataMin - 2', 'dataMax + 2']} 
                    tickFormatter={(value) => `${Math.round(value)}°C`}
                    axisLine={false} 
                    tickLine={false}
                  />
                  <Tooltip 
                    contentStyle={{ backgroundColor: '#ffffff', borderColor: '#e5e7eb', borderRadius: '8px', boxShadow: '0 4px 6px -1px rgb(0 0 0 / 0.1)' }}
                    itemStyle={{ color: '#f97316', fontWeight: 'bold' }}
                    formatter={(value: any) => [Number(value).toFixed(1), "Avg Temp (°C)"]} 
                  />
                  <Line type="monotone" dataKey="avgTemp" name="Avg Temp (°C)" stroke="#f97316" strokeWidth={3} dot={false} activeDot={{ r: 6, strokeWidth: 0 }} />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Resource Usage Trend */}
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col">
            <div className="flex items-center gap-2 mb-6">
              <HardDrive className="h-5 w-5 text-blue-500" />
              <h2 className="text-base font-extrabold text-gray-800">Memory Allocation Trend</h2>
            </div>
            <div className="flex-1 w-full min-h-0">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={historyData} margin={{ left: -5, right: 10, top: 10, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#d1d5db" vertical={false} />
                  <XAxis dataKey="time" stroke="#9ca3af" tick={{ fill: '#6b7280', fontSize: 12 }} tickMargin={10} axisLine={false} tickLine={false} />
                  {/* กำหนด tickFormatter ให้หารด้วย 1024 และเติมคำว่า GB */}
                  <YAxis 
                    stroke="#9ca3af" 
                    tick={{ fill: '#6b7280', fontSize: 12, fontWeight: 500 }} 
                    tickFormatter={(value) => `${(value / 1024).toFixed(0)} GB`}
                    axisLine={false} 
                    tickLine={false}
                  />
                  <Tooltip 
                    contentStyle={{ backgroundColor: '#ffffff', borderColor: '#e5e7eb', borderRadius: '8px', boxShadow: '0 4px 6px -1px rgb(0 0 0 / 0.1)' }}
                    itemStyle={{ color: '#3b82f6', fontWeight: 'bold' }}
                    formatter={(value: any) => [(Number(value) / 1024).toFixed(2), "RAM Used (GB)"]} 
                  />
                  <defs>
                    <linearGradient id="colorRam" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.2}/>
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <Area type="monotone" dataKey="totalRam" name="RAM Used (MB)" stroke="#3b82f6" strokeWidth={3} fillOpacity={1} fill="url(#colorRam)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* --- Node Details Grid --- */}
        <div className="bg-white border border-gray-200 rounded-xl shadow-sm p-6 flex flex-col mt-2">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <Server className="h-5 w-5 text-gray-500" />
              <h2 className="text-base font-extrabold text-gray-800">Node Status</h2>
            </div>
            <span className="text-xs font-bold bg-gray-100 text-gray-600 px-3 py-1 rounded-full">
              {onlineNodes.length} / {totalNodes} Online
            </span>
          </div>
          
          {/* ลบ 2xl:grid-cols-5 ทิ้งไป และปรับ gap-3 เป็น gap-4 หรือ 5 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 max-h-[500px] overflow-y-auto pr-2 custom-scrollbar">
            {controlPlane && <NodeGridItem key={controlPlane.ID} node={controlPlane} />}
            {workerNodes.map(node => (
              <NodeGridItem key={node.ID} node={node} />
            ))}
          </div>
        </div>

      </div>
    </div>
  );
}