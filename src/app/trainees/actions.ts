"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { createTrainee, deleteTrainee, updateTrainee } from "@/lib/services/trainees";
import { runAction, type ActionState } from "@/lib/form";
import type { TraineeInput } from "@/lib/validation";

function str(fd: FormData, key: string): string | undefined {
  const v = fd.get(key);
  if (typeof v !== "string") return undefined;
  const t = v.trim();
  return t === "" ? undefined : t;
}

function toInput(fd: FormData): TraineeInput {
  return {
    name: str(fd, "name") ?? "",
    phone: str(fd, "phone"),
    email: str(fd, "email"),
    status: (str(fd, "status") as TraineeInput["status"]) ?? "active",
    skillLevel: str(fd, "skillLevel") as TraineeInput["skillLevel"],
    monthlyFee: str(fd, "monthlyFee") ?? 0,
    joinedAt: str(fd, "joinedAt"),
    notes: str(fd, "notes"),
    emergencyContact: { name: str(fd, "ecName"), phone: str(fd, "ecPhone") },
  } as TraineeInput;
}

export async function saveTraineeAction(
  _prev: ActionState,
  fd: FormData,
): Promise<ActionState> {
  const id = str(fd, "id");
  const result = await runAction(async () => {
    if (id) {
      const updated = await updateTrainee(id, toInput(fd));
      if (!updated) throw new Error("Trainee not found");
    } else {
      await createTrainee(toInput(fd));
    }
  });
  if (!result.ok) return result;

  revalidatePath("/trainees");
  if (id) revalidatePath(`/trainees/${id}`);
  redirect(id ? `/trainees/${id}` : "/trainees");
}

export async function deleteTraineeAction(fd: FormData): Promise<void> {
  const id = String(fd.get("id") ?? "");
  await deleteTrainee(id);
  revalidatePath("/trainees");
  revalidatePath("/payments");
  redirect("/trainees");
}
