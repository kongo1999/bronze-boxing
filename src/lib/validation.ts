/** Zod input schemas. Validate at the boundary (API routes + server actions). */
import { z } from "zod";
import {
  ATTENDANCE_STATUSES,
  PAYMENT_TYPES,
  REMINDER_PRIORITIES,
  SESSION_TYPES,
  SKILL_LEVELS,
  TRAINEE_STATUSES,
} from "./constants";

const optionalTrimmed = z
  .string()
  .trim()
  .optional()
  .transform((v) => (v === "" ? undefined : v));

export const traineeInputSchema = z.object({
  name: z.string().trim().min(1, "Name is required"),
  phone: optionalTrimmed,
  email: optionalTrimmed,
  status: z.enum(TRAINEE_STATUSES).default("active"),
  skillLevel: z.enum(SKILL_LEVELS).optional(),
  monthlyFee: z.coerce.number().min(0).default(0),
  joinedAt: z.coerce.date().optional(),
  notes: optionalTrimmed,
  emergencyContact: z
    .object({ name: optionalTrimmed, phone: optionalTrimmed })
    .optional(),
});
export type TraineeInput = z.input<typeof traineeInputSchema>;

export const sessionInputSchema = z.object({
  title: z.string().trim().min(1, "Title is required"),
  type: z.enum(SESSION_TYPES),
  start: z.coerce.date(),
  durationMin: z.coerce.number().int().min(1).default(60),
  location: optionalTrimmed,
  capacity: z.coerce.number().int().min(1).optional(),
  fee: z.coerce.number().min(0).optional(),
  notes: optionalTrimmed,
  traineeIds: z.array(z.string()).default([]),
});
export type SessionInput = z.input<typeof sessionInputSchema>;

/** Recurring class: same as a session plus the weekdays and how far ahead. */
export const recurringSessionInputSchema = sessionInputSchema.extend({
  daysOfWeek: z.array(z.coerce.number().int().min(0).max(6)).min(1),
  weeks: z.coerce.number().int().min(1).max(52).default(8),
});
export type RecurringSessionInput = z.input<typeof recurringSessionInputSchema>;

export const attendanceSchema = z.object({
  traineeId: z.string().min(1),
  status: z.enum(ATTENDANCE_STATUSES),
});

export const paymentInputSchema = z.object({
  trainee: z.string().min(1, "Trainee is required"),
  amount: z.coerce.number().positive("Amount must be greater than 0"),
  date: z.coerce.date().optional(),
  type: z.enum(PAYMENT_TYPES),
  periodMonth: z
    .string()
    .regex(/^\d{4}-\d{2}$/, "Use YYYY-MM")
    .optional(),
  session: optionalTrimmed,
  note: optionalTrimmed,
});
export type PaymentInput = z.input<typeof paymentInputSchema>;

export const reminderInputSchema = z.object({
  title: z.string().trim().min(1, "Title is required"),
  notes: optionalTrimmed,
  dueDate: z.coerce.date(),
  priority: z.enum(REMINDER_PRIORITIES).default("normal"),
  done: z.coerce.boolean().optional(),
});
export type ReminderInput = z.input<typeof reminderInputSchema>;
