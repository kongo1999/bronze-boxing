// Mirrors the Go API JSON shapes (see backend/internal/models).

export type TraineeStatus = "active" | "inactive";
export type SessionType = "group" | "private";
export type SessionStatus = "scheduled" | "completed" | "cancelled";
export type AttendanceStatus = "booked" | "attended" | "no_show";
export type SubState = "paid" | "partial" | "unpaid";
export type Priority = "low" | "normal" | "high";

export interface Trainee {
  id: string;
  name: string;
  phone?: string;
  skillLevel?: "beginner" | "intermediate" | "advanced" | "";
  monthlyFee: number;
  status: TraineeStatus;
  notes?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Attendee {
  trainee: string;
  traineeName?: string;
  status: AttendanceStatus;
}

export interface Session {
  id: string;
  title: string;
  type: SessionType;
  start: string;
  durationMin: number;
  location?: string;
  capacity?: number;
  fee?: number;
  seriesId?: string;
  status: SessionStatus;
  attendees: Attendee[];
  createdAt: string;
  updatedAt: string;
}

export interface Payment {
  id: string;
  trainee?: string;
  traineeName?: string;
  amount: number;
  type: "subscription" | "private" | "dropin" | "sale" | "other";
  periodMonth?: string;
  date: string;
  note?: string;
  saleId?: string;
  createdAt: string;
  voidedAt?: string;
  voidReason?: string;
}

export interface Reminder {
  id: string;
  title: string;
  dueDate: string;
  priority: Priority;
  done: boolean;
  createdAt: string;
}

export interface Expense {
  id: string;
  amount: number;
  category: "rent" | "equipment" | "utilities" | "supplies" | "wages" | "other";
  note?: string;
  date: string;
  createdAt: string;
  voidedAt?: string;
  voidReason?: string;
}

export interface InventoryItem {
  id: string;
  name: string;
  sku?: string;
  stock: number;
  price: number;
  costPrice?: number;
  lowStockThreshold?: number;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Sale {
  id: string;
  item: string;
  itemName: string;
  trainee?: string;
  traineeName?: string;
  qty: number;
  unitPrice: number;
  total: number;
  date: string;
  paymentId?: string;
  createdAt: string;
  voidedAt?: string;
  voidReason?: string;
}

export interface SubStatus {
  trainee: Trainee;
  due: number;
  amountPaid: number;
  state: SubState;
}

export interface Dashboard {
  today: string;
  month: string;
  monthRevenue: number;
  activeTrainees: number;
  overdueCount: number;
  todaySessions: Session[];
  weekReminders: Reminder[];
  overdueSubscriptions: SubStatus[];
}

export interface Financials {
  income: number;
  outgoings: number;
  net: number;
  byType: Record<string, number>;
  byCategory: Record<string, number>;
  from: string;
  to: string;
}

export interface SearchResults {
  trainees: Trainee[];
  sessions: Session[];
  payments: Payment[];
  inventory: InventoryItem[];
}
