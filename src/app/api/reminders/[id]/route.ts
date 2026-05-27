import { ZodError } from "zod";
import { deleteReminder, setReminderDone, updateReminder } from "@/lib/services/reminders";

export const runtime = "nodejs";

export async function PATCH(request: Request, ctx: RouteContext<"/api/reminders/[id]">) {
  const { id } = await ctx.params;
  try {
    const body = await request.json();
    // A bare { done } toggles completion; anything else is a full update.
    const reminder =
      Object.keys(body).length === 1 && typeof body.done === "boolean"
        ? await setReminderDone(id, body.done)
        : await updateReminder(id, body);
    if (!reminder) return Response.json({ error: "Not found" }, { status: 404 });
    return Response.json(reminder);
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}

export async function DELETE(_req: Request, ctx: RouteContext<"/api/reminders/[id]">) {
  const { id } = await ctx.params;
  const ok = await deleteReminder(id);
  if (!ok) return Response.json({ error: "Not found" }, { status: 404 });
  return new Response(null, { status: 204 });
}
