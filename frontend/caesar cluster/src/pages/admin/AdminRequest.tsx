import { useState, useEffect } from "react";
import { Plus, Edit2, Trash2, ChevronLeft, ChevronRight } from "lucide-react";
import { usePageSearch } from "@/hooks/usePageSearch";
import { requestTemplatesScope } from "@/config/searchScopes";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";
import { requestTemplateApi, type RequestTemplate, type CreateRequestTemplateDTO } from "../../api/adminrequest";
import {
  nodetelemetry,
  type NodeTelemetry,
} from "@/api/mornitorequest";
import { Skeleton } from "@/components/ui/skeleton";
import { TableRowsSkeleton } from "@/components/ui/PageSkeletons";
import { notify, confirmAction } from "@/lib/modal";

type ViewState = "list" | "create" | "edit";

export default function AdminRequest() {
  const [currentView, setCurrentView] = useState<ViewState>("list");
  const [templates, setTemplates] = useState<RequestTemplate[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [editingTemplate, setEditingTemplate] = useState<RequestTemplate | null>(null);
  
  const fetchTemplates = async () => {
    try {
      setIsLoading(true);
      const data = await requestTemplateApi.getAll();
      setTemplates(data || []);
    } catch (error) {
      console.error("ดึงข้อมูล Template ไม่สำเร็จ:", error);
      setTemplates([]); 
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchTemplates();
  }, []);

  const handleToggleStatus = async (id: number, currentStatus: boolean) => {
    try {
      const newStatus = !currentStatus;
      await requestTemplateApi.update(id, { is_active: newStatus } as Partial<CreateRequestTemplateDTO> & { is_active?: boolean });
      
      setTemplates((prev) =>
        prev.map((t) => (t.id === id ? { ...t, is_active: newStatus } : t))
      );
    } catch (error) {
      console.error("อัปเดตสถานะไม่สำเร็จ:", error);
      notify.error("ไม่สามารถเปลี่ยนสถานะได้");
    }
  };

  const handleCreateClick = () => {
    setEditingTemplate(null);
    setCurrentView("create");
  };

  const handleEditClick = (template: RequestTemplate) => {
    setEditingTemplate(template);
    setCurrentView("edit");
  };

  const handleFormSuccess = () => {
    setCurrentView("list");
    fetchTemplates(); // โหลดข้อมูลใหม่หลังจากบันทึกหรือลบเสร็จ
  };

  return (
    <div className="flex flex-col gap-6 w-full max-w-5xl mx-auto">
      <div className="flex justify-end h-10">
        {currentView === "list" && (
          <button
            onClick={handleCreateClick}
            className="flex size-10 items-center justify-center rounded-xl bg-green-600 text-white hover:bg-green-700 transition-colors shadow-sm"
          >
            <Plus size={26} />
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
          <div className="mb-6 flex items-center justify-between">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-10 w-72 rounded-full" />
          </div>
          <table className="w-full text-left text-base">
            <tbody>
              <TableRowsSkeleton rows={5} cols={5} />
            </tbody>
          </table>
        </div>
      ) : (
        <>
          {currentView === "list" && (
            <ListView 
              data={templates} 
              onEdit={handleEditClick} 
              onToggleStatus={handleToggleStatus} 
            />
          )}
          {currentView === "create" && (
            <FormView 
              mode="create" 
              onBack={() => setCurrentView("list")} 
              onSuccess={handleFormSuccess} 
            />
          )}
          {currentView === "edit" && (
            <FormView 
              mode="edit" 
              initialData={editingTemplate} 
              onBack={() => setCurrentView("list")} 
              onSuccess={handleFormSuccess} 
            />
          )}
        </>
      )}
    </div>
  );
}


interface ListViewProps {
  data: RequestTemplate[];
  onEdit: (template: RequestTemplate) => void;
  onToggleStatus: (id: number, currentStatus: boolean) => void;
}

function ListView({ data, onEdit, onToggleStatus }: ListViewProps) {
  const [currentPage, setCurrentPage] = useState(1);
  const itemsPerPage = 5; // กำหนดจำนวนแถวต่อหน้า

  // 1. กรองข้อมูลด้วยช่องค้นหาบน Topbar (หน้านี้ลงทะเบียน scope ไว้ตอนที่ ListView อยู่บนจอ
  //    ระหว่างอยู่ในฟอร์มเพิ่ม/แก้ไข ช่องค้นหาจะกลับไปเป็นโหมดข้ามหน้าเองอัตโนมัติ)
  const { results: filteredData, isFiltering, highlightTerms } = usePageSearch(
    requestTemplatesScope,
    data,
  );

  // เปลี่ยนคำค้นแล้วจำนวนแถวเปลี่ยน ถ้ายังค้างอยู่หน้า 3 อาจกลายเป็นหน้าว่าง — ดีดกลับหน้าแรก
  useEffect(() => {
    setCurrentPage(1);
  }, [filteredData.length]);

  // 2. คำนวณข้อมูลสำหรับแบ่งหน้า (Pagination)
  const totalPages = Math.ceil(filteredData.length / itemsPerPage) || 1;
  const startIndex = (currentPage - 1) * itemsPerPage;
  // ตัดข้อมูลมาแสดงแค่ 5 ตัวตามหน้าปัจจุบัน
  const paginatedData = filteredData.slice(startIndex, startIndex + itemsPerPage);

  return (
    <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold text-[#BB6653]">Request Option</h2>
        <SearchStatus />
      </div>

      <table className="w-full text-left text-base text-[#211a14]">
        <thead>
          <tr className="border-b border-black/10 text-[#BB6653]">
            <th className="pb-4 font-semibold">Option Name</th>
            <th className="pb-4 font-semibold">Relate Subject</th>
            <th className="pb-4 font-semibold">Resources (CPU/RAM/STORAGE)</th>
            <th className="pb-4 font-semibold text-center">Status</th>
            <th className="pb-4 font-semibold text-center">Action</th>
          </tr>
        </thead>
        <tbody>
          {paginatedData.length === 0 ? (
            <tr>
              <td colSpan={5} className="py-8 text-center text-neutral-500">
                {isFiltering ? "ไม่พบแม่แบบที่ตรงกับคำค้นหา" : "ยังไม่มีแม่แบบในระบบ"}
              </td>
            </tr>
          ) : (
            paginatedData.map((item) => (
              <tr key={item.id} className="border-b border-black/5 transition-colors last:border-0 hover:bg-black/[0.02]">
                <td className="py-4 text-[#211a14]/70 break-all max-w-[200px]">
                  <Highlight text={item.option_name} terms={highlightTerms} />
                </td>
                <td className="py-4 text-[#211a14]/70 break-all max-w-[200px]">
                  <Highlight text={item.relate_subject} terms={highlightTerms} />
                </td>
                <td className="py-4 text-[#211a14]/70 break-all max-w-[200px]">
                  {item.cpu_limit_milli / 1000} Core / {(item.ram_limit_mb / 1024).toFixed(1)} GB / {item.storage_gb} GB
                </td>
                <td className="py-4 text-center">
                  <div className="flex justify-center">
                    <button
                      onClick={() => onToggleStatus(item.id, item.is_active)}
                      className={`relative inline-flex h-6 w-12 items-center rounded-full border-2 transition-colors duration-200 ease-in-out focus:outline-none ${
                        item.is_active 
                          ? 'border-emerald-500/50 bg-emerald-50' 
                          : 'border-red-400/40 bg-white'
                      }`}
                      title={item.is_active ? "ปิดใช้งาน" : "เปิดใช้งาน"}
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full shadow-sm transition-all duration-200 ease-in-out ${
                          item.is_active 
                            ? 'translate-x-6 bg-emerald-500' 
                            : 'translate-x-1 bg-red-400/70'
                        }`}
                      />
                    </button>
                  </div>
                </td>
                <td className="py-4 text-center">
                  <button onClick={() => onEdit(item)} className="text-[#BB6653] transition-colors hover:text-[#F08B51]">
                    <Edit2 size={20} />
                  </button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      {/* ส่วนควบคุมหน้า (Pagination Controls) */}
      {filteredData.length > 0 && (
        <div className="mt-6 flex items-center justify-between text-base text-[#211a14]/60">
          <div>
            แสดง {startIndex + 1} ถึง {Math.min(startIndex + itemsPerPage, filteredData.length)} จากทั้งหมด {filteredData.length} รายการ
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
              disabled={currentPage === 1}
              className="flex size-8 items-center justify-center rounded-lg border border-black/10 hover:bg-black/5 disabled:opacity-30 disabled:hover:bg-transparent"
            >
              <ChevronLeft size={20} />
            </button>
            <span className="px-2 font-medium">
              หน้า {currentPage} / {totalPages}
            </span>
            <button
              onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage === totalPages}
              className="flex size-8 items-center justify-center rounded-lg border border-black/10 hover:bg-black/5 disabled:opacity-30 disabled:hover:bg-transparent"
            >
              <ChevronRight size={20} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

interface FormViewProps {
  mode: "create" | "edit";
  initialData?: RequestTemplate | null;
  onBack: () => void;
  onSuccess: () => void;
}

function FormView({ mode, initialData, onBack, onSuccess }: FormViewProps) {
  const [isSubmitting, setIsSubmitting] = useState(false);
  const inputClass = "w-full rounded-xl bg-[#F08B51]/90 px-4 py-3 text-white placeholder:text-white/70 outline-none focus:ring-2 focus:ring-[#BB6653]";
  // --- 🚨 ส่วนที่ต้องแทรกเพิ่ม (คำนวณขีดจำกัดทรัพยากร) ---
  const [isUnlocked, setIsUnlocked] = useState(false);
  const [nodesInfo, setNodesInfo] = useState<NodeTelemetry[]>([]);

  useEffect(() => {
    nodetelemetry.getAll().then(setNodesInfo).catch(console.error);
  }, []);

  const workers = nodesInfo.filter(n => n.NodeName !== "intelnuc");

  const clusterCpuCores = workers.length > 0 ? workers.length * 4 : 8; 

  const clusterRamGb = workers.length > 0 ? Math.floor(workers.reduce((sum, n) => sum + (n.RamTotalMB || 0), 0) / 1024) : 8;

  const intelnucNode = nodesInfo.find(n => n.NodeName === "intelnuc");
  const clusterStorageGb = intelnucNode ? Math.floor(intelnucNode.UseableStorage) : 2000;

  const MAX_CPU = isUnlocked ? clusterCpuCores : 8;
  const MAX_RAM = isUnlocked ? clusterRamGb : 8;
  const MAX_STORAGE = isUnlocked ? clusterStorageGb : 15;
  const [formData, setFormData] = useState({
    option_name: "",
    category: "",
    description: "",
    relate_subject: "",
    cpu_limit_milli: "",
    ram_limit_mb: "",
    storage_gb: "",
  });

  useEffect(() => {
      if (mode === "edit" && initialData) {
        setFormData({
          option_name: initialData.option_name,
          category: initialData.category,
          description: initialData.description,
          relate_subject: initialData.relate_subject,
          cpu_limit_milli: initialData.cpu_limit_milli.toString(),
          ram_limit_mb: initialData.ram_limit_mb.toString(),
          storage_gb: initialData.storage_gb.toString(),
        });

        // ตรวจสอบว่ามีค่าไหนเกิน Limit มาตรฐาน (CPU > 8000 milli, RAM > 8192 MB, Storage > 15 GB) หรือไม่
        const isCpuExceeded = initialData.cpu_limit_milli > 8000;
        const isRamExceeded = initialData.ram_limit_mb > 8192;
        const isStorageExceeded = initialData.storage_gb > 15;

        if (isCpuExceeded || isRamExceeded || isStorageExceeded) {
          setIsUnlocked(true); // เปิดปลดล็อกให้ทันที
        }
      }
    }, [mode, initialData]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleSave = async () => {
    try {
      setIsSubmitting(true);
      
      // แปลงข้อมูลให้ตรงกับ CreateRequestTemplateDTO
      const payload: CreateRequestTemplateDTO = {
        option_name: formData.option_name,
        category: formData.category,
        description: formData.description,
        relate_subject: formData.relate_subject,
        cpu_limit_milli: Number(formData.cpu_limit_milli),
        ram_limit_mb: Number(formData.ram_limit_mb),
        storage_gb: Number(formData.storage_gb),
      };

      if (mode === "create") {
        await requestTemplateApi.create(payload);
      } else if (mode === "edit" && initialData) {
        await requestTemplateApi.update(initialData.id, payload);
      }
      
      onSuccess();
    } catch (error) {
      console.error("บันทึกข้อมูลไม่สำเร็จ:", error);
      notify.error("เกิดข้อผิดพลาดในการบันทึกข้อมูล");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!initialData) return;

    const confirmed = await confirmAction({
      title: "ลบ Template นี้ใช่หรือไม่?",
      description: "การกระทำนี้ไม่สามารถย้อนกลับได้",
      confirmText: "ลบ Template",
      destructive: true,
    });
    if (!confirmed) return;

    try {
      setIsSubmitting(true);
      await requestTemplateApi.delete(initialData.id);
      onSuccess();
    } catch (error) {
      console.error("ลบข้อมูลไม่สำเร็จ:", error);
      notify.error("เกิดข้อผิดพลาดในการลบข้อมูล");
      setIsSubmitting(false);
    }
  };

  return (
    <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
      <div className="flex flex-col gap-5">
        
        <div className="grid grid-cols-2 gap-5">
          <div className="flex flex-col gap-1.5">
            <label className="text-base font-semibold text-[#BB6653] ml-1">Option Name</label>
            <input name="option_name" value={formData.option_name} onChange={handleChange} placeholder="Option Name" className={inputClass} />
          </div>
          <div className="flex flex-col gap-1.5">
            <label className="text-base font-semibold text-[#BB6653] ml-1">Category</label>
            <input name="category" value={formData.category} onChange={handleChange} placeholder="Category" className={inputClass} />
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-base font-semibold text-[#BB6653] ml-1">Description</label>
          <textarea name="description" value={formData.description} onChange={handleChange} placeholder="Description" rows={3} className={inputClass} />
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-base font-semibold text-[#BB6653] ml-1">Relate Subject</label>
          <input name="relate_subject" value={formData.relate_subject} onChange={handleChange} placeholder="Relate subject" className={inputClass} />
        </div>

        {/* 🚨 วางทับโค้ด Grid ของเดิมทั้งหมด 🚨 */}
        <div className="flex flex-col gap-4 mt-2">
          {/* สวิตช์ปลดล็อกขีดจำกัด */}
          <div className="flex items-center justify-between rounded-xl bg-white/40 border border-black/10 px-4 py-3">
            <span className="text-sm font-semibold text-[#211a14]/60">
              ปลดล็อกขีดจำกัดทรัพยากร <span className="font-normal text-[11px]">(ดึง Max Limit จาก Cluster จริง)</span>
            </span>
            <button
              type="button"
              onClick={() => {
                const nextUnlocked = !isUnlocked;
                setIsUnlocked(nextUnlocked);
                
                // 🚨 เพิ่มตรงนี้: ถ้า "ปิด" ปลดล็อก ให้ดึงค่าที่เกินกลับมาที่ลิมิตมาตรฐาน
                if (!nextUnlocked) {
                  setFormData(prev => ({
                    ...prev,
                    // 8 Cores = 8000 milli
                    cpu_limit_milli: Number(prev.cpu_limit_milli) > 8000 ? "8000" : prev.cpu_limit_milli,
                    // 8 GB = 8192 MB
                    ram_limit_mb: Number(prev.ram_limit_mb) > 8192 ? "8192" : prev.ram_limit_mb,
                    // 15 GB
                    storage_gb: Number(prev.storage_gb) > 15 ? "15" : prev.storage_gb
                  }));
                }
              }}
              className={`relative inline-flex h-6 w-12 items-center rounded-full border-2 transition-colors duration-200 ease-in-out focus:outline-none ${
                isUnlocked ? 'border-emerald-500/50 bg-emerald-50' : 'border-black/20 bg-white'
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full shadow-sm transition-all duration-200 ease-in-out ${
                  isUnlocked ? 'translate-x-6 bg-emerald-500' : 'translate-x-1 bg-gray-400'
                }`}
              />
            </button>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-8 rounded-2xl bg-[#F08B51]/10 p-5 border border-[#BB6653]/20">
            {/* CPU Slider */}
            <div className="flex flex-col gap-3">
              <div className="flex justify-between items-center">
                <label className="text-sm font-bold text-[#BB6653]">CPU (Cores)</label>
                <input 
                  type="number" min={0.1} max={MAX_CPU} step={0.1}
                  value={formData.cpu_limit_milli ? Number(formData.cpu_limit_milli) / 1000 : ""}
                  onChange={(e) => {
                    const val = Math.min(Math.max(Number(e.target.value), 0.1), MAX_CPU);
                    setFormData(p => ({ ...p, cpu_limit_milli: String(val * 1000) }));
                  }}
                  className="w-20 rounded-lg border border-black/10 bg-white px-2 py-1 text-right text-sm font-bold text-[#211a14] outline-none focus:ring-2 focus:ring-[#BB6653]"
                />
              </div>
              <input 
                type="range" min={0.1} max={MAX_CPU} step={0.1}
                value={formData.cpu_limit_milli ? Number(formData.cpu_limit_milli) / 1000 : 0.1}
                onChange={(e) => setFormData(p => ({ ...p, cpu_limit_milli: String(Number(e.target.value) * 1000) }))}
                className="w-full accent-[#BB6653]"
              />
            </div>

            {/* RAM Slider */}
            <div className="flex flex-col gap-3">
              <div className="flex justify-between items-center">
                <label className="text-sm font-bold text-[#BB6653]">Memory (GB)</label>
                <input 
                  type="number" min={0.1} max={MAX_RAM} step={0.1}
                  value={formData.ram_limit_mb ? parseFloat((Number(formData.ram_limit_mb) / 1024).toFixed(1)) : ""}
                  onChange={(e) => {
                    const val = Math.min(Math.max(Number(e.target.value), 0.1), MAX_RAM);
                    setFormData(p => ({ ...p, ram_limit_mb: String(Math.round(val * 1024)) }));
                  }}
                  className="w-20 rounded-lg border border-black/10 bg-white px-2 py-1 text-right text-sm font-bold text-[#211a14] outline-none focus:ring-2 focus:ring-[#BB6653]"
                />
              </div>
              <input 
                type="range" min={0.1} max={MAX_RAM} step={0.1}
                value={formData.ram_limit_mb ? parseFloat((Number(formData.ram_limit_mb) / 1024).toFixed(1)) : 0.1}
                onChange={(e) => setFormData(p => ({ ...p, ram_limit_mb: String(Math.round(Number(e.target.value) * 1024)) }))}
                className="w-full accent-[#BB6653]"
              />
            </div>

            {/* Storage Slider */}
            <div className="flex flex-col gap-3">
              <div className="flex justify-between items-center">
                <label className="text-sm font-bold text-[#BB6653]">Storage (GB)</label>
                <input 
                  type="number" min={1} max={MAX_STORAGE} step={1}
                  value={formData.storage_gb}
                  onChange={(e) => {
                    const val = Math.min(Math.max(Number(e.target.value), 1), MAX_STORAGE);
                    setFormData(p => ({ ...p, storage_gb: String(val) }));
                  }}
                  className="w-20 rounded-lg border border-black/10 bg-white px-2 py-1 text-right text-sm font-bold text-[#211a14] outline-none focus:ring-2 focus:ring-[#BB6653]"
                />
              </div>
              <input 
                type="range" min={1} max={MAX_STORAGE} step={1}
                value={formData.storage_gb || 1}
                onChange={(e) => setFormData(p => ({ ...p, storage_gb: e.target.value }))}
                className="w-full accent-[#BB6653]"
              />
            </div>
          </div>
        </div>

      </div>

      <div className="mt-8 flex justify-end gap-4">
        {mode === "edit" && (
          <>
            <button
              onClick={onBack}
              disabled={isSubmitting}
              className="rounded-xl border border-black/20 px-6 py-2.5 text-base font-medium text-[#211a14] hover:bg-black/5 transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button 
              onClick={handleDelete}
              disabled={isSubmitting}
              className="flex items-center gap-2 rounded-xl border border-red-500 px-6 py-2.5 text-base font-medium text-red-500 hover:bg-red-50 transition-colors disabled:opacity-50"
            >
              <Trash2 size={18} /> Delete
            </button>
          </>
        )}
        
        {mode === "create" && (
           <button
             onClick={onBack}
             disabled={isSubmitting}
             className="rounded-xl border border-black/20 px-6 py-2.5 text-base font-medium text-[#211a14] hover:bg-black/5 transition-colors disabled:opacity-50"
           >
             Cancel
           </button>
        )}

        <button 
          onClick={handleSave}
          disabled={isSubmitting}
          className="rounded-xl bg-green-600 px-8 py-2.5 text-base font-medium text-white hover:bg-green-700 transition-colors disabled:opacity-50"
        >
          {isSubmitting ? "กำลังบันทึก..." : "Save change"}
        </button>
      </div>
    </div>
  );
}