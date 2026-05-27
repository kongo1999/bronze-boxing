import { ZodError } from "zod";
import { createSession, listSessions } from "@/lib/services/sessions";
import type { SessionType } from "@/lib/constants";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const from = searchParams.get("from");
  const to = searchParams.get("to");
  const type = searchParams.get("type") as SessionType | null;
  const data = await listSessions({
    from: from ? new Date(from) : undefined,
    to: to ? new Date(to) : undefined,
    type: type ?? undefined,
  });
  return Response.json(data);
}

export async function POST(request: Request) {
  try {
    const body = await request.json();
    const session = await createSession(body);
    return Response.json(session, { status: 201 });
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}
