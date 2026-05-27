import { PageHeader } from "@/components/ui/PageHeader";
import { TraineeForm } from "@/components/trainees/TraineeForm";

export default function NewTraineePage() {
  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Roster" title="Add trainee" />
      <TraineeForm />
    </div>
  );
}
