"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import {
  createRecurringSessions,
  createSession,
  deleteSession,
  setAttendance,
  setSessionStatus,
  updateSession,
} from "@/lib/services/sessions";
import { runAction, type ActionState } from "@/lib/form";
import type { AttendanceStatus, SessionStatus } from "@/lib/constants";
import type { SessionInput } from "@/lib/validation";

function str(fd: FormData, key: string): string | undefined {
  const v = fd.get(key);
  if (typeof v !== "string") return undefined;
  const t = v.trim();
  return t === "" ? undefined : t;
}

function toInput(fd: FormData): SessionInput {
  return {
    title: str(fd, "title") ?? "",
    type: (str(fd, "type") as SessionInput["type"]) ?? "group",
    start: str(fd, "start") ?? "",
    durationMin: str(fd, "durationMin") ?? 60,
    location: str(fd, "location"),
    capacity: str(fd, "capacity"),
    fee: str(fd, "fee"),
    notes: str(fd, "notes"),
    traineeIds: fd.getAll("traineeIds").map(String),
  } as SessionInput;
}

export async function saveSessionAction(
  _prev: ActionState,
  fd: FormData,
): Promise<ActionState> {
  const id = str(fd, "id");
  const repeat = fd.get("repeat") === "on" && !id;

  const result = await runAction(async () => {
    if (id) {
      const updated = await updateSession(id, toInput(fd));
      if (!updated) throw new Error("Session not found");
    } else if (repeat) {
      await createRecurringSessions({
        ...toInput(fd),
        daysOfWeek: fd.getAll("daysOfWeek").map(String),
        weeks: str(fd, "weeks") ?? 8,
      } as never);
    } else {
      await createSession(toInput(fd));
    }
  });
  if (!result.ok) return result;

  revalidatePath("/schedule");
  if (id) revalidatePath(`/schedule/${id}`);
  redirect(id ? `/schedule/${id}` : "/schedule");
}

export async function deleteSessionAction(fd: FormData): Promise<void> {
  const id = String(fd.get("id") ?? "");
  const series = fd.get("series") === "on";
  await deleteSession(id, { series });
  revalidatePath("/schedule");
  redirect("/schedule");
}

export async function setAttendanceAction(
  sessionId: string,
  traineeId: string,
  status: AttendanceStatus,
): Promise<void> {
  await setAttendance(sessionId, traineeId, status);
  revalidatePath(`/schedule/${sessionId}`);
}

export async function setSessionStatusAction(
  id: string,
  status: SessionStatus,
): Promise<void> {
  await setSessionStatus(id, status);
  revalidatePath(`/schedule/${id}`);
  revalidatePath("/schedule");
}
