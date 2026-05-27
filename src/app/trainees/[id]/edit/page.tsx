import { notFound } from "next/navigation";
import { getTrainee } from "@/lib/services/trainees";
import { PageHeader } from "@/components/ui/PageHeader";
import { TraineeForm } from "@/components/trainees/TraineeForm";

export default async function EditTraineePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const trainee = await getTrainee(id);
  if (!trainee) notFound();

  return (
    <div className="space-y-4">
      <PageHeader eyebrow={trainee.name} title="Edit trainee" />
      <TraineeForm trainee={trainee} />
    </div>
  );
}
