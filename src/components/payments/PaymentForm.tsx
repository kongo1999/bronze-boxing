"use client";

import Link from "next/link";
import { useActionState, useState } from "react";
import { createPaymentAction } from "@/app/payments/actions";
import { initialActionState } from "@/lib/form";
import { Button, buttonClasses } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field, Input, Select } from "@/components/ui/Field";
import { CURRENCY, PAYMENT_TYPES, type PaymentType } from "@/lib/constants";
import { monthKey, toDateInputValue } from "@/lib/utils";
import type { TraineeDTO } from "@/lib/dto";

const TYPE_LABELS: Record<PaymentType, string> = {
  subscription: "Monthly subscription",
  private: "Private session",
  dropin: "Drop-in",
  other: "Other",
};

export type PaymentDefaults = {
  trainee?: string;
  type?: PaymentType;
  amount?: string;
  periodMonth?: string;
  session?: string;
  returnTo?: string;
};

export function PaymentForm({
  trainees,
  defaults,
}: {
  trainees: TraineeDTO[];
  defaults?: PaymentDefaults;
}) {
  const [state, action, pending] = useActionState(createPaymentAction, initialActionState);
  const fe = state.fieldErrors ?? {};
  const [type, setType] = useState<PaymentType>(defaults?.type ?? "subscription");

  return (
    <form action={action} className="space-y-4">
      {defaults?.session ? <input type="hidden" name="session" value={defaults.session} /> : null}
      <input type="hidden" name="returnTo" value={defaults?.returnTo ?? "/payments"} />

      <Card className="space-y-4 p-4">
        <Field label="Trainee" htmlFor="trainee" error={fe.trainee}>
          <Select id="trainee" name="trainee" defaultValue={defaults?.trainee ?? ""} required>
            <option value="" disabled>
              Choose trainee…
            </option>
            {trainees.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
                {t.status === "inactive" ? " (inactive)" : ""}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="What for?" htmlFor="type">
          <Select
            id="type"
            name="type"
            value={type}
            onChange={(e) => setType(e.target.value as PaymentType)}
          >
            {PAYMENT_TYPES.map((t) => (
              <option key={t} value={t}>
                {TYPE_LABELS[t]}
              </option>
            ))}
          </Select>
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label={`Amount (${CURRENCY})`} htmlFor="amount" error={fe.amount}>
            <Input
              id="amount"
              name="amount"
              type="number"
              min="0"
              step="1"
              inputMode="numeric"
              defaultValue={defaults?.amount ?? ""}
              placeholder="120"
              autoFocus
              required
            />
          </Field>
          <Field label="Date received" htmlFor="date">
            <Input id="date" name="date" type="date" defaultValue={toDateInputValue(new Date())} />
          </Field>
        </div>

        {type === "subscription" ? (
          <Field label="For which month?" htmlFor="periodMonth" error={fe.periodMonth}>
            <Input
              id="periodMonth"
              name="periodMonth"
              type="month"
              defaultValue={defaults?.periodMonth ?? monthKey()}
            />
          </Field>
        ) : null}

        <Field label="Note" htmlFor="note">
          <Input id="note" name="note" defaultValue="" placeholder="optional" />
        </Field>
      </Card>

      <div className="rounded-xl border border-line bg-elevated/50 px-3.5 py-2.5 text-xs text-faint">
        Cash payment. Recorded against the studio ledger.
      </div>

      {fe._form ? <p className="text-sm text-overdue">{fe._form}</p> : null}

      <div className="flex gap-3">
        <Button type="submit" disabled={pending} className="flex-1">
          {pending ? "Saving…" : "Record payment"}
        </Button>
        <Link href={defaults?.returnTo ?? "/payments"} className={buttonClasses("ghost")}>
          Cancel
        </Link>
      </div>
    </form>
  );
}
