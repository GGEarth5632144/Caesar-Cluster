import { useState, useEffect } from "react";
import { Plus, Edit2, Trash2, CheckSquare, Square, Search, ChevronLeft, ChevronRight } from "lucide-react";
import { requestTemplateApi, type RequestTemplate, type CreateRequestTemplateDTO } from "../../api/adminrequest";
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
            <Plus size={24} />
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
          <div className="mb-6 flex items-center justify-between">
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-10 w-72 rounded-full" />
          </div>
          <table className="w-full text-left text-sm">
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
  const [searchTerm, setSearchTerm] = useState("");
  const [currentPage, setCurrentPage] = useState(1);
  const itemsPerPage = 5; // กำหนดจำนวนแถวต่อหน้า

  // 1. กรองข้อมูลตามคำค้นหา
  const filteredData = data.filter((item) => {
    const searchLower = searchTerm.toLowerCase();
    return (
      item.option_name.toLowerCase().includes(searchLower) ||
      item.category.toLowerCase().includes(searchLower) ||
      item.relate_subject.toLowerCase().includes(searchLower)
    );
  });

  // ฟังก์ชันจัดการเมื่อพิมพ์ช่องค้นหา (ให้กลับไปหน้า 1 เสมอ)
  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchTerm(e.target.value);
    setCurrentPage(1);
  };

  // 2. คำนวณข้อมูลสำหรับแบ่งหน้า (Pagination)
  const totalPages = Math.ceil(filteredData.length / itemsPerPage) || 1;
  const startIndex = (currentPage - 1) * itemsPerPage;
  // ตัดข้อมูลมาแสดงแค่ 5 ตัวตามหน้าปัจจุบัน
  const paginatedData = filteredData.slice(startIndex, startIndex + itemsPerPage);

  return (
    <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-xl font-bold text-[#BB6653]">Request Option</h2>
        <div className="relative w-72">
          <div className="absolute inset-y-0 left-3 flex items-center pointer-events-none">
            <Search size={18} className="text-[#BB6653]/60" />
          </div>
          <input
            type="text"
            placeholder="Search"
            value={searchTerm}
            onChange={handleSearchChange}
            className="w-full rounded-full border border-black/10 bg-white py-2.5 pl-10 pr-4 text-sm text-[#211a14] outline-none transition-shadow focus:ring-2 focus:ring-[#BB6653]/50"
          />
        </div>
      </div>

      <table className="w-full text-left text-sm text-[#211a14]">
        <thead>
          <tr className="border-b border-black/10 text-[#BB6653]">
            <th className="pb-4 font-semibold">Option Name</th>
            <th className="pb-4 font-semibold">Relate Subject</th>
            <th className="pb-4 font-semibold">Required Resources</th>
            <th className="pb-4 font-semibold text-center">Status</th>
            <th className="pb-4 font-semibold text-center">Action</th>
          </tr>
        </thead>
        <tbody>
          {paginatedData.length === 0 ? (
            <tr>
              <td colSpan={5} className="py-8 text-center text-neutral-500">
                ไม่พบข้อมูลที่ค้นหา
              </td>
            </tr>
          ) : (
            paginatedData.map((item) => (
              <tr key={item.id} className="border-b border-black/5 transition-colors last:border-0 hover:bg-black/[0.02]">
                <td className="py-4 font-medium">{item.option_name}</td>
                <td className="py-4">{item.relate_subject}</td>
                <td className="py-4 text-[#211a14]/70">
                  {item.cpu_limit_milli / 1000} Core / {Math.floor(item.ram_limit_mb / 1000)} GB
                </td>
                <td className="py-4 text-center text-[#BB6653]">
                  <div className="flex justify-center">
                    <button 
                      onClick={() => onToggleStatus(item.id, item.is_active)}
                      className="transition-colors hover:text-[#F08B51] focus:outline-none"
                    >
                      {item.is_active ? <CheckSquare size={20} /> : <Square size={20} />}
                    </button>
                  </div>
                </td>
                <td className="py-4 text-center">
                  <button onClick={() => onEdit(item)} className="text-[#BB6653] transition-colors hover:text-[#F08B51]">
                    <Edit2 size={18} />
                  </button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      {/* ส่วนควบคุมหน้า (Pagination Controls) */}
      {filteredData.length > 0 && (
        <div className="mt-6 flex items-center justify-between text-sm text-[#211a14]/60">
          <div>
            แสดง {startIndex + 1} ถึง {Math.min(startIndex + itemsPerPage, filteredData.length)} จากทั้งหมด {filteredData.length} รายการ
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
              disabled={currentPage === 1}
              className="flex size-8 items-center justify-center rounded-lg border border-black/10 hover:bg-black/5 disabled:opacity-30 disabled:hover:bg-transparent"
            >
              <ChevronLeft size={18} />
            </button>
            <span className="px-2 font-medium">
              หน้า {currentPage} / {totalPages}
            </span>
            <button
              onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage === totalPages}
              className="flex size-8 items-center justify-center rounded-lg border border-black/10 hover:bg-black/5 disabled:opacity-30 disabled:hover:bg-transparent"
            >
              <ChevronRight size={18} />
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
  const cpuOptions = [1000, 2000, 3000];
  const ramOptions = [1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000];

  // ผูก State เข้ากับฟิลด์ต่างๆ ตาม DTO
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
    // ถ้อยู่ในโหมด Edit ให้ดึงค่าเก่ามาแสดงในฟอร์ม
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
            <label className="text-sm font-semibold text-[#BB6653] ml-1">Option Name</label>
            <input name="option_name" value={formData.option_name} onChange={handleChange} placeholder="Option Name" className={inputClass} />
          </div>
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-semibold text-[#BB6653] ml-1">Category</label>
            <input name="category" value={formData.category} onChange={handleChange} placeholder="Category" className={inputClass} />
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-sm font-semibold text-[#BB6653] ml-1">Description</label>
          <textarea name="description" value={formData.description} onChange={handleChange} placeholder="Description" rows={3} className={inputClass} />
        </div>

        <div className="flex flex-col gap-1.5">
          <label className="text-sm font-semibold text-[#BB6653] ml-1">Relate Subject</label>
          <input name="relate_subject" value={formData.relate_subject} onChange={handleChange} placeholder="Relate subject" className={inputClass} />
        </div>

        <div className="grid grid-cols-3 gap-5">
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-semibold text-[#BB6653] ml-1">CPU Limit (Milli)</label>
            <select name="cpu_limit_milli" value={formData.cpu_limit_milli} onChange={handleChange} className={inputClass}>
              <option value="" disabled>เลือก CPU</option>
              {cpuOptions.map((val) => (
                <option key={val} value={val} className="text-[#211a14] bg-white">{val}</option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-semibold text-[#BB6653] ml-1">Memory Limit (MB)</label>
            <select name="ram_limit_mb" value={formData.ram_limit_mb} onChange={handleChange} className={inputClass}>
              <option value="" disabled>เลือก RAM</option>
              {ramOptions.map((val) => (
                <option key={val} value={val} className="text-[#211a14] bg-white">{val}</option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-semibold text-[#BB6653] ml-1">Storage (GB)</label>
            <input name="storage_gb" value={formData.storage_gb} onChange={handleChange} placeholder="Storage (GB)" type="number" className={inputClass} />
          </div>
        </div>

      </div>

      <div className="mt-8 flex justify-end gap-4">
        {mode === "edit" && (
          <>
            <button
              onClick={onBack}
              disabled={isSubmitting}
              className="rounded-xl border border-black/20 px-6 py-2.5 text-sm font-medium text-[#211a14] hover:bg-black/5 transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button 
              onClick={handleDelete}
              disabled={isSubmitting}
              className="flex items-center gap-2 rounded-xl border border-red-500 px-6 py-2.5 text-sm font-medium text-red-500 hover:bg-red-50 transition-colors disabled:opacity-50"
            >
              <Trash2 size={16} /> Delete
            </button>
          </>
        )}
        
        {mode === "create" && (
           <button
             onClick={onBack}
             disabled={isSubmitting}
             className="rounded-xl border border-black/20 px-6 py-2.5 text-sm font-medium text-[#211a14] hover:bg-black/5 transition-colors disabled:opacity-50"
           >
             Cancel
           </button>
        )}

        <button 
          onClick={handleSave}
          disabled={isSubmitting}
          className="rounded-xl bg-green-600 px-8 py-2.5 text-sm font-medium text-white hover:bg-green-700 transition-colors disabled:opacity-50"
        >
          {isSubmitting ? "กำลังบันทึก..." : "Save change"}
        </button>
      </div>
    </div>
  );
}