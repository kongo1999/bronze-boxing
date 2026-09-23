// Mirrors the Go API JSON shapes (see backend/internal/models).

export type TraineeStatus = "active" | "inactive";
export type SessionType = "group" | "private";
export type SessionStatus = "scheduled" | "completed" | "cancelled";
export type AttendanceStatus = "booked" | "attended" | "no_show";
/**
 * Dues state for one trainee and month. `partial` is never a kind of paid:
 * something was paid, less than what's due. `unverified` rows were imported
 * from records older than the dues history and stay out of paid/unpaid counts
 * until reconciled. `upcoming` is a future month with nothing paid yet.
 */
export type SubState = "paid" | "partial" | "unpaid" | "waived" | "unverified" | "upcoming";
export type ChargeSource = "normal" | "adjusted" | "imported_unverified" | "projected";
export type Priority = "low" | "normal" | "high";

export interface Trainee {
  id: string;
  name: string;
  phone?: string;
  skillLevel?: "beginner" | "intermediate" | "advanced" | "";
  /** The fee going forward (latest recorded terms). */
  monthlyFee: number;
  status: TraineeStatus | "removed";
  /** First month billed at monthlyFee — later than now when a change is scheduled. */
  feeFromMonth?: string;
  notes?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SubscriptionTerm {
  id: string;
  trainee: string;
  effectiveDate: string;
  billingFromMonth: string;
  monthlyFee: number;
  status: TraineeStatus;
  source?: string;
  reason?: string;
  createdAt: string;
  actor?: string;
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
  /** The month a subscription fee covers (YYYY-MM) — not when it was paid. */
  periodMonth?: string;
  /** When the cash was received. */
  date: string;
  note?: string;
  saleId?: string;
  createdAt: string;
  createdBy?: string;
  voidedAt?: string;
  voidReason?: string;
  voidedBy?: string;
}

export interface Reminder {
  id: string;
  title: string;
  /** Studio-local calendar day (YYYY-MM-DD). */
  dueDay: string;
  dueDate: string;
  priority: Priority;
  done: boolean;
  doneAt?: string;
  snoozedUntil?: string;
  relatedType?: "trainee" | "session" | "item";
  relatedId?: string;
  relatedLabel?: string;
  recurrence?: "" | "daily" | "weekly" | "monthly";
  seriesId?: string;
  nextId?: string;
  createdAt: string;
}

export interface Expense {
  id: string;
  amount: number;
  category: "rent" | "equipment" | "utilities" | "supplies" | "wages" | "other";
  note?: string;
  date: string;
  createdAt: string;
  createdBy?: string;
  voidedAt?: string;
  voidReason?: string;
  voidedBy?: string;
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
  unitCost?: number;
  listPrice?: number;
  priceReason?: string;
  total: number;
  returnedQty?: number;
  returnedTotal?: number;
  date: string;
  paymentId?: string;
  createdAt: string;
  createdBy?: string;
  voidedAt?: string;
  voidReason?: string;
  voidedBy?: string;
}

export interface StockMovement {
  id: string;
  item: string;
  itemName: string;
  delta: number;
  kind: "opening" | "sale" | "return" | "restock" | "damage" | "correction" | "sale_edit" | "sale_void";
  reason?: string;
  refType?: string;
  ref?: string;
  stockAfter: number;
  at: string;
  actor?: string;
}

/** One trainee's dues for one month (GET /subscriptions?m=). */
export interface SubStatus {
  trainee: Trainee;
  chargeId?: string;
  periodMonth: string;
  due: number;
  amountPaid: number;
  remaining: number;
  state: SubState;
  source: ChargeSource;
  /** A future month computed from the terms, not yet issued. */
  projected?: boolean;
  paymentCount: number;
  lastPaymentDate?: string;
}

/** A stored monthly charge (GET /subscription-charges). */
export interface Charge {
  id: string;
  trainee: string;
  traineeName: string;
  periodMonth: string;
  due: number;
  paidAmount: number;
  remaining: number;
  state: SubState;
  source: ChargeSource;
  createdAt: string;
  updatedAt: string;
}

export interface Dashboard {
  today: string;
  month: string;
  monthRevenue: number;
  activeTrainees: number;
  overdueCount: number;
  partialCount: number;
  unpaidCount: number;
  outstanding: number;
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

// One line of the append-only audit trail (GET /audit/:entity/:id). `before`
// and `after` are whole-record snapshots as plain objects, so the UI diffs
// whichever fields it cares to show.
export interface AuditEntry {
  id: string;
  entity: "payment" | "expense" | "sale" | "charge" | "item" | "trainee" | "series" | "session" | "plan";
  ref: string;
  action: "create" | "update" | "void" | "adjust" | "terms" | "return" | string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  actor?: string;
  reason?: string;
  at: string;
}
