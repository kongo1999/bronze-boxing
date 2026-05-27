/**
 * Data Transfer Objects: JSON-safe shapes returned by the service layer.
 *
 * Mongoose documents contain ObjectIds and Dates that can't cross the React
 * Server Component -> Client Component boundary. Every service returns these
 * plain objects instead (ids as strings, dates as ISO strings).
 */
import type {
  AttendanceStatus,
  PaymentMethod,
  PaymentType,
  ReminderPriority,
  SessionStatus,
  SessionType,
  SkillLevel,
  TraineeStatus,
} from "./constants";
import type { IPayment, IReminder, ITrainee } from "./models";

export interface TraineeDTO {
  id: string;
  name: string;
  phone?: string;
  email?: string;
  status: TraineeStatus;
  skillLevel?: SkillLevel;
  monthlyFee: number;
  joinedAt: string;
  notes?: string;
  emergencyContact?: { name?: string; phone?: string };
  createdAt: string;
  updatedAt: string;
}

export interface AttendeeDTO {
  traineeId: string;
  traineeName?: string;
  status: AttendanceStatus;
}

export interface SessionDTO {
  id: string;
  title: string;
  type: SessionType;
  start: string;
  durationMin: number;
  location?: string;
  capacity?: number;
  attendees: AttendeeDTO[];
  seriesId?: string;
  fee?: number;
  status: SessionStatus;
  notes?: string;
}

export interface PaymentDTO {
  id: string;
  traineeId: string;
  traineeName?: string;
  amount: number;
  date: string;
  type: PaymentType;
  method: PaymentMethod;
  periodMonth?: string;
  sessionId?: string;
  note?: string;
}

export interface ReminderDTO {
  id: string;
  title: string;
  notes?: string;
  dueDate: string;
  done: boolean;
  priority: ReminderPriority;
}

export function mapTrainee(doc: ITrainee): TraineeDTO {
  return {
    id: String(doc._id),
    name: doc.name,
    phone: doc.phone || undefined,
    email: doc.email || undefined,
    status: doc.status,
    skillLevel: doc.skillLevel || undefined,
    monthlyFee: doc.monthlyFee ?? 0,
    joinedAt: doc.joinedAt.toISOString(),
    notes: doc.notes || undefined,
    emergencyContact:
      doc.emergencyContact?.name || doc.emergencyContact?.phone
        ? doc.emergencyContact
        : undefined,
    createdAt: doc.createdAt.toISOString(),
    updatedAt: doc.updatedAt.toISOString(),
  };
}

export function mapReminder(doc: IReminder): ReminderDTO {
  return {
    id: String(doc._id),
    title: doc.title,
    notes: doc.notes || undefined,
    dueDate: doc.dueDate.toISOString(),
    done: doc.done,
    priority: doc.priority,
  };
}

/** Payment doc whose `trainee` may be populated with a name. */
type PaymentLean = Omit<IPayment, "trainee" | "session"> & {
  trainee: { _id: unknown; name?: string } | unknown;
  session?: unknown;
};

export function mapPayment(doc: PaymentLean): PaymentDTO {
  const traineeRef = doc.trainee as { _id?: unknown; name?: string } | null;
  const populated =
    traineeRef && typeof traineeRef === "object" && "name" in traineeRef;
  return {
    id: String((doc as { _id: unknown })._id),
    traineeId: String(populated ? traineeRef!._id : doc.trainee),
    traineeName: populated ? traineeRef!.name : undefined,
    amount: doc.amount,
    date: doc.date.toISOString(),
    type: doc.type,
    method: doc.method,
    periodMonth: doc.periodMonth || undefined,
    sessionId: doc.session ? String(doc.session) : undefined,
    note: doc.note || undefined,
  };
}
