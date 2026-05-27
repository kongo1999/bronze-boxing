import Link from "next/link";
import { endOfDay, endOfWeek, startOfDay } from "date-fns";
import { BellPlus, Plus } from "lucide-react";
import { listReminders } from "@/lib/services/reminders";
import { PageHeader } from "@/components/ui/PageHeader";
import { EmptyState } from "@/components/ui/EmptyState";
import { buttonClasses } from "@/components/ui/Button";
import { ReminderItem } from "@/components/reminders/ReminderItem";
import type { ReminderDTO } from "@/lib/dto";

export const dynamic = "force-dynamic";

export default async function RemindersPage() {
  const reminders = await listReminders({ includeDone: true });
  const now = new Date();
  const todayStart = startOfDay(now);
  const todayEnd = endOfDay(now);
  const weekEnd = endOfWeek(now, { weekStartsOn: 1 });

  const active = reminders.filter((r) => !r.done);
  const due = (r: ReminderDTO) => new Date(r.dueDate);

  const overdue = active.filter((r) => due(r) < todayStart);
  const today = active.filter((r) => due(r) >= todayStart && due(r) <= todayEnd);
  const thisWeek = active.filter((r) => due(r) > todayEnd && due(r) <= weekEnd);
  const later = active.filter((r) => due(r) > weekEnd);
  const done = reminders.filter((r) => r.done).slice(0, 10);

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="To-do"
        title="Reminders"
        action={
          <Link href="/reminders/new" className={buttonClasses("primary", "sm")}>
            <Plus className="h-4 w-4" />
            New
          </Link>
        }
      />

      {reminders.length === 0 ? (
        <EmptyState
          icon={BellPlus}
          title="Nothing to remember yet"
          description="Jot down what you need to do today or this week."
          action={
            <Link href="/reminders/new" className={buttonClasses("primary", "sm")}>
              <Plus className="h-4 w-4" />
              New reminder
            </Link>
          }
        />
      ) : (
        <>
          <Group title="Overdue" items={overdue} overdue />
          <Group title="Today" items={today} />
          <Group title="This week" items={thisWeek} />
          <Group title="Later" items={later} />
          <Group title="Done" items={done} muted />
        </>
      )}
    </div>
  );
}

function Group({
  title,
  items,
  overdue = false,
  muted = false,
}: {
  title: string;
  items: ReminderDTO[];
  overdue?: boolean;
  muted?: boolean;
}) {
  if (items.length === 0) return null;
  return (
    <section className="space-y-2">
      <h2
        className={`px-1 label-eyebrow text-[0.625rem] ${overdue ? "text-overdue" : "text-faint"}`}
      >
        {title} · {items.length}
      </h2>
      <ul className={`space-y-2 ${muted ? "opacity-70" : ""}`}>
        {items.map((r) => (
          <li key={r.id}>
            <ReminderItem reminder={r} overdue={overdue} />
          </li>
        ))}
      </ul>
    </section>
  );
}
