import React, { useEffect, useState, useMemo } from "react";
import {
  Cpu,
  AlertTriangle,
  RefreshCw,
  Users,
  ClipboardList,
  Activity,
  HardDrive,
  Clock3,
  Zap,
  PlugZap,
} from "lucide-react";
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";

// ─── API Imports ───
import { 
  nodetelemetry, 
  type NodeTelemetry, 
  type ClusterHistoryData, 
  type PowerNode, 
  type PowerHistoryData 
} from "@/api/mornitorequest";
import { adminVmRequestApi, type AdminVmRequest } from "@/api/requests";
import { userManagementApi } from "@/api/adminuser";
import { getApiErrorMessage } from "@/api/authApi";

// ─── Main Component ───
export default function AdminDashboard() {
  const [nodes, setNodes] = useState<NodeTelemetry[]>([]);
  const [historyData, setHistoryData] = useState<ClusterHistoryData[]>([]);
  const [powers, setPowers] = useState<PowerNode[]>([]);
  const [powerHistory, setPowerHistory] = useState<PowerHistoryData[]>([]);
  const [requests, setRequests] = useState<AdminVmRequest[]>([]);
  const [totalUsers, setTotalUsers] = useState<number>(0);

  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);
  const [timeRange, setTimeRange] = useState("24h");

  const fetchDashboardData = async () => {
    setLoading(true);
    try {
      // โหลดทุกสรรพสิ่งมาพร้อมกันในทีเดียว!
      const [nodesData, historyRes, powerData, powerHistData, reqData, usersData] = await Promise.all([
        nodetelemetry.getAll(),
        nodetelemetry.getHistory(timeRange),
        nodetelemetry.getAllPower(),
        nodetelemetry.getPowerHistory(timeRange),
        adminVmRequestApi.listAll(),
        userManagementApi.getAll(),
      ]);

      setNodes(nodesData.sort((a, b) => a.NodeName.localeCompare(b.NodeName)));
      setHistoryData(historyRes);
      setPowers(powerData.sort((a, b) => a.ModbusID - b.ModbusID));
      setPowerHistory(powerHistData);
      setRequests(reqData);
      setTotalUsers(usersData.length);

      setLastUpdate(new Date());
      setError(null);
    } catch (err) {
      setError(getApiErrorMessage(err, "ไม่สามารถดึงข้อมูลระบบได้"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDashboardData();
    const interval = setInterval(fetchDashboardData, 30000);
    return () => clearInterval(interval);
  }, [timeRange]);

  // ─── Data Processing ───
  // Nodes & Telemetry
  const totalNodes = nodes.length;
  const onlineNodes = nodes.filter((n) => n.IsUp === 1);
  const avgTemp = onlineNodes.length > 0 ? onlineNodes.reduce((s, n) => s + n.Temperature, 0) / onlineNodes.length : 0;
  
  // Power
  const totalWatt = powers.reduce((sum, p) => sum + p.Watt, 0);
  const activePowerNodes = powers.filter(p => p.Watt > 0).length;

  // VM Requests
  const pendingRequests = requests.filter(r => r.status === 'pending');
  const approvedRequests = requests.filter(r => r.status === 'approved');
  const deniedRequests = requests.filter(r => r.status === 'denied');

  const requestStatusData = [
    { name: 'Pending', value: pendingRequests.length, color: '#f59e0b' },
    { name: 'Approved', value: approvedRequests.length, color: '#10b981' },
    { name: 'Denied', value: deniedRequests.length, color: '#ef4444' },
  ];

  const requestTimelineData = useMemo(() => {
    const grouped: Record<string, number> = {};
    requests.forEach(req => {
      const date = new Date(req.created_at).toLocaleDateString('th-TH', { day: 'numeric', month: 'short' });
      grouped[date] = (grouped[date] || 0) + 1;
    });
    return Object.entries(grouped).slice(-7).map(([date, count]) => ({ date, requests: count }));
  }, [requests]);

  if (loading && nodes.length === 0) {
    return (
      <div className="max-w-[1400px] mx-auto font-mono flex h-screen items-center justify-center bg-[#FFFDF6]">
        <div className="flex flex-col items-center gap-3 text-[#BB6653]/60">
          <RefreshCw className="h-10 w-10 animate-spin" />
          <p className="text-xs font-bold uppercase tracking-widest">Gathering System Intel...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-[1400px] mx-auto font-sans flex flex-col gap-6 p-6 bg-[#FFFDF6] min-h-screen">
      
      {/* ── Header ── */}
      <div className="rounded-3xl bg-white shadow-sm p-6 border border-black/5 flex flex-col md:flex-row items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-black text-[#BB6653] flex items-center gap-3">
            <Activity className="h-7 w-7" />
            Caesar Admin Command Center
          </h1>
          <p className="text-xs font-bold text-gray-400 mt-1">
            {lastUpdate ? `Last updated: ${lastUpdate.toLocaleTimeString("th-TH")}` : "Loading..."}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <select 
            value={timeRange}
            onChange={(e) => setTimeRange(e.target.value)}
            className="bg-gray-50 border border-gray-200 text-[#BB6653] font-bold rounded-xl py-2 px-4 outline-none text-sm cursor-pointer"
          >
            <option value="1h">Last 1 Hour</option>
            <option value="6h">Last 6 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
          </select>
          <button
            onClick={fetchDashboardData}
            disabled={loading}
            className="flex items-center gap-2 rounded-xl bg-[#BB6653] px-5 py-2.5 text-sm font-bold text-white hover:bg-[#F08B51] transition-colors disabled:opacity-50 cursor-pointer"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            Sync Data
          </button>
        </div>
      </div>

      {/* ── Error Banner ── */}
      {error && (
        <div className="flex items-center gap-3 bg-red-50 border border-red-200 text-red-700 px-5 py-4 rounded-2xl">
          <AlertTriangle className="h-5 w-5 shrink-0" />
          <p className="text-sm font-bold">{error}</p>
        </div>
      )}

      {/* ── Row 1: KPI Stat Cards (6 Columns on Large Screens) ── */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-5">
        
        {/* Card 1: VM Requests */}
        <div className={`rounded-3xl bg-white shadow-sm p-5 border-l-4 ${pendingRequests.length > 0 ? 'border-orange-500' : 'border-gray-200'}`}>
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Pending Resorce</p>
              <h2 className={`text-2xl font-black mt-2 ${pendingRequests.length > 0 ? 'text-orange-500 animate-pulse' : 'text-gray-800'}`}>
                {pendingRequests.length}
              </h2>
            </div>
            <div className={`p-2.5 rounded-xl ${pendingRequests.length > 0 ? 'bg-orange-50 text-orange-500' : 'bg-gray-50 text-gray-400'}`}>
              <ClipboardList className="h-5 w-5" />
            </div>
          </div>
        </div>

        {/* Card 2: Users */}
        <div className="rounded-3xl bg-white shadow-sm p-5 border-l-4 border-blue-400">
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Total Users</p>
              <h2 className="text-2xl font-black mt-2 text-gray-800">{totalUsers}</h2>
            </div>
            <div className="p-2.5 rounded-xl bg-blue-50 text-blue-500">
              <Users className="h-5 w-5" />
            </div>
          </div>
        </div>

        {/* Card 3: Nodes */}
        <div className="rounded-3xl bg-white shadow-sm p-5 border-l-4 border-emerald-400">
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Nodes Online</p>
              <h2 className="text-2xl font-black mt-2 text-gray-800">{onlineNodes.length} <span className="text-sm text-gray-400 font-medium">/ {totalNodes}</span></h2>
            </div>
            <div className="p-2.5 rounded-xl bg-emerald-50 text-emerald-500">
              <Cpu className="h-5 w-5" />
            </div>
          </div>
        </div>

        {/* Card 4: Temp */}
        <div className="rounded-3xl bg-white shadow-sm p-5 border-l-4 border-[#BB6653]">
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Avg Temp</p>
              <h2 className={`text-2xl font-black mt-2 ${avgTemp >= 75 ? 'text-red-500' : 'text-gray-800'}`}>
                {avgTemp.toFixed(1)} <span className="text-sm text-gray-400 font-medium">°C</span>
              </h2>
            </div>
            <div className="p-2.5 rounded-xl bg-[#BB6653]/10 text-[#BB6653]">
              <Activity className="h-5 w-5" />
            </div>
          </div>
        </div>

        {/* Card 5: Power */}
        <div className="rounded-3xl bg-white shadow-sm p-5 border-l-4 border-amber-400">
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Total Power</p>
              <h2 className="text-2xl font-black mt-2 text-gray-800">
                {totalWatt.toFixed(0)} <span className="text-sm text-gray-400 font-medium">W</span>
              </h2>
            </div>
            <div className="p-2.5 rounded-xl bg-amber-50 text-amber-500">
              <Zap className="h-5 w-5" />
            </div>
          </div>
        </div>

        {/* Card 6: Active Load */}
        <div className="rounded-3xl bg-white shadow-sm p-5 border-l-4 border-indigo-400">
          <div className="flex justify-between items-start">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-widest text-gray-400">Active Relays</p>
              <h2 className="text-2xl font-black mt-2 text-gray-800">
                {activePowerNodes} <span className="text-sm text-gray-400 font-medium">/ {powers.length}</span>
              </h2>
            </div>
            <div className="p-2.5 rounded-xl bg-indigo-50 text-indigo-500">
              <PlugZap className="h-5 w-5" />
            </div>
          </div>
        </div>

      </div>

      {/* ── Row 2: Analytics Charts (2x2 Grid) ── */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        
        {/* Chart 1: VM Donut */}
        <div className="bg-white rounded-3xl shadow-sm p-6 border border-black/5 flex flex-col h-[360px]">
          <h2 className="text-sm font-extrabold text-gray-800 mb-4">Resorce Requests Status</h2>
          <div className="flex-1 w-full min-h-0">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie data={requestStatusData} cx="50%" cy="50%" innerRadius={65} outerRadius={95} paddingAngle={5} dataKey="value">
                  {requestStatusData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }} />
                <Legend verticalAlign="bottom" height={36} iconType="circle" />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Chart 2: VM Trend Bar */}
        <div className="bg-white rounded-3xl shadow-sm p-6 border border-black/5 flex flex-col h-[360px]">
          <h2 className="text-sm font-extrabold text-gray-800 mb-4">Resorce Requests Trend (Last 7 Days)</h2>
          <div className="flex-1 w-full min-h-0">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={requestTimelineData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f3f4f6" vertical={false} />
                <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#9ca3af' }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 11, fill: '#9ca3af' }} axisLine={false} tickLine={false} allowDecimals={false} />
                <Tooltip cursor={{ fill: '#f3f4f6' }} contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }} />
                <Bar dataKey="requests" name="New Requests" fill="#BB6653" radius={[6, 6, 0, 0]} barSize={32} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Chart 3: Cluster RAM Area */}
        <div className="bg-white rounded-3xl shadow-sm p-6 border border-black/5 flex flex-col h-[360px]">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-extrabold text-gray-800">Cluster Memory Trend</h2>
            <HardDrive className="h-4 w-4 text-blue-500" />
          </div>
          <div className="flex-1 w-full min-h-0">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={historyData} margin={{ top: 10, right: 10, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f3f4f6" vertical={false} />
                <XAxis 
                  dataKey="time" 
                  tick={{ fontSize: 11, fill: '#9ca3af' }} 
                  tickMargin={10} 
                  axisLine={false} 
                  tickLine={false} 
                  tickFormatter={(val) => {
                    const date = new Date(val);
                    return isNaN(date.getTime()) ? val : date.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' });
                  }}
                />
                <YAxis tick={{ fontSize: 11, fill: '#9ca3af' }} tickFormatter={(val) => `${(val/1024).toFixed(0)}G`} axisLine={false} tickLine={false} />
                <Tooltip 
                  labelFormatter={(label) => new Date(label).toLocaleTimeString('th-TH')}
                  formatter={(value: any) => [(Number(value)/1024).toFixed(1) + " GB", "RAM Used"]}
                  contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
                />
                <defs>
                  <linearGradient id="colorRam" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/>
                    <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <Area type="monotone" dataKey="totalRam" stroke="#3b82f6" strokeWidth={2.5} fillOpacity={1} fill="url(#colorRam)" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Chart 4: Power Consumption Area */}
        <div className="bg-white rounded-3xl shadow-sm p-6 border border-black/5 flex flex-col h-[360px]">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-extrabold text-gray-800">Total Power Consumption Trend</h2>
            <Zap className="h-4 w-4 text-amber-500" />
          </div>
          <div className="flex-1 w-full min-h-0">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={powerHistory} margin={{ top: 10, right: 10, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f3f4f6" vertical={false} />
                <XAxis 
                  dataKey="time" 
                  tick={{ fontSize: 11, fill: '#9ca3af' }} 
                  tickMargin={10} 
                  axisLine={false} 
                  tickLine={false} 
                  tickFormatter={(val) => {
                    const date = new Date(val);
                    return isNaN(date.getTime()) ? val : date.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' });
                  }}
                />
                <YAxis tick={{ fontSize: 11, fill: '#9ca3af' }} tickFormatter={(val) => `${val}W`} axisLine={false} tickLine={false} />
                <Tooltip 
                  labelFormatter={(label) => new Date(label).toLocaleTimeString('th-TH')}
                  formatter={(value: any) => [Number(value).toFixed(2) + " W", "Total Power"]}
                  contentStyle={{ borderRadius: '12px', border: 'none', boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}
                />
                <defs>
                  <linearGradient id="colorPower" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3}/>
                    <stop offset="95%" stopColor="#f59e0b" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <Area type="monotone" dataKey="totalWatt" stroke="#f59e0b" strokeWidth={2.5} fillOpacity={1} fill="url(#colorPower)" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

      </div>

      {/* ── Row 3: Action Required (Full Width Table/List) ── */}
      <div className="bg-white rounded-3xl shadow-sm p-6 border border-black/5 flex flex-col">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-base font-extrabold text-gray-800 flex items-center gap-2">
            <Clock3 className="h-5 w-5 text-orange-500" />
            Action Required: Pending Resorce Requests
          </h2>
          <span className="text-xs font-bold bg-orange-100 text-orange-600 px-3 py-1 rounded-full">
            {pendingRequests.length} Pending
          </span>
        </div>
        
        <div className="max-h-[300px] overflow-y-auto pr-2 custom-scrollbar flex flex-col gap-3">
          {pendingRequests.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-10 text-gray-400">
              <ClipboardList className="h-12 w-12 mb-2 opacity-20" />
              <p className="text-sm font-bold">No pending requests! All clear.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {pendingRequests.map(req => (
                <div key={req.id} className="p-4 rounded-2xl bg-orange-50/40 border border-orange-100 hover:shadow-md transition-all flex flex-col justify-between gap-3">
                  <div className="flex justify-between items-start">
                    <div>
                      <p className="text-sm font-bold text-gray-800">{req.requester_name}</p>
                      <p className="text-xs font-mono text-gray-500">{req.requester_student_id}</p>
                    </div>
                    <span className="text-[11px] font-bold bg-white text-orange-500 px-2.5 py-1 rounded-xl border border-orange-200">
                      {req.namespace_name.toUpperCase()}
                    </span>
                  </div>
                  <div className="flex items-center justify-between bg-white p-2.5 rounded-xl border border-gray-100 text-xs font-mono font-semibold text-gray-600">
                    <span>CPU: {req.cpu_limit_milli}m</span>
                    <span>RAM: {req.ram_limit_mb}MB</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

    </div>
  );
}