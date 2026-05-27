"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  Banknote,
  Bell,
  BellPlus,
  CalendarDays,
  CalendarPlus,
  House,
  Plus,
  UserPlus,
  Users,
  Wallet,
  X,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { APP_NAME } from "@/lib/constants";

type NavItem = { href: string; label: string; icon: LucideIcon };

const tabs: NavItem[] = [
  { href: "/", label: "Home", icon: House },
  { href: "/schedule", label: "Schedule", icon: CalendarDays },
  { href: "/trainees", label: "Crew", icon: Users },
  { href: "/payments", label: "Money", icon: Wallet },
];

const railItems: NavItem[] = [...tabs, { href: "/reminders", label: "Reminders", icon: Bell }];

const addActions: NavItem[] = [
  { href: "/schedule/new", label: "New session", icon: CalendarPlus },
  { href: "/payments/new", label: "Log payment", icon: Banknote },
  { href: "/trainees/new", label: "Add trainee", icon: UserPlus },
  { href: "/reminders/new", label: "New reminder", icon: BellPlus },
];

function isActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

export function Navigation() {
  const pathname = usePathname();
  const [addOpen, setAddOpen] = useState(false);

  // Close the quick-add sheet on Escape. (Link clicks close it inline below.)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setAddOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <>
      {/* Desktop side rail */}
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-60 flex-col border-r border-line bg-surface px-4 py-6 md:flex">
        <Link href="/" className="mb-8 flex items-center gap-2 px-2">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-bronze font-display text-base font-bold text-bronze-ink">
            B
          </span>
          <span className="font-display text-lg font-semibold tracking-tight">
            {APP_NAME}
          </span>
        </Link>
        <nav className="flex flex-col gap-1">
          {railItems.map(({ href, label, icon: Icon }) => {
            const active = isActive(pathname, href);
            return (
              <Link
                key={href}
                href={href}
                className={cn(
                  "flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors",
                  active
                    ? "bg-bronze/15 text-bronze"
                    : "text-muted hover:bg-elevated hover:text-fg",
                )}
              >
                <Icon className="h-5 w-5" strokeWidth={active ? 2.25 : 1.75} />
                {label}
              </Link>
            );
          })}
        </nav>
        <button
          type="button"
          onClick={() => setAddOpen(true)}
          className="mt-6 inline-flex h-11 items-center justify-center gap-2 rounded-xl bg-bronze font-medium text-bronze-ink transition-[background-color] hover:bg-bronze-strong"
        >
          <Plus className="h-5 w-5" strokeWidth={2.5} />
          Quick add
        </button>
      </aside>

      {/* Mobile bottom nav */}
      <nav className="fixed inset-x-0 bottom-0 z-40 border-t border-line bg-surface/95 backdrop-blur md:hidden">
        <div
          className="mx-auto grid max-w-md grid-cols-5 items-center px-2"
          style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
        >
          {tabs.slice(0, 2).map((item) => (
            <Tab key={item.href} item={item} active={isActive(pathname, item.href)} />
          ))}
          <div className="flex justify-center">
            <button
              type="button"
              onClick={() => setAddOpen(true)}
              aria-label="Quick add"
              className="-mt-6 grid h-14 w-14 place-items-center rounded-full bg-bronze text-bronze-ink shadow-lift transition-transform active:scale-95"
            >
              <Plus className="h-7 w-7" strokeWidth={2.5} />
            </button>
          </div>
          {tabs.slice(2).map((item) => (
            <Tab key={item.href} item={item} active={isActive(pathname, item.href)} />
          ))}
        </div>
      </nav>

      {/* Quick-add sheet (mobile bottom sheet / desktop popover) */}
      <div
        className={cn(
          "fixed inset-0 z-50 transition-opacity duration-200",
          addOpen ? "opacity-100" : "pointer-events-none opacity-0",
        )}
        aria-hidden={!addOpen}
      >
        <div
          className="absolute inset-0 bg-canvas/70 backdrop-blur-sm"
          onClick={() => setAddOpen(false)}
        />
        <div
          className={cn(
            "absolute inset-x-0 bottom-0 rounded-t-2xl border-t border-line bg-surface p-4 shadow-lift transition-transform duration-200 ease-[var(--ease-out-quart)]",
            "md:inset-auto md:bottom-6 md:left-6 md:w-64 md:rounded-2xl md:border",
            addOpen ? "translate-y-0" : "translate-y-6",
          )}
          style={{ paddingBottom: "calc(1rem + env(safe-area-inset-bottom))" }}
        >
          <div className="mb-3 flex items-center justify-between">
            <p className="label-eyebrow text-[0.625rem] text-faint">Quick add</p>
            <button
              type="button"
              onClick={() => setAddOpen(false)}
              aria-label="Close"
              className="text-faint hover:text-fg"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
          <div className="grid grid-cols-2 gap-2 md:grid-cols-1">
            {addActions.map(({ href, label, icon: Icon }) => (
              <Link
                key={href}
                href={href}
                onClick={() => setAddOpen(false)}
                className="flex items-center gap-3 rounded-xl border border-line bg-elevated px-3 py-3 text-sm font-medium transition-colors hover:border-bronze/40 hover:text-bronze"
              >
                <Icon className="h-5 w-5 text-bronze" strokeWidth={2} />
                {label}
              </Link>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}

function Tab({ item, active }: { item: NavItem; active: boolean }) {
  const { href, label, icon: Icon } = item;
  return (
    <Link
      href={href}
      className={cn(
        "flex flex-col items-center gap-1 py-2.5 text-[0.6875rem] font-medium transition-colors",
        active ? "text-bronze" : "text-faint hover:text-muted",
      )}
    >
      <Icon className="h-[1.35rem] w-[1.35rem]" strokeWidth={active ? 2.25 : 1.75} />
      {label}
    </Link>
  );
}
