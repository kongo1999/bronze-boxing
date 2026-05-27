/** App-wide constants and small configuration knobs. */

export const APP_NAME = "Bronze Boxing";
export const APP_TAGLINE = "Studio command center";

/** Currency symbol used across the ledger UI. Cash-only for now. */
export const CURRENCY = process.env.NEXT_PUBLIC_CURRENCY ?? "$";

export const SESSION_TYPES = ["group", "private"] as const;
export type SessionType = (typeof SESSION_TYPES)[number];

export const SESSION_STATUSES = ["scheduled", "completed", "cancelled"] as const;
export type SessionStatus = (typeof SESSION_STATUSES)[number];

export const ATTENDANCE_STATUSES = ["booked", "attended", "no_show"] as const;
export type AttendanceStatus = (typeof ATTENDANCE_STATUSES)[number];

export const TRAINEE_STATUSES = ["active", "inactive"] as const;
export type TraineeStatus = (typeof TRAINEE_STATUSES)[number];

export const SKILL_LEVELS = ["beginner", "intermediate", "advanced"] as const;
export type SkillLevel = (typeof SKILL_LEVELS)[number];

export const PAYMENT_TYPES = ["subscription", "private", "dropin", "other"] as const;
export type PaymentType = (typeof PAYMENT_TYPES)[number];

export const PAYMENT_METHODS = ["cash"] as const;
export type PaymentMethod = (typeof PAYMENT_METHODS)[number];

export const REMINDER_PRIORITIES = ["low", "normal", "high"] as const;
export type ReminderPriority = (typeof REMINDER_PRIORITIES)[number];
