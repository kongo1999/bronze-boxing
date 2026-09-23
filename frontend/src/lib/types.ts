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
  /** Set when a former trainee is archived off the everyday roster. */
  archivedAt?: string;
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
  /** The session plan this booking is credited to, if any. */
  planId?: string;
}

/** A trainee's session allowance ("12 private sessions") and its progress. */
export interface SessionPlan {
  id: string;
  trainee: string;
  traineeName: string;
  title: string;
  targetCount: number;
  startDate: string;
  endDate?: string;
  sessionType?: SessionType | "";
  status: "active" | "completed" | "cancelled";
  notes?: string;
  createdAt: string;
  updatedAt: string;
  progress: PlanProgress;
}

export interface PlanProgress {
  target: number;
  /** Credited: session completed AND the trainee marked attended. */
  completed: number;
  remaining: number;
  upcomingBooked: number;
  /** Completed classes where the trainee is still "booked". */
  attendanceNeeded: number;
  noShows: number;
  cancelled: number;
  unassignedSlots: number;
  credited: string[];
  upcoming: string[];
}

export interface SeriesProgress {
  seriesId: string;
  title: string;
  /** The denominator: occurrences planned (doesn't shrink on a cancellation). */
  planned: number;
  created: number;
  completed: number;
  scheduled: number;
  cancelled: number;
  attendanceNeeded: number;
  inferred?: boolean;
  status: string;
}

export interface SeriesDetail {
  progress: SeriesProgress;
  series: { id?: string; seriesId: string; weekdays?: number[]; time?: string; fromDay?: string; toDay?: string; plannedCount?: number };
  occurrences: { id: string; start: string; status: SessionStatus; booked: number; attendanceNeeded: boolean }[];
}

export interface RecurringPreview {
  count: number;
  conflicts: number;
  occurrences: { start: string; day: string; conflict?: { title: string; start: string; type: SessionType } }[];
  planProblem?: { code: string; error: string; details?: Record<string, unknown> };
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
  /** How it was paid; absent on payments recorded before methods existed. */
  method?: PayMethod;
  reference?: string;
  saleId?: string;
  createdAt: string;
  createdBy?: string;
  voidedAt?: string;
  voidReason?: string;
  voidedBy?: string;
}

export type PayMethod = "cash" | "card" | "bank_transfer" | "other";

export interface Receipt {
  studio: string;
  studioInfo: { name: string; address?: string; phone?: string; currency: string };
  number: string;
  payment: Payment;
  issued: string;
  void: boolean;
  method: PayMethod | "unspecified";
  cashDay: string;
  timezone: string;
  charge?: Charge;
}

export interface Page<T> {
  items: T[];
  total: number;
  hasMore: boolean;
  offset: number;
  limit: number;
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
  method?: PayMethod;
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

export interface SaleReturn {
  id: string;
  sale: string;
  item: string;
  itemName: string;
  qty: number;
  /** Refunded. */
  amount: number;
  date: string;
  reason: string;
  createdAt: string;
  actor?: string;
}

/** GET /sales/:id/receipt */
export interface SaleReceipt {
  studioInfo: { name: string; address?: string; phone?: string; currency: string };
  number: string;
  sale: Sale;
  returns: SaleReturn[];
  /** What the buyer paid once returns are refunded. */
  net: number;
  issued: string;
  void: boolean;
  method: PayMethod;
  cashDay: string;
  timezone: string;
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

/** Net cash for a period: money in minus recorded money out, by cash date. */
export interface Financials {
  income: number;
  outgoings: number;
  net: number;
  byType: Record<string, number>;
  byCategory: Record<string, number>;
  byMethod: Record<string, number>;
  counts: Record<string, number>;
  from: string;
  to: string;
  previous: { income: number; outgoings: number; net: number; from: string; to: string };
}

export interface LedgerRow {
  kind: "payment" | "sale" | "return" | "expense";
  id: string;
  sale?: string;
  date: string;
  day: string;
  detail: string;
  type: string;
  method?: string;
  reference?: string;
  periodMonth?: string;
  trainee?: string;
  note?: string;
  in: number;
  out: number;
  voided: boolean;
  voidReason?: string;
}

export interface LedgerPage extends Page<LedgerRow> {
  totals: { income: number; outgoings: number; net: number };
  from: string;
  to: string;
}

export interface ClosingDay {
  day: string;
  cash: number;
  unspecified: number;
  card: number;
  transfer: number;
  other: number;
  expected: number;
  counted?: number;
  difference?: number;
  note?: string;
  actor?: string;
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
