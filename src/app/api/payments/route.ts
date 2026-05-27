import { ZodError } from "zod";
import { createPayment, listPayments } from "@/lib/services/payments";
import type { PaymentType } from "@/lib/constants";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const data = await listPayments({
    traineeId: searchParams.get("traineeId") ?? undefined,
    type: (searchParams.get("type") as PaymentType | null) ?? undefined,
    from: searchParams.get("from") ? new Date(searchParams.get("from")!) : undefined,
    to: searchParams.get("to") ? new Date(searchParams.get("to")!) : undefined,
  });
  return Response.json(data);
}

export async function POST(request: Request) {
  try {
    const body = await request.json();
    const payment = await createPayment(body);
    return Response.json(payment, { status: 201 });
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}
