import Link from "next/link";
import { addDays, addWeeks, endOfWeek, startOfWeek } from "date-fns";
import { CalendarPlus, ChevronLeft, ChevronRight } from "lucide-react";
import { listSessions } from "@/lib/services/sessions";
import { PageHeader } from "@/components/ui/PageHeader";
import { Badge } from "@/components/ui/Badge";
import { buttonClasses } from "@/components/ui/Button";
import { cn, formatTime } from "@/lib/utils";
import type { SessionDTO } from "@/lib/dto";

function sameDay(a: Date, b: Date) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

const shortDate = (d: Date) =>
  d.toLocaleDateString("en-US", { month: "short", day: "numeric" });

export default async function SchedulePage({
  searchParams,
}: {
  searchParams: Promise<{ w?: string }>;
}) {
  const { w } = await searchParams;
  const offset = Number(w ?? 0) || 0;
  const base = addWeeks(new Date(), offset);
  const weekStart = startOfWeek(base, { weekStartsOn: 1 });
  const weekEnd = endOfWeek(base, { weekStartsOn: 1 });
  const now = new Date();

  const sessions = await listSessions({ from: weekStart, to: weekEnd });
  const days = Array.from({ length: 7 }, (_, i) => addDays(weekStart, i));

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Schedule"
        title="Schedule"
        action={
          <Link href="/schedule/new" className={buttonClasses("primary", "sm")}>
            <CalendarPlus className="h-4 w-4" />
            New
          </Link>
        }
      />

      <div className="flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
        <Link href={`/schedule?w=${offset - 1}`} className={buttonClasses("ghost", "icon")} aria-label="Previous week">
          <ChevronLeft className="h-5 w-5" />
        </Link>
        <div className="text-center">
          <p className="font-display font-semibold tracking-tight">
            {shortDate(weekStart)} – {shortDate(weekEnd)}
          </p>
          {offset === 0 ? (
            <p className="text-xs text-faint">This week</p>
          ) : (
            <Link href="/schedule" className="text-xs text-bronze">
              Back to this week
            </Link>
          )}
        </div>
        <Link href={`/schedule?w=${offset + 1}`} className={buttonClasses("ghost", "icon")} aria-label="Next week">
          <ChevronRight className="h-5 w-5" />
        </Link>
      </div>

      <div className="space-y-4">
        {days.map((day) => {
          const daySessions = sessions.filter((s) => sameDay(new Date(s.start), day));
          const isToday = sameDay(day, now);
          return (
            <section key={day.toISOString()}>
              <div className="mb-1.5 flex items-center gap-2 px-1">
                <h2
                  className={cn(
                    "label-eyebrow text-[0.6875rem]",
                    isToday ? "text-bronze" : "text-faint",
                  )}
                >
                  {day.toLocaleDateString("en-US", { weekday: "short", day: "numeric" })}
                </h2>
                {isToday ? <span className="text-[0.625rem] font-medium text-bronze">Today</span> : null}
              </div>
              {daySessions.length === 0 ? (
                <p className="px-1 text-sm text-faint/70">—</p>
              ) : (
                <ul className="space-y-2">
                  {daySessions.map((s) => (
                    <li key={s.id}>
                      <SessionRow session={s} />
                    </li>
                  ))}
                </ul>
              )}
            </section>
          );
        })}
      </div>
    </div>
  );
}

function SessionRow({ session: s }: { session: SessionDTO }) {
  const cancelled = s.status === "cancelled";
  const sub =
    s.type === "private"
      ? (s.attendees[0]?.traineeName ?? "Private session")
      : `${s.attendees.length} in${s.capacity ? ` / ${s.capacity}` : ""}`;
  return (
    <Link
      href={`/schedule/${s.id}`}
      className={cn(
        "flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30",
        cancelled && "opacity-50",
      )}
    >
      <div className="w-14 shrink-0 text-center">
        <p className="font-display text-sm font-semibold tnum">{formatTime(s.start)}</p>
        <p className="text-[0.625rem] text-faint">{s.durationMin}m</p>
      </div>
      <div className="min-w-0 flex-1">
        <p className={cn("truncate font-medium", cancelled && "line-through")}>{s.title}</p>
        <p className="truncate text-xs text-muted">
          {sub}
          {s.location ? ` · ${s.location}` : ""}
        </p>
      </div>
      <Badge tone={s.type === "group" ? "bronze" : "info"}>
        {s.type === "group" ? "Group" : "Private"}
      </Badge>
    </Link>
  );
}
