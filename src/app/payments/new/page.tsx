import { listTrainees } from "@/lib/services/trainees";
import { PageHeader } from "@/components/ui/PageHeader";
import { PaymentForm, type PaymentDefaults } from "@/components/payments/PaymentForm";
import { PAYMENT_TYPES, type PaymentType } from "@/lib/constants";

export default async function NewPaymentPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | undefined>>;
}) {
  const sp = await searchParams;
  const trainees = await listTrainees();

  const type =
    sp.type && PAYMENT_TYPES.includes(sp.type as PaymentType)
      ? (sp.type as PaymentType)
      : undefined;

  const returnTo = sp.session
    ? `/schedule/${sp.session}`
    : sp.trainee
      ? `/trainees/${sp.trainee}`
      : "/payments";

  const defaults: PaymentDefaults = {
    trainee: sp.trainee,
    type,
    amount: sp.amount,
    periodMonth: sp.periodMonth,
    session: sp.session,
    returnTo,
  };

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Ledger" title="Log payment" />
      <PaymentForm trainees={trainees} defaults={defaults} />
    </div>
  );
}
