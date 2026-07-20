import { useState } from "react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuthStore } from "@/store/authStore";
import { getInitials } from "@/lib/utils";

function splitName(fullName: string): [string, string] {
  const parts = fullName.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return ["", ""];
  if (parts.length === 1) return [parts[0], ""];
  return [parts[0], parts.slice(1).join(" ")];
}

// backend ยังไม่มีคอลัมน์เก็บ email/year_of_study/major (ดู note เดียวกันใน Register.tsx)
// เลยโชว์เป็นค่า mock ไปก่อนจนกว่า /api/me จะคืนค่าพวกนี้จริง
const MOCK_EMAIL = "example@gmail.com";
const MOCK_YEAR_OF_STUDY = "4";
const MOCK_MAJOR = "Computer Engineering";

const readOnlyInputClass =
  "disabled:bg-[#EFE6D2] disabled:text-[#211a14]/70 disabled:opacity-100";

export default function Profile() {
  const user = useAuthStore((state) => state.user);
  const [initialFirstName, initialLastName] = splitName(user?.real_name ?? "");

  const [firstName, setFirstName] = useState(initialFirstName);
  const [lastName, setLastName] = useState(initialLastName);

  const initials = getInitials(user?.real_name ?? "") || "U";

  const handleReset = () => {
    setFirstName(initialFirstName);
    setLastName(initialLastName);
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-6 rounded-3xl bg-[#FFFDF6] p-8">
        <Avatar className="size-20">
          <AvatarFallback className="bg-[#F08B51] text-2xl text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
        <div>
          <h1 className="text-2xl font-bold text-[#211a14]">
            {user?.real_name || "User"}
          </h1>
          <p className="mt-1 text-sm font-medium text-[#BB6653]">
            {user?.student_id} · Year {MOCK_YEAR_OF_STUDY} · {MOCK_MAJOR}
          </p>
          <p className="mt-1 text-sm text-[#211a14]/50">{MOCK_EMAIL}</p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
        <div className="rounded-3xl bg-[#FFFDF6] p-8">
          <p className="text-sm font-semibold tracking-wide text-[#BB6653] uppercase">
            Personal Information
          </p>

          <div className="mt-5 grid gap-5 sm:grid-cols-2">
            <Field label="First name">
              <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} />
            </Field>
            <Field label="Last name">
              <Input value={lastName} onChange={(e) => setLastName(e.target.value)} />
            </Field>
          </div>

          <Field label="Email" readOnly className="mt-5">
            <Input value={MOCK_EMAIL} disabled className={readOnlyInputClass} />
          </Field>

          <div className="mt-5 grid gap-5 sm:grid-cols-2">
            <Field label="Student number" readOnly>
              <Input value={user?.student_id ?? ""} disabled className={readOnlyInputClass} />
            </Field>
            <Field label="Year of study" readOnly>
              <Input value={MOCK_YEAR_OF_STUDY} disabled className={readOnlyInputClass} />
            </Field>
          </div>

          <div className="mt-6 flex gap-3">
            <Button className="bg-[#F08B51] text-white hover:bg-[#F08B51]/90">
              Save changes
            </Button>
            <Button type="button" variant="outline" onClick={handleReset}>
              Reset
            </Button>
          </div>
        </div>

        <div className="rounded-3xl bg-[#FFFDF6] p-8">
          <p className="text-sm font-semibold tracking-wide text-[#BB6653] uppercase">
            Security
          </p>

          <div className="mt-5 flex items-start justify-between gap-3 border-b border-black/5 pb-5">
            <div>
              <p className="font-semibold text-[#211a14]">SSH Key</p>
              <p className="mt-1 flex items-center gap-1.5 text-sm text-[#211a14]/60">
                <span className="size-2 rounded-full bg-green-500" />
                Uploaded · ed25519
              </p>
            </div>
            <Button type="button" variant="outline" size="sm">
              Manage
            </Button>
          </div>

          <div className="mt-5 flex items-start justify-between gap-3">
            <div>
              <p className="font-semibold text-[#211a14]">Password</p>
              <p className="mt-1 text-sm text-[#211a14]/60">Last changed 3 months ago</p>
            </div>
            <Button type="button" variant="outline" size="sm">
              Change
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function Field({
  label,
  readOnly,
  className,
  children,
}: {
  label: string;
  readOnly?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={className}>
      <label className="text-sm text-[#211a14]/70">
        {label}
        {readOnly && <span className="ml-1 text-[#BB6653]/70">· read only</span>}
      </label>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}
