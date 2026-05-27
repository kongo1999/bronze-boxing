"use client";

import { useTransition } from "react";
import { Ban, Check, RotateCcw, Trash2 } from "lucide-react";
import { deleteSessionAction, setSessionStatusAction } from "@/app/schedule/actions";
import { Button } from "@/components/ui/Button";
import type { SessionStatus } from "@/lib/constants";

export function SessionControls({
  id,
  status,
  seriesId,
}: {
  id: string;
  status: SessionStatus;
  seriesId?: string;
}) {
  const [pending, start] = useTransition();

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {status !== "completed" ? (
          <Button
            variant="subtle"
            size="sm"
            disabled={pending}
            onClick={() => start(() => setSessionStatusAction(id, "completed"))}
          >
            <Check className="h-4 w-4" />
            Mark done
          </Button>
        ) : null}
        {status !== "cancelled" ? (
          <Button
            variant="subtle"
            size="sm"
            disabled={pending}
            onClick={() => start(() => setSessionStatusAction(id, "cancelled"))}
          >
            <Ban className="h-4 w-4" />
            Cancel
          </Button>
        ) : null}
        {status !== "scheduled" ? (
          <Button
            variant="subtle"
            size="sm"
            disabled={pending}
            onClick={() => start(() => setSessionStatusAction(id, "scheduled"))}
          >
            <RotateCcw className="h-4 w-4" />
            Reopen
          </Button>
        ) : null}
      </div>

      <div className="flex flex-wrap gap-2">
        <form
          action={deleteSessionAction}
          onSubmit={(e) => {
            if (!confirm("Delete this session?")) e.preventDefault();
          }}
        >
          <input type="hidden" name="id" value={id} />
          <Button type="submit" variant="danger" size="sm">
            <Trash2 className="h-4 w-4" />
            Delete
          </Button>
        </form>
        {seriesId ? (
          <form
            action={deleteSessionAction}
            onSubmit={(e) => {
              if (!confirm("Delete the entire recurring series? This removes all its sessions."))
                e.preventDefault();
            }}
          >
            <input type="hidden" name="id" value={id} />
            <input type="hidden" name="series" value="on" />
            <Button type="submit" variant="danger" size="sm">
              <Trash2 className="h-4 w-4" />
              Delete series
            </Button>
          </form>
        ) : null}
      </div>
    </div>
  );
}
