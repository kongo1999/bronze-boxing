import { ZodError } from "zod";
import { deleteTrainee, getTrainee, updateTrainee } from "@/lib/services/trainees";

export const runtime = "nodejs";

export async function GET(_req: Request, ctx: RouteContext<"/api/trainees/[id]">) {
  const { id } = await ctx.params;
  const trainee = await getTrainee(id);
  if (!trainee) return Response.json({ error: "Not found" }, { status: 404 });
  return Response.json(trainee);
}

export async function PATCH(request: Request, ctx: RouteContext<"/api/trainees/[id]">) {
  const { id } = await ctx.params;
  try {
    const body = await request.json();
    const trainee = await updateTrainee(id, body);
    if (!trainee) return Response.json({ error: "Not found" }, { status: 404 });
    return Response.json(trainee);
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}

export async function DELETE(_req: Request, ctx: RouteContext<"/api/trainees/[id]">) {
  const { id } = await ctx.params;
  const ok = await deleteTrainee(id);
  if (!ok) return Response.json({ error: "Not found" }, { status: 404 });
  return new Response(null, { status: 204 });
}
