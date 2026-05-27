import { ZodError } from "zod";
import { createTrainee, listTrainees } from "@/lib/services/trainees";
import type { TraineeStatus } from "@/lib/constants";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const status = searchParams.get("status") as TraineeStatus | null;
  const q = searchParams.get("q");
  const data = await listTrainees({
    status: status ?? undefined,
    search: q ?? undefined,
  });
  return Response.json(data);
}

export async function POST(request: Request) {
  try {
    const body = await request.json();
    const trainee = await createTrainee(body);
    return Response.json(trainee, { status: 201 });
  } catch (err) {
    if (err instanceof ZodError) {
      return Response.json({ error: "Validation failed", issues: err.issues }, { status: 400 });
    }
    throw err;
  }
}
