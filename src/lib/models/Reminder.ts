import { Schema, model, models, type Model, type Types } from "mongoose";
import { REMINDER_PRIORITIES, type ReminderPriority } from "../constants";

export interface IReminder {
  _id: Types.ObjectId;
  title: string;
  notes?: string;
  dueDate: Date;
  done: boolean;
  priority: ReminderPriority;
  createdAt: Date;
  updatedAt: Date;
}

const ReminderSchema = new Schema<IReminder>(
  {
    title: { type: String, required: true, trim: true },
    notes: { type: String, trim: true },
    dueDate: { type: Date, required: true, index: true },
    done: { type: Boolean, default: false, index: true },
    priority: { type: String, enum: REMINDER_PRIORITIES, default: "normal" },
  },
  { timestamps: true },
);

export const Reminder: Model<IReminder> =
  (models.Reminder as Model<IReminder>) || model<IReminder>("Reminder", ReminderSchema);
