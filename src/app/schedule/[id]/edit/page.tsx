import { notFound } from "next/navigation";
import { getSession } from "@/lib/services/sessions";
import { listTrainees } from "@/lib/services/trainees";
import { PageHeader } from "@/components/ui/PageHeader";
import { SessionForm } from "@/components/schedule/SessionForm";

export default async function EditSessionPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const [session, trainees] = await Promise.all([getSession(id), listTrainees()]);
  if (!session) notFound();

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Schedule" title="Edit session" />
      <SessionForm session={session} trainees={trainees} />
    </div>
  );
}
