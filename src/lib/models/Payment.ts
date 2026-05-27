import { Schema, model, models, type Model, type Types } from "mongoose";
import {
  PAYMENT_METHODS,
  PAYMENT_TYPES,
  type PaymentMethod,
  type PaymentType,
} from "../constants";

export interface IPayment {
  _id: Types.ObjectId;
  trainee: Types.ObjectId;
  amount: number;
  date: Date;
  type: PaymentType;
  method: PaymentMethod;
  /** For subscription payments: which month it covers, e.g. "2026-05". */
  periodMonth?: string;
  /** Optional link to the private session this payment covers. */
  session?: Types.ObjectId;
  note?: string;
  createdAt: Date;
  updatedAt: Date;
}

const PaymentSchema = new Schema<IPayment>(
  {
    trainee: { type: Schema.Types.ObjectId, ref: "Trainee", required: true, index: true },
    amount: { type: Number, required: true, min: 0 },
    date: { type: Date, required: true, default: () => new Date(), index: true },
    type: { type: String, enum: PAYMENT_TYPES, required: true },
    method: { type: String, enum: PAYMENT_METHODS, default: "cash" },
    periodMonth: { type: String, index: true },
    session: { type: Schema.Types.ObjectId, ref: "Session" },
    note: { type: String, trim: true },
  },
  { timestamps: true },
);

// Fast lookup of "did this trainee pay their subscription for month X?"
PaymentSchema.index({ trainee: 1, type: 1, periodMonth: 1 });

export const Payment: Model<IPayment> =
  (models.Payment as Model<IPayment>) || model<IPayment>("Payment", PaymentSchema);
