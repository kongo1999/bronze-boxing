// Demo / mock data layer for client presentations.
//
// When enabled (production builds with no backend, e.g. the Vercel preview),
// the API client short-circuits here instead of hitting /api. Data lives in
// mutable module arrays so the demo is interactive in-session; a reload
// resets it. The rules mirror the live API — the same fuzzy matcher, dues
// states (partial is never "paid"), recurring series with a planned count,
// session-plan progress, month filters by studio day, and the monthly report
// — and it raises the same coded errors (OVERPAYMENT, REASON_REQUIRED, …).
// An on-screen banner says nothing here is saved. Local `npm run dev` keeps
// using the real Go API.

import { ApiError } from "./api-error";
import { rank, normalize } from "./fuzzy";
import { addDays, currentMonth, dayOf as studioDay, monthOf, shiftMonthKey, studioInstant, todayKey } from "./studio";
import type {
  Trainee, Session, Payment, Reminder, Expense, InventoryItem, Sale, SaleReturn, StockMovement,
  SubStatus, Dashboard, Financials, SearchResponse, SearchHit, SearchKind, Attendee, AuditEntry, LedgerRow,
  SessionPlan, PlanProgress, SeriesProgress, MonthlyReport, Charge,
} from "./types";

// Demo on in production unless explicitly disabled; off in dev unless forced.
export const isDemo =
  import.meta.env.VITE_DEMO === "true" ||
  (import.meta.env.PROD && import.meta.env.VITE_DEMO !== "false");

const now = new Date();
const nowISO = now.toISOString();
const MONTH = currentMonth();
const TODAY = todayKey();
/** Studio day `offset` days from today, at a studio wall-clock time. */
const at = (offset: number, time = "09:00") => studioInstant(addDays(TODAY, offset), time).toISOString();
let seq = 1000;
const genId = () => `demo-${++seq}`;
const cents = (v: number) => Math.round(v * 100);
const amt = (c: number) => c / 100;

function fail(status: number, code: string, message: string, field?: string, details?: Record<string, unknown>): never {
  throw new ApiError(message, status, code, field, details);
}
function needReason(reason: unknown, what: string): string {
  const r = typeof reason === "string" ? reason.trim() : "";
  if (!r) fail(400, "REASON_REQUIRED", `a reason is required to ${what}`, "reason");
  return r;
}

// ── Trainees ───────────────────────────────────────────────────────────────
const trainees: Trainee[] = [
  { id: "t1", name: "Karim Haddad", phone: "03 111 222", skillLevel: "intermediate", monthlyFee: 120, status: "active", notes: "Southpaw. Working on the jab.", createdAt: nowISO, updatedAt: nowISO },
  { id: "t2", name: "Rami Khoury", phone: "03 333 444", skillLevel: "beginner", monthlyFee: 100, status: "active", createdAt: nowISO, updatedAt: nowISO },
  { id: "t3", name: "Jad Saliba", phone: "70 555 666", skillLevel: "advanced", monthlyFee: 120, status: "active", notes: "Prepping for amateur bout.", createdAt: nowISO, updatedAt: nowISO },
  { id: "t4", name: "Nour Aoun", phone: "76 777 888", skillLevel: "beginner", monthlyFee: 80, status: "active", createdAt: nowISO, updatedAt: nowISO },
  { id: "t5", name: "Tarek Mansour", phone: "71 999 000", skillLevel: "intermediate", monthlyFee: 100, status: "active", createdAt: nowISO, updatedAt: nowISO },
  { id: "t6", name: "Sami Dabboussi", phone: "03 121 212", skillLevel: "beginner", monthlyFee: 0, status: "active", notes: "Drop-in only.", createdAt: nowISO, updatedAt: nowISO },
  { id: "t7", name: "Lara Fares", phone: "78 343 434", skillLevel: "intermediate", monthlyFee: 90, status: "active", createdAt: nowISO, updatedAt: nowISO },
  { id: "t8", name: "Omar Zein", phone: "70 565 656", skillLevel: "beginner", monthlyFee: 0, status: "inactive", notes: "On a break.", createdAt: nowISO, updatedAt: nowISO },
];
const tName = (id?: string) => trainees.find((t) => t.id === id)?.name ?? "";

// ── Plans and series ───────────────────────────────────────────────────────
type StoredPlan = Omit<SessionPlan, "progress">;
const plans: StoredPlan[] = [
  { id: "pl1", trainee: "t3", traineeName: "Jad Saliba", title: "12 private sessions", targetCount: 12, startDate: `${MONTH}-01`, sessionType: "private", status: "active", createdAt: nowISO, updatedAt: nowISO },
];
interface StoredSeries {
  seriesId: string;
  title: string;
  type: Session["type"];
  weekdays: number[];
  time: string;
  fromDay: string;
  toDay: string;
  plannedCount: number;
  status: string;
  createdAt: string;
}
const seriesDocs: StoredSeries[] = [];

// ── Sessions: this month's group classes and Jad's privates, as series ──────
const groupRoster: Attendee[] = [
  { trainee: "t1", traineeName: "Karim Haddad", status: "booked" },
  { trainee: "t2", traineeName: "Rami Khoury", status: "booked" },
  { trainee: "t3", traineeName: "Jad Saliba", status: "booked" },
  { trainee: "t4", traineeName: "Nour Aoun", status: "booked" },
  { trainee: "t7", traineeName: "Lara Fares", status: "booked" },
];
const sessions: Session[] = [];
(() => {
  const first = `${MONTH}-01`;
  const days = new Date(Number(MONTH.slice(0, 4)), Number(MONTH.slice(5, 7)), 0).getDate();
  const last = `${MONTH}-${String(days).padStart(2, "0")}`;
  seriesDocs.push(
    { seriesId: "ser-group", title: "Evening Group Class", type: "group", weekdays: [1, 3, 5], time: "18:00", fromDay: first, toDay: last, plannedCount: 0, status: "active", createdAt: nowISO },
    { seriesId: "ser-jad", title: "Private — Jad", type: "private", weekdays: [2], time: "17:00", fromDay: first, toDay: last, plannedCount: 0, status: "active", createdAt: nowISO },
  );
  for (let d = 0; d < days; d++) {
    const day = addDays(first, d);
    const wd = new Date(`${day}T12:00:00Z`).getUTCDay();
    const past = day < TODAY;
    if ([1, 3, 5].includes(wd)) {
      sessions.push({
        id: `s-grp-${day}`, title: "Evening Group Class", type: "group", start: studioInstant(day, "18:00").toISOString(), durationMin: 60,
        location: "Main floor", capacity: 12, seriesId: "ser-group", status: past ? "completed" : "scheduled",
        attendees: groupRoster.map((a, i) => ({ ...a, status: past ? (i === 1 ? "no_show" : "attended") : "booked" })),
        createdAt: nowISO, updatedAt: nowISO,
      });
      seriesDocs[0].plannedCount++;
    }
    if (wd === 2) {
      sessions.push({
        id: `s-pvt-${day}`, title: "Private — Jad", type: "private", start: studioInstant(day, "17:00").toISOString(), durationMin: 45,
        location: "Ring 1", fee: 35, seriesId: "ser-jad", status: past ? "completed" : "scheduled",
        attendees: [{ trainee: "t3", traineeName: "Jad Saliba", status: past ? "attended" : "booked", planId: "pl1" }],
        createdAt: nowISO, updatedAt: nowISO,
      });
      seriesDocs[1].plannedCount++;
    }
  }
  sessions.push({
    id: "s-today-am", title: "Morning Conditioning", type: "group", start: at(0, "07:30"), durationMin: 50,
    location: "Main floor", capacity: 10, status: "scheduled",
    attendees: [
      { trainee: "t4", traineeName: "Nour Aoun", status: "booked" },
      { trainee: "t5", traineeName: "Tarek Mansour", status: "booked" },
      { trainee: "t6", traineeName: "Sami Dabboussi", status: "booked" },
    ], createdAt: nowISO, updatedAt: nowISO,
  });
})();

// ── Money ──────────────────────────────────────────────────────────────────
const payments: Payment[] = [
  { id: "p1", trainee: "t1", traineeName: "Karim Haddad", amount: 120, type: "subscription", periodMonth: MONTH, date: at(-2, "10:00"), method: "cash", createdAt: nowISO },
  { id: "p2", trainee: "t4", traineeName: "Nour Aoun", amount: 80, type: "subscription", periodMonth: MONTH, date: at(-5, "11:00"), method: "card", createdAt: nowISO },
  { id: "p3", trainee: "t7", traineeName: "Lara Fares", amount: 90, type: "subscription", periodMonth: MONTH, date: at(-6, "09:00"), method: "bank_transfer", reference: "TRX-4471", createdAt: nowISO },
  { id: "p4", trainee: "t2", traineeName: "Rami Khoury", amount: 50, type: "subscription", periodMonth: MONTH, date: at(-1, "18:00"), method: "cash", note: "Half now, half later", createdAt: nowISO },
  { id: "p5", trainee: "t3", traineeName: "Jad Saliba", amount: 35, type: "private", date: at(-3, "16:00"), method: "cash", createdAt: nowISO },
  { id: "p6", trainee: "t6", traineeName: "Sami Dabboussi", amount: 20, type: "dropin", date: at(-4, "19:00"), method: "cash", createdAt: nowISO },
];
const expenses: Expense[] = [
  { id: "e1", amount: 800, category: "rent", note: "Gym space — monthly", date: at(-20, "10:00"), createdAt: nowISO },
  { id: "e2", amount: 240, category: "equipment", note: "Two pairs of focus mitts", date: at(-5, "12:00"), createdAt: nowISO },
  { id: "e3", amount: 95, category: "utilities", note: "Electricity", date: at(-3, "14:00"), createdAt: nowISO },
  { id: "e4", amount: 60, category: "supplies", note: "Cleaning + hand tape", date: at(-8, "12:00"), createdAt: nowISO },
];

// ── Reminders ──────────────────────────────────────────────────────────────
const reminders: Reminder[] = [
  { id: "r1", title: "Fix the speed-bag bracket", dueDay: addDays(TODAY, -1), dueDate: at(-1, "00:00"), priority: "high", done: false, createdAt: nowISO },
  { id: "r2", title: "Call Rami about his schedule", dueDay: TODAY, dueDate: at(0, "00:00"), priority: "normal", done: false, relatedType: "trainee", relatedId: "t2", relatedLabel: "Rami Khoury", createdAt: nowISO },
  { id: "r3", title: "Order new gloves (size M)", dueDay: addDays(TODAY, 2), dueDate: at(2, "00:00"), priority: "normal", done: false, relatedType: "item", relatedId: "i2", relatedLabel: "Boxing Gloves 12oz", createdAt: nowISO },
  { id: "r4", title: "Renew gym insurance", dueDay: addDays(TODAY, 4), dueDate: at(4, "00:00"), priority: "high", done: false, createdAt: nowISO },
  { id: "r5", title: "Wipe down the ring canvas", dueDay: addDays(TODAY, -2), dueDate: at(-2, "00:00"), priority: "low", done: true, createdAt: nowISO },
];

// ── Shop ───────────────────────────────────────────────────────────────────
const inventory: InventoryItem[] = [
  { id: "i1", name: "Hand Wraps (4.5m)", sku: "WRAP-45", stock: 24, price: 8, costPrice: 4, lowStockThreshold: 6, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i2", name: "Boxing Gloves 12oz", sku: "GLV-12", stock: 3, price: 45, costPrice: 28, lowStockThreshold: 3, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i3", name: "Water Bottle 750ml", sku: "H2O-750", stock: 30, price: 5, costPrice: 2, lowStockThreshold: 10, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i4", name: "Bronze Boxing Tee", sku: "TEE-BB", stock: 15, price: 20, costPrice: 9, lowStockThreshold: 5, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i5", name: "Jump Rope", sku: "ROPE", stock: 2, price: 12, costPrice: 5, lowStockThreshold: 4, active: true, createdAt: nowISO, updatedAt: nowISO },
];
const sales: Sale[] = [
  { id: "sa1", item: "i1", itemName: "Hand Wraps (4.5m)", trainee: "t1", traineeName: "Karim Haddad", qty: 1, unitPrice: 8, listPrice: 8, total: 8, date: at(-1, "17:00"), createdAt: nowISO },
  { id: "sa2", item: "i4", itemName: "Bronze Boxing Tee", trainee: "t7", traineeName: "Lara Fares", qty: 2, unitPrice: 20, listPrice: 20, total: 40, method: "card", date: at(-3, "18:00"), createdAt: nowISO },
  { id: "sa3", item: "i3", itemName: "Water Bottle 750ml", qty: 3, unitPrice: 5, listPrice: 5, total: 15, date: at(-4, "19:00"), createdAt: nowISO },
];
const returns: SaleReturn[] = [];
const movements: StockMovement[] = [];
(() => {
  const opened = studioInstant(`${MONTH}-01`, "00:00").toISOString();
  for (const it of inventory) {
    const sold = sales.filter((s) => s.item === it.id).reduce((n, s) => n + s.qty, 0);
    movements.push({ id: genId(), item: it.id, itemName: it.name, delta: it.stock + sold, kind: "opening", reason: "Opening stock", stockAfter: it.stock + sold, at: opened });
  }
  for (const s of [...sales].sort((a, b) => a.date.localeCompare(b.date))) {
    const before = movements.filter((m) => m.item === s.item).reduce((n, m) => n + m.delta, 0);
    movements.push({ id: genId(), item: s.item, itemName: s.itemName, delta: -s.qty, kind: "sale", refType: "sale", ref: s.id, stockAfter: before - s.qty, at: s.date });
  }
})();
function move(it: InventoryItem, delta: number, kind: StockMovement["kind"], reason = "", ref?: string, refType?: string) {
  movements.push({ id: genId(), item: it.id, itemName: it.name, delta, kind, reason, ref, refType, stockAfter: it.stock, at: new Date().toISOString(), actor: "demo" });
}

// ── Audit trail: real history of what's done in-session ─────────────────────
const audit: AuditEntry[] = [];
function logAudit(entity: AuditEntry["entity"], ref: string, action: AuditEntry["action"], before: unknown, after: unknown, reason = ""): void {
  audit.unshift({
    id: genId(), entity, ref, action, actor: "demo", reason,
    before: { ...(before as Record<string, unknown>) },
    after: { ...(after as Record<string, unknown>) },
    at: new Date().toISOString(),
  } as AuditEntry);
}

// ── Rules (mirroring the Go backend) ────────────────────────────────────────
const live = <T extends { voidedAt?: string }>(arr: T[]) => arr.filter((x) => !x.voidedAt);
const inMonth = (iso: string, m: string) => monthOf(iso) === m;

/** Dues for a month. The demo's history starts this month: earlier months have none. */
function subscriptions(month = MONTH): SubStatus[] {
  if (month < MONTH) return [];
  return trainees
    .filter((t) => t.status === "active" && t.monthlyFee > 0 && !t.archivedAt)
    .map((t) => {
      const mine = live(payments).filter((p) => p.type === "subscription" && p.periodMonth === month && p.trainee === t.id);
      const paidC = mine.reduce((s, p) => s + cents(p.amount), 0);
      const dueC = cents(t.monthlyFee);
      const state: SubStatus["state"] = paidC >= dueC ? "paid" : paidC > 0 ? "partial" : month > MONTH ? "upcoming" : "unpaid";
      return {
        trainee: t, chargeId: `c-${t.id}-${month}`, periodMonth: month, due: t.monthlyFee, amountPaid: amt(paidC),
        remaining: amt(Math.max(0, dueC - paidC)), state, source: "normal" as const, projected: month > MONTH,
        paymentCount: mine.length, lastPaymentDate: mine.map((p) => p.date).sort().pop(),
      };
    });
}
function charges(trainee: string): Charge[] {
  const out: Charge[] = [];
  for (const m of [MONTH]) {
    const s = subscriptions(m).find((x) => x.trainee.id === trainee);
    if (s) out.push({ id: s.chargeId!, trainee, traineeName: s.trainee.name, periodMonth: m, due: s.due, paidAmount: s.amountPaid, remaining: s.remaining, state: s.state, source: "normal", createdAt: nowISO, updatedAt: nowISO });
  }
  return out;
}

function ledgerRows(): LedgerRow[] {
  const rows: LedgerRow[] = [];
  for (const p of payments) rows.push({ kind: "payment", id: p.id, date: p.date, day: studioDay(p.date), detail: p.traineeName || "—", type: p.type, method: p.method, reference: p.reference, periodMonth: p.periodMonth, trainee: p.trainee, note: p.note, in: p.amount, out: 0, voided: !!p.voidedAt, voidReason: p.voidReason });
  for (const s of sales) if (!s.paymentId) rows.push({ kind: "sale", id: s.id, date: s.date, day: studioDay(s.date), detail: `${s.itemName} × ${s.qty}${s.traineeName ? ` → ${s.traineeName}` : ""}`, type: "sale", method: s.method, trainee: s.trainee, in: s.total, out: 0, voided: !!s.voidedAt, voidReason: s.voidReason });
  for (const rt of returns) {
    const s = sales.find((x) => x.id === rt.sale);
    rows.push({ kind: "return", id: rt.id, sale: rt.sale, date: rt.date, day: studioDay(rt.date), detail: `${rt.itemName} × ${rt.qty} returned`, type: "return", method: s?.method, note: rt.reason, in: -rt.amount, out: 0, voided: !!s?.voidedAt, voidReason: s?.voidedAt ? "sale voided" : undefined });
  }
  for (const e of expenses) rows.push({ kind: "expense", id: e.id, date: e.date, day: studioDay(e.date), detail: e.category, type: e.category, note: e.note, in: 0, out: e.amount, voided: !!e.voidedAt, voidReason: e.voidReason });
  return rows.sort((a, b) => b.date.localeCompare(a.date));
}
const methodKey = (r: LedgerRow) => (r.kind === "expense" ? "" : r.method || (r.kind === "payment" ? "unspecified" : "cash"));

function summary(month: string) {
  const rows = ledgerRows().filter((r) => inMonth(r.date, month));
  const byType: Record<string, number> = {};
  const byCategory: Record<string, number> = {};
  const byMethod: Record<string, number> = {};
  const counts: Record<string, number> = {};
  let inC = 0;
  let outC = 0;
  for (const r of rows) {
    if (r.voided) { counts.voided = (counts.voided ?? 0) + 1; continue; }
    counts[r.kind] = (counts[r.kind] ?? 0) + 1;
    if (r.kind === "expense") { outC += cents(r.out); byCategory[r.type] = amt(cents(byCategory[r.type] ?? 0) + cents(r.out)); continue; }
    inC += cents(r.in);
    byType[r.type] = amt(cents(byType[r.type] ?? 0) + cents(r.in));
    byMethod[methodKey(r)] = amt(cents(byMethod[methodKey(r)] ?? 0) + cents(r.in));
  }
  return { rows, income: amt(inC), outgoings: amt(outC), net: amt(inC - outC), byType, byCategory, byMethod, counts };
}
function financials(month: string): Financials {
  const s = summary(month);
  const p = summary(shiftMonthKey(month, -1));
  const from = studioInstant(`${month}-01`).toISOString();
  const to = studioInstant(`${shiftMonthKey(month, 1)}-01`).toISOString();
  return {
    income: s.income, outgoings: s.outgoings, net: s.net, byType: s.byType, byCategory: s.byCategory, byMethod: s.byMethod,
    counts: s.counts, from, to, previous: { income: p.income, outgoings: p.outgoings, net: p.net, from: studioInstant(`${shiftMonthKey(month, -1)}-01`).toISOString(), to: from },
  };
}

function planProgress(p: StoredPlan): PlanProgress {
  const pr: PlanProgress = { target: p.targetCount, completed: 0, remaining: 0, upcomingBooked: 0, attendanceNeeded: 0, noShows: 0, cancelled: 0, unassignedSlots: 0, credited: [], upcoming: [] };
  for (const s of sessions) {
    const a = s.attendees.find((x) => x.planId === p.id && x.trainee === p.trainee);
    if (!a) continue;
    if (s.status === "cancelled") pr.cancelled++;
    else if (a.status === "no_show") pr.noShows++;
    else if (s.status === "completed" && a.status === "attended") { pr.completed++; pr.credited.push(s.id); }
    else if (s.status === "completed") pr.attendanceNeeded++;
    else { pr.upcomingBooked++; pr.upcoming.push(s.id); }
  }
  pr.remaining = Math.max(p.targetCount - pr.completed, 0);
  pr.unassignedSlots = Math.max(p.targetCount - pr.completed - pr.upcomingBooked - pr.attendanceNeeded, 0);
  return pr;
}
const withProgress = (p: StoredPlan): SessionPlan => ({ ...p, progress: planProgress(p) });

function seriesProgress(id: string): SeriesProgress {
  const doc = seriesDocs.find((d) => d.seriesId === id);
  const occ = sessions.filter((s) => s.seriesId === id);
  const pr: SeriesProgress = { seriesId: id, title: doc?.title ?? occ[0]?.title ?? "", planned: doc?.plannedCount ?? occ.length, created: occ.length, completed: 0, scheduled: 0, cancelled: 0, attendanceNeeded: 0, inferred: !doc, status: doc?.status ?? "active" };
  for (const s of occ) {
    if (s.status === "completed") { pr.completed++; if (s.attendees.some((a) => a.status === "booked")) pr.attendanceNeeded++; }
    else if (s.status === "cancelled") pr.cancelled++;
    else pr.scheduled++;
  }
  return pr;
}

/** Studio-day occurrences of a weekly pattern, as the API builds them. */
function occurrences(weekdays: number[], time: string, fromDay: string, toDay: string): string[] {
  const out: string[] = [];
  for (let d = fromDay; d <= toDay && out.length < 200; d = addDays(d, 1)) {
    if (weekdays.includes(new Date(`${d}T12:00:00Z`).getUTCDay())) out.push(studioInstant(d, time).toISOString());
  }
  return out;
}

function remindCounts() {
  const eff = (r: Reminder) => (r.snoozedUntil && r.snoozedUntil > r.dueDay ? r.snoozedUntil : r.dueDay);
  const open = reminders.filter((r) => !r.done);
  return { overdue: open.filter((r) => eff(r) < TODAY).length, today: open.filter((r) => eff(r) === TODAY).length, open: open.length };
}
const shortage = (i: InventoryItem) => (i.stock <= 0 ? "out" : i.lowStockThreshold && i.stock <= i.lowStockThreshold ? "low" : "");

function dashboard(): Dashboard {
  const subs = subscriptions();
  const owing = subs.filter((s) => s.state === "partial" || s.state === "unpaid");
  const weekEnd = addDays(TODAY, 7);
  const rc = remindCounts();
  const active = inventory.filter((i) => i.active);
  const nearing = plans
    .filter((p) => p.status === "active")
    .map((p) => ({ p, pr: planProgress(p) }))
    .filter(({ p, pr }) => pr.remaining <= 2 || (p.endDate && p.endDate <= weekEnd))
    .map(({ p, pr }) => ({ id: p.id, trainee: p.trainee, traineeName: p.traineeName, title: p.title, remaining: pr.remaining, endDate: p.endDate }));
  const next = sessions.filter((s) => s.status === "scheduled" && s.start > new Date().toISOString()).sort((a, b) => a.start.localeCompare(b.start))[0] ?? null;
  return {
    today: nowISO, month: MONTH,
    monthRevenue: summary(MONTH).income,
    activeTrainees: trainees.filter((t) => t.status === "active").length,
    overdueCount: owing.length,
    partialCount: owing.filter((s) => s.state === "partial").length,
    unpaidCount: owing.filter((s) => s.state === "unpaid").length,
    outstanding: amt(owing.reduce((sum, s) => sum + cents(s.remaining), 0)),
    todaySessions: sessions.filter((s) => studioDay(s.start) === TODAY),
    weekReminders: reminders.filter((r) => r.dueDay >= TODAY && r.dueDay < weekEnd),
    overdueSubscriptions: owing,
    remindersOverdue: rc.overdue, remindersToday: rc.today,
    lowStock: active.filter((i) => shortage(i) === "low").length,
    outOfStock: active.filter((i) => shortage(i) === "out").length,
    plansNearing: nearing, nextSession: next,
  };
}

function report(month: string): MonthlyReport {
  const f = financials(month);
  const s = summary(month);
  const pct = (n: number, d: number) => (d ? Math.round((n / d) * 1000) / 10 : null);
  const change = (c: number, p: number) => (p ? pct(c - p, Math.abs(p)) : null);
  let pay = 0, sold = 0, refunds = 0;
  for (const r of s.rows) {
    if (r.voided) continue;
    if (r.kind === "payment") pay += cents(r.in);
    if (r.kind === "sale") sold += cents(r.in);
    if (r.kind === "return") refunds -= cents(r.in);
  }
  const subs = subscriptions(month).filter((x) => !x.projected);
  const payer = (x: SubStatus) => ({ traineeId: x.trainee.id, name: x.trainee.name, due: x.due, paid: x.amountPaid, remaining: x.remaining });
  const monthEnd = studioInstant(`${shiftMonthKey(month, 1)}-01`).toISOString();
  const nowIso = new Date().toISOString();
  const inM = sessions.filter((x) => inMonth(x.start, month));
  const kept = inM.filter((x) => x.status !== "cancelled");
  const started = kept.filter((x) => x.start <= nowIso);
  const attendedBy = new Set<string>();
  const missedBy = new Set<string>();
  let attended = 0, noShow = 0, unmarked = 0, ahead = 0, capB = 0, capC = 0, capN = 0;
  for (const x of kept) {
    if (x.capacity) { capN++; capB += x.attendees.length; capC += x.capacity; }
    for (const a of x.attendees) {
      if (a.status === "attended") { attended++; attendedBy.add(a.trainee); }
      else if (a.status === "no_show") { noShow++; missedBy.add(a.trainee); }
      else if (x.start <= nowIso) unmarked++;
      else ahead++;
    }
  }
  const seriesIds = [...new Set(kept.map((x) => x.seriesId).filter((x): x is string => !!x))];
  const monthSales = live(sales).filter((x) => inMonth(x.date, month) && !x.paymentId);
  const byItem = new Map<string, { itemId: string; name: string; units: number; revenue: number }>();
  for (const x of monthSales) {
    const it = byItem.get(x.item) ?? { itemId: x.item, name: x.itemName, units: 0, revenue: 0 };
    it.units += x.qty;
    it.revenue = amt(cents(it.revenue) + cents(x.total));
    byItem.set(x.item, it);
  }
  const items = [...byItem.values()];
  const monthReturns = returns.filter((r) => inMonth(r.date, month) && !sales.find((x) => x.id === r.sale)?.voidedAt);
  const active = plans.filter((p) => p.status === "active");
  return {
    month, from: studioInstant(`${month}-01`).toISOString(), to: monthEnd, generatedAt: nowIso, timezone: "Asia/Beirut", partial: nowIso < monthEnd,
    financial: {
      traineePayments: amt(pay), shopSales: amt(sold), refunds: amt(refunds), shopNet: amt(sold - refunds),
      income: f.income, expenses: f.outgoings, net: f.net, byType: f.byType, byCategory: f.byCategory, byMethod: f.byMethod, counts: f.counts,
      previous: { income: f.previous.income, outgoings: f.previous.outgoings, net: f.previous.net },
      incomeChangePct: change(f.income, f.previous.income), expensesChangePct: change(f.outgoings, f.previous.outgoings), netChangePct: change(f.net, f.previous.net),
    },
    subscriptions: {
      billed: subs.length, totalBilled: amt(subs.reduce((n, x) => n + cents(x.due), 0)), paidTowardPeriod: amt(subs.reduce((n, x) => n + cents(x.amountPaid), 0)),
      outstanding: amt(subs.filter((x) => x.state === "partial" || x.state === "unpaid").reduce((n, x) => n + cents(x.remaining), 0)),
      paid: subs.filter((x) => x.state === "paid").length, partial: subs.filter((x) => x.state === "partial").length, unpaid: subs.filter((x) => x.state === "unpaid").length,
      waived: 0, unverified: 0, unverifiedPaid: 0,
      partialPayers: subs.filter((x) => x.state === "partial").map(payer), unpaidPayers: subs.filter((x) => x.state === "unpaid").map(payer),
    },
    trainees: {
      activeAtMonthEnd: trainees.filter((t) => t.status === "active" && t.createdAt < monthEnd).length,
      joined: trainees.filter((t) => inMonth(t.createdAt, month)).length,
      inactivated: trainees.filter((t) => t.archivedAt && inMonth(t.archivedAt, month)).length,
      attended: attendedBy.size,
    },
    sessions: {
      total: inM.length, scheduled: inM.filter((x) => x.status === "scheduled").length, completed: inM.filter((x) => x.status === "completed").length,
      cancelled: inM.filter((x) => x.status === "cancelled").length, group: kept.filter((x) => x.type === "group").length, private: kept.filter((x) => x.type === "private").length,
      started: started.length, notMarkedDone: started.filter((x) => x.status === "scheduled").length,
      completionPct: pct(kept.filter((x) => x.status === "completed").length, started.length),
      seriesCreated: seriesDocs.filter((d) => inMonth(d.createdAt, month)).length,
      series: seriesIds.map((id) => { const p = seriesProgress(id); return { seriesId: id, title: p.title, completed: p.completed, planned: p.planned, inMonth: kept.filter((x) => x.seriesId === id).length }; }),
      plansActive: active.length,
      planCredits: sessions.filter((x) => inMonth(x.start, month) && x.status === "completed").reduce((n, x) => n + x.attendees.filter((a) => a.planId && a.status === "attended").length, 0),
      planRemaining: active.reduce((n, p) => n + planProgress(p).remaining, 0),
    },
    attendance: {
      attended, noShow, unmarked, bookedAhead: ahead, people: attendedBy.size, noShowPeople: missedBy.size, ratePct: pct(attended, attended + noShow),
      cappedClasses: capN, cappedBooked: capB, cappedCapacity: capC, occupancyPct: pct(capB, capC),
    },
    inventory: {
      unitsSold: monthSales.reduce((n, x) => n + x.qty, 0), salesRevenue: amt(monthSales.reduce((n, x) => n + cents(x.total), 0)),
      unitsReturned: monthReturns.reduce((n, r) => n + r.qty, 0), refunds: amt(monthReturns.reduce((n, r) => n + cents(r.amount), 0)),
      topByUnits: [...items].sort((a, b) => b.units - a.units || a.name.localeCompare(b.name)).slice(0, 5),
      topByRevenue: [...items].sort((a, b) => b.revenue - a.revenue || a.name.localeCompare(b.name)).slice(0, 5),
      lowNow: inventory.filter((i) => shortage(i) === "low").length, outNow: inventory.filter((i) => shortage(i) === "out").length,
      stock: inventory.map((i) => {
        const mine = movements.filter((m) => m.item === i.id);
        const tracked = mine.some((m) => m.at < monthEnd);
        return { itemId: i.id, name: i.name, atMonthEnd: tracked ? mine.filter((m) => m.at < monthEnd).reduce((n, m) => n + m.delta, 0) : null, now: i.stock };
      }).sort((a, b) => a.name.localeCompare(b.name)),
    },
  };
}

// ── Router ─────────────────────────────────────────────────────────────────
function rm<T extends { id: string }>(arr: T[], id: string): void {
  const i = arr.findIndex((x) => x.id === id);
  if (i >= 0) arr.splice(i, 1);
}
/** A page when ?limit= is given, else the plain array — like the API. */
function listOrPage<T>(rows: T[], params: URLSearchParams) {
  if (!params.has("limit")) return rows;
  const limit = Number(params.get("limit") ?? 20);
  const offset = Number(params.get("offset") ?? 0);
  return { items: rows.slice(offset, offset + limit), total: rows.length, hasMore: offset + limit < rows.length, offset, limit };
}
/** ?m= or ?from=&to= (instants or studio days), half-open. */
function periodFilter(params: URLSearchParams): ((iso: string) => boolean) | null {
  const m = params.get("m");
  if (m) return (iso) => inMonth(iso, m);
  const from = params.get("from");
  const to = params.get("to");
  if (!from && !to) return null;
  const edge = (v: string) => (/^\d{4}-\d{2}-\d{2}$/.test(v) ? studioInstant(v).toISOString() : new Date(v).toISOString());
  const f = from ? edge(from) : "";
  const t = to ? edge(to) : "";
  return (iso) => (!f || iso >= f) && (!t || iso < t);
}
function searched<T>(rows: T[], q: string | null, fields: (x: T) => (string | undefined)[]): T[] {
  return q && q.trim() ? rank(rows, q, fields).map((x) => x.item) : rows;
}

export function demoResolve<T>(rawPath: string, method: string, bodyStr?: BodyInit | null): T {
  const [path, query = ""] = rawPath.split("?");
  const params = new URLSearchParams(query);
  const body = typeof bodyStr === "string" && bodyStr ? JSON.parse(bodyStr) : {};
  const seg = path.split("/").filter(Boolean);
  const r = (v: unknown): T => v as unknown as T;
  const q = params.get("q");

  if (path === "/health") return r({ status: "ok", db: true, authRequired: false, timezone: "Asia/Beirut", time: nowISO });
  if (path === "/dashboard") return r(dashboard());
  if (path === "/financials") return r(financials(params.get("m") ?? MONTH));
  if (path === "/reports/monthly") {
    const m = params.get("m") ?? MONTH;
    if (m > MONTH) fail(400, "VALIDATION", "a report can't be for a month that hasn't started", "m");
    return r(report(m));
  }
  if (path === "/reports/monthly/export") {
    const rep = report(params.get("m") ?? MONTH);
    return r(["Section,Metric,Value", `Report,Month,${rep.month}`, `Financial,Total income,${rep.financial.income.toFixed(2)}`, `Financial,Total expenses,${rep.financial.expenses.toFixed(2)}`, `Financial,Net cash,${rep.financial.net.toFixed(2)}`, `Subscriptions,Outstanding,${rep.subscriptions.outstanding.toFixed(2)}`].join("\n"));
  }
  if (path === "/financials/export") {
    const esc = (s: string) => (/[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s);
    const m = params.get("m") ?? MONTH;
    const rows = ledgerRows().filter((x) => inMonth(x.date, m)).reverse().map((x) => [x.day, x.kind, x.detail, x.type, x.note ?? "", x.in.toFixed(2), x.out.toFixed(2), x.voided ? "VOID" : ""].map(esc).join(","));
    const f = financials(m);
    return r(["Date,Kind,Detail,Type,Note,In,Out,Status", ...rows, "", `,,,,Total in,${f.income.toFixed(2)},,`, `,,,,Total out,,${f.outgoings.toFixed(2)},`, `,,,,Net cash,${f.net.toFixed(2)},,`].join("\n"));
  }
  if (path === "/subscriptions") return r(subscriptions(params.get("m") ?? MONTH));
  if (path === "/subscription-charges") return r(charges(params.get("trainee") ?? ""));
  if (path === "/ledger") {
    const m = params.get("m") ?? MONTH;
    const kind = params.get("kind") ?? "";
    const type = params.get("type") ?? "";
    const base = ledgerRows().filter((x) => inMonth(x.date, m) && (!kind || (kind === "income" ? x.kind !== "expense" : x.kind === kind)) && (!type || x.type === type));
    const rows = searched(base, q, (x) => [x.detail, x.note, x.reference, x.type, x.periodMonth]);
    const liveRows = rows.filter((x) => !x.voided);
    const income = amt(liveRows.reduce((n, x) => n + cents(x.in), 0));
    const outgoings = amt(liveRows.reduce((n, x) => n + cents(x.out), 0));
    const offset = Number(params.get("offset") ?? 0);
    const limit = Number(params.get("limit") ?? 50);
    return r({ items: rows.slice(offset, offset + limit), total: rows.length, hasMore: offset + limit < rows.length, offset, limit, totals: { income, outgoings, net: amt(cents(income) - cents(outgoings)) }, from: "", to: "" });
  }
  if (seg[0] === "cash-closings") return r(method === "GET" ? [] : { ok: true });
  if (seg[0] === "audit" && seg.length === 3) return r(audit.filter((a) => a.entity === seg[1] && a.ref === seg[2]));

  if (path === "/search") {
    const limit = Number(params.get("limit") ?? 5);
    const offset = Number(params.get("offset") ?? 0);
    const kinds = (params.get("kinds") ?? "").split(",").filter(Boolean);
    const hit = (h: Omit<SearchHit, "score">): SearchHit => ({ ...h, score: 0 });
    const pools: [SearchKind, SearchHit[], (h: SearchHit) => (string | undefined)[]][] = [
      ["trainee", trainees.map((t) => hit({ kind: "trainee", id: t.id, label: t.name, sub: t.phone, date: t.createdAt, flag: t.archivedAt ? "archived" : t.status === "inactive" ? "inactive" : undefined })), (h) => [h.label, h.sub]],
      ["session", sessions.map((s) => hit({ kind: "session", id: s.id, label: s.title, sub: s.attendees.map((a) => a.traineeName).slice(0, 3).join(", "), date: s.start, flag: s.status === "scheduled" ? undefined : s.status, month: monthOf(s.start) })), (h) => [h.label, h.sub]],
      ["payment", payments.map((p) => hit({ kind: "payment", id: p.id, label: p.traineeName || p.type, sub: p.type, date: p.date, amount: p.amount, flag: p.voidedAt ? "void" : undefined, month: monthOf(p.date) })), (h) => [h.label, h.sub]],
      ["sale", sales.map((s) => hit({ kind: "sale", id: s.id, label: `${s.itemName} × ${s.qty}`, sub: s.traineeName || "Walk-in", date: s.date, amount: s.total, flag: s.voidedAt ? "void" : undefined, month: monthOf(s.date) })), (h) => [h.label, h.sub]],
      ["item", inventory.map((i) => hit({ kind: "item", id: i.id, label: i.name, sub: i.sku, amount: i.price, flag: i.active ? undefined : "archived" })), (h) => [h.label, h.sub]],
      ["expense", expenses.map((e) => hit({ kind: "expense", id: e.id, label: e.category, sub: e.note, date: e.date, amount: e.amount, flag: e.voidedAt ? "void" : undefined, month: monthOf(e.date) })), (h) => [h.label, h.sub]],
      ["reminder", reminders.map((x) => hit({ kind: "reminder", id: x.id, label: x.title, sub: x.relatedLabel, date: x.dueDate, flag: x.done ? "done" : undefined })), (h) => [h.label, h.sub]],
    ];
    const res: SearchResponse = { query: normalize(q ?? ""), groups: [] };
    let allTypos = true;
    let best: { score: number; corrected: string } | undefined;
    for (const [kind, pool, fields] of pools) {
      if (kinds.length && !kinds.includes(kind)) continue;
      const ranked = rank(pool, q ?? "", fields);
      if (!ranked.length || !(q ?? "").trim()) continue;
      for (const x of ranked) {
        if (!x.typo) allTypos = false;
        if (!best || x.score > best.score) best = x;
      }
      res.groups.push({ kind, total: ranked.length, hasMore: offset + limit < ranked.length, items: ranked.slice(offset, offset + limit).map((x) => ({ ...x.item, score: x.score, typo: x.typo })) });
    }
    if (best && allTypos && best.corrected) res.didYouMean = best.corrected;
    return r(res);
  }

  // /trainees
  if (seg[0] === "trainees") {
    if (seg.length === 1) {
      if (method === "POST") {
        if (!String(body.name ?? "").trim()) fail(400, "VALIDATION", "name is required", "name");
        const t: Trainee = { id: genId(), status: "active", monthlyFee: 0, ...body, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
        trainees.push(t);
        return r(t);
      }
      const arch = params.get("archived");
      const base = trainees.filter((t) => (arch === "include" || arch === "1" ? true : arch === "only" ? !!t.archivedAt : !t.archivedAt));
      return r(searched(base.sort((a, b) => a.name.localeCompare(b.name)), q, (t) => [t.name, t.phone]));
    }
    const id = seg[1];
    const t = trainees.find((x) => x.id === id);
    if (seg[2] === "session-plans") {
      if (method === "POST") {
        if (!(body.targetCount >= 1)) fail(400, "VALIDATION", "targetCount must be at least 1", "targetCount");
        const p: StoredPlan = { id: genId(), trainee: id, traineeName: tName(id), title: body.title || `${body.targetCount} sessions`, targetCount: body.targetCount, startDate: body.startDate || TODAY, endDate: body.endDate || undefined, sessionType: body.sessionType || "", status: "active", notes: body.notes, createdAt: nowISO, updatedAt: nowISO };
        plans.push(p);
        return r(withProgress(p));
      }
      return r(plans.filter((p) => p.trainee === id).map(withProgress));
    }
    if (seg[2] === "links") return r({ payments: payments.filter((p) => p.trainee === id).length, sessions: sessions.filter((s) => s.attendees.some((a) => a.trainee === id)).length, sales: sales.filter((s) => s.trainee === id).length, charges: charges(id).length, plans: plans.filter((p) => p.trainee === id).length });
    if (seg[2] === "terms") return r([]);
    if (seg[2] === "archive" && t) { t.archivedAt = new Date().toISOString(); t.status = "inactive"; logAudit("trainee", id, "archive", {}, t, body.reason ?? ""); return r(t); }
    if (seg[2] === "unarchive" && t) { t.archivedAt = undefined; return r(t); }
    if (method === "PUT" && t) { const before = { ...t }; Object.assign(t, body, { updatedAt: new Date().toISOString() }); logAudit("trainee", id, "update", before, t); return r(t); }
    if (method === "DELETE") { rm(trainees, id); return r({ ok: true }); }
    if (!t) fail(404, "NOT_FOUND", "trainee not found");
    return r(t);
  }

  if (seg[0] === "session-plans") {
    if (seg.length === 1) {
      const ids = (params.get("trainees") ?? "").split(",").filter(Boolean);
      const st = params.get("status");
      return r(plans.filter((p) => (!ids.length || ids.includes(p.trainee)) && (!st || p.status === st)).map(withProgress));
    }
    const p = plans.find((x) => x.id === seg[1]);
    if (!p) fail(404, "NOT_FOUND", "plan not found");
    if (method === "PATCH") {
      if (body.status !== undefined || body.targetCount !== undefined || body.endDate !== undefined) needReason(body.reason, "change a session plan's target, dates or status");
      const before = { ...p };
      const { reason: _reason, ...changes } = body;
      Object.assign(p, changes, { updatedAt: new Date().toISOString() });
      logAudit("plan", p.id, "update", before, p, String(_reason ?? ""));
    }
    return r(withProgress(p));
  }

  // /sessions
  if (seg[0] === "sessions") {
    if (seg[1] === "recurring") {
      const occ = occurrences(body.weekdays ?? [], body.time ?? "18:00", body.fromDay, body.toDay ?? body.fromDay);
      if (!occ.length) fail(400, "VALIDATION", "none of the chosen weekdays fall between those dates", "weekdays");
      if (seg[2] === "preview") return r({ count: occ.length, conflicts: 0, occurrences: occ.map((s) => ({ start: s, day: studioDay(s) })) });
      const seriesId = genId();
      seriesDocs.push({ seriesId, title: body.title, type: body.type, weekdays: body.weekdays, time: body.time, fromDay: body.fromDay, toDay: body.toDay, plannedCount: occ.length, status: "active", createdAt: new Date().toISOString() });
      const made = occ.map((start) => {
        const s: Session = {
          id: genId(), title: body.title, type: body.type ?? "group", start, durationMin: body.durationMin ?? 60, location: body.location, capacity: body.capacity ?? 0, seriesId, status: "scheduled",
          attendees: (body.attendees ?? []).map((a: { trainee: string; planId?: string }) => ({ trainee: a.trainee, traineeName: tName(a.trainee), status: "booked" as const, planId: a.planId || undefined })),
          createdAt: nowISO, updatedAt: nowISO,
        };
        sessions.push(s);
        return s;
      });
      return r({ seriesId, plannedCount: made.length, created: made.length, sessions: made });
    }
    if (seg[1] === "series") {
      if (seg.length === 2) return r((params.get("ids") ?? "").split(",").filter(Boolean).map(seriesProgress));
      const id = seg[2];
      if (seg[3] === "progress") {
        const doc = seriesDocs.find((d) => d.seriesId === id);
        const occ = sessions.filter((s) => s.seriesId === id).sort((a, b) => a.start.localeCompare(b.start));
        return r({ progress: seriesProgress(id), series: { seriesId: id, ...doc }, occurrences: occ.map((s) => ({ id: s.id, start: s.start, status: s.status, booked: s.attendees.length, attendanceNeeded: s.status === "completed" && s.attendees.some((a) => a.status === "booked") })) });
      }
      needReason(body.reason, "change a series");
      return r({ ok: true, changed: 0 });
    }
    if (seg.length === 1) {
      if (method === "POST") {
        const s: Session = { id: genId(), type: "group", status: "scheduled", durationMin: 60, ...body, attendees: (body.attendees ?? []).map((a: Attendee) => ({ ...a, traineeName: tName(a.trainee), status: a.status ?? "booked" })), createdAt: nowISO, updatedAt: nowISO };
        sessions.push(s);
        return r(s);
      }
      const inPeriod = periodFilter(params);
      const tr = params.get("trainee");
      const plan = params.get("plan");
      const ser = params.get("series");
      let rows = sessions.filter((s) => (!inPeriod || inPeriod(s.start)) && (!tr || s.attendees.some((a) => a.trainee === tr)) && (!plan || s.attendees.some((a) => a.planId === plan)) && (!ser || s.seriesId === ser));
      rows = [...rows].sort((a, b) => (params.get("order") === "desc" ? b.start.localeCompare(a.start) : a.start.localeCompare(b.start)));
      return r(listOrPage(searched(rows, q, (s) => [s.title, s.attendees.map((a) => a.traineeName).join(" "), s.location, s.type]), params));
    }
    const id = seg[1];
    const s = sessions.find((x) => x.id === id);
    if (!s) fail(404, "NOT_FOUND", "session not found");
    const notStarted = () => new Date(s.start).getTime() - 30 * 60_000 > Date.now();
    if (seg[2] === "attendance") {
      if (seg[3] === "bulk") {
        if (notStarted()) fail(422, "NOT_STARTED", "this class hasn't started yet — record it once it begins", "status");
        for (const a of s.attendees) if ((body.trainees ?? []).includes(a.trainee) || (body.only && a.status === body.only)) a.status = body.status;
        return r(s);
      }
      if (body.status && body.status !== "booked" && notStarted()) fail(422, "NOT_STARTED", "this class hasn't started yet — record it once it begins", "status");
      const a = s.attendees.find((x) => x.trainee === body.trainee);
      if (a) {
        a.status = body.status ?? a.status;
        if (body.planId !== undefined) a.planId = body.planId || undefined;
      } else {
        if (s.capacity && s.attendees.length >= s.capacity) fail(409, "CAPACITY_EXCEEDED", `this class holds ${s.capacity} and ${s.attendees.length + 1} would be booked`, "attendees");
        s.attendees.push({ trainee: body.trainee, traineeName: tName(body.trainee), status: body.status ?? "booked", planId: body.planId || undefined });
      }
      return r(s);
    }
    if (method === "PUT") {
      if (body.status === "completed" && s.status !== "completed" && notStarted()) fail(422, "NOT_STARTED", "this class hasn't started yet — record it once it begins", "status");
      const { attendees, ...rest } = body;
      Object.assign(s, rest, { updatedAt: new Date().toISOString() });
      if (attendees) s.attendees = attendees.map((a: Attendee) => ({ ...a, traineeName: tName(a.trainee), status: a.status ?? "booked", planId: a.planId || undefined }));
      return r(s);
    }
    if (method === "DELETE") {
      if (s.attendees.some((a) => a.status !== "booked")) fail(409, "CONFLICT", "attendance is recorded on this class — cancel it instead");
      rm(sessions, id);
      return r({ ok: true });
    }
    return r(s);
  }

  // /payments
  if (seg[0] === "payments") {
    if (seg.length === 1) {
      if (method === "POST") {
        const amount = Number(body.amount);
        if (!(amount > 0)) fail(400, "VALIDATION", "amount must be positive", "amount");
        if (body.type === "subscription") {
          if (!body.trainee) fail(400, "VALIDATION", "pick whose dues this pays", "trainee");
          const due = subscriptions(body.periodMonth ?? MONTH).find((x) => x.trainee.id === body.trainee);
          if (!due) fail(400, "NO_CHARGE", "there are no dues for that month", "periodMonth");
          if (cents(amount) > cents(due.remaining)) fail(409, "OVERPAYMENT", `only ${due.remaining} remains for that month`, "amount", { due: due.due, paid: due.amountPaid, remaining: due.remaining });
        }
        const date = body.day ? studioInstant(body.day, "12:00").toISOString() : new Date().toISOString();
        const p: Payment = { id: genId(), type: "other", ...body, amount, date, traineeName: body.trainee ? tName(body.trainee) : undefined, createdAt: new Date().toISOString() };
        payments.unshift(p);
        logAudit("payment", p.id, "create", {}, p);
        return r(p);
      }
      const inPeriod = periodFilter(params);
      const t = params.get("trainee");
      const pm = params.get("periodMonth");
      const ty = params.get("type");
      const rows = payments.filter((p) => (!inPeriod || inPeriod(p.date)) && (!t || p.trainee === t) && (!pm || p.periodMonth === pm) && (!ty || p.type === ty)).sort((a, b) => b.date.localeCompare(a.date));
      return r(listOrPage(searched(rows, q, (p) => [p.traineeName || p.type, p.note, p.reference, p.type, p.periodMonth]), params));
    }
    if (seg[1] === "export") {
      const esc = (s: string) => (/[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s);
      const rows = payments.map((p) => [studioDay(p.date), p.traineeName ?? "", p.type, p.periodMonth ?? "", p.amount.toFixed(2), p.note ?? ""].map(esc).join(","));
      return r(["Date,Trainee,Type,Period,Amount,Note", ...rows].join("\n"));
    }
    const id = seg[1];
    const p = payments.find((x) => x.id === id);
    if (!p) fail(404, "NOT_FOUND", "payment not found");
    if (seg[2] === "receipt") {
      const due = p.type === "subscription" && p.trainee ? charges(p.trainee).find((c) => c.periodMonth === p.periodMonth) : undefined;
      return r({ studio: "Bronze Boxing Club", studioInfo: { name: "Bronze Boxing Club", currency: "$" }, number: id.slice(-8).toUpperCase(), payment: p, issued: new Date().toISOString(), void: !!p.voidedAt, method: p.method ?? "unspecified", cashDay: studioDay(p.date), timezone: "Asia/Beirut", charge: due });
    }
    if (method === "PUT") {
      if (p.voidedAt) fail(409, "VOIDED_LOCKED", "payment is voided and can no longer be changed");
      const reason = body.amount !== undefined && cents(body.amount) !== cents(p.amount) ? needReason(body.reason, "change a payment's amount") : "";
      const before = { ...p };
      const { reason: _r, day, ...rest } = body;
      Object.assign(p, rest, day ? { date: studioInstant(day, "12:00").toISOString() } : {});
      p.traineeName = p.trainee ? tName(p.trainee) : undefined;
      logAudit("payment", id, "update", before, p, reason);
      return r(p);
    }
    if (method === "DELETE" || seg[2] === "void") {
      if (p.voidedAt) fail(409, "ALREADY_VOIDED", "payment is already voided");
      const reason = needReason(body.reason ?? params.get("reason"), "void a payment");
      const before = { ...p };
      p.voidedAt = new Date().toISOString();
      p.voidReason = reason;
      logAudit("payment", id, "void", before, p, reason);
      return r({ ok: true, voided: true });
    }
    return r(p);
  }

  // /reminders
  if (seg[0] === "reminders") {
    if (seg[1] === "counts") return r(remindCounts());
    if (seg.length === 1) {
      if (method === "POST") {
        const rem: Reminder = { id: genId(), priority: "normal", done: false, ...body, dueDate: studioInstant(body.dueDay ?? TODAY).toISOString(), createdAt: new Date().toISOString() };
        reminders.push(rem);
        return r(rem);
      }
      const st = params.get("status");
      return r(reminders.filter((x) => (!st || (st === "done" ? x.done : !x.done)) && (!params.get("priority") || x.priority === params.get("priority"))));
    }
    const id = seg[1];
    const rem = reminders.find((x) => x.id === id);
    if (!rem) fail(404, "NOT_FOUND", "reminder not found");
    if (seg[2] === "snooze") { rem.snoozedUntil = body.until ?? addDays(TODAY, body.days ?? 1); return r(rem); }
    if (method === "PUT") {
      const wasDone = rem.done;
      Object.assign(rem, body);
      if (body.dueDay) { rem.dueDate = studioInstant(body.dueDay).toISOString(); rem.snoozedUntil = undefined; }
      if (!wasDone && rem.done) rem.doneAt = new Date().toISOString();
      return r(rem);
    }
    if (method === "DELETE") { rm(reminders, id); return r({ ok: true }); }
    return r(rem);
  }

  // /expenses
  if (seg[0] === "expenses") {
    if (seg.length === 1) {
      if (method === "POST") {
        const date = body.day ? studioInstant(body.day, "12:00").toISOString() : new Date().toISOString();
        const e: Expense = { id: genId(), category: "other", ...body, date, createdAt: new Date().toISOString() };
        expenses.unshift(e);
        return r(e);
      }
      const inPeriod = periodFilter(params);
      return r(expenses.filter((e) => !inPeriod || inPeriod(e.date)));
    }
    const id = seg[1];
    const e = expenses.find((x) => x.id === id);
    if (!e) fail(404, "NOT_FOUND", "expense not found");
    if (method === "PUT") {
      if (e.voidedAt) fail(409, "VOIDED_LOCKED", "expense is voided and can no longer be changed");
      const reason = body.amount !== undefined && cents(body.amount) !== cents(e.amount) ? needReason(body.reason, "change an expense's amount") : "";
      const before = { ...e };
      const { reason: _r, ...rest } = body;
      Object.assign(e, rest);
      logAudit("expense", id, "update", before, e, reason);
      return r(e);
    }
    if (method === "DELETE" || seg[2] === "void") {
      if (e.voidedAt) fail(409, "ALREADY_VOIDED", "expense is already voided");
      const reason = needReason(body.reason ?? params.get("reason"), "void an expense");
      const before = { ...e };
      e.voidedAt = new Date().toISOString();
      e.voidReason = reason;
      logAudit("expense", id, "void", before, e, reason);
      return r({ ok: true, voided: true });
    }
    return r(e);
  }

  // /inventory
  if (seg[0] === "inventory") {
    if (seg.length === 1) {
      if (method === "POST") {
        const it: InventoryItem = { id: genId(), stock: 0, price: 0, active: true, ...body, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
        inventory.push(it);
        move(it, it.stock, "opening", "Opening stock");
        return r(it);
      }
      const act = params.get("active");
      const st = params.get("stock");
      return r(inventory.filter((i) => (!act || String(i.active) === act) && (!st || (st === "short" ? !!shortage(i) : shortage(i) === st))));
    }
    const id = seg[1];
    const it = inventory.find((x) => x.id === id);
    if (!it) fail(404, "ITEM_NOT_FOUND", "item not found");
    if (seg[2] === "movements") return r(movements.filter((m) => m.item === id).sort((a, b) => b.at.localeCompare(a.at)));
    if (seg[2] === "archive" || seg[2] === "unarchive") { it.active = seg[2] === "unarchive"; return r(it); }
    if (seg[2] === "adjustments") {
      const before = { ...it };
      if (body.kind === "restock") { it.stock += body.qty; if (body.unitCost !== undefined) it.costPrice = body.unitCost; move(it, body.qty, "restock", body.reason); }
      else if (body.kind === "damage") {
        const reason = needReason(body.reason, "write off stock");
        if (body.qty > it.stock) fail(409, "INSUFFICIENT_STOCK", `not enough stock: ${it.stock} × ${it.name} left`, "qty", { available: it.stock });
        it.stock -= body.qty;
        move(it, -body.qty, "damage", reason);
      } else {
        const reason = needReason(body.reason, "correct the stock count");
        const delta = body.count - it.stock;
        it.stock = body.count;
        move(it, delta, "correction", reason);
      }
      logAudit("item", id, body.kind, before, it, body.reason ?? "");
      return r({ item: it, movement: movements[movements.length - 1] });
    }
    if (seg[2] === "sell" && method === "POST") {
      const qty = Number(body.qty ?? 1);
      if (!it.active) fail(409, "ITEM_INACTIVE", `${it.name} is archived and can't be sold`);
      if (!(qty >= 1)) fail(400, "VALIDATION", "quantity must be at least 1", "qty");
      if (qty > it.stock) fail(409, "INSUFFICIENT_STOCK", `not enough stock: ${it.stock} × ${it.name} left`, "qty", { available: it.stock });
      const unit = body.unitPrice ?? it.price;
      if (cents(unit) !== cents(it.price)) needReason(body.priceReason, "sell at a different price");
      it.stock -= qty;
      const sale: Sale = { id: genId(), item: it.id, itemName: it.name, trainee: body.trainee, traineeName: body.trainee ? tName(body.trainee) : undefined, qty, unitPrice: unit, listPrice: it.price, unitCost: it.costPrice, priceReason: body.priceReason, method: body.method, total: amt(cents(unit) * qty), date: new Date().toISOString(), createdAt: new Date().toISOString() };
      sales.unshift(sale);
      move(it, -qty, "sale", "", sale.id, "sale");
      return r(sale);
    }
    if (method === "PUT") { const { stock: _s, ...rest } = body; Object.assign(it, rest, { updatedAt: new Date().toISOString() }); return r(it); }
    if (method === "DELETE") {
      if (sales.some((s) => s.item === id) || movements.some((m) => m.item === id && m.kind !== "opening")) fail(409, "CONFLICT", `${it.name} has sales or stock history — archive it instead`);
      rm(inventory, id);
      return r({ ok: true });
    }
    return r(it);
  }

  // /sales
  if (seg[0] === "sales") {
    if (seg.length === 1) {
      const inPeriod = periodFilter(params);
      const t = params.get("trainee");
      const item = params.get("item");
      const rows = sales.filter((s) => (!inPeriod || inPeriod(s.date)) && (!t || s.trainee === t) && (!item || s.item === item)).sort((a, b) => b.date.localeCompare(a.date));
      return r(listOrPage(searched(rows, q, (s) => [s.itemName, s.traineeName]), params));
    }
    const id = seg[1];
    const sale = sales.find((x) => x.id === id);
    if (!sale) fail(404, "NOT_FOUND", "sale not found");
    const it = inventory.find((x) => x.id === sale.item);
    if (seg[2] === "returns") {
      if (method !== "POST") return r(returns.filter((x) => x.sale === id));
      if (sale.voidedAt) fail(409, "VOIDED_LOCKED", "this sale is voided — nothing left to return");
      const reason = needReason(body.reason, "record a return");
      const left = sale.qty - (sale.returnedQty ?? 0);
      if (body.qty > left) fail(400, "RETURN_EXCEEDS_SALE", `only ${left} of this sale can still come back`, "qty");
      const refund = body.amount ?? amt(cents(sale.unitPrice) * body.qty);
      const rt: SaleReturn = { id: genId(), sale: id, item: sale.item, itemName: sale.itemName, qty: body.qty, amount: refund, date: body.day ? studioInstant(body.day, "12:00").toISOString() : new Date().toISOString(), reason, createdAt: new Date().toISOString(), actor: "demo" };
      returns.push(rt);
      sale.returnedQty = (sale.returnedQty ?? 0) + body.qty;
      sale.returnedTotal = amt(cents(sale.returnedTotal ?? 0) + cents(refund));
      if (it) { it.stock += body.qty; move(it, body.qty, "return", reason, rt.id, "return"); }
      return r({ return: rt, sale, restocked: it ? body.qty : 0 });
    }
    if (seg[2] === "receipt") {
      const mine = returns.filter((x) => x.sale === id);
      return r({ studioInfo: { name: "Bronze Boxing Club", currency: "$" }, number: `S-${id.slice(-8).toUpperCase()}`, sale, returns: mine, net: amt(cents(sale.total) - cents(sale.returnedTotal ?? 0)), issued: new Date().toISOString(), void: !!sale.voidedAt, method: sale.method ?? "cash", cashDay: studioDay(sale.date), timezone: "Asia/Beirut" });
    }
    if (method === "DELETE" || seg[2] === "void") {
      if (sale.voidedAt) fail(409, "ALREADY_VOIDED", "sale is already voided");
      const reason = needReason(body.reason ?? params.get("reason"), "void a sale");
      const before = { ...sale };
      const back = sale.qty - (sale.returnedQty ?? 0);
      if (it && back > 0) { it.stock += back; move(it, back, "sale_void", reason, id, "sale"); }
      sale.voidedAt = new Date().toISOString();
      sale.voidReason = reason;
      logAudit("sale", id, "void", before, sale, reason);
      return r({ ok: true, voided: true, restocked: it ? back : 0 });
    }
    if (method === "PUT") {
      if (sale.voidedAt) fail(409, "VOIDED_LOCKED", "sale is voided and can no longer be corrected");
      const before = { ...sale };
      let reason = "";
      if (typeof body.qty === "number" && body.qty !== sale.qty) {
        reason = needReason(body.reason, "change a sale's quantity");
        const delta = body.qty - sale.qty;
        if (it) {
          if (delta > it.stock) fail(409, "INSUFFICIENT_STOCK", `not enough stock: ${it.stock} × ${it.name} left`, "qty");
          it.stock -= delta;
          move(it, -delta, "sale_edit", reason, id, "sale");
        }
        sale.qty = body.qty;
        sale.total = amt(cents(sale.unitPrice) * body.qty);
      }
      if (body.trainee !== undefined) { sale.trainee = body.trainee || undefined; sale.traineeName = body.trainee ? tName(body.trainee) : undefined; }
      logAudit("sale", id, "update", before, sale, reason);
      return r(sale);
    }
    return r(sale);
  }

  // Fallback: an endpoint the demo doesn't model yet answers harmlessly.
  return r({ ok: true });
}
