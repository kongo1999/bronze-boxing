import { ZodError } from "zod";
import { createReminder, listReminders } from "@/lib/services/reminders";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const data = await listReminders({
    includeDone: searchParams.get("includeDone") === "true",
    from: searchParams.get("from") ? new Date(searchParams.get("from")!) : undefined,
    to: searchParams.get("to") ? new Date(searchParams.get("to")!) : undefined,
  });
  return Response.json(data);
}

export async function POST(request: Request) {
  try {
    const body = await request.json();
    const reminder = await createReminder(body);
    return Response.json(reminder, { status: 201 });
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}
