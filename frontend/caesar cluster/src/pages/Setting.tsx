import { useState } from "react";
import { Loader2, Pencil } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authApi, getApiErrorMessage } from "@/api/authApi";
import { notify } from "@/lib/modal";
import { useAuthStore } from "@/store/authStore";
import { getInitials } from "@/lib/utils";

function splitName(fullName: string): [string, string] {
  const parts = fullName.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return ["", ""];
  if (parts.length === 1) return [parts[0], ""];
  return [parts[0], parts.slice(1).join(" ")];
}

const editInputClass = "bg-white";
const MIN_PASSWORD_LENGTH = 8;

export default function Setting() {
  const user = useAuthStore((state) => state.user);
  const refreshUser = useAuthStore((state) => state.refreshUser);
  const [savedFirstName, savedLastName] = splitName(user?.real_name ?? "");

  const [isEditing, setIsEditing] = useState(false);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [isSavingProfile, setIsSavingProfile] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isChangingPassword, setIsChangingPassword] = useState(false);

  const initials = getInitials(user?.real_name ?? "") || "U";
  const major = user?.major || "—";

  const nextRealName = `${firstName.trim()} ${lastName.trim()}`.trim();
  const profileDirty = nextRealName !== (user?.real_name ?? "").trim();
  const canSaveProfile = profileDirty && firstName.trim().length > 0 && !isSavingProfile;

  const passwordTooShort = newPassword.length > 0 && newPassword.length < MIN_PASSWORD_LENGTH;
  const passwordMismatch = confirmPassword.length > 0 && newPassword !== confirmPassword;
  const canChangePassword =
    currentPassword.length > 0 &&
    newPassword.length >= MIN_PASSWORD_LENGTH &&
    newPassword === confirmPassword &&
    !isChangingPassword;

  // ใช้ค่าล่าสุดจาก store ทุกครั้งที่กด Edit
  const startEditing = () => {
    setFirstName(savedFirstName);
    setLastName(savedLastName);
    setIsEditing(true);
  };

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSaveProfile) return;

    setIsSavingProfile(true);
    try {
      const updated = await authApi.updateProfile({ real_name: nextRealName });
      refreshUser(updated);
      setIsEditing(false);
      notify.success("บันทึกข้อมูลส่วนตัวแล้ว");
    } catch (err) {
      console.error("Failed to update profile:", err);
      notify.error("บันทึกข้อมูลไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsSavingProfile(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canChangePassword) return;

    setIsChangingPassword(true);
    try {
      const { message } = await authApi.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      notify.success("เปลี่ยนรหัสผ่านสำเร็จ", message);
    } catch (err) {
      console.error("Failed to change password:", err);
      notify.error("เปลี่ยนรหัสผ่านไม่สำเร็จ", getApiErrorMessage(err, "โปรดลองใหม่อีกครั้ง"));
    } finally {
      setIsChangingPassword(false);
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-6 rounded-3xl bg-[#FFFDF6] p-8">
        <Avatar className="size-20">
          <AvatarFallback className="bg-[#F08B51] text-3xl text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
        <div>
          <h1 className="text-3xl font-bold text-[#211a14]">
            {user?.real_name || "User"}
          </h1>
          <p className="mt-1 text-base font-medium text-[#BB6653]">
            {user?.student_id} · {major}
          </p>
          <p className="mt-1 text-base text-[#211a14]/50">{user?.gmail}</p>
        </div>
      </div>

      <div className="rounded-3xl bg-[#FFFDF6] p-8">
        <form onSubmit={handleSaveProfile}>
          <div className="flex items-center justify-between gap-3">
            <p className="text-base font-semibold tracking-wide text-[#BB6653] uppercase">
              Personal Information
            </p>
            {!isEditing && (
              <Button type="button" variant="outline" onClick={startEditing}>
                <Pencil />
                Edit
              </Button>
            )}
          </div>

          <div className="mt-5 grid gap-5 sm:grid-cols-2">
            <Field label="First name">
              {isEditing ? (
                <Input
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  disabled={isSavingProfile}
                  maxLength={100}
                  required
                  autoFocus
                  className={editInputClass}
                />
              ) : (
                <Value>{savedFirstName}</Value>
              )}
            </Field>
            <Field label="Last name">
              {isEditing ? (
                <Input
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  disabled={isSavingProfile}
                  maxLength={100}
                  className={editInputClass}
                />
              ) : (
                <Value>{savedLastName}</Value>
              )}
            </Field>
            <Field label="Email">
              <Value>{user?.gmail}</Value>
            </Field>
            <Field label="รหัสประจำตัว">
              <Value>{user?.student_id}</Value>
            </Field>
          </div>

          {isEditing && (
            <>
              <p className="mt-5 text-sm text-[#211a14]/50">
                อีเมลและรหัสประจำตัวผูกกับรายชื่อผู้มีสิทธิ์ หากต้องการแก้ไขกรุณาติดต่อผู้ดูแลระบบ
              </p>
              <div className="mt-6 flex gap-3">
                <Button
                  type="submit"
                  disabled={!canSaveProfile}
                  className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90"
                >
                  {isSavingProfile && <Loader2 className="animate-spin" />}
                  Save changes
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setIsEditing(false)}
                  disabled={isSavingProfile}
                >
                  Cancel
                </Button>
              </div>
            </>
          )}
        </form>

        <div className="my-8 h-px bg-black/5" />

        <form onSubmit={handleChangePassword}>
          <p className="text-base font-semibold tracking-wide text-[#BB6653] uppercase">
            Change Password
          </p>
          <p className="mt-1 text-base text-[#211a14]/50">
            ต้องใส่รหัสผ่านปัจจุบันก่อน — รหัสผ่านใหม่ต้องมีอย่างน้อย {MIN_PASSWORD_LENGTH} ตัวอักษร
          </p>

          <Field label="Current password" className="mt-5">
            <Input
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              disabled={isChangingPassword}
              className={editInputClass}
            />
          </Field>

          <div className="mt-5 grid gap-5 sm:grid-cols-2">
            <Field
              label="New password"
              error={passwordTooShort ? `อย่างน้อย ${MIN_PASSWORD_LENGTH} ตัวอักษร` : undefined}
            >
              <Input
                type="password"
                autoComplete="new-password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                disabled={isChangingPassword}
                className={editInputClass}
              />
            </Field>
            <Field
              label="Confirm new password"
              error={passwordMismatch ? "รหัสผ่านไม่ตรงกัน" : undefined}
            >
              <Input
                type="password"
                autoComplete="new-password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                disabled={isChangingPassword}
                className={editInputClass}
              />
            </Field>
          </div>

          <div className="mt-6">
            <Button
              type="submit"
              disabled={!canChangePassword}
              className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90"
            >
              {isChangingPassword && <Loader2 className="animate-spin" />}
              Change password
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

function Field({
  label,
  error,
  className,
  children,
}: {
  label: string;
  error?: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={className}>
      <label className="text-base text-[#211a14]/70">{label}</label>
      <div className="mt-1.5">{children}</div>
      {error && <p className="mt-1 text-sm text-red-500">{error}</p>}
    </div>
  );
}

// สูงเท่า <Input> หน้าจึงไม่กระตุกตอนสลับโหมด
function Value({ children }: { children?: React.ReactNode }) {
  return (
    <p className="flex h-8 items-center truncate rounded-lg border border-transparent bg-[#FFF8E8] px-2.5 text-base text-[#211a14] md:text-sm">
      {children || "—"}
    </p>
  );
}
