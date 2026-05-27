import { deletePayment } from "@/lib/services/payments";

export const runtime = "nodejs";

export async function DELETE(_req: Request, ctx: RouteContext<"/api/payments/[id]">) {
  const { id } = await ctx.params;
  const ok = await deletePayment(id);
  if (!ok) return Response.json({ error: "Not found" }, { status: 404 });
  return new Response(null, { status: 204 });
}
