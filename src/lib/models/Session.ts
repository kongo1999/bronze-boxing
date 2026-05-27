import { Schema, model, models, type Model, type Types } from "mongoose";
import {
  ATTENDANCE_STATUSES,
  SESSION_STATUSES,
  SESSION_TYPES,
  type AttendanceStatus,
  type SessionStatus,
  type SessionType,
} from "../constants";

export interface IAttendee {
  trainee: Types.ObjectId;
  status: AttendanceStatus;
}

export interface ISession {
  _id: Types.ObjectId;
  title: string;
  type: SessionType;
  /** Full start datetime of this occurrence. */
  start: Date;
  durationMin: number;
  location?: string;
  /** Max attendees for group classes (undefined = unlimited). */
  capacity?: number;
  attendees: IAttendee[];
  /** Groups occurrences created together as a recurring class. */
  seriesId?: string;
  /** Price for a private session (cash); group classes bill via subscription. */
  fee?: number;
  status: SessionStatus;
  notes?: string;
  createdAt: Date;
  updatedAt: Date;
}

const AttendeeSchema = new Schema<IAttendee>(
  {
    trainee: { type: Schema.Types.ObjectId, ref: "Trainee", required: true },
    status: { type: String, enum: ATTENDANCE_STATUSES, default: "booked" },
  },
  { _id: false },
);

const SessionSchema = new Schema<ISession>(
  {
    title: { type: String, required: true, trim: true },
    type: { type: String, enum: SESSION_TYPES, required: true, index: true },
    start: { type: Date, required: true, index: true },
    durationMin: { type: Number, default: 60, min: 1 },
    location: { type: String, trim: true },
    capacity: { type: Number, min: 1 },
    attendees: { type: [AttendeeSchema], default: [] },
    seriesId: { type: String, index: true },
    fee: { type: Number, min: 0 },
    status: { type: String, enum: SESSION_STATUSES, default: "scheduled", index: true },
    notes: { type: String, trim: true },
  },
  { timestamps: true },
);

export const Session: Model<ISession> =
  (models.Session as Model<ISession>) || model<ISession>("Session", SessionSchema);
