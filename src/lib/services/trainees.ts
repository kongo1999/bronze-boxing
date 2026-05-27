import { isValidObjectId } from "mongoose";
import { dbConnect } from "../db";
import { Payment, Session, Trainee, type ITrainee } from "../models";
import { mapTrainee, type TraineeDTO } from "../dto";
import { traineeInputSchema, type TraineeInput } from "../validation";
import type { TraineeStatus } from "../constants";

export async function listTrainees(
  opts: { status?: TraineeStatus; search?: string } = {},
): Promise<TraineeDTO[]> {
  await dbConnect();
  const query: Record<string, unknown> = {};
  if (opts.status) query.status = opts.status;
  if (opts.search?.trim()) query.name = { $regex: opts.search.trim(), $options: "i" };
  const docs = await Trainee.find(query).sort({ status: 1, name: 1 }).lean<ITrainee[]>();
  return docs.map(mapTrainee);
}

export async function getTrainee(id: string): Promise<TraineeDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const doc = await Trainee.findById(id).lean<ITrainee | null>();
  return doc ? mapTrainee(doc) : null;
}

export async function createTrainee(input: TraineeInput): Promise<TraineeDTO> {
  await dbConnect();
  const data = traineeInputSchema.parse(input);
  const doc = await Trainee.create(data);
  return mapTrainee(doc.toObject() as ITrainee);
}

export async function updateTrainee(
  id: string,
  input: TraineeInput,
): Promise<TraineeDTO | null> {
  if (!isValidObjectId(id)) return null;
  await dbConnect();
  const data = traineeInputSchema.parse(input);
  const doc = await Trainee.findByIdAndUpdate(id, data, {
    new: true,
    runValidators: true,
  }).lean<ITrainee | null>();
  return doc ? mapTrainee(doc) : null;
}

export async function deleteTrainee(id: string): Promise<boolean> {
  if (!isValidObjectId(id)) return false;
  await dbConnect();
  const res = await Trainee.findByIdAndDelete(id);
  if (!res) return false;
  // Cascade: drop their payments and remove them from any session rosters.
  await Promise.all([
    Payment.deleteMany({ trainee: id }),
    Session.updateMany({}, { $pull: { attendees: { trainee: id } } }),
  ]);
  return true;
}

export async function countActiveTrainees(): Promise<number> {
  await dbConnect();
  return Trainee.countDocuments({ status: "active" });
}
