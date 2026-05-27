"use client";

import Link from "next/link";
import { useActionState } from "react";
import { saveTraineeAction } from "@/app/trainees/actions";
import { initialActionState } from "@/lib/form";
import { Button, buttonClasses } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field, Input, Select, Textarea } from "@/components/ui/Field";
import { SKILL_LEVELS, TRAINEE_STATUSES } from "@/lib/constants";
import { toDateInputValue } from "@/lib/utils";
import type { TraineeDTO } from "@/lib/dto";

export function TraineeForm({ trainee }: { trainee?: TraineeDTO }) {
  const [state, action, pending] = useActionState(saveTraineeAction, initialActionState);
  const fe = state.fieldErrors ?? {};
  const editing = Boolean(trainee);

  return (
    <form action={action} className="space-y-4">
      {trainee ? <input type="hidden" name="id" value={trainee.id} /> : null}

      <Card className="space-y-4 p-4">
        <Field label="Name" htmlFor="name" error={fe.name}>
          <Input
            id="name"
            name="name"
            defaultValue={trainee?.name}
            placeholder="e.g. Karim Haddad"
            autoFocus={!editing}
            required
          />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Phone" htmlFor="phone">
            <Input id="phone" name="phone" type="tel" defaultValue={trainee?.phone} placeholder="03 123 456" />
          </Field>
          <Field label="Email" htmlFor="email">
            <Input id="email" name="email" type="email" defaultValue={trainee?.email} placeholder="optional" />
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Status" htmlFor="status">
            <Select id="status" name="status" defaultValue={trainee?.status ?? "active"}>
              {TRAINEE_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {s[0].toUpperCase() + s.slice(1)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Skill level" htmlFor="skillLevel">
            <Select id="skillLevel" name="skillLevel" defaultValue={trainee?.skillLevel ?? ""}>
              <option value="">—</option>
              {SKILL_LEVELS.map((s) => (
                <option key={s} value={s}>
                  {s[0].toUpperCase() + s.slice(1)}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      </Card>

      <Card className="space-y-4 p-4">
        <p className="label-eyebrow text-[0.625rem] text-faint">Membership</p>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Monthly fee" htmlFor="monthlyFee" hint="Cash, 0 if none" error={fe.monthlyFee}>
            <Input
              id="monthlyFee"
              name="monthlyFee"
              type="number"
              min="0"
              step="1"
              inputMode="numeric"
              defaultValue={trainee?.monthlyFee || ""}
              placeholder="0"
            />
          </Field>
          <Field label="Joined" htmlFor="joinedAt">
            <Input
              id="joinedAt"
              name="joinedAt"
              type="date"
              defaultValue={toDateInputValue(trainee?.joinedAt ?? new Date())}
            />
          </Field>
        </div>
      </Card>

      <Card className="space-y-4 p-4">
        <p className="label-eyebrow text-[0.625rem] text-faint">Emergency contact</p>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name" htmlFor="ecName">
            <Input id="ecName" name="ecName" defaultValue={trainee?.emergencyContact?.name} placeholder="optional" />
          </Field>
          <Field label="Phone" htmlFor="ecPhone">
            <Input id="ecPhone" name="ecPhone" type="tel" defaultValue={trainee?.emergencyContact?.phone} placeholder="optional" />
          </Field>
        </div>
        <Field label="Notes" htmlFor="notes">
          <Textarea id="notes" name="notes" defaultValue={trainee?.notes} placeholder="Injuries, goals, anything to remember" />
        </Field>
      </Card>

      {fe._form ? <p className="text-sm text-overdue">{fe._form}</p> : null}

      <div className="flex gap-3">
        <Button type="submit" disabled={pending} className="flex-1">
          {pending ? "Saving…" : editing ? "Save changes" : "Add trainee"}
        </Button>
        <Link
          href={trainee ? `/trainees/${trainee.id}` : "/trainees"}
          className={buttonClasses("ghost")}
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}
