import { isValidObjectId, Types } from "mongoose";
import { endOfMonth } from "date-fns";
import { dbConnect } from "../db";
import { Payment, Trainee, type IPayment, type ITrainee } from "../models";
import { mapPayment, mapTrainee, type PaymentDTO, type TraineeDTO } from "../dto";
import { paymentInputSchema, type PaymentInput } from "../validation";
import { monthKey } from "../utils";
import type { PaymentType } from "../constants";

export type SubscriptionState = "paid" | "partial" | "unpaid";

export interface SubscriptionStatus {
  trainee: TraineeDTO;
  due: number;
  amountPaid: number;
  state: SubscriptionState;
}

function monthRange(month: string): { start: Date; end: Date } {
  const [year, m] = month.split("-").map(Number);
  const start = new Date(year, m - 1, 1, 0, 0, 0, 0);
  return { start, end: endOfMonth(start) };
}

export async function listPayments(
  opts: { traineeId?: string; type?: PaymentType; from?: Date; to?: Date } = {},
): Promise<PaymentDTO[]> {
  await dbConnect();
  const query: Record<string, unknown> = {};
  if (opts.traineeId && isValidObjectId(opts.traineeId)) query.trainee = opts.traineeId;
  if (opts.type) query.type = opts.type;
  if (opts.from || opts.to) {
    query.date = {};
    if (opts.from) (query.date as Record<string, Date>).$gte = opts.from;
    if (opts.to) (query.date as Record<string, Date>).$lte = opts.to;
  }
  const docs = await Payment.find(query)
    .sort({ date: -1 })
    .populate("trainee", "name")
    .lean();
  return docs.map((d) => mapPayment(d as never));
}

export async function createPayment(input: PaymentInput): Promise<PaymentDTO> {
  await dbConnect();
  const data = paymentInputSchema.parse(input);
  // Subscription payments default their period to the month they were received.
  if (data.type === "subscription" && !data.periodMonth) {
    data.periodMonth = monthKey(data.date ?? new Date());
  }
  const doc = await Payment.create(data);
  return mapPayment(doc.toObject() as never);
}

export async function deletePayment(id: string): Promise<boolean> {
  if (!isValidObjectId(id)) return false;
  await dbConnect();
  const res = await Payment.findByIdAndDelete(id);
  return res !== null;
}

/** Total cash received within a given YYYY-MM month. */
export async function getMonthRevenue(month: string): Promise<number> {
  await dbConnect();
  const { start, end } = monthRange(month);
  const res = await Payment.aggregate<{ total: number }>([
    { $match: { date: { $gte: start, $lte: end } } },
    { $group: { _id: null, total: { $sum: "$amount" } } },
  ]);
  return res[0]?.total ?? 0;
}

/** Subscription paid/partial/unpaid state per active subscriber for a month. */
export async function getSubscriptionStatuses(
  month: string,
): Promise<SubscriptionStatus[]> {
  await dbConnect();
  const trainees = await Trainee.find({ status: "active", monthlyFee: { $gt: 0 } })
    .sort({ name: 1 })
    .lean<ITrainee[]>();
  const paid = await Payment.find({ type: "subscription", periodMonth: month }).lean<
    IPayment[]
  >();

  const paidByTrainee = new Map<string, number>();
  for (const p of paid) {
    const key = String(p.trainee);
    paidByTrainee.set(key, (paidByTrainee.get(key) ?? 0) + p.amount);
  }

  return trainees.map((t) => {
    const due = t.monthlyFee;
    const amountPaid = paidByTrainee.get(String(t._id)) ?? 0;
    const state: SubscriptionState =
      amountPaid >= due ? "paid" : amountPaid > 0 ? "partial" : "unpaid";
    return { trainee: mapTrainee(t), due, amountPaid, state };
  });
}

export async function countOverdueSubscriptions(month: string): Promise<number> {
  const statuses = await getSubscriptionStatuses(month);
  return statuses.filter((s) => s.state !== "paid").length;
}

export async function getTraineeTotalPaid(traineeId: string): Promise<number> {
  if (!isValidObjectId(traineeId)) return 0;
  await dbConnect();
  const res = await Payment.aggregate<{ total: number }>([
    { $match: { trainee: new Types.ObjectId(traineeId) } },
    { $group: { _id: null, total: { $sum: "$amount" } } },
  ]);
  return res[0]?.total ?? 0;
}
