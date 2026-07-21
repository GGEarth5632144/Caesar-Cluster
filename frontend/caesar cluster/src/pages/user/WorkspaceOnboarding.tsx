import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import type { LucideIcon } from "lucide-react";
import { Package, Cpu, Layers, HardDrive, ArrowLeft, Check, Loader2, Clock } from "lucide-react";

import { cn } from "@/lib/utils";
import axiosClient from "@/api/axiosClient";
import { getApiErrorMessage } from "@/api/authApi";
import { vmRequestApi, type VmRequest } from "@/api/requests";
import type { RequestTemplate } from "@/api/adminrequest";

export default function WorkspaceOnboarding() {
  const navigate = useNavigate();
  const [step, setStep] = useState<1 | 2>(1);
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);
  // รายละเอียดคำขอที่ผู้ใช้เขียนเอง — จะถูกส่งเป็น description ไปให้ admin เห็นใน AdminRequestQueue
  const [description, setDescription] = useState("");

  const [templates, setTemplates] = useState<RequestTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // สถานะคำขอล่าสุดของผู้ใช้ — ถ้ามีคำขอ pending อยู่แล้ว ให้โชว์หน้ารอผลแทนฟอร์ม
  const [latestRequest, setLatestRequest] = useState<VmRequest | null>(null);
  const [checkingRequests, setCheckingRequests] = useState(true);

  const fetchLatestRequest = async () => {
    setCheckingRequests(true);
    try {
      const mine = await vmRequestApi.listMine();
      setLatestRequest(mine.length > 0 ? mine[0] : null);
    } catch (err) {
      console.error(err);
    } finally {
      setCheckingRequests(false);
    }
  };

  useEffect(() => {
    fetchLatestRequest();
  }, []);

  useEffect(() => {
    if (step === 2) {
      setLoading(true);
      setError(null);

      axiosClient
        .get<{ success: boolean; data: RequestTemplate[] }>("/request-templates")
        .then((res) => {
          setTemplates(res.data.data);
        })
        .catch((err) => {
          console.error(err);
          setError(getApiErrorMessage(err, "ไม่สามารถดึงข้อมูลโควตาจากระบบได้"));
        })
        .finally(() => {
          setLoading(false);
        });
    }
  }, [step]);

  const handleStart = () => {
    setStep(2);
  };

  const canSubmit =
    !!selectedTemplateId && description.trim().length > 0 && !loading && !isSubmitting;

  const handleSubmitRequest = async () => {
    const template = templates.find((t) => t.id === selectedTemplateId);
    if (!template || !description.trim()) return;

    setIsSubmitting(true);
    setError(null);
    try {
      const created = await vmRequestApi.create({
        description: description.trim(),
        namespace_name: "solo",
        request_template_id: template.id,
        cpu_limit_milli: template.cpu_limit_milli,
        ram_limit_mb: template.ram_limit_mb,
      });
      setLatestRequest(created);
    } catch (err) {
      console.error(err);
      setError(getApiErrorMessage(err, "ส่งคำขอไม่สำเร็จ"));
    } finally {
      setIsSubmitting(false);
    }
  };

  if (checkingRequests) {
    return (
      <div className="flex h-full min-h-[400px] flex-col items-center justify-center gap-3 text-[#BB6653] font-mono">
        <Loader2 size={36} className="animate-spin" />
        <p className="text-sm font-medium">Checking your request status...</p>
      </div>
    );
  }

  // มีคำขอที่ยังไม่ถูกดำเนินการ (pending) → โชว์หน้ารอ admin อนุมัติแทนฟอร์ม
  if (latestRequest && latestRequest.status === "pending") {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 px-4 py-10 text-center font-mono">
        <div className="flex size-20 items-center justify-center rounded-2xl bg-[#FFF8E8] text-[#F08B51]">
          <Clock size={34} />
        </div>
        <h1 className="text-3xl font-bold text-[#211a14]">คำขอของคุณกำลังรอการอนุมัติ</h1>
        <p className="max-w-xl text-base text-[#211a14]/60">
          ทีมงานได้รับคำขอสร้าง Virtual Machine ของคุณแล้ว กรุณารอ Admin ตรวจสอบและอนุมัติ
        </p>
        <button
          type="button"
          onClick={() => navigate("/request-resources")}
          className="mt-2 rounded-xl bg-[#BB6653] px-6 py-3 text-sm font-bold text-white shadow-md transition-colors hover:bg-[#F08B51]"
        >
          ดูสถานะคำขอ
        </button>
      </div>
    );
  }

  // คำขอล่าสุดเพิ่งได้รับอนุมัติ แต่ session ปัจจุบันยังไม่มี namespace_id (ต้อง refresh token)
  if (latestRequest && latestRequest.status === "approved") {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 px-4 py-10 text-center font-mono">
        <div className="flex size-20 items-center justify-center rounded-2xl bg-[#DEE8CE] text-[#5A8F5A]">
          <Check size={34} />
        </div>
        <h1 className="text-3xl font-bold text-[#211a14]">Virtual Machine ของคุณพร้อมใช้งานแล้ว!</h1>
        <p className="max-w-xl text-base text-[#211a14]/60">
          กรุณาเข้าสู่ระบบใหม่อีกครั้งเพื่อโหลดข้อมูล Space ล่าสุดของคุณ
        </p>
      </div>
    );
  }

  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3 px-4 py-10 text-center font-mono">
      <h1 className="text-5xl font-bold text-[#211a14]">Welcome to Caesar Cluster</h1>
      <p className="max-w-2xl text-lg text-[#211a14]/60">
        You don't have any virtual machines yet. Create your first VM to get a
        namespace and start computing.
      </p>

      <div className="mt-8 w-full max-w-3xl rounded-3xl border border-black/5 bg-[#FFFDF6] p-12 transition-all shadow-sm">

        {step === 1 && (
          <>
            <div className="flex flex-col items-center gap-3 text-center">
              <div className="flex size-20 items-center justify-center rounded-2xl bg-[#FBDFDA] text-[#BB6653]">
                <Package size={34} />
              </div>
              <h2 className="text-2xl font-semibold text-[#211a14]">Create your first VM</h2>
              <p className="max-w-lg text-base text-[#211a14]/60">
                Set up your workspace and pick a resource quota to get
                started.
              </p>
            </div>

            <div className="mt-10">
              <VmOptionCard
                icon={Cpu}
                iconBg="bg-[#FBDFDA]"
                iconColor="text-[#BB6653]"
                title="Create VM"
                description="Set up your workspace to start computing."
                selected={false}
                onClick={handleStart}
              />
            </div>
          </>
        )}

        {step === 2 && (
          <div className="text-left space-y-8 animate-in fade-in duration-200">
            <div className="flex items-center gap-4 border-b border-black/5 pb-4">
              <button
                type="button"
                disabled={isSubmitting}
                onClick={() => setStep(1)}
                className="p-2 hover:bg-black/5 rounded-xl text-[#211a14]/60 transition-colors disabled:opacity-50"
                title="ย้อนกลับ"
              >
                <ArrowLeft size={20} />
              </button>
              <div>
                <h2 className="text-xl font-bold text-[#211a14]">
                  Configure your VM
                </h2>
                <p className="text-sm text-[#211a14]/60">เลือกโควตาทรัพยากรที่ต้องการยื่นขอ</p>
              </div>
            </div>

            {loading && (
              <div className="flex flex-col items-center justify-center py-12 gap-3 text-[#BB6653]">
                <Loader2 size={32} className="animate-spin" />
                <p className="text-sm font-medium">กำลังโหลดเทมเพลต Quota ทั้งหมด...</p>
              </div>
            )}

            {error && (
              <div className="p-4 rounded-xl bg-red-50 text-red-600 text-sm font-medium text-center border border-red-100">
                {error}
              </div>
            )}

            {!loading && !error && (
              <div className="space-y-3">
                <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">
                  1. Select Available Quota
                </label>

                {templates.length === 0 ? (
                  <p className="text-sm text-gray-400 py-6 text-center bg-black/[0.01] rounded-2xl border border-dashed border-black/10">
                    ไม่มีกลุ่มโควตาเปิดให้คุณใช้งานในขณะนี้
                  </p>
                ) : (
                  <div className="grid gap-4 sm:grid-cols-2">
                    {templates.map((template) => {
                      const isSelected = selectedTemplateId === template.id;
                      const coreDisplay = template.cpu_limit_milli / 1000;
                      const ramDisplay = template.ram_limit_mb >= 1024
                        ? `${(template.ram_limit_mb / 1024).toFixed(0)} GB`
                        : `${template.ram_limit_mb} MB`;

                      return (
                        <div
                          key={template.id}
                          onClick={() => !isSubmitting && setSelectedTemplateId(template.id)}
                          className={cn(
                            "relative cursor-pointer rounded-2xl border p-5 transition-all bg-[#FFFDF6] flex flex-col justify-between",
                            isSelected
                              ? "border-[#BB6653] ring-2 ring-[#BB6653]/10 shadow-sm"
                              : "border-black/10 hover:border-[#F08B51]/50",
                            isSubmitting && "opacity-50 cursor-not-allowed"
                          )}
                        >
                          {isSelected && (
                            <div className="absolute top-0 right-0 bg-[#DEE8CE] text-[#BB6653] font-bold px-3 py-1 rounded-bl-xl text-[11px] flex items-center gap-0.5 shadow-sm">
                              <Check size={12} strokeWidth={3} /> Selected
                            </div>
                          )}

                          <div>
                            <div className="flex items-center justify-between gap-2">
                              <span className="text-[10px] font-bold text-[#211a14]/40 uppercase tracking-wider block">
                                {template.option_name}
                              </span>
                              <span className="text-[10px] font-semibold bg-black/5 px-2 py-0.5 rounded text-[#211a14]/60">
                                {template.category}
                              </span>
                            </div>

                            <h3 className="font-semibold text-[#211a14] mt-1 pr-16 text-sm sm:text-base line-clamp-1">
                              {template.relate_subject}
                            </h3>

                            <p className="text-xs text-[#211a14]/50 mt-1 mb-4 line-clamp-2 min-h-[2rem]">
                              {template.description}
                            </p>
                          </div>

                          <div className="grid grid-cols-3 gap-1 pt-2.5 border-t border-black/5 text-[11px] font-medium text-[#211a14]/60">
                            <div className="flex items-center gap-1">
                              <Cpu size={13} className="text-[#BB6653]" />
                              {coreDisplay} Cores
                            </div>
                            <div className="flex items-center gap-1">
                              <Layers size={13} className="text-[#BB6653]" />
                              {ramDisplay}
                            </div>
                            <div className="flex items-center gap-1">
                              <HardDrive size={13} className="text-[#BB6653]" />
                              {template.storage_gb} GB
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            )}

            {!loading && !error && (
              <div className="space-y-3">
                <label className="text-xs font-bold uppercase tracking-wider text-[#BB6653]">
                  2. Request Details
                </label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  disabled={isSubmitting}
                  rows={4}
                  placeholder="อธิบายเหตุผลหรือรายละเอียดที่ต้องการขอ เช่น ใช้สำหรับวิชา... / โปรเจกต์... เพื่อให้ admin พิจารณา"
                  className="w-full resize-none rounded-2xl border border-black/10 bg-[#FFFDF6] px-4 py-3 text-sm text-[#211a14] placeholder:text-[#211a14]/30 outline-none transition-colors focus:border-[#BB6653] focus:ring-2 focus:ring-[#BB6653]/10 disabled:opacity-60"
                />
              </div>
            )}

            <div className="flex justify-end pt-2">
              <button
                type="button"
                disabled={!canSubmit}
                onClick={handleSubmitRequest}
                className={cn(
                  "rounded-xl px-6 py-3 text-sm font-bold text-white shadow-md transition-all flex items-center gap-2",
                  canSubmit
                    ? "bg-[#BB6653] hover:bg-[#F08B51]"
                    : "bg-[#211a14]/20 cursor-not-allowed shadow-none"
                )}
              >
                {isSubmitting && <Loader2 size={16} className="animate-spin" />}
                {isSubmitting ? "กำลังส่งคำขอ..." : "Submit Request"}
              </button>
            </div>
          </div>
        )}

      </div>
    </div>
  );
}

interface VmOptionCardProps {
  icon: LucideIcon;
  iconBg: string;
  iconColor: string;
  title: string;
  description: string;
  selected: boolean;
  onClick: () => void;
}

function VmOptionCard({
  icon: Icon,
  iconBg,
  iconColor,
  title,
  description,
  selected,
  onClick,
}: VmOptionCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex flex-col items-center gap-2.5 rounded-2xl border px-8 py-10 text-center transition-colors w-full",
        selected
          ? "border-[#BB6653] bg-[#FFF8E8]"
          : "border-black/10 hover:border-black/20 hover:bg-black/[0.02]"
      )}
    >
      <div className={cn("flex size-14 items-center justify-center rounded-xl", iconBg, iconColor)}>
        <Icon size={26} />
      </div>
      <p className="text-lg font-semibold text-[#211a14]">{title}</p>
      <p className="text-base text-[#211a14]/60">{description}</p>
    </button>
  );
}
