"use client";

import Link from "next/link";
import { useActionState, useState } from "react";
import { Repeat, Users } from "lucide-react";
import { saveSessionAction } from "@/app/schedule/actions";
import { initialActionState } from "@/lib/form";
import { Button, buttonClasses } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field, Input, Textarea } from "@/components/ui/Field";
import { cn, toDateTimeInputValue } from "@/lib/utils";
import type { SessionDTO } from "@/lib/dto";
import type { TraineeDTO } from "@/lib/dto";
import type { SessionType } from "@/lib/constants";

const DAYS = [
  { v: "1", label: "Mon" },
  { v: "2", label: "Tue" },
  { v: "3", label: "Wed" },
  { v: "4", label: "Thu" },
  { v: "5", label: "Fri" },
  { v: "6", label: "Sat" },
  { v: "0", label: "Sun" },
];

function defaultStart(): string {
  const d = new Date();
  d.setHours(18, 0, 0, 0);
  return toDateTimeInputValue(d);
}

export function SessionForm({
  session,
  trainees,
}: {
  session?: SessionDTO;
  trainees: TraineeDTO[];
}) {
  const [state, action, pending] = useActionState(saveSessionAction, initialActionState);
  const fe = state.fieldErrors ?? {};
  const editing = Boolean(session);

  const [type, setType] = useState<SessionType>(session?.type ?? "group");
  const [repeat, setRepeat] = useState(false);
  const selectedIds = new Set(session?.attendees.map((a) => a.traineeId));

  // Only active trainees, plus anyone already on this session (even if inactive).
  const pickable = trainees.filter(
    (t) => t.status === "active" || selectedIds.has(t.id),
  );

  return (
    <form action={action} className="space-y-4">
      {session ? <input type="hidden" name="id" value={session.id} /> : null}

      <Card className="space-y-4 p-4">
        <Field label="Title" htmlFor="title" error={fe.title}>
          <Input
            id="title"
            name="title"
            defaultValue={session?.title}
            placeholder={type === "group" ? "e.g. Evening Group Class" : "e.g. Private — Karim"}
            autoFocus={!editing}
            required
          />
        </Field>

        <div className="grid grid-cols-2 gap-2">
          <TypeToggle value={type} onChange={setType} />
        </div>
        <input type="hidden" name="type" value={type} />

        <div className="grid grid-cols-2 gap-3">
          <Field label="Starts" htmlFor="start" error={fe.start}>
            <Input
              id="start"
              name="start"
              type="datetime-local"
              defaultValue={toDateTimeInputValue(session?.start) || defaultStart()}
              required
            />
          </Field>
          <Field label="Duration (min)" htmlFor="durationMin">
            <Input
              id="durationMin"
              name="durationMin"
              type="number"
              min="1"
              step="1"
              inputMode="numeric"
              defaultValue={session?.durationMin ?? 60}
            />
          </Field>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Location" htmlFor="location">
            <Input id="location" name="location" defaultValue={session?.location} placeholder="Main floor" />
          </Field>
          {type === "group" ? (
            <Field label="Capacity" htmlFor="capacity" hint="Optional">
              <Input
                id="capacity"
                name="capacity"
                type="number"
                min="1"
                inputMode="numeric"
                defaultValue={session?.capacity ?? ""}
                placeholder="12"
              />
            </Field>
          ) : (
            <Field label="Fee" htmlFor="fee" hint="Cash, optional">
              <Input
                id="fee"
                name="fee"
                type="number"
                min="0"
                inputMode="numeric"
                defaultValue={session?.fee ?? ""}
                placeholder="30"
              />
            </Field>
          )}
        </div>
      </Card>

      <Card className="space-y-3 p-4">
        <div className="flex items-center justify-between">
          <p className="label-eyebrow text-[0.625rem] text-faint">
            {type === "group" ? "Attendees" : "Trainee"}
          </p>
          <span className="text-xs text-faint">
            {selectedIds.size > 0 ? `${selectedIds.size} selected` : "none"}
          </span>
        </div>
        {pickable.length === 0 ? (
          <p className="text-sm text-muted">
            No trainees yet.{" "}
            <Link href="/trainees/new" className="text-bronze">
              Add one
            </Link>
            .
          </p>
        ) : (
          <div className="max-h-56 space-y-1 overflow-y-auto">
            {pickable.map((t) => (
              <label
                key={t.id}
                className="flex cursor-pointer items-center gap-3 rounded-xl px-2 py-2 hover:bg-elevated"
              >
                <input
                  type="checkbox"
                  name="traineeIds"
                  value={t.id}
                  defaultChecked={selectedIds.has(t.id)}
                  className="h-4 w-4 accent-bronze"
                />
                <span className="text-sm">{t.name}</span>
              </label>
            ))}
          </div>
        )}
      </Card>

      {!editing ? (
        <Card className="space-y-3 p-4">
          <label className="flex cursor-pointer items-center gap-3">
            <input
              type="checkbox"
              name="repeat"
              checked={repeat}
              onChange={(e) => setRepeat(e.target.checked)}
              className="h-4 w-4 accent-bronze"
            />
            <span className="flex items-center gap-2 text-sm font-medium">
              <Repeat className="h-4 w-4 text-bronze" />
              Repeat weekly
            </span>
          </label>
          {repeat ? (
            <div className="space-y-3 pt-1">
              <div className="flex flex-wrap gap-1.5">
                {DAYS.map((d) => (
                  <label
                    key={d.v}
                    className="cursor-pointer rounded-full border border-line px-3 py-1.5 text-xs font-medium text-muted has-[:checked]:border-bronze has-[:checked]:bg-bronze/15 has-[:checked]:text-bronze"
                  >
                    <input type="checkbox" name="daysOfWeek" value={d.v} className="sr-only" />
                    {d.label}
                  </label>
                ))}
              </div>
              <Field label="For how many weeks?" htmlFor="weeks">
                <Input id="weeks" name="weeks" type="number" min="1" max="52" defaultValue={8} />
              </Field>
              <p className="text-xs text-faint">
                Creates one session on each selected weekday, every week.
              </p>
            </div>
          ) : null}
        </Card>
      ) : null}

      <Card className="p-4">
        <Field label="Notes" htmlFor="notes">
          <Textarea id="notes" name="notes" defaultValue={session?.notes} placeholder="Focus, drills, anything to prep" />
        </Field>
      </Card>

      {fe._form ? <p className="text-sm text-overdue">{fe._form}</p> : null}

      <div className="flex gap-3">
        <Button type="submit" disabled={pending} className="flex-1">
          {pending ? "Saving…" : editing ? "Save changes" : repeat ? "Create series" : "Create session"}
        </Button>
        <Link
          href={session ? `/schedule/${session.id}` : "/schedule"}
          className={buttonClasses("ghost")}
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}

function TypeToggle({
  value,
  onChange,
}: {
  value: SessionType;
  onChange: (v: SessionType) => void;
}) {
  const options: { v: SessionType; label: string }[] = [
    { v: "group", label: "Group class" },
    { v: "private", label: "Private" },
  ];
  return (
    <div className="col-span-2 grid grid-cols-2 gap-2">
      {options.map((o) => (
        <button
          key={o.v}
          type="button"
          onClick={() => onChange(o.v)}
          className={cn(
            "flex items-center justify-center gap-2 rounded-xl border px-3 py-2.5 text-sm font-medium transition-colors",
            value === o.v
              ? "border-bronze bg-bronze/15 text-bronze"
              : "border-line text-muted hover:bg-elevated",
          )}
        >
          <Users className="h-4 w-4" />
          {o.label}
        </button>
      ))}
    </div>
  );
}
