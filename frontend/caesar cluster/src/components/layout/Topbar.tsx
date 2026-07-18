import { Bell, Search } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";

interface TopbarProps {
  title: string;
  userName: string;
}

export default function Topbar({ title, userName }: TopbarProps) {
  const initials = userName.trim().slice(0, 2).toUpperCase() || "U";

  return (
    <header className="flex h-16 shrink-0 items-center justify-between gap-4 bg-[#BB6653] px-6 text-white">
      <h1 className="text-lg font-semibold whitespace-nowrap">{title}</h1>

      <div className="flex w-full max-w-md items-center gap-2 rounded-full bg-[#FFF8E8] px-4 py-2 text-[#211a14]">
        <Search size={16} className="text-[#211a14]/60" />
        <input
          placeholder="What are you looking for?"
          className="w-full bg-transparent text-sm outline-none placeholder:text-[#211a14]/50"
        />
      </div>

      <div className="flex items-center gap-4">
        <button
          type="button"
          className="text-white/90 hover:text-white"
          aria-label="การแจ้งเตือน"
        >
          <Bell size={18} />
        </button>
        <Avatar size="sm">
          <AvatarFallback className="bg-[#F08B51] text-white">
            {initials}
          </AvatarFallback>
        </Avatar>
      </div>
    </header>
  );
}
