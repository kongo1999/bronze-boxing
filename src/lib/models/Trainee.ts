import { Schema, model, models, type Model, type Types } from "mongoose";
import {
  SKILL_LEVELS,
  TRAINEE_STATUSES,
  type SkillLevel,
  type TraineeStatus,
} from "../constants";

export interface ITrainee {
  _id: Types.ObjectId;
  name: string;
  phone?: string;
  email?: string;
  status: TraineeStatus;
  skillLevel?: SkillLevel;
  /** Monthly subscription amount owed (cash). 0 = no recurring subscription. */
  monthlyFee: number;
  joinedAt: Date;
  notes?: string;
  emergencyContact?: { name?: string; phone?: string };
  /** Reserved for v2: links a trainee record to a real login account. */
  userId?: Types.ObjectId;
  createdAt: Date;
  updatedAt: Date;
}

const TraineeSchema = new Schema<ITrainee>(
  {
    name: { type: String, required: true, trim: true },
    phone: { type: String, trim: true },
    email: { type: String, trim: true, lowercase: true },
    status: { type: String, enum: TRAINEE_STATUSES, default: "active", index: true },
    skillLevel: { type: String, enum: SKILL_LEVELS },
    monthlyFee: { type: Number, default: 0, min: 0 },
    joinedAt: { type: Date, default: () => new Date() },
    notes: { type: String, trim: true },
    emergencyContact: {
      name: { type: String, trim: true },
      phone: { type: String, trim: true },
    },
    userId: { type: Schema.Types.ObjectId, ref: "User" },
  },
  { timestamps: true },
);

export const Trainee: Model<ITrainee> =
  (models.Trainee as Model<ITrainee>) || model<ITrainee>("Trainee", TraineeSchema);
