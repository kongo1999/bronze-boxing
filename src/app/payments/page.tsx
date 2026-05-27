import Link from "next/link";
import { endOfMonth } from "date-fns";
import { ChevronLeft, ChevronRight, Plus, Receipt } from "lucide-react";
import {
  getMonthRevenue,
  getSubscriptionStatuses,
  listPayments,
} from "@/lib/services/payments";
import { PageHeader } from "@/components/ui/PageHeader";
import { StatTile } from "@/components/ui/StatTile";
import { Badge, type BadgeTone } from "@/components/ui/Badge";
import { Avatar } from "@/components/ui/Avatar";
import { buttonClasses } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { DeletePaymentButton } from "@/components/payments/DeletePaymentButton";
import { formatLongDate, formatMoney, monthKey, monthLabel } from "@/lib/utils";

const STATE_TONE: Record<string, BadgeTone> = {
  paid: "paid",
  partial: "partial",
  unpaid: "overdue",
};
const STATE_LABEL: Record<string, string> = {
  paid: "Paid",
  partial: "Partial",
  unpaid: "Unpaid",
};

function shiftMonth(key: string, delta: number): string {
  const [y, m] = key.split("-").map(Number);
  return monthKey(new Date(y, m - 1 + delta, 1));
}

const TYPE_LABEL: Record<string, string> = {
  subscription: "Subscription",
  private: "Private session",
  dropin: "Drop-in",
  other: "Other",
};

export default async function PaymentsPage({
  searchParams,
}: {
  searchParams: Promise<{ m?: string }>;
}) {
  const { m } = await searchParams;
  const month = m && /^\d{4}-\d{2}$/.test(m) ? m : monthKey();
  const [y, mm] = month.split("-").map(Number);
  const start = new Date(y, mm - 1, 1, 0, 0, 0, 0);
  const end = endOfMonth(start);

  const [statuses, revenue, payments] = await Promise.all([
    getSubscriptionStatuses(month),
    getMonthRevenue(month),
    listPayments({ from: start, to: end }),
  ]);

  const paidCount = statuses.filter((s) => s.state === "paid").length;
  const overdue = statuses.length - paidCount;
  const isCurrent = month === monthKey();

  return (
    <div className="space-y-4">
      <PageHeader
        eyebrow="Ledger"
        title="Money"
        action={
          <Link href={`/payments/new`} className={buttonClasses("primary", "sm")}>
            <Plus className="h-4 w-4" />
            Log
          </Link>
        }
      />

      <div className="flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
        <Link href={`/payments?m=${shiftMonth(month, -1)}`} className={buttonClasses("ghost", "icon")} aria-label="Previous month">
          <ChevronLeft className="h-5 w-5" />
        </Link>
        <div className="text-center">
          <p className="font-display font-semibold tracking-tight">{monthLabel(month)}</p>
          {isCurrent ? (
            <p className="text-xs text-faint">This month</p>
          ) : (
            <Link href="/payments" className="text-xs text-bronze">
              Back to this month
            </Link>
          )}
        </div>
        <Link href={`/payments?m=${shiftMonth(month, 1)}`} className={buttonClasses("ghost", "icon")} aria-label="Next month">
          <ChevronRight className="h-5 w-5" />
        </Link>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <StatTile label="Collected" value={formatMoney(revenue)} sub="cash this month" accent />
        <StatTile
          label="Subscriptions"
          value={`${paidCount}/${statuses.length}`}
          sub={overdue > 0 ? `${overdue} unpaid` : "all paid"}
        />
      </div>

      <section className="space-y-2">
        <h2 className="px-1 label-eyebrow text-[0.625rem] text-faint">
          Subscriptions · {monthLabel(month)}
        </h2>
        {statuses.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
            No active subscribers. Set a monthly fee on a trainee to track dues.
          </p>
        ) : (
          <ul className="space-y-2">
            {statuses.map((s) => (
              <li
                key={s.trainee.id}
                className="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
              >
                <Link href={`/trainees/${s.trainee.id}`} className="flex min-w-0 items-center gap-2.5">
                  <Avatar name={s.trainee.name} className="h-8 w-8 text-xs" />
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{s.trainee.name}</p>
                    <p className="text-xs text-faint tnum">
                      {formatMoney(s.amountPaid)} / {formatMoney(s.due)}
                    </p>
                  </div>
                </Link>
                <div className="flex shrink-0 items-center gap-2">
                  <Badge tone={STATE_TONE[s.state]}>{STATE_LABEL[s.state]}</Badge>
                  {s.state !== "paid" ? (
                    <Link
                      href={`/payments/new?trainee=${s.trainee.id}&type=subscription&periodMonth=${month}&amount=${
                        s.due - s.amountPaid
                      }`}
                      className={buttonClasses("primary", "sm")}
                    >
                      Collect
                    </Link>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="px-1 label-eyebrow text-[0.625rem] text-faint">Transactions</h2>
        {payments.length === 0 ? (
          <EmptyState
            icon={Receipt}
            title="No payments this month"
            description="Cash you collect will show up here."
          />
        ) : (
          <ul className="space-y-2">
            {payments.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{p.traineeName ?? "—"}</p>
                  <p className="truncate text-xs text-faint">
                    {TYPE_LABEL[p.type] ?? p.type}
                    {p.periodMonth ? ` · ${monthLabel(p.periodMonth)}` : ""} · {formatLongDate(p.date)}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="font-display text-sm tnum">{formatMoney(p.amount)}</span>
                  <DeletePaymentButton id={p.id} returnTo={`/payments?m=${month}`} />
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
