import { useEffect, useState } from "react";
import { Server, ServerCrash, Thermometer, Cpu, Activity, AlertTriangle, RefreshCw } from "lucide-react";

import { nodetelemetry, type NodeTelemetry } from "@/api/mornitorequest";
import { getApiErrorMessage } from "@/api/authApi";

export default function IPCmanagement() {
  const [nodes, setNodes] = useState<NodeTelemetry[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);

  const fetchClusterData = async () => {
    setLoading(true); // เพิ่มให้ปุ่มรู้สถานะว่ากำลังโหลดอยู่
    try {
      const data = await nodetelemetry.getAll();
      const sortedData = data.sort((a, b) => a.NodeName.localeCompare(b.NodeName));

      setNodes(sortedData);
      setLastUpdate(new Date());
      setError(null);
    } catch (err) {
      setError(getApiErrorMessage(err, "ไม่สามารถดึงข้อมูลคลัสเตอร์ได้"));
    } finally {
      setLoading(false);
    }
  };

  // ดึงข้อมูลแค่ครั้งแรกตอนเปิดหน้าเว็บเท่านั้น
  useEffect(() => {
    fetchClusterData();
  }, []);

  // -------------------------------------------------------------------------
  // 1. แยกข้อมูล Control Plane (intelnuc) และ Worker Nodes (node01-node40)
  // -------------------------------------------------------------------------
  const controlPlane = nodes.find((n) => n.NodeName === "intelnuc");
  const workerNodes = nodes.filter((n) => n.NodeName !== "intelnuc");

  const totalWorkers = workerNodes.length;
  const upWorkers = workerNodes.filter((n) => n.IsUp === 1).length;

  // -------------------------------------------------------------------------
  // 2. Component ย่อยสำหรับวาด "การ์ด 1 ใบ" (ใช้ร่วมกันทั้ง NUC และ Worker)
  // -------------------------------------------------------------------------
  const renderNodeCard = (node: NodeTelemetry, isControlPlane: boolean = false) => {
    const isOffline = node.IsUp === 0;
    const isHot = node.Temperature >= 75;
    const isWarm = node.Temperature >= 60 && node.Temperature < 75;

    let cardBorder = "border-neutral-200 bg-white";
    let tempColor = "text-emerald-600";
    let tempBg = "bg-emerald-50";

    if (isOffline) {
      cardBorder = "border-red-300 bg-red-50 opacity-80";
    } else if (isHot) {
      cardBorder = "border-red-500 bg-white shadow-sm shadow-red-100";
      tempColor = "text-red-600 font-bold animate-pulse";
      tempBg = "bg-red-100";
    } else if (isWarm) {
      cardBorder = "border-amber-400 bg-white shadow-sm shadow-amber-50";
      tempColor = "text-amber-600 font-bold";
      tempBg = "bg-amber-100";
    }

    const ramDisplay = node.RamUsedMB > 1024 
      ? `${(node.RamUsedMB / 1024).toFixed(2)} GB` 
      : `${node.RamUsedMB.toFixed(0)} MB`;

    return (
      <div key={node.ID} className={`relative p-5 rounded-2xl border transition-all duration-300 ${cardBorder}`}>
        {isControlPlane && (
          <div className="absolute -top-2.5 -right-2.5 bg-indigo-600 text-white text-[10px] font-bold px-3 py-1 rounded-full shadow-md z-10">
            Control Plane
          </div>
        )}

        <div className="flex justify-between items-center mb-4 border-b border-neutral-100 pb-3">
          <div className="flex items-center gap-2">
            {isOffline ? <ServerCrash className="h-5 w-5 text-red-500" /> : <Server className="h-5 w-5 text-neutral-600" />}
            <span className={`font-bold text-base ${isOffline ? 'text-red-700' : 'text-neutral-800'}`}>
              {node.NodeName}
            </span>
          </div>
          
          <span className="relative flex h-3.5 w-3.5">
            {isOffline ? (
              <span className="relative inline-flex rounded-full h-3.5 w-3.5 bg-red-500"></span>
            ) : (
              <>
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-3.5 w-3.5 bg-emerald-500"></span>
              </>
            )}
          </span>
        </div>

        {isOffline ? (
          <div className="py-6 text-center text-red-500 font-medium text-sm flex flex-col items-center gap-2">
            <ServerCrash className="h-8 w-8 opacity-50" />
            Connection Lost
          </div>
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <div className="flex justify-between items-center">
              <div className="flex items-center gap-1.5 text-neutral-500">
                <Thermometer className="h-4 w-4" />
                <span>Temp</span>
              </div>
              <span className={`px-2 py-0.5 rounded-md text-xs font-mono ${tempColor} ${tempBg}`}>
                {node.Temperature.toFixed(1)} °C
              </span>
            </div>

            <div className="flex justify-between items-center">
              <div className="flex items-center gap-1.5 text-neutral-500">
                <Cpu className="h-4 w-4" />
                <span>RAM Used</span>
              </div>
              <span className="font-medium font-mono text-neutral-700">
                {ramDisplay}
              </span>
            </div>

            <div className="flex justify-between items-center">
              <div className="flex items-center gap-1.5 text-neutral-500">
                <Activity className="h-4 w-4" />
                <span>Processes</span>
              </div>
              <span className="font-medium font-mono text-neutral-700">
                {node.Procs}
              </span>
            </div>
          </div>
        )}
      </div>
    );
  };

  if (loading && nodes.length === 0) {
    return (
      <div className="flex h-full min-h-[400px] items-center justify-center rounded-xl bg-neutral-100">
        <div className="flex flex-col items-center gap-3 text-neutral-500">
          <RefreshCw className="h-8 w-8 animate-spin" />
          <p>กำลังเชื่อมต่อข้อมูลคลัสเตอร์...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-2">
      {/* ---------------- Header & Stats ---------------- */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-white p-5 rounded-2xl shadow-sm border border-neutral-200">
        <div>
          <h1 className="text-xl font-bold text-neutral-800 flex items-center gap-2">
            <Server className="h-6 w-6 text-indigo-600" />
            Cluster Node Monitor
          </h1>
          <p className="text-sm text-neutral-500 mt-1">
            อัปเดตล่าสุด: {lastUpdate ? lastUpdate.toLocaleTimeString('th-TH') : "กำลังรอข้อมูล"}
          </p>
        </div>
        
        <div className="flex items-center gap-6">
          {/* ปุ่มรีเฟรชข้อมูล */}
          <button
            onClick={fetchClusterData}
            disabled={loading}
            className="flex items-center gap-2 rounded-lg bg-indigo-50 px-4 py-2 text-sm font-medium text-indigo-600 transition-colors hover:bg-indigo-100 disabled:opacity-50"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            รีเฟรชข้อมูล
          </button>

          <div className="flex flex-col items-end">
            <span className="text-sm font-medium text-neutral-500">สถานะ Worker Nodes</span>
            <span className={`text-lg font-bold ${upWorkers === totalWorkers ? 'text-emerald-600' : 'text-amber-500'}`}>
              {upWorkers} / {totalWorkers} Online
            </span>
          </div>
        </div>
      </div>

      {error && (
        <div className="flex items-center gap-3 bg-red-50 border border-red-200 text-red-700 p-4 rounded-xl">
          <AlertTriangle className="h-5 w-5 flex-shrink-0" />
          <p className="text-sm font-medium">{error}</p>
        </div>
      )}

      {/* ---------------- Control Plane Section ---------------- */}
      {controlPlane && (
        <div className="flex flex-col gap-3">
          <h2 className="text-lg font-bold text-neutral-700 px-1">Control Plane</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
            {renderNodeCard(controlPlane, true)}
          </div>
        </div>
      )}

      <hr className="border-neutral-200 my-2" />

      {/* ---------------- Worker Nodes Section ---------------- */}
      <div className="flex flex-col gap-3">
        <h2 className="text-lg font-bold text-neutral-700 px-1">Worker Nodes</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
          {workerNodes.map((node) => renderNodeCard(node))}
        </div>
      </div>

    </div>
  );
}