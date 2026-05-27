"use client";

import { useTransition } from "react";
import { setAttendanceAction } from "@/app/schedule/actions";
import { Avatar } from "@/components/ui/Avatar";
import { cn } from "@/lib/utils";
import type { AttendeeDTO } from "@/lib/dto";
import type { AttendanceStatus } from "@/lib/constants";

const OPTIONS: { v: AttendanceStatus; label: string; active: string }[] = [
  { v: "attended", label: "Here", active: "border-paid/40 bg-paid/15 text-paid" },
  { v: "no_show", label: "No-show", active: "border-overdue/40 bg-overdue/15 text-overdue" },
];

export function AttendanceControls({
  sessionId,
  attendees,
}: {
  sessionId: string;
  attendees: AttendeeDTO[];
}) {
  const [pending, start] = useTransition();

  if (attendees.length === 0) {
    return <p className="text-sm text-muted">No one is on this session yet.</p>;
  }

  return (
    <ul className="space-y-2">
      {attendees.map((a) => (
        <li
          key={a.traineeId}
          className="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2"
        >
          <Avatar name={a.traineeName ?? "?"} className="h-8 w-8 text-xs" />
          <span className="min-w-0 flex-1 truncate text-sm">{a.traineeName ?? "Unknown"}</span>
          <div className="flex gap-1">
            {OPTIONS.map((o) => {
              const on = a.status === o.v;
              return (
                <button
                  key={o.v}
                  type="button"
                  disabled={pending}
                  onClick={() =>
                    start(() =>
                      setAttendanceAction(sessionId, a.traineeId, on ? "booked" : o.v),
                    )
                  }
                  className={cn(
                    "rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors disabled:opacity-50",
                    on ? o.active : "border-line text-faint hover:text-fg",
                  )}
                >
                  {o.label}
                </button>
              );
            })}
          </div>
        </li>
      ))}
    </ul>
  );
}
