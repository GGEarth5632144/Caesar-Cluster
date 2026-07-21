import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Upload, FileSpreadsheet, CheckCircle2, AlertTriangle, Users } from "lucide-react";

import {
  eligibleStudentsApi,
  type PreviewEligibleStudentsResponse,
  type ConfirmEligibleStudentsResponse,
} from "@/api/eligibleStudents";
import { getApiErrorMessage } from "@/api/authApi";

type ViewState = "upload" | "preview" | "done";

export default function AdminImportStudents() {
  const navigate = useNavigate();
  const [currentView, setCurrentView] = useState<ViewState>("upload");
  const [fileName, setFileName] = useState<string>("");
  const [preview, setPreview] = useState<PreviewEligibleStudentsResponse | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [result, setResult] = useState<ConfirmEligibleStudentsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setFileName(file.name);
    setError("");
    setIsLoading(true);
    try {
      const data = await eligibleStudentsApi.preview(file);
      setPreview(data);
      // ค่าเริ่มต้น: เลือกทุกคนที่ผ่าน validate ไว้ก่อน — admin ค่อยกดติ๊กออกทีหลังถ้าไม่อยากเพิ่มบางคน
      setSelectedIds(new Set(data.valid.map((v) => v.student_id)));
      setCurrentView("preview");
    } catch (err) {
      console.error("preview eligible students error:", err);
      setError(getApiErrorMessage(err, "อ่านไฟล์ไม่สำเร็จ กรุณาตรวจสอบว่าเป็นไฟล์ .xlsx หรือ .xls ที่ export จากระบบทะเบียน"));
    } finally {
      setIsLoading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  const toggleOne = (studentId: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(studentId)) next.delete(studentId);
      else next.add(studentId);
      return next;
    });
  };

  const toggleAll = () => {
    if (!preview) return;
    setSelectedIds((prev) =>
      prev.size === preview.valid.length ? new Set() : new Set(preview.valid.map((v) => v.student_id)),
    );
  };

  const handleConfirm = async () => {
    if (!preview) return;
    const studentsToSubmit = preview.valid.filter((v) => selectedIds.has(v.student_id));
    if (studentsToSubmit.length === 0) return;

    setIsLoading(true);
    setError("");
    try {
      const data = await eligibleStudentsApi.confirm(studentsToSubmit);
      setResult(data);
      setCurrentView("done");
    } catch (err) {
      console.error("confirm eligible students error:", err);
      setError(getApiErrorMessage(err, "บันทึกรายชื่อไม่สำเร็จ"));
    } finally {
      setIsLoading(false);
    }
  };

  const handleReset = () => {
    setPreview(null);
    setSelectedIds(new Set());
    setResult(null);
    setFileName("");
    setError("");
    setCurrentView("upload");
  };

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div className="rounded-3xl bg-[#FFFDF6] p-8 shadow-sm">
        <h2 className="text-xl font-bold text-[#BB6653]">Import Students</h2>
        <p className="mt-1 text-sm text-[#211a14]/60">
          อัปโหลดไฟล์รายชื่อนักศึกษา (.xlsx หรือ .xls) จากระบบทะเบียน — ระบบจะตรวจสอบก่อน ยังไม่บันทึกจนกว่าจะกดยืนยัน
        </p>

        {error && (
          <div className="mt-4 flex items-center gap-2 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-600">
            <AlertTriangle size={18} />
            {error}
          </div>
        )}

        {currentView === "upload" && (
          <div className="mt-6 flex flex-col items-center justify-center gap-3 rounded-2xl border-2 border-dashed border-black/15 py-14">
            <FileSpreadsheet size={40} className="text-[#BB6653]/60" />
            <p className="text-sm text-[#211a14]/60">
              {isLoading ? "กำลังอ่านไฟล์..." : "เลือกไฟล์ .xlsx หรือ .xls เพื่ออัปโหลด"}
            </p>
            <label className="mt-2 flex cursor-pointer items-center gap-2 rounded-xl bg-[#F08B51] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#F08B51]/90">
              <Upload size={16} />
              เลือกไฟล์
              <input
                ref={fileInputRef}
                type="file"
                accept=".xlsx,.xls"
                className="hidden"
                onChange={handleFileChange}
                disabled={isLoading}
              />
            </label>
          </div>
        )}

        {currentView === "preview" && preview && (
          <PreviewView
            fileName={fileName}
            data={preview}
            selectedIds={selectedIds}
            onToggleOne={toggleOne}
            onToggleAll={toggleAll}
            isSubmitting={isLoading}
            onConfirm={handleConfirm}
            onCancel={handleReset}
          />
        )}

        {currentView === "done" && result && (
          <DoneView
            result={result}
            onImportAnother={handleReset}
            onGoToUserManagement={() => navigate("/user-management")}
          />
        )}
      </div>
    </div>
  );
}

function PreviewView({
  fileName,
  data,
  selectedIds,
  onToggleOne,
  onToggleAll,
  isSubmitting,
  onConfirm,
  onCancel,
}: {
  fileName: string;
  data: PreviewEligibleStudentsResponse;
  selectedIds: Set<string>;
  onToggleOne: (studentId: string) => void;
  onToggleAll: () => void;
  isSubmitting: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const allSelected = data.valid.length > 0 && selectedIds.size === data.valid.length;

  return (
    <div className="mt-6 flex flex-col gap-6">
      <p className="text-sm text-[#211a14]/70">
        ไฟล์: <span className="font-medium">{fileName}</span>
      </p>

      <div className="grid grid-cols-3 gap-4">
        <SummaryTile label="รายชื่อใหม่" value={data.summary.new} color="text-green-600" />
        <SummaryTile label="อัปเดตข้อมูล" value={data.summary.updated} color="text-[#F08B51]" />
        <SummaryTile label="ไม่เปลี่ยนแปลง" value={data.summary.unchanged} color="text-[#211a14]/60" />
      </div>

      {data.invalid.length > 0 && (
        <div>
          <p className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-red-600">
            <AlertTriangle size={16} />
            แถวที่มีปัญหา ({data.invalid.length} แถว — จะไม่ถูก import)
          </p>
          <div className="max-h-48 overflow-y-auto rounded-xl border border-red-100">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-red-100 bg-red-50 text-red-600">
                  <th className="px-4 py-2 font-semibold">แถวที่</th>
                  <th className="px-4 py-2 font-semibold">สาเหตุ</th>
                </tr>
              </thead>
              <tbody>
                {data.invalid.map((row) => (
                  <tr key={row.row} className="border-b border-red-50 last:border-0">
                    <td className="px-4 py-2">{row.row}</td>
                    <td className="px-4 py-2">{row.reason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div>
        <div className="mb-2 flex items-center justify-between">
          <p className="text-sm font-semibold text-[#BB6653]">
            เลือกรายชื่อที่จะ import ({selectedIds.size}/{data.valid.length} คน)
          </p>
          <button
            type="button"
            onClick={onToggleAll}
            className="text-xs font-medium text-[#BB6653] underline-offset-2 hover:underline"
          >
            {allSelected ? "ยกเลิกเลือกทั้งหมด" : "เลือกทั้งหมด"}
          </button>
        </div>
        <div className="max-h-64 overflow-y-auto rounded-xl border border-black/10">
          <table className="w-full text-left text-sm text-[#211a14]">
            <thead>
              <tr className="border-b border-black/10 bg-black/[0.02] text-[#BB6653]">
                <th className="w-10 px-4 py-2">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={onToggleAll}
                    className="size-4 accent-[#BB6653]"
                  />
                </th>
                <th className="px-4 py-2 font-semibold">รหัสประจำตัว</th>
                <th className="px-4 py-2 font-semibold">ชื่อ-สกุล</th>
                <th className="px-4 py-2 font-semibold">สาขาวิชา</th>
                <th className="px-4 py-2 font-semibold">สถานภาพ</th>
              </tr>
            </thead>
            <tbody>
              {data.valid.map((item) => {
                const checked = selectedIds.has(item.student_id);
                return (
                  <tr
                    key={item.student_id}
                    onClick={() => onToggleOne(item.student_id)}
                    className={`cursor-pointer border-b border-black/5 last:border-0 hover:bg-black/[0.02] ${
                      checked ? "" : "opacity-50"
                    }`}
                  >
                    <td className="px-4 py-2" onClick={(e) => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => onToggleOne(item.student_id)}
                        className="size-4 accent-[#BB6653]"
                      />
                    </td>
                    <td className="px-4 py-2">{item.student_id}</td>
                    <td className="px-4 py-2">{item.real_name}</td>
                    <td className="px-4 py-2">{item.major}</td>
                    <td className="px-4 py-2">{item.enrollment_status}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      <div className="flex justify-end gap-4">
        <button
          onClick={onCancel}
          disabled={isSubmitting}
          className="rounded-xl border border-black/20 px-6 py-2.5 text-sm font-medium text-[#211a14] transition-colors hover:bg-black/5 disabled:opacity-50"
        >
          ยกเลิก
        </button>
        <button
          onClick={onConfirm}
          disabled={isSubmitting || selectedIds.size === 0}
          className="rounded-xl bg-green-600 px-8 py-2.5 text-sm font-medium text-white transition-colors hover:bg-green-700 disabled:opacity-50"
        >
          {isSubmitting ? "กำลังบันทึก..." : `ยืนยัน Import (${selectedIds.size} คน)`}
        </button>
      </div>
    </div>
  );
}

function SummaryTile({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="rounded-2xl border border-black/10 px-5 py-4 text-center">
      <p className={`text-2xl font-bold ${color}`}>{value}</p>
      <p className="mt-1 text-xs text-[#211a14]/60">{label}</p>
    </div>
  );
}

function DoneView({
  result,
  onImportAnother,
  onGoToUserManagement,
}: {
  result: ConfirmEligibleStudentsResponse;
  onImportAnother: () => void;
  onGoToUserManagement: () => void;
}) {
  return (
    <div className="mt-6 flex flex-col items-center gap-3 py-10 text-center">
      <CheckCircle2 size={44} className="text-green-600" />
      <p className="text-lg font-semibold text-[#211a14]">Import สำเร็จ</p>
      <p className="text-sm text-[#211a14]/60">
        บันทึก/อัปเดตแล้ว {result.upserted} จากทั้งหมด {result.submitted} รายการ
      </p>
      <div className="mt-4 flex gap-3">
        <button
          onClick={onImportAnother}
          className="rounded-xl border border-[#F08B51] px-6 py-2.5 text-sm font-medium text-[#F08B51] transition-colors hover:bg-[#F08B51]/10"
        >
          Import ไฟล์อื่น
        </button>
        <button
          onClick={onGoToUserManagement}
          className="inline-flex items-center gap-2 rounded-xl bg-[#F08B51] px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-[#F08B51]/90"
        >
          <Users size={16} />
          ไปที่ User Management
        </button>
      </div>
    </div>
  );
}
