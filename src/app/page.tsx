import Link from "next/link";
import { ArrowRight, CircleCheck } from "lucide-react";
import { getDashboardData } from "@/lib/services/dashboard";
import { PageHeader } from "@/components/ui/PageHeader";
import { StatTile } from "@/components/ui/StatTile";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Avatar } from "@/components/ui/Avatar";
import { buttonClasses } from "@/components/ui/Button";
import { ReminderItem } from "@/components/reminders/ReminderItem";
import { formatMoney, formatTime, monthLabel } from "@/lib/utils";

// Always reflect current DB state (live admin tool, not a static page).
export const dynamic = "force-dynamic";

export default async function Dashboard() {
  const d = await getDashboardData();
  const hour = new Date(d.today).getHours();
  const greeting = hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  const dateLabel = new Date(d.today).toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
  });

  return (
    <div className="space-y-5">
      <PageHeader eyebrow={dateLabel} title={greeting} />

      <div className="grid grid-cols-2 gap-3">
        <StatTile
          label={`${monthLabel(d.month)} · cash in`}
          value={formatMoney(d.monthRevenue)}
          sub="collected"
          accent
        />
        <StatTile
          label="Active crew"
          value={d.activeTrainees}
          sub={d.overdueCount > 0 ? `${d.overdueCount} owe dues` : "all paid up"}
        />
      </div>

      {/* Today's sessions */}
      <section className="space-y-2">
        <SectionHeader title="Today's sessions" href="/schedule" cta="Schedule" />
        {d.todaySessions.length === 0 ? (
          <Card className="px-4 py-5 text-sm text-muted">No classes on the calendar today.</Card>
        ) : (
          <ul className="space-y-2">
            {d.todaySessions.map((s) => {
              const cancelled = s.status === "cancelled";
              return (
                <li key={s.id}>
                  <Link
                    href={`/schedule/${s.id}`}
                    className={`flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30 ${
                      cancelled ? "opacity-50" : ""
                    }`}
                  >
                    <div className="w-14 shrink-0 text-center">
                      <p className="font-display text-sm font-semibold tnum">{formatTime(s.start)}</p>
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className={`truncate font-medium ${cancelled ? "line-through" : ""}`}>
                        {s.title}
                      </p>
                      <p className="truncate text-xs text-muted">
                        {s.type === "private"
                          ? (s.attendees[0]?.traineeName ?? "Private")
                          : `${s.attendees.length} booked`}
                      </p>
                    </div>
                    <Badge tone={s.type === "group" ? "bronze" : "info"}>
                      {s.type === "group" ? "Group" : "Private"}
                    </Badge>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {/* Overdue subscriptions */}
      {d.overdueSubscriptions.length > 0 ? (
        <section className="space-y-2">
          <SectionHeader title="Needs collecting" href="/payments" cta="Money" />
          <ul className="space-y-2">
            {d.overdueSubscriptions.slice(0, 5).map((s) => (
              <li
                key={s.trainee.id}
                className="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
              >
                <Link href={`/trainees/${s.trainee.id}`} className="flex min-w-0 items-center gap-2.5">
                  <Avatar name={s.trainee.name} className="h-8 w-8 text-xs" />
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{s.trainee.name}</p>
                    <p className="text-xs text-overdue tnum">owes {formatMoney(s.due - s.amountPaid)}</p>
                  </div>
                </Link>
                <Link
                  href={`/payments/new?trainee=${s.trainee.id}&type=subscription&periodMonth=${d.month}&amount=${
                    s.due - s.amountPaid
                  }`}
                  className={buttonClasses("primary", "sm")}
                >
                  Collect
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {/* Reminders for day & week */}
      <section className="space-y-2">
        <SectionHeader title="Reminders" href="/reminders" cta="All" />
        {d.weekReminders.length === 0 ? (
          <Card className="flex items-center gap-2.5 px-4 py-5 text-sm text-muted">
            <CircleCheck className="h-4 w-4 text-paid" />
            Nothing due this week.
          </Card>
        ) : (
          <ul className="space-y-2">
            {d.weekReminders.map((r) => (
              <li key={r.id}>
                <ReminderItem reminder={r} />
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function SectionHeader({ title, href, cta }: { title: string; href: string; cta: string }) {
  return (
    <div className="flex items-center justify-between px-1">
      <h2 className="label-eyebrow text-[0.625rem] text-faint">{title}</h2>
      <Link href={href} className="inline-flex items-center gap-1 text-sm font-medium text-bronze">
        {cta}
        <ArrowRight className="h-3.5 w-3.5" />
      </Link>
    </div>
  );
}
