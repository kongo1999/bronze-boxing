import { isValidObjectId } from "mongoose";
import { dbConnect } from "../db";
import { Reminder, type IReminder } from "../models";
import { mapReminder, type ReminderDTO } from "../dto";
import { reminderInputSchema, type ReminderInput } from "../validation";

export async function listReminders(
  opts: { from?: Date; to?: Date; includeDone?: boolean } = {},
): Promise<ReminderDTO[]> {
  await dbConnect();
  const query: Record<string, unknown> = {};
  if (opts.from || opts.to) {
    query.dueDate = {};
    if (opts.from) (query.dueDate as Record<string, Date>).$gte = opts.from;
    if (opts.to) (query.dueDate as Record<string, Date>).$lte = opts.to;
  }
  if (!opts.includeDone) query.done = false;
  const docs = await Reminder.find(query).sort({ dueDate: 1 }).lean<IReminder[]>();
  return docs.map(mapReminder);
}

export async function createReminder(input: ReminderInput): Promise<ReminderDTO> {
  await dbConnect();
  const data = reminderInputSchema.parse(input);
  const doc = await Reminder.create(data);
  return mapReminder(doc.toObject() as IReminder);
}

export async function updateReminder(
  id: string,
  input: ReminderInput,
): Promise<ReminderDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const data = reminderInputSchema.parse(input);
  const doc = await Reminder.findByIdAndUpdate(id, data, {
    new: true,
    runValidators: true,
  }).lean<IReminder | null>();
  return doc ? mapReminder(doc) : null;
}

export async function setReminderDone(
  id: string,
  done: boolean,
): Promise<ReminderDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const doc = await Reminder.findByIdAndUpdate(
    id,
    { done },
    { new: true },
  ).lean<IReminder | null>();
  return doc ? mapReminder(doc) : null;
}

export async function deleteReminder(id: string): Promise<boolean> {
  if (!isValidObjectId(id)) return false;
  await dbConnect();
  const res = await Reminder.findByIdAndDelete(id);
  return res !== null;
}
