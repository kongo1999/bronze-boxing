import Link from "next/link";
import { Bell } from "lucide-react";
import { APP_NAME } from "@/lib/constants";

/** Mobile-only header. On desktop the side rail carries the wordmark instead. */
export function TopBar() {
  return (
    <header className="sticky top-0 z-30 flex items-center justify-between border-b border-line bg-canvas/90 px-4 py-3 backdrop-blur md:hidden">
      <Link href="/" className="flex items-center gap-2">
        <span className="grid h-7 w-7 place-items-center rounded-md bg-bronze font-display text-sm font-bold text-bronze-ink">
          B
        </span>
        <span className="font-display text-base font-semibold tracking-tight">
          {APP_NAME}
        </span>
      </Link>
      <Link
        href="/reminders"
        aria-label="Reminders"
        className="text-muted transition-colors hover:text-fg"
      >
        <Bell className="h-5 w-5" />
      </Link>
    </header>
  );
}
