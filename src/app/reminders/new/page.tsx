import { PageHeader } from "@/components/ui/PageHeader";
import { ReminderForm } from "@/components/reminders/ReminderForm";

export default function NewReminderPage() {
  return (
    <div className="space-y-4">
      <PageHeader eyebrow="To-do" title="New reminder" />
      <ReminderForm />
    </div>
  );
}
