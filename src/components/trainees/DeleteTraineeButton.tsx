"use client";

import { Trash2 } from "lucide-react";
import { deleteTraineeAction } from "@/app/trainees/actions";
import { Button } from "@/components/ui/Button";

export function DeleteTraineeButton({ id, name }: { id: string; name: string }) {
  return (
    <form
      action={deleteTraineeAction}
      onSubmit={(e) => {
        if (!confirm(`Delete ${name}? This also removes their payment history.`)) {
          e.preventDefault();
        }
      }}
    >
      <input type="hidden" name="id" value={id} />
      <Button type="submit" variant="danger" size="sm">
        <Trash2 className="h-4 w-4" />
        Delete trainee
      </Button>
    </form>
  );
}
