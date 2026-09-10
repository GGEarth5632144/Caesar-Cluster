import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Search, UserPlus, Edit2, Trash2, Boxes, Loader2, Users } from "lucide-react";
import { cn } from "@/lib/utils";
import { TableRowsSkeleton, SimpleRowsSkeleton } from "@/components/ui/PageSkeletons";
import { AdminModal } from "@/components/ui/admin-modal";
import { userManagementApi, type User, type UpdateUserDTO } from "@/api/adminuser";
import {
  eligibleStudentsApi,
  enrollmentStatusLabel,
  type EligibleStudent,
} from "@/api/eligibleStudents";
import { PATHS } from "@/config/routes";
import { notify, confirmAction } from "@/lib/modal";
import { usePageSearch } from "@/hooks/usePageSearch";
import { usersScope } from "@/config/searchScopes";
import { SearchStatus } from "@/components/ui/search-status";
import { Highlight } from "@/components/ui/highlight";
type YearTab = "all" | "1" | "2" | "3" | "4" | "5+" | "admin";

export default function UserManagement() {
  const navigate = useNavigate();
  const [users, setUsers] = useState<User[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [activeTab, setActiveTab] = useState<YearTab>("all");

  // State สำหรับเก็บข้อมูล User ที่กำลังถูกแก้ไข (ถ้าเป็น null คือปิด Modal)
  const [editingUser, setEditingUser] = useState<User | null>(null);
  // เปิด/ปิด modal "ตรวจสอบรายชื่อผู้มีสิทธิ์"
  const [showEligibleList, setShowEligibleList] = useState(false);

  const fetchUsers = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await userManagementApi.getAll();
      
      if (Array.isArray(data)) {
        setUsers(data);
      } else {
        setUsers([]);
        console.error("API did not return an array:", data);
      }
    } catch (err: any) {
      console.error("Failed to fetch users:", err);
      setError("ไม่สามารถดึงข้อมูลผู้ใช้งานได้ โปรดลองใหม่อีกครั้ง");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  const handleDelete = async (id: number, name: string) => {
    const confirmed = await confirmAction({
      title: `ลบผู้ใช้งาน "${name}"?`,
      description: "การกระทำนี้ไม่สามารถย้อนกลับได้",
      confirmText: "ลบผู้ใช้งาน",
      destructive: true,
    });
    if (!confirmed) return;

    try {
      await userManagementApi.delete(id);
      setUsers((prev) => prev.filter((user) => user.id !== id));
    } catch (err) {
      console.error("Failed to delete user:", err);
      notify.error("เกิดข้อผิดพลาดในการลบผู้ใช้งาน");
    }
  };

  // ฟังก์ชันนี้จะถูกเรียกเมื่อ Modal ทำการอัปเดตข้อมูลสำเร็จ
  const handleUpdateSuccess = (updatedUser: User) => {
    setUsers((prev) => prev.map((u) => (u.id === updatedUser.id ? updatedUser : u)));
    setEditingUser(null);
  };

  // แท็บชั้นปีกรองก่อน แล้วค่อยส่งที่เหลือให้ช่องค้นหาด้านบนกรองต่อ
  // เรียงแบบนี้เพราะแท็บคือ "ขอบเขตที่กำลังดูอยู่" ส่วนคำค้นคือการหาของในขอบเขตนั้น
  // ตัวเลข n/m ที่ Topbar โชว์จึงหมายถึง "เจอกี่คนในแท็บนี้" ซึ่งตรงกับสิ่งที่ตาเห็น
  const usersInTab = useMemo(
    () =>
      users.filter((user) => {
        if (activeTab === "admin") return user.role_id === 2;
        if (activeTab === "all") return true;
        if (user.role_id === 2) return false;
        if (activeTab === "5+") return user.year_level >= 5;
        return user.year_level.toString() === activeTab;
      }),
    [users, activeTab],
  );

  const {
    results: filteredUsers,
    isFiltering,
    highlightTerms,
  } = usePageSearch(usersScope, usersInTab);

  const tabs: { id: YearTab; label: string }[] = [
    { id: "all", label: "ทั้งหมด" },
    { id: "1", label: "ปี 1" },
    { id: "2", label: "ปี 2" },
    { id: "3", label: "ปี 3" },
    { id: "4", label: "ปี 4" },
    { id: "5+", label: "ปี 5+" },
    { id: "admin", label: "ผู้ดูแลระบบ" },
  ];

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6 font-mono">
      
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold text-[#BB6653]">User Management</h2>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <button
            onClick={() => setShowEligibleList(true)}
            className="inline-flex items-center gap-2 rounded-xl border-2 border-[#BB6653] bg-transparent px-5 py-2 text-base font-bold text-[#BB6653] shadow-sm hover:bg-[#BB6653]/10 transition-colors"
          >
            <Users size={20} />
            ตรวจสอบรายชื่อผู้มีสิทธิ์
          </button>
          
          {/* ปุ่มเพิ่ม (ปุ่มหลัก สีทึบ) — พาไปหน้า Import Students เพื่ออัปโหลดไฟล์รายชื่อจากทะเบียน */}
          <button
            onClick={() => navigate(`/${PATHS.adminImportStudents}`)}
            className="inline-flex items-center gap-2 rounded-xl bg-[#BB6653] px-5 py-2.5 text-base font-bold text-white shadow-sm hover:bg-[#F08B51] transition-colors"
          >
            <UserPlus size={20} />
            เพิ่มรายชื่อผู้มีสิทธิ์
          </button>
        </div>
      </div>

      <div className="rounded-3xl bg-[#FFFDF6] p-6 shadow-sm sm:p-8">
        <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between rounded-2xl bg-white p-4 border border-black/5">
          <div className="flex flex-wrap gap-2">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "px-4 py-2 text-base font-bold rounded-xl transition-colors",
                  activeTab === tab.id
                    ? "bg-[#BB6653] text-white"
                    : "bg-[#FFF8E8] text-[#211a14]/60 hover:bg-[#F08B51]/20"
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <SearchStatus className="shrink-0" />
        </div>

        <div className="-mx-6 overflow-x-auto sm:mx-0">
          <table className="w-full min-w-[800px] table-fixed text-left text-base text-[#211a14]">
            <colgroup>
              <col className="w-[30%]" />
              <col className="w-[25%]" />
              <col className="w-[20%]" />
              <col className="w-[15%]" />
              <col className="w-[10%]" />
            </colgroup>
            <thead>
              <tr className="border-b border-black/10 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
                <th className="px-6 pb-4 sm:px-3">Student Info</th>
                <th className="px-3 pb-4">Contact</th>
                <th className="px-3 pb-4">Namespace</th>
                <th className="px-3 pb-4 text-center">Role / Year</th>
                <th className="px-6 pb-4 text-center sm:px-3">Action</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <TableRowsSkeleton rows={6} cols={5} />
              ) : error ? (
                <tr>
                  <td colSpan={5} className="py-10">
                    <div className="p-4 mx-auto max-w-sm rounded-xl bg-red-50 text-center text-red-600 text-base border border-red-100">
                      {error}
                    </div>
                  </td>
                </tr>
              ) : filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-16 text-center text-neutral-500">
                    <div className="flex flex-col items-center justify-center gap-2">
                      <Search className="size-8 text-[#BB6653]/30" />
                      <p>
                        {isFiltering
                          ? "ไม่มีใครในแท็บนี้ตรงกับคำค้นหา"
                          : "ไม่พบรายชื่อนักศึกษาในหมวดหมู่นี้"}
                      </p>
                    </div>
                  </td>
                </tr>
              ) : (
                filteredUsers.map((user) => {
                  const initials = (user.real_name || user.student_id || "U")
                    .split(" ")
                    .map((s) => s[0])
                    .join("")
                    .slice(0, 2)
                    .toUpperCase();
                  const isAdmin = user.role_id === 2;

                  return (
                    <tr key={user.id} className="border-b border-black/5 last:border-0 hover:bg-black/[0.02] transition-colors">
                      <td className="px-6 py-4 sm:px-3">
                        <div className="flex items-center gap-3">
                          <div className={cn(
                            "flex size-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white",
                            isAdmin ? "bg-red-500" : "bg-[#F08B51]"
                          )}>
                            {initials}
                          </div>
                          <div className="min-w-0">
                            <div className="truncate font-semibold text-[#211a14]" title={user.real_name}>
                              <Highlight text={user.real_name} terms={highlightTerms} />{" "}
                              {user.nick_name && (
                                <span className="text-[#211a14]/50">
                                  (<Highlight text={user.nick_name} terms={highlightTerms} />)
                                </span>
                              )}
                            </div>
                            <div className="text-sm text-[#211a14]/60 mt-0.5">
                              <Highlight text={user.student_id} terms={highlightTerms} />
                            </div>
                          </div>
                        </div>
                      </td>

                      <td className="px-3 py-4 text-[#211a14]/70">
                        <div className="truncate" title={user.gmail}>
                          {user.gmail ? <Highlight text={user.gmail} terms={highlightTerms} /> : "—"}
                        </div>
                      </td>

                      {/* เนมสเปซที่สังกัด — โควตาของ space นี้ไปดู/แก้ที่หน้า Namespace Management
                          (โควตาผูกกับ namespace ไม่ใช่ user แก้ตรงนี้ทีเดียวจะกระทบทุกคนในกลุ่ม
                          ซึ่งไม่ใช่สิ่งที่หน้า "จัดการผู้ใช้รายคน" ควรทำได้) */}
                      <td className="px-3 py-4 text-[#211a14]/70">
                        {user.namespace_id ? (
                          <button
                            type="button"
                            onClick={() =>
                              navigate(`/${PATHS.namespaceManagement}`, {
                                // ฝากคำค้นไปด้วย หน้าปลายทางจะเปิดมาพร้อมกรองเหลือ space นี้อันเดียว
                                // ชื่อ namespace ผ่านกฎ DNS-1123 อยู่แล้ว (ตัวเล็ก/ตัวเลข/ขีดกลาง)
                                // จึงไม่มีช่องว่างมาทำให้ตัวแยกคำของช่องค้นหาตัดผิดที่
                                state: user.namespace_name ? { search: `name:${user.namespace_name}` } : undefined,
                              })
                            }
                            title="ไปที่หน้าจัดการเนมสเปซเพื่อปรับโควตา"
                            className="inline-flex max-w-full items-center gap-1.5 rounded-lg px-2 py-1 text-sm font-semibold text-[#BB6653] transition-colors hover:bg-[#F08B51]/15"
                          >
                            <Boxes size={15} className="shrink-0" />
                            <span className="truncate">
                              <Highlight text={user.namespace_name || `#${user.namespace_id}`} terms={highlightTerms} />
                            </span>
                          </button>
                        ) : (
                          <span className="text-sm text-[#211a14]/40">ยังไม่มี space</span>
                        )}
                      </td>

                      <td className="px-3 py-4 text-center">
                        {isAdmin ? (
                          <span className="inline-flex rounded-full bg-red-50 px-2.5 py-1 text-sm font-bold text-red-600">
                            Admin
                          </span>
                        ) : (
                          <span className="inline-flex rounded-full bg-[#FFF8E8] px-2.5 py-1 text-sm font-bold text-[#BB6653]">
                            Year {user.year_level}
                          </span>
                        )}
                      </td>

                      <td className="px-6 py-4 text-center sm:px-3">
                        <div className="flex items-center justify-center gap-2">
                          <button 
                            className="p-1.5 text-[#BB6653] hover:text-[#F08B51] transition-colors rounded-lg hover:bg-black/5" 
                            title="Edit User"
                            onClick={() => setEditingUser(user)}
                          >
                            <Edit2 size={18} />
                          </button>
                          <button 
                            onClick={() => handleDelete(user.id, user.real_name)}
                            className="p-1.5 text-red-400 hover:text-red-600 transition-colors rounded-lg hover:bg-red-50" 
                            title="Delete User"
                          >
                            <Trash2 size={18} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* ---------------- Edit Modal ---------------- */}
      {editingUser && (
        <EditUserModal
          user={editingUser}
          onClose={() => setEditingUser(null)}
          onSuccess={handleUpdateSuccess}
        />
      )}

      {/* ---------------- ตรวจสอบรายชื่อผู้มีสิทธิ์ Modal ---------------- */}
      {showEligibleList && <EligibleStudentsModal onClose={() => setShowEligibleList(false)} />}
    </div>
  );
}

// ==========================================
// Modal Component สำหรับแก้ไขข้อมูล
// ==========================================
interface EditUserModalProps {
  user: User;
  onClose: () => void;
  onSuccess: (updatedUser: User) => void;
}

function EditUserModal({ user, onClose, onSuccess }: EditUserModalProps) {
  const [isSubmitting, setIsSubmitting] = useState(false);
  
  const [formData, setFormData] = useState({
    student_id: user.student_id,
    real_name: user.real_name,
    nick_name: user.nick_name || "",
    gmail: user.gmail || "",
    role_id: user.role_id.toString(),
  });

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    
    try {
      // แปลงข้อมูลตัวเลขก่อนส่งไป API — ไม่ส่ง year เพราะเป็นค่าที่คำนวณสดจาก student_id เสมอ
      // (ดู entity.YearLevel ฝั่ง backend) แก้ตรงนี้ไปก็ไม่มีผล ไม่ใช่ฟิลด์ที่แก้ไขได้
      const payload: UpdateUserDTO = {
        student_id: formData.student_id,
        real_name: formData.real_name,
        nick_name: formData.nick_name,
        gmail: formData.gmail,
        role_id: parseInt(formData.role_id, 10),
      };

      const updatedUser = await userManagementApi.update(user.id, payload);
      onSuccess(updatedUser);
    } catch (err) {
      console.error("Failed to update user:", err);
      notify.error("เกิดข้อผิดพลาดในการอัปเดตข้อมูล");
    } finally {
      setIsSubmitting(false);
    }
  };

  const inputClass = "w-full rounded-xl border border-black/10 bg-white px-4 py-2.5 text-base text-[#211a14] outline-none focus:border-[#BB6653] focus:ring-1 focus:ring-[#BB6653]";
  const labelClass = "mb-1.5 block text-sm font-bold uppercase tracking-wider text-[#BB6653]";

  return (
    <AdminModal
      onClose={onClose}
      busy={isSubmitting}
      title="Edit User"
      subtitle={`กำลังแก้ไขข้อมูลของ ${user.real_name}`}
      onSubmit={handleSubmit}
      footer={(close) => (
        <>
          <button
            type="button"
            onClick={close}
            disabled={isSubmitting}
            className="rounded-xl px-5 py-2.5 text-base font-bold text-[#211a14]/60 hover:bg-black/5 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            className="inline-flex items-center justify-center min-w-[120px] rounded-xl bg-green-600 px-5 py-2.5 text-base font-bold text-white hover:bg-green-700 transition-colors disabled:opacity-50"
          >
            {isSubmitting ? <Loader2 size={18} className="animate-spin" /> : "Save Changes"}
          </button>
        </>
      )}
    >
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        {/* รหัสนักศึกษา */}
        <div>
          <label className={labelClass}>Student ID</label>
          <input name="student_id" value={formData.student_id} onChange={handleChange} required className={inputClass} />
        </div>

        {/* อีเมล */}
        <div>
          <label className={labelClass}>Gmail</label>
          <input name="gmail" type="email" value={formData.gmail} onChange={handleChange} required className={inputClass} />
        </div>

        {/* ชื่อจริง */}
        <div>
          <label className={labelClass}>Real Name</label>
          <input name="real_name" value={formData.real_name} onChange={handleChange} required className={inputClass} />
        </div>

        {/* ชื่อเล่น */}
        <div>
          <label className={labelClass}>Nickname</label>
          <input name="nick_name" value={formData.nick_name} onChange={handleChange} className={inputClass} />
        </div>

        {/* ชั้นปี — คำนวณสดจาก student_id เสมอ แก้ไขตรงนี้ไม่ได้ (เปลี่ยน Student ID แล้วบันทึก ค่านี้จะขยับตาม) */}
        <div>
          <label className={labelClass}>Year</label>
          <input
            value={`Year ${user.year_level}`}
            disabled
            className={`${inputClass} disabled:bg-black/5 disabled:text-[#211a14]/60`}
          />
        </div>

        {/* ตำแหน่ง (Role) */}
        <div>
          <label className={labelClass}>Role</label>
          <select name="role_id" value={formData.role_id} onChange={handleChange} className={inputClass}>
            <option value="1">User</option>
            <option value="2">Admin</option>
          </select>
        </div>

        {/* เนมสเปซที่สังกัด — อ่านอย่างเดียว ย้ายผู้ใช้ข้าม space จากที่นี่ไม่ได้
            โควตาของ space ไปปรับที่หน้า Namespace Management */}
        <div className="sm:col-span-2">
          <label className={labelClass}>Namespace</label>
          <div className="rounded-xl border border-black/10 bg-black/[0.03] px-4 py-3 text-base text-[#211a14]/60">
            {user.namespace_id
              ? `${user.namespace_name || `#${user.namespace_id}`} — โควตาของ space นี้ปรับได้ที่หน้า Namespace Management`
              : "ผู้ใช้ยังไม่มี namespace — จะสังกัด space เมื่อสร้างหรือเข้าร่วมกลุ่มแล้ว"}
          </div>
        </div>
      </div>
    </AdminModal>
  );
}

// ==========================================
// Modal ตรวจสอบรายชื่อผู้มีสิทธิ์ (eligible_students ทั้งหมด — ทั้งที่ import มาแล้วและยัง)
// ==========================================
interface EligibleStudentsModalProps {
  onClose: () => void;
}

function EligibleStudentsModal({ onClose }: EligibleStudentsModalProps) {
  const [students, setStudents] = useState<EligibleStudent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState("");

  useEffect(() => {
    (async () => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await eligibleStudentsApi.listAll();
        setStudents(Array.isArray(data) ? data : []);
      } catch (err) {
        console.error("Failed to fetch eligible students:", err);
        setError("ไม่สามารถดึงรายชื่อผู้มีสิทธิ์ได้ โปรดลองใหม่อีกครั้ง");
      } finally {
        setIsLoading(false);
      }
    })();
  }, []);

  const filtered = students.filter((s) => {
    if (!searchTerm) return true;
    const lower = searchTerm.toLowerCase();
    return (
      s.student_id.toLowerCase().includes(lower) ||
      s.real_name.toLowerCase().includes(lower) ||
      s.major.toLowerCase().includes(lower)
    );
  });

  return (
    <AdminModal
      onClose={onClose}
      size="lg"
      title="รายชื่อผู้มีสิทธิ์"
      subtitle={
        `ทั้งหมดในตาราง eligible_students (${students.length} คน) — คือรายชื่อที่ import เข้ามาแล้ว ` +
        `ไม่ว่าจะสมัครเข้าระบบจริงหรือยังก็ตาม`
      }
      toolbar={
        <div className="relative w-full sm:w-72">
          <div className="pointer-events-none absolute inset-y-0 left-3 flex items-center">
            <Search size={18} className="text-[#BB6653]/60" />
          </div>
          <input
            type="text"
            placeholder="ค้นหารหัส/ชื่อ/สาขา"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full rounded-xl border border-black/10 bg-white py-2 pl-9 pr-3 text-base text-[#211a14] outline-none focus:ring-2 focus:ring-[#BB6653]/50"
          />
        </div>
      }
    >
      {isLoading ? (
        <SimpleRowsSkeleton rows={7} cols={4} />
      ) : error ? (
        <div className="mx-auto max-w-sm rounded-xl border border-red-100 bg-red-50 p-4 text-center text-base text-red-600">
          {error}
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-2 py-16 text-neutral-500">
          <Search className="size-8 text-[#BB6653]/30" />
          <p>ไม่พบรายชื่อที่ค้นหา</p>
        </div>
      ) : (
        <table className="w-full text-left text-base text-[#211a14]">
          <thead>
            <tr className="border-b border-black/10 text-sm font-bold uppercase tracking-wider text-[#BB6653]">
              <th className="pb-3 pr-3">รหัสประจำตัว</th>
              <th className="pb-3 pr-3">ชื่อ-สกุล</th>
              <th className="pb-3 pr-3">สาขาวิชา</th>
              <th className="pb-3">สถานภาพ</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((s) => {
              const isActive = s.enrollment_status === 10 || s.enrollment_status === 11;
              return (
                <tr key={s.student_id} className="border-b border-black/5 last:border-0">
                  <td className="py-3 pr-3 font-medium">{s.student_id}</td>
                  <td className="py-3 pr-3">{s.real_name || "—"}</td>
                  <td className="py-3 pr-3 text-[#211a14]/70">{s.major}</td>
                  <td className="py-3">
                    <span
                      className={cn(
                        "inline-flex rounded-full px-2.5 py-1 text-sm font-bold",
                        isActive ? "bg-green-50 text-green-700" : "bg-black/5 text-[#211a14]/60"
                      )}
                    >
                      {enrollmentStatusLabel(s.enrollment_status)}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </AdminModal>
  );
}