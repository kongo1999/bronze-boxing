"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import {
  createReminder,
  deleteReminder,
  setReminderDone,
  updateReminder,
} from "@/lib/services/reminders";
import { runAction, type ActionState } from "@/lib/form";
import type { ReminderInput } from "@/lib/validation";

function str(fd: FormData, key: string): string | undefined {
  const v = fd.get(key);
  if (typeof v !== "string") return undefined;
  const t = v.trim();
  return t === "" ? undefined : t;
}

function toInput(fd: FormData): ReminderInput {
  return {
    title: str(fd, "title") ?? "",
    notes: str(fd, "notes"),
    dueDate: str(fd, "dueDate") ?? "",
    priority: (str(fd, "priority") as ReminderInput["priority"]) ?? "normal",
  } as ReminderInput;
}

export async function saveReminderAction(
  _prev: ActionState,
  fd: FormData,
): Promise<ActionState> {
  const id = str(fd, "id");
  const result = await runAction(async () => {
    if (id) {
      const updated = await updateReminder(id, toInput(fd));
      if (!updated) throw new Error("Reminder not found");
    } else {
      await createReminder(toInput(fd));
    }
  });
  if (!result.ok) return result;

  revalidatePath("/reminders");
  revalidatePath("/");
  redirect("/reminders");
}

export async function toggleReminderAction(id: string, done: boolean): Promise<void> {
  await setReminderDone(id, done);
  revalidatePath("/reminders");
  revalidatePath("/");
}

export async function deleteReminderAction(fd: FormData): Promise<void> {
  await deleteReminder(String(fd.get("id") ?? ""));
  revalidatePath("/reminders");
  revalidatePath("/");
  redirect("/reminders");
}
