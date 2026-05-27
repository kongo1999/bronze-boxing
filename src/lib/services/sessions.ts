import { randomUUID } from "crypto";
import { isValidObjectId } from "mongoose";
import {
  addDays,
  setHours,
  setMilliseconds,
  setMinutes,
  setSeconds,
  startOfDay,
  startOfWeek,
} from "date-fns";
import { dbConnect } from "../db";
import { Session, type ISession } from "../models";
import type { AttendeeDTO, SessionDTO } from "../dto";
import {
  recurringSessionInputSchema,
  sessionInputSchema,
  type RecurringSessionInput,
  type SessionInput,
} from "../validation";
import type { AttendanceStatus, SessionStatus, SessionType } from "../constants";

type RawAttendee = { trainee: unknown; status: AttendanceStatus };
type SessionLean = Omit<ISession, "attendees"> & { attendees: RawAttendee[] };

function mapAttendee(a: RawAttendee): AttendeeDTO {
  const t = a.trainee as { _id?: unknown; name?: string } | null;
  const populated = !!t && typeof t === "object" && "name" in t;
  return {
    traineeId: String(populated ? t!._id : a.trainee),
    traineeName: populated ? t!.name : undefined,
    status: a.status,
  };
}

function mapSession(doc: SessionLean): SessionDTO {
  return {
    id: String(doc._id),
    title: doc.title,
    type: doc.type,
    start: doc.start.toISOString(),
    durationMin: doc.durationMin,
    location: doc.location || undefined,
    capacity: doc.capacity ?? undefined,
    attendees: (doc.attendees ?? []).map(mapAttendee),
    seriesId: doc.seriesId || undefined,
    fee: doc.fee ?? undefined,
    status: doc.status,
    notes: doc.notes || undefined,
  };
}

export async function listSessions(
  opts: { from?: Date; to?: Date; type?: SessionType } = {},
): Promise<SessionDTO[]> {
  await dbConnect();
  const query: Record<string, unknown> = {};
  if (opts.from || opts.to) {
    query.start = {};
    if (opts.from) (query.start as Record<string, Date>).$gte = opts.from;
    if (opts.to) (query.start as Record<string, Date>).$lte = opts.to;
  }
  if (opts.type) query.type = opts.type;
  const docs = await Session.find(query)
    .sort({ start: 1 })
    .populate("attendees.trainee", "name")
    .lean();
  return docs.map((d) => mapSession(d as never));
}

export async function getSession(id: string): Promise<SessionDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const doc = await Session.findById(id).populate("attendees.trainee", "name").lean();
  return doc ? mapSession(doc as never) : null;
}

function attendeesFrom(traineeIds: string[]): { trainee: string; status: AttendanceStatus }[] {
  return [...new Set(traineeIds)]
    .filter(isValidObjectId)
    .map((trainee) => ({ trainee, status: "booked" as const }));
}

export async function createSession(input: SessionInput): Promise<SessionDTO> {
  await dbConnect();
  const { traineeIds, ...data } = sessionInputSchema.parse(input);
  const doc = await Session.create({ ...data, attendees: attendeesFrom(traineeIds) });
  return mapSession(doc.toObject() as never);
}

/** Create one occurrence per selected weekday, for N weeks ahead. */
export async function createRecurringSessions(
  input: RecurringSessionInput,
): Promise<{ seriesId: string; count: number }> {
  await dbConnect();
  const { traineeIds, daysOfWeek, weeks, ...data } = recurringSessionInputSchema.parse(input);
  const seriesId = randomUUID();
  const attendees = attendeesFrom(traineeIds);
  const weekStart = startOfWeek(data.start, { weekStartsOn: 1 });
  const minDate = startOfDay(data.start);

  const docs: Record<string, unknown>[] = [];
  for (let w = 0; w < weeks; w++) {
    for (const dow of daysOfWeek) {
      const offsetFromMonday = (dow + 6) % 7;
      let date = addDays(weekStart, w * 7 + offsetFromMonday);
      date = setMilliseconds(
        setSeconds(setMinutes(setHours(date, data.start.getHours()), data.start.getMinutes()), 0),
        0,
      );
      if (date < minDate) continue;
      docs.push({ ...data, start: date, attendees, seriesId, status: "scheduled" });
    }
  }
  const created = await Session.insertMany(docs);
  return { seriesId, count: created.length };
}

export async function updateSession(
  id: string,
  input: SessionInput,
): Promise<SessionDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const existing = await Session.findById(id);
  if (!existing) return null;

  const { traineeIds, ...data } = sessionInputSchema.parse(input);
  // Preserve attendance marks for trainees who remain on the roster.
  const prev = new Map(existing.attendees.map((a) => [String(a.trainee), a.status]));
  const attendees = attendeesFrom(traineeIds).map((a) => ({
    trainee: a.trainee,
    status: prev.get(a.trainee) ?? "booked",
  }));

  existing.set({ ...data, attendees });
  await existing.save();
  return getSession(id);
}

export async function setSessionStatus(
  id: string,
  status: SessionStatus,
): Promise<SessionDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const doc = await Session.findByIdAndUpdate(id, { status }, { new: true })
    .populate("attendees.trainee", "name")
    .lean();
  return doc ? mapSession(doc as never) : null;
}

export async function setAttendance(
  sessionId: string,
  traineeId: string,
  status: AttendanceStatus,
): Promise<SessionDTO | null> {
  if (!isValidObjectId(sessionId) || !isValidObjectId(traineeId)) return null;
  await dbConnect();
  const session = await Session.findById(sessionId);
  if (!session) return null;
  const attendee = session.attendees.find((a) => String(a.trainee) === traineeId);
  if (attendee) {
    attendee.status = status;
  } else {
    session.attendees.push({ trainee: traineeId as never, status });
  }
  await session.save();
  return getSession(sessionId);
}

export async function deleteSession(
  id: string,
  opts: { series?: boolean } = {},
): Promise<number> {
  if (!isValidObjectId(id)) return 0;
  await dbConnect();
  const doc = await Session.findById(id).lean<ISession | null>();
  if (!doc) return 0;
  if (opts.series && doc.seriesId) {
    const res = await Session.deleteMany({ seriesId: doc.seriesId });
    return res.deletedCount ?? 0;
  }
  await Session.findByIdAndDelete(id);
  return 1;
}
