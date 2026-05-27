"use client";

import Link from "next/link";
import { useActionState } from "react";
import { saveReminderAction } from "@/app/reminders/actions";
import { initialActionState } from "@/lib/form";
import { Button, buttonClasses } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field, Input, Select, Textarea } from "@/components/ui/Field";
import { REMINDER_PRIORITIES } from "@/lib/constants";
import { toDateInputValue } from "@/lib/utils";
import type { ReminderDTO } from "@/lib/dto";

export function ReminderForm({ reminder }: { reminder?: ReminderDTO }) {
  const [state, action, pending] = useActionState(saveReminderAction, initialActionState);
  const fe = state.fieldErrors ?? {};

  return (
    <form action={action} className="space-y-4">
      {reminder ? <input type="hidden" name="id" value={reminder.id} /> : null}
      <Card className="space-y-4 p-4">
        <Field label="Reminder" htmlFor="title" error={fe.title}>
          <Input
            id="title"
            name="title"
            defaultValue={reminder?.title}
            placeholder="e.g. Order new hand wraps"
            autoFocus
            required
          />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Due" htmlFor="dueDate" error={fe.dueDate}>
            <Input
              id="dueDate"
              name="dueDate"
              type="date"
              defaultValue={toDateInputValue(reminder?.dueDate ?? new Date())}
              required
            />
          </Field>
          <Field label="Priority" htmlFor="priority">
            <Select id="priority" name="priority" defaultValue={reminder?.priority ?? "normal"}>
              {REMINDER_PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {p[0].toUpperCase() + p.slice(1)}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <Field label="Notes" htmlFor="notes">
          <Textarea id="notes" name="notes" defaultValue={reminder?.notes} placeholder="optional" />
        </Field>
      </Card>

      {fe._form ? <p className="text-sm text-overdue">{fe._form}</p> : null}

      <div className="flex gap-3">
        <Button type="submit" disabled={pending} className="flex-1">
          {pending ? "Saving…" : reminder ? "Save changes" : "Add reminder"}
        </Button>
        <Link href="/reminders" className={buttonClasses("ghost")}>
          Cancel
        </Link>
      </div>
    </form>
  );
}
