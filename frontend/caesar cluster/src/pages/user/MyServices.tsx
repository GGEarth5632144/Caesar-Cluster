import { useState, useEffect } from "react";
import {
  Box,
  Cpu,
  Layers,
  Network,
  Loader2,
  Plus,
  X,
  Check,
  AlertTriangle,
} from "lucide-react";

import { cn } from "@/lib/utils";
import axiosClient from "@/api/axiosClient";
import { serviceApi, type AppService } from "@/api/services";
import { getApiErrorMessage } from "@/api/authApi";
import type { RequestTemplate } from "@/api/adminrequest";

function initialsOf(name: string) {
  const cleaned = name.replace(/[^a-zA-Z0-9]/g, "");
  return (cleaned.slice(0, 2) || "??").toUpperCase();
}

function statusBadge(status: AppService["status"]) {
  switch (status) {
    case "running":
      return { label: "Running", dot: "bg-green-600", text: "text-green-700", bg: "bg-green-50" };
    case "creating":
      return { label: "Deploying...", dot: "bg-[#F08B51] animate-pulse", text: "text-[#F08B51]", bg: "bg-[#FFF8E8]" };
    case "failed":
    default:
      return { label: "Failed", dot: "bg-red-500", text: "text-red-600", bg: "bg-red-50" };
  }
}

export default function MyServices() {
  const [services, setServices] = useState<AppService[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [pendingDeleteId, setPendingDeleteId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);

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
  }, []);

  const runningCount = services.filter((s) => s.status === "running").length;
  const deployingCount = services.filter((s) => s.status === "creating").length;

  const handleDelete = async (id: number) => {
    setDeletingId(id);
    try {
      await serviceApi.remove(id);
      setServices((prev) => prev.filter((s) => s.id !== id));
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
          <h1 className="text-4xl font-bold text-[#211a14]">My Services</h1>
          <p className="text-sm text-[#211a14]/50 mt-1">
            {loading
              ? "Loading..."
              : `${services.length} total · ${runningCount} running · ${deployingCount} deploying`}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-2 self-start rounded-xl bg-[#BB6653] px-5 py-3 text-sm font-bold text-white shadow-md transition-colors hover:bg-[#F08B51]"
        >
          <Plus size={16} strokeWidth={3} /> New Service
        </button>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-red-50 text-red-600 text-sm border border-red-100 max-w-3xl">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex h-full min-h-[300px] flex-col items-center justify-center gap-3 text-[#BB6653]">
          <Loader2 size={36} className="animate-spin" />
          <p className="text-sm font-medium">Loading your services...</p>
        </div>
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
                    <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-[#FBDFDA] text-sm font-bold text-[#BB6653]">
                      {initialsOf(svc.name)}
                    </div>
                    <div className="min-w-0">
                      <p className="font-semibold text-[#211a14] truncate">{svc.name}</p>
                      <p className="text-xs text-[#211a14]/45 truncate">{svc.image}</p>
                    </div>
                  </div>
                  <span
                    className={cn(
                      "inline-flex shrink-0 items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-bold whitespace-nowrap",
                      badge.bg,
                      badge.text
                    )}
                  >
                    <span className={cn("size-1.5 rounded-full", badge.dot)} />
                    {badge.label}
                  </span>
                </div>

                <div className="grid grid-cols-3 gap-2 pt-3 border-t border-black/5 text-xs font-medium text-[#211a14]/70">
                  <div className="flex items-center gap-1.5">
                    <Cpu size={14} className="text-[#BB6653]" />
                    {(svc.cpu_milli / 1000).toFixed(1)} cores
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Layers size={14} className="text-[#BB6653]" />
                    {svc.ram_mb >= 1024 ? `${(svc.ram_mb / 1024).toFixed(1)} GB` : `${svc.ram_mb} MB`}
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Network size={14} className="text-[#BB6653]" />
                    {svc.node_port ? `:${svc.node_port}` : "—"}
                  </div>
                </div>

                {isConfirming ? (
                  <div className="flex items-center gap-2 pt-1">
                    <button
                      type="button"
                      disabled={isDeleting}
                      onClick={() => handleDelete(svc.id)}
                      className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-xl bg-red-500 px-3 py-2 text-xs font-bold text-white transition-colors hover:bg-red-600 disabled:opacity-60"
                    >
                      {isDeleting ? <Loader2 size={13} className="animate-spin" /> : "Confirm delete"}
                    </button>
                    <button
                      type="button"
                      disabled={isDeleting}
                      onClick={() => setPendingDeleteId(null)}
                      className="rounded-xl border border-black/10 px-3 py-2 text-xs font-bold text-[#211a14]/60 transition-colors hover:bg-black/[0.03]"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => setPendingDeleteId(svc.id)}
                    className="inline-flex items-center justify-center gap-1.5 rounded-xl border border-black/10 px-3 py-2 text-xs font-bold text-[#211a14]/60 transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600"
                  >
                    <X size={13} /> Delete
                  </button>
                )}
              </div>
            );
          })}

          <button
            type="button"
            onClick={() => setShowCreate(true)}
            className="rounded-2xl border-2 border-dashed border-black/10 p-6 flex flex-col items-center justify-center gap-2 text-[#211a14]/40 transition-colors hover:border-[#BB6653]/40 hover:text-[#BB6653] min-h-[168px]"
          >
            <Plus size={26} />
            <span className="text-sm font-semibold">Deploy a new service</span>
          </button>
        </div>
      )}

      {showCreate && (
        <CreateServiceModal
          onClose={() => setShowCreate(false)}
          onCreated={(svc) => {
            setServices((prev) => [svc, ...prev]);
            setShowCreate(false);
          }}
        />
      )}
    </div>
  );
}

interface CreateServiceModalProps {
  onClose: () => void;
  onCreated: (svc: AppService) => void;
}

function CreateServiceModal({ onClose, onCreated }: CreateServiceModalProps) {
  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  const [mode, setMode] = useState<"preset" | "custom">("preset");
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);
  const [customCores, setCustomCores] = useState("0.5");
  const [customRamMb, setCustomRamMb] = useState("512");

  const [templates, setTemplates] = useState<RequestTemplate[]>([]);
  const [loadingTemplates, setLoadingTemplates] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoadingTemplates(true);
    axiosClient
      .get<{ success: boolean; data: RequestTemplate[] }>("/request-templates")
      .then((res) => setTemplates(res.data.data))
      .catch((err) => console.error(err))
      .finally(() => setLoadingTemplates(false));
  }, []);

  const canSubmit =
    image.trim().length >= 3 &&
    name.trim().length >= 3 &&
    (mode === "preset" ? selectedTemplateId !== null : Number(customCores) > 0 && Number(customRamMb) > 0);

  const handleSubmit = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const svc = await serviceApi.create({
        name: name.trim(),
        image: image.trim(),
        ...(mode === "preset"
          ? { request_template_id: selectedTemplateId! }
          : {
              cpu_milli: Math.round(Number(customCores) * 1000),
              ram_mb: Math.round(Number(customRamMb)),
            }),
      });
      onCreated(svc);
    } catch (err) {
      console.error(err);
      setError(getApiErrorMessage(err, "Deploy ไม่สำเร็จ"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4 font-mono">
      <div className="w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-3xl bg-[#FFF8E8] border border-black/5 shadow-xl">
        <div className="flex items-center justify-between px-6 py-5 border-b border-black/5">
          <div>
            <h2 className="text-lg font-bold text-[#211a14]">Deploy a new service</h2>
            <p className="text-xs text-[#211a14]/50 mt-0.5">
              Point us at a container image — we handle the rest.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="p-2 rounded-xl text-[#211a14]/50 hover:bg-black/5 transition-colors disabled:opacity-50"
          >
            <X size={18} />
          </button>
        </div>

        <div className="px-6 py-5 flex flex-col gap-5">
          {error && (
            <div className="flex items-start gap-2 p-3 rounded-xl bg-red-50 text-red-600 text-xs border border-red-100">
              <AlertTriangle size={14} className="shrink-0 mt-0.5" />
              {error}
            </div>
          )}

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">
              Container Image
            </label>
            <div className="flex items-center gap-2 rounded-xl border border-black/10 bg-white px-3.5 py-2.5">
              <Box size={16} className="text-[#211a14]/30 shrink-0" />
              <input
                value={image}
                onChange={(e) => setImage(e.target.value)}
                disabled={submitting}
                placeholder="nginx:latest, ghcr.io/you/app:tag"
                className="w-full bg-transparent text-sm text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
              />
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">
              Service Name
            </label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={submitting}
              placeholder="my-web-app"
              className="w-full rounded-xl border border-black/10 bg-white px-3.5 py-2.5 text-sm text-[#211a14] placeholder:text-[#211a14]/30 outline-none disabled:opacity-60"
            />
            <p className="text-[11px] text-[#211a14]/40">
              lowercase letters, numbers and hyphens only — must start/end with a letter or number
            </p>
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">
                Resources
              </label>
              <div className="flex rounded-lg bg-black/5 p-0.5 text-xs font-semibold">
                <button
                  type="button"
                  disabled={submitting}
                  onClick={() => setMode("preset")}
                  className={cn(
                    "px-3 py-1 rounded-md transition-colors",
                    mode === "preset" ? "bg-white text-[#211a14] shadow-sm" : "text-[#211a14]/40"
                  )}
                >
                  Preset
                </button>
                <button
                  type="button"
                  disabled={submitting}
                  onClick={() => setMode("custom")}
                  className={cn(
                    "px-3 py-1 rounded-md transition-colors",
                    mode === "custom" ? "bg-white text-[#211a14] shadow-sm" : "text-[#211a14]/40"
                  )}
                >
                  Custom
                </button>
              </div>
            </div>

            {mode === "preset" ? (
              loadingTemplates ? (
                <div className="flex items-center justify-center gap-2 py-6 text-[#BB6653]">
                  <Loader2 size={20} className="animate-spin" />
                </div>
              ) : templates.length === 0 ? (
                <p className="text-xs text-gray-400 py-4 text-center bg-black/[0.02] rounded-xl border border-dashed border-black/10">
                  ไม่มี preset เปิดใช้งานอยู่ในขณะนี้ — ลองใช้แท็บ Custom แทน
                </p>
              ) : (
                <div className="grid grid-cols-2 gap-2.5">
                  {templates.map((tpl) => {
                    const isSelected = selectedTemplateId === tpl.id;
                    return (
                      <button
                        type="button"
                        key={tpl.id}
                        disabled={submitting}
                        onClick={() => setSelectedTemplateId(tpl.id)}
                        className={cn(
                          "relative text-left rounded-xl border p-3 transition-all bg-white",
                          isSelected
                            ? "border-[#BB6653] ring-2 ring-[#BB6653]/10"
                            : "border-black/10 hover:border-[#F08B51]/50"
                        )}
                      >
                        {isSelected && (
                          <Check size={14} strokeWidth={3} className="absolute top-2 right-2 text-[#BB6653]" />
                        )}
                        <p className="text-xs font-bold text-[#211a14] pr-4 truncate">{tpl.option_name}</p>
                        <p className="text-[11px] text-[#211a14]/50 mt-1">
                          {(tpl.cpu_limit_milli / 1000).toFixed(1)} cores ·{" "}
                          {tpl.ram_limit_mb >= 1024 ? `${(tpl.ram_limit_mb / 1024).toFixed(1)} GB` : `${tpl.ram_limit_mb} MB`}
                        </p>
                      </button>
                    );
                  })}
                </div>
              )
            ) : (
              <div className="grid grid-cols-2 gap-3">
                <div className="flex flex-col gap-1">
                  <label className="text-[11px] text-[#211a14]/50">CPU (cores)</label>
                  <input
                    type="number"
                    step="0.1"
                    min="0.1"
                    max="3"
                    value={customCores}
                    disabled={submitting}
                    onChange={(e) => setCustomCores(e.target.value)}
                    className="rounded-xl border border-black/10 bg-white px-3 py-2 text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <label className="text-[11px] text-[#211a14]/50">Memory (MB)</label>
                  <input
                    type="number"
                    step="128"
                    min="128"
                    max="2048"
                    value={customRamMb}
                    disabled={submitting}
                    onChange={(e) => setCustomRamMb(e.target.value)}
                    className="rounded-xl border border-black/10 bg-white px-3 py-2 text-sm text-[#211a14] outline-none disabled:opacity-60"
                  />
                </div>
              </div>
            )}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 px-6 py-4 border-t border-black/5">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-xl px-4 py-2.5 text-sm font-bold text-[#211a14]/60 transition-colors hover:bg-black/5 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!canSubmit || submitting}
            onClick={handleSubmit}
            className={cn(
              "inline-flex items-center gap-2 rounded-xl px-5 py-2.5 text-sm font-bold text-white shadow-md transition-all",
              canSubmit && !submitting
                ? "bg-[#BB6653] hover:bg-[#F08B51]"
                : "bg-[#211a14]/20 cursor-not-allowed shadow-none"
            )}
          >
            {submitting && <Loader2 size={14} className="animate-spin" />}
            {submitting ? "Deploying..." : "Deploy"}
          </button>
        </div>
      </div>
    </div>
  );
}
