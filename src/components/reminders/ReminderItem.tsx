"use client";

import { useTransition } from "react";
import { Check, Trash2 } from "lucide-react";
import { deleteReminderAction, toggleReminderAction } from "@/app/reminders/actions";
import { cn, formatDate } from "@/lib/utils";
import type { ReminderDTO } from "@/lib/dto";

export function ReminderItem({
  reminder: r,
  overdue = false,
}: {
  reminder: ReminderDTO;
  overdue?: boolean;
}) {
  const [pending, start] = useTransition();

  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-opacity",
        pending && "opacity-50",
      )}
    >
      <button
        type="button"
        aria-label={r.done ? "Mark not done" : "Mark done"}
        disabled={pending}
        onClick={() => start(() => toggleReminderAction(r.id, !r.done))}
        className={cn(
          "grid h-5 w-5 shrink-0 place-items-center rounded-md border transition-colors",
          r.done
            ? "border-paid bg-paid/20 text-paid"
            : "border-line text-transparent hover:border-bronze",
        )}
      >
        <Check className="h-3.5 w-3.5" strokeWidth={3} />
      </button>

      <div className="min-w-0 flex-1">
        <p className={cn("truncate text-sm font-medium", r.done && "text-faint line-through")}>
          {r.title}
        </p>
        <p
          className={cn(
            "truncate text-xs",
            overdue && !r.done ? "text-overdue" : "text-faint",
          )}
        >
          {formatDate(r.dueDate)}
          {r.notes ? ` · ${r.notes}` : ""}
        </p>
      </div>

      {!r.done && r.priority === "high" ? (
        <span className="h-2 w-2 shrink-0 rounded-full bg-overdue" aria-label="High priority" />
      ) : null}

      <form
        action={deleteReminderAction}
        onSubmit={(e) => {
          if (!confirm("Delete this reminder?")) e.preventDefault();
        }}
      >
        <input type="hidden" name="id" value={r.id} />
        <button
          type="submit"
          aria-label="Delete reminder"
          className="text-faint transition-colors hover:text-overdue"
        >
          <Trash2 className="h-4 w-4" />
        </button>
      </form>
    </div>
  );
}
