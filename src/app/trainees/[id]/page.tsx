import Link from "next/link";
import { notFound } from "next/navigation";
import { Mail, Pencil, Phone, Plus } from "lucide-react";
import { getTrainee } from "@/lib/services/trainees";
import { getTraineeTotalPaid, listPayments } from "@/lib/services/payments";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { StatTile } from "@/components/ui/StatTile";
import { Badge, type BadgeTone } from "@/components/ui/Badge";
import { Avatar } from "@/components/ui/Avatar";
import { buttonClasses } from "@/components/ui/Button";
import { DeleteTraineeButton } from "@/components/trainees/DeleteTraineeButton";
import { formatLongDate, formatMoney, monthKey, monthLabel } from "@/lib/utils";

const SUB_TONE: Record<string, BadgeTone> = {
  paid: "paid",
  partial: "partial",
  unpaid: "overdue",
};

export default async function TraineeProfile({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const trainee = await getTrainee(id);
  if (!trainee) notFound();

  const [payments, totalPaid] = await Promise.all([
    listPayments({ traineeId: id }),
    getTraineeTotalPaid(id),
  ]);

  const month = monthKey();
  const paidThisMonth = payments
    .filter((p) => p.type === "subscription" && p.periodMonth === month)
    .reduce((sum, p) => sum + p.amount, 0);
  const subState =
    trainee.monthlyFee <= 0
      ? null
      : paidThisMonth >= trainee.monthlyFee
        ? "paid"
        : paidThisMonth > 0
          ? "partial"
          : "unpaid";

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Trainee"
        title={trainee.name}
        action={
          <Link href={`/trainees/${id}/edit`} className={buttonClasses("ghost", "sm")}>
            <Pencil className="h-4 w-4" />
            Edit
          </Link>
        }
      />

      <div className="flex items-center gap-3">
        <Avatar name={trainee.name} className="h-14 w-14 text-lg" />
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone={trainee.status === "active" ? "paid" : "neutral"}>
            {trainee.status === "active" ? "Active" : "Inactive"}
          </Badge>
          {trainee.skillLevel ? (
            <Badge tone="bronze">
              {trainee.skillLevel[0].toUpperCase() + trainee.skillLevel.slice(1)}
            </Badge>
          ) : null}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <StatTile label="Total paid" value={formatMoney(totalPaid)} sub="all time" />
        <StatTile
          label="Monthly fee"
          value={trainee.monthlyFee > 0 ? formatMoney(trainee.monthlyFee) : "—"}
          sub={trainee.monthlyFee > 0 ? "cash / month" : "no subscription"}
        />
      </div>

      {subState ? (
        <Card className="flex items-center justify-between gap-3 p-4">
          <div>
            <p className="label-eyebrow text-[0.625rem] text-faint">{monthLabel(month)}</p>
            <div className="mt-1 flex items-center gap-2">
              <Badge tone={SUB_TONE[subState]}>
                {subState === "paid" ? "Paid" : subState === "partial" ? "Partial" : "Unpaid"}
              </Badge>
              {subState !== "unpaid" ? (
                <span className="text-sm text-muted tnum">
                  {formatMoney(paidThisMonth)} of {formatMoney(trainee.monthlyFee)}
                </span>
              ) : null}
            </div>
          </div>
          {subState !== "paid" ? (
            <Link
              href={`/payments/new?trainee=${id}&type=subscription`}
              className={buttonClasses("primary", "sm")}
            >
              <Plus className="h-4 w-4" />
              Collect
            </Link>
          ) : null}
        </Card>
      ) : null}

      <Card className="divide-y divide-line p-0">
        <DetailRow
          label="Phone"
          value={
            trainee.phone ? (
              <a href={`tel:${trainee.phone}`} className="inline-flex items-center gap-1.5 text-bronze">
                <Phone className="h-3.5 w-3.5" />
                {trainee.phone}
              </a>
            ) : undefined
          }
        />
        <DetailRow
          label="Email"
          value={
            trainee.email ? (
              <a href={`mailto:${trainee.email}`} className="inline-flex items-center gap-1.5 text-bronze">
                <Mail className="h-3.5 w-3.5" />
                {trainee.email}
              </a>
            ) : undefined
          }
        />
        <DetailRow label="Joined" value={formatLongDate(trainee.joinedAt)} />
        {trainee.emergencyContact?.name || trainee.emergencyContact?.phone ? (
          <DetailRow
            label="Emergency"
            value={`${trainee.emergencyContact.name ?? ""}${
              trainee.emergencyContact.phone ? ` · ${trainee.emergencyContact.phone}` : ""
            }`}
          />
        ) : null}
        {trainee.notes ? <DetailRow label="Notes" value={trainee.notes} /> : null}
      </Card>

      <section className="space-y-2">
        <div className="flex items-center justify-between px-1">
          <h2 className="label-eyebrow text-[0.625rem] text-faint">Payment history</h2>
          <Link
            href={`/payments/new?trainee=${id}`}
            className="text-sm font-medium text-bronze"
          >
            Log payment
          </Link>
        </div>
        {payments.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
            No payments recorded yet.
          </p>
        ) : (
          <ul className="space-y-2">
            {payments.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between rounded-xl border border-line bg-surface px-3.5 py-2.5"
              >
                <div>
                  <p className="text-sm font-medium capitalize">{p.type}</p>
                  <p className="text-xs text-faint">
                    {formatLongDate(p.date)}
                    {p.periodMonth ? ` · ${monthLabel(p.periodMonth)}` : ""}
                  </p>
                </div>
                <span className="font-display text-sm tnum">{formatMoney(p.amount)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="pt-2">
        <DeleteTraineeButton id={id} name={trainee.name} />
      </div>
    </div>
  );
}



function DetailRow({ label, value }: { label: string; value?: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <span className="text-sm text-faint">{label}</span>
      <span className="text-right text-sm">{value ?? <span className="text-faint">—</span>}</span>
    </div>
  );
}