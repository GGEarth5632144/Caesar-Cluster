import { Bell, Search } from "lucide-react";
import { Link } from "react-router-dom";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { getInitials } from "@/lib/utils";

interface TopbarProps {
  title: string;
  userName: string;
}

export default function Topbar({ title, userName }: TopbarProps) {
  const initials = getInitials(userName) || "U";

  return (
    <header className="flex h-20 shrink-0 items-center gap-4 bg-[#BB6653] px-4 text-white sm:gap-6 sm:px-8">
      <h1 className="shrink-0 text-xl font-semibold whitespace-nowrap">{title}</h1>

      <div className="hidden min-w-0 flex-1 justify-center lg:flex">
        <div className="flex w-full max-w-lg items-center gap-2.5 rounded-full bg-[#FFF8E8] px-5 py-3 text-[#211a14]">
          <Search size={18} className="shrink-0 text-[#211a14]/60" />
          <input
            placeholder="What are you looking for?"
            className="w-full min-w-0 bg-transparent text-sm outline-none placeholder:text-[#211a14]/50"
          />
        </div>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-5">
        <Link
          to="/alert"
          className="cursor-pointer text-white/90 hover:text-white transition-colors"
          aria-label="การแจ้งเตือน"
        >
          <Bell size={22} />
        </Link>
        <Link to="/profile">
          <Avatar>
            <AvatarFallback className="bg-[#F08B51] text-white">
              {initials}
            </AvatarFallback>
          </Avatar>
        </Link>
      </div>
    </header>
  );
}
