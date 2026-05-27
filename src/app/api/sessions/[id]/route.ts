import { ZodError } from "zod";
import { deleteSession, getSession, updateSession } from "@/lib/services/sessions";

export const runtime = "nodejs";

export async function GET(_req: Request, ctx: RouteContext<"/api/sessions/[id]">) {
  const { id } = await ctx.params;
  const session = await getSession(id);
  if (!session) return Response.json({ error: "Not found" }, { status: 404 });
  return Response.json(session);
}

export async function PATCH(request: Request, ctx: RouteContext<"/api/sessions/[id]">) {
  const { id } = await ctx.params;
  try {
    const body = await request.json();
    const session = await updateSession(id, body);
    if (!session) return Response.json({ error: "Not found" }, { status: 404 });
    return Response.json(session);
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}

export async function DELETE(request: Request, ctx: RouteContext<"/api/sessions/[id]">) {
  const { id } = await ctx.params;
  const { searchParams } = new URL(request.url);
  const series = searchParams.get("series") === "true";
  const count = await deleteSession(id, { series });
  if (count === 0) return Response.json({ error: "Not found" }, { status: 404 });
  return Response.json({ deleted: count });
}
