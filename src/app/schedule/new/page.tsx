import { listTrainees } from "@/lib/services/trainees";
import { PageHeader } from "@/components/ui/PageHeader";
import { SessionForm } from "@/components/schedule/SessionForm";

export const dynamic = "force-dynamic";

export default async function NewSessionPage() {
  const trainees = await listTrainees();
  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Schedule" title="New session" />
      <SessionForm trainees={trainees} />
    </div>
  );
}
