"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { createPayment, deletePayment } from "@/lib/services/payments";
import { runAction, type ActionState } from "@/lib/form";
import type { PaymentInput } from "@/lib/validation";

function str(fd: FormData, key: string): string | undefined {
  const v = fd.get(key);
  if (typeof v !== "string") return undefined;
  const t = v.trim();
  return t === "" ? undefined : t;
}

function toInput(fd: FormData): PaymentInput {
  return {
    trainee: str(fd, "trainee") ?? "",
    amount: str(fd, "amount") ?? "",
    date: str(fd, "date"),
    type: (str(fd, "type") as PaymentInput["type"]) ?? "subscription",
    periodMonth: str(fd, "periodMonth"),
    session: str(fd, "session"),
    note: str(fd, "note"),
  } as PaymentInput;
}

export async function createPaymentAction(
  _prev: ActionState,
  fd: FormData,
): Promise<ActionState> {
  const returnTo = str(fd, "returnTo") ?? "/payments";
  const result = await runAction(async () => {
    await createPayment(toInput(fd));
  });
  if (!result.ok) return result;

  revalidatePath("/payments");
  revalidatePath("/");
  const trainee = str(fd, "trainee");
  if (trainee) revalidatePath(`/trainees/${trainee}`);
  redirect(returnTo);
}

export async function deletePaymentAction(fd: FormData): Promise<void> {
  const id = String(fd.get("id") ?? "");
  await deletePayment(id);
  revalidatePath("/payments");
  revalidatePath("/");
  redirect(String(fd.get("returnTo") ?? "/payments"));
}
