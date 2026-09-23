// Demo / mock data layer for client presentations.
//
// When enabled (production builds with no backend, e.g. the Vercel preview),
// the API client short-circuits here instead of hitting /api. Data lives in
// mutable module arrays so the demo is interactive in-session: toggling a
// reminder, selling an item, adding a payment all reflect immediately. A page
// reload resets to this seed. Local `npm run dev` keeps using the real Go API.

import { rank, normalize } from "./fuzzy";
import type {
  Trainee, Session, Payment, Reminder, Expense, InventoryItem, Sale,
  SubStatus, Dashboard, Financials, SearchResponse, SearchHit, SearchKind, Attendee, AuditEntry, LedgerRow,
} from "./types";

// Demo on in production unless explicitly disabled; off in dev unless forced.
export const isDemo =
  import.meta.env.VITE_DEMO === "true" ||
  (import.meta.env.PROD && import.meta.env.VITE_DEMO !== "false");

const now = new Date();
const iso = (dayOffset: number, h = 9, m = 0): string => {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + dayOffset, h, m, 0, 0);
  return d.toISOString();
};
const nowISO = now.toISOString();
const dayKey = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const dayOf = (offset: number) => dayKey(new Date(now.getFullYear(), now.getMonth(), now.getDate() + offset));
const MONTH = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
let seq = 1000;
const genId = () => `demo-${++seq}`;

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
const tName = (id: string) => trainees.find((t) => t.id === id)?.name ?? "";

// ── Sessions (fill the current month so the calendar shows activity) ─────────
const groupRoster: Attendee[] = [
  { trainee: "t1", traineeName: "Karim Haddad", status: "booked" },
  { trainee: "t2", traineeName: "Rami Khoury", status: "booked" },
  { trainee: "t3", traineeName: "Jad Saliba", status: "booked" },
  { trainee: "t4", traineeName: "Nour Aoun", status: "booked" },
  { trainee: "t7", traineeName: "Lara Fares", status: "booked" },
];
const sessions: Session[] = [];
(() => {
  const y = now.getFullYear();
  const mo = now.getMonth();
  const daysInMonth = new Date(y, mo + 1, 0).getDate();
  for (let d = 1; d <= daysInMonth; d++) {
    const date = new Date(y, mo, d);
    const wd = date.getDay();
    const past = date < new Date(now.getFullYear(), now.getMonth(), now.getDate());
    if ([1, 3, 5].includes(wd)) {
      const start = new Date(y, mo, d, 18, 0).toISOString();
      sessions.push({
        id: `s-grp-${d}`, title: "Evening Group Class", type: "group", start, durationMin: 60,
        location: "Main floor", capacity: 12, status: past ? "completed" : "scheduled",
        attendees: groupRoster.map((a, i) => ({ ...a, status: past ? (i === 1 ? "no_show" : "attended") : "booked" })),
        createdAt: nowISO, updatedAt: nowISO,
      });
    }
    if (wd === 2) {
      const start = new Date(y, mo, d, 17, 0).toISOString();
      sessions.push({
        id: `s-pvt-${d}`, title: "Private — Jad", type: "private", start, durationMin: 45,
        location: "Ring 1", fee: 35, status: past ? "completed" : "scheduled",
        attendees: [{ trainee: "t3", traineeName: "Jad Saliba", status: past ? "attended" : "booked" }],
        createdAt: nowISO, updatedAt: nowISO,
      });
    }
  }
  // Guarantee something today for the schedule view.
  sessions.push({
    id: "s-today-am", title: "Morning Conditioning", type: "group", start: iso(0, 7, 30), durationMin: 50,
    location: "Main floor", capacity: 10, status: "scheduled",
    attendees: [
      { trainee: "t4", traineeName: "Nour Aoun", status: "booked" },
      { trainee: "t5", traineeName: "Tarek Mansour", status: "booked" },
      { trainee: "t6", traineeName: "Sami Dabboussi", status: "booked" },
    ], createdAt: nowISO, updatedAt: nowISO,
  });
})();

// ── Payments (this month, crew fees + sessions) ──────────────────────────────
const payments: Payment[] = [
  { id: "p1", trainee: "t1", traineeName: "Karim Haddad", amount: 120, type: "subscription", periodMonth: MONTH, date: iso(-2, 10), createdAt: nowISO },
  { id: "p2", trainee: "t4", traineeName: "Nour Aoun", amount: 80, type: "subscription", periodMonth: MONTH, date: iso(-5, 11), createdAt: nowISO },
  { id: "p3", trainee: "t7", traineeName: "Lara Fares", amount: 90, type: "subscription", periodMonth: MONTH, date: iso(-6, 9), createdAt: nowISO },
  { id: "p4", trainee: "t2", traineeName: "Rami Khoury", amount: 50, type: "subscription", periodMonth: MONTH, date: iso(-1, 18), note: "Half now, half later", createdAt: nowISO },
  { id: "p5", trainee: "t3", traineeName: "Jad Saliba", amount: 35, type: "private", date: iso(-3, 16), createdAt: nowISO },
  { id: "p6", trainee: "t6", traineeName: "Sami Dabboussi", amount: 20, type: "dropin", date: iso(-4, 19), createdAt: nowISO },
];

// ── Reminders ────────────────────────────────────────────────────────────────
const reminders: Reminder[] = [
  { id: "r1", title: "Fix the speed-bag bracket", dueDay: dayOf(-1), dueDate: iso(-1, 0), priority: "high", done: false, createdAt: nowISO },
  { id: "r2", title: "Call Rami about his schedule", dueDay: dayOf(0), dueDate: iso(0, 0), priority: "normal", done: false, createdAt: nowISO },
  { id: "r3", title: "Order new gloves (size M)", dueDay: dayOf(2), dueDate: iso(2, 0), priority: "normal", done: false, createdAt: nowISO },
  { id: "r4", title: "Renew gym insurance", dueDay: dayOf(4), dueDate: iso(4, 0), priority: "high", done: false, createdAt: nowISO },
  { id: "r5", title: "Wipe down the ring canvas", dueDay: dayOf(-2), dueDate: iso(-2, 0), priority: "low", done: true, createdAt: nowISO },
];

// ── Expenses (this month) ─────────────────────────────────────────────────────
const expenses: Expense[] = [
  { id: "e1", amount: 800, category: "rent", note: "Gym space — monthly", date: iso(-20, 10), createdAt: nowISO },
  { id: "e2", amount: 240, category: "equipment", note: "Two pairs of focus mitts", date: iso(-5, 12), createdAt: nowISO },
  { id: "e3", amount: 95, category: "utilities", note: "Electricity", date: iso(-3, 14), createdAt: nowISO },
  { id: "e4", amount: 60, category: "supplies", note: "Cleaning + hand tape", date: iso(-8, 12), createdAt: nowISO },
];

// ── Inventory + sales ─────────────────────────────────────────────────────────
const inventory: InventoryItem[] = [
  { id: "i1", name: "Hand Wraps (4.5m)", sku: "WRAP-45", stock: 24, price: 8, costPrice: 4, lowStockThreshold: 6, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i2", name: "Boxing Gloves 12oz", sku: "GLV-12", stock: 3, price: 45, costPrice: 28, lowStockThreshold: 3, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i3", name: "Water Bottle 750ml", sku: "H2O-750", stock: 30, price: 5, costPrice: 2, lowStockThreshold: 10, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i4", name: "Bronze Boxing Tee", sku: "TEE-BB", stock: 15, price: 20, costPrice: 9, lowStockThreshold: 5, active: true, createdAt: nowISO, updatedAt: nowISO },
  { id: "i5", name: "Jump Rope", sku: "ROPE", stock: 2, price: 12, costPrice: 5, lowStockThreshold: 4, active: true, createdAt: nowISO, updatedAt: nowISO },
];
const sales: Sale[] = [
  { id: "sa1", item: "i1", itemName: "Hand Wraps (4.5m)", trainee: "t1", traineeName: "Karim Haddad", qty: 1, unitPrice: 8, total: 8, date: iso(-1, 17), createdAt: nowISO },
  { id: "sa2", item: "i4", itemName: "Bronze Boxing Tee", trainee: "t7", traineeName: "Lara Fares", qty: 2, unitPrice: 20, total: 40, date: iso(-3, 18), createdAt: nowISO },
  { id: "sa3", item: "i3", itemName: "Water Bottle 750ml", qty: 3, unitPrice: 5, total: 15, date: iso(-4, 19), createdAt: nowISO },
];

// ── Audit trail ───────────────────────────────────────────────────────────────
// The demo records real history for what you actually do in-session, so the
// History panel shows the same thing it would against the live API — rather
// than fabricated entries that were never true.
const audit: AuditEntry[] = [];
function logAudit(entity: AuditEntry["entity"], ref: string, action: AuditEntry["action"], before: unknown, after: unknown): void {
  audit.unshift({
    id: genId(), entity, ref, action,
    before: { ...(before as Record<string, unknown>) },
    after: { ...(after as Record<string, unknown>) },
    at: new Date().toISOString(),
  });
}

// ── Computed (mirror the Go backend) ─────────────────────────────────────────
// Voided records stay in lists but never count toward money totals.
const live = <T extends { voidedAt?: string }>(arr: T[]) => arr.filter((x) => !x.voidedAt);
function subscriptions(month = MONTH): SubStatus[] {
  return trainees
    .filter((t) => t.status === "active" && t.monthlyFee > 0)
    .map((t) => {
      const mine = live(payments).filter((p) => p.type === "subscription" && p.periodMonth === month && p.trainee === t.id);
      const paid = Math.round(mine.reduce((s, p) => s + p.amount, 0) * 100) / 100;
      const state: SubStatus["state"] = paid >= t.monthlyFee ? "paid" : paid > 0 ? "partial" : month > MONTH ? "upcoming" : "unpaid";
      const last = mine.map((p) => p.date).sort().pop();
      return {
        trainee: t, chargeId: `c-${t.id}-${month}`, periodMonth: month, due: t.monthlyFee, amountPaid: paid,
        remaining: Math.max(0, Math.round((t.monthlyFee - paid) * 100) / 100), state, source: "normal" as const,
        paymentCount: mine.length, lastPaymentDate: last,
      };
    });
}
function financials(): Financials {
  let income = 0;
  const byType: Record<string, number> = {};
  for (const p of live(payments)) { income += p.amount; byType[p.type] = (byType[p.type] ?? 0) + p.amount; }
  for (const s of live(sales)) { if (!s.paymentId) { income += s.total; byType.sale = (byType.sale ?? 0) + s.total; } }
  let outgoings = 0;
  const byCategory: Record<string, number> = {};
  for (const e of live(expenses)) { outgoings += e.amount; byCategory[e.category] = (byCategory[e.category] ?? 0) + e.amount; }
  const byMethod: Record<string, number> = {};
  for (const p of live(payments)) byMethod[p.method ?? "unspecified"] = (byMethod[p.method ?? "unspecified"] ?? 0) + p.amount;
  for (const s of live(sales)) if (!s.paymentId) byMethod.cash = (byMethod.cash ?? 0) + s.total;
  return {
    income, outgoings, net: income - outgoings, byType, byCategory, byMethod,
    counts: { payment: live(payments).length, sale: live(sales).length, expense: live(expenses).length },
    from: iso(-now.getDate() + 1, 0), to: iso(31, 0),
    previous: { income: 0, outgoings: 0, net: 0, from: iso(-now.getDate() - 30, 0), to: iso(-now.getDate() + 1, 0) },
  };
}
function ledgerRows(): LedgerRow[] {
  const rows: LedgerRow[] = [];
  for (const p of payments) rows.push({ kind: "payment", id: p.id, date: p.date, day: p.date.slice(0, 10), detail: p.traineeName ?? "—", type: p.type, method: p.method, reference: p.reference, periodMonth: p.periodMonth, trainee: p.trainee, note: p.note, in: p.amount, out: 0, voided: !!p.voidedAt, voidReason: p.voidReason });
  for (const s of sales) if (!s.paymentId) rows.push({ kind: "sale", id: s.id, date: s.date, day: s.date.slice(0, 10), detail: `${s.itemName} × ${s.qty}`, type: "sale", in: s.total, out: 0, voided: !!s.voidedAt, voidReason: s.voidReason });
  for (const e of expenses) rows.push({ kind: "expense", id: e.id, date: e.date, day: e.date.slice(0, 10), detail: e.category, type: e.category, note: e.note, in: 0, out: e.amount, voided: !!e.voidedAt, voidReason: e.voidReason });
  return rows.sort((a, b) => b.date.localeCompare(a.date));
}
function dashboard(): Dashboard {
  const todayStr = now.toDateString();
  const todaySessions = sessions.filter((s) => new Date(s.start).toDateString() === todayStr);
  const subs = subscriptions();
  const overdue = subs.filter((s) => s.state === "partial" || s.state === "unpaid");
  const weekEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 7);
  const weekReminders = reminders.filter((r) => { const d = new Date(r.dueDate); return d >= new Date(todayStr) && d < weekEnd; });
  return {
    today: nowISO, month: MONTH,
    monthRevenue: live(payments).reduce((s, p) => s + p.amount, 0),
    activeTrainees: trainees.filter((t) => t.status === "active").length,
    overdueCount: overdue.length,
    partialCount: overdue.filter((s) => s.state === "partial").length,
    unpaidCount: overdue.filter((s) => s.state === "unpaid").length,
    outstanding: overdue.reduce((sum, s) => sum + s.remaining, 0),
    todaySessions, weekReminders, overdueSubscriptions: overdue,
  };
}

// ── Router ───────────────────────────────────────────────────────────────────
function rm<T extends { id: string }>(arr: T[], id: string): void {
  const i = arr.findIndex((x) => x.id === id);
  if (i >= 0) arr.splice(i, 1);
}

export function demoResolve<T>(rawPath: string, method: string, bodyStr?: BodyInit | null): T {
  const [path, query = ""] = rawPath.split("?");
  const params = new URLSearchParams(query);
  const body = typeof bodyStr === "string" && bodyStr ? JSON.parse(bodyStr) : {};
  const seg = path.split("/").filter(Boolean); // e.g. ["trainees","t1"]
  const r = (v: unknown): T => v as unknown as T;

  // Collection-level
  if (path === "/health") return r({ status: "ok", db: true });
  if (path === "/dashboard") return r(dashboard());
  if (path === "/financials") return r(financials());
  if (path === "/financials/export") {
    const esc = (s: string) => (/[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s);
    const rows: string[] = [];
    for (const p of payments) rows.push([p.date.slice(0, 10), "payment", p.traineeName ?? "", p.type, p.note ?? "", p.amount.toFixed(2), "0.00", p.voidedAt ? "VOID" : ""].map(esc).join(","));
    for (const s of sales) rows.push([s.date.slice(0, 10), "sale", s.itemName, "sale", "", s.total.toFixed(2), "0.00", s.voidedAt ? "VOID" : ""].map(esc).join(","));
    for (const e of expenses) rows.push([e.date.slice(0, 10), "expense", e.category, "expense", e.note ?? "", "0.00", e.amount.toFixed(2), e.voidedAt ? "VOID" : ""].map(esc).join(","));
    const f = financials();
    return r(["Date,Kind,Detail,Type,Note,In,Out,Status", ...rows.sort(), "", `,,,,Total income,${f.income.toFixed(2)},,`, `,,,,Total outgoings,,${f.outgoings.toFixed(2)},`, `,,,,Net,${f.net.toFixed(2)},,`].join("\n"));
  }
  if (path === "/subscriptions") return r(subscriptions(params.get("m") ?? MONTH));
  if (seg[0] === "subscription-charges") return r({ ok: true });
  if (path === "/ledger") {
    const kind = params.get("kind") ?? "";
    const type = params.get("type") ?? "";
    const q = (params.get("q") ?? "").toLowerCase();
    const rows = ledgerRows().filter((x) =>
      (!kind || (kind === "income" ? x.kind !== "expense" : x.kind === kind)) &&
      (!type || x.type === type) &&
      (!q || `${x.detail} ${x.note ?? ""} ${x.reference ?? ""}`.toLowerCase().includes(q)));
    const liveRows = rows.filter((x) => !x.voided);
    const income = liveRows.reduce((n, x) => n + x.in, 0);
    const outgoings = liveRows.reduce((n, x) => n + x.out, 0);
    const offset = Number(params.get("offset") ?? 0);
    const limit = Number(params.get("limit") ?? 50);
    return r({ items: rows.slice(offset, offset + limit), total: rows.length, hasMore: offset + limit < rows.length, offset, limit, totals: { income, outgoings, net: income - outgoings }, from: "", to: "" });
  }
  if (seg[0] === "cash-closings") return r(method === "GET" ? [] : { ok: true });
  if (seg[0] === "audit" && seg.length === 3) {
    return r(audit.filter((a) => a.entity === seg[1] && a.ref === seg[2]));
  }

  if (path === "/search") {
    // Same matcher and response shape as the API.
    const q = params.get("q") ?? "";
    const limit = Number(params.get("limit") ?? 5);
    const offset = Number(params.get("offset") ?? 0);
    const kinds = (params.get("kinds") ?? "").split(",").filter(Boolean);
    const pools: [SearchKind, SearchHit[], (h: SearchHit) => (string | undefined)[]][] = [
      ["trainee", trainees.map((t) => ({ kind: "trainee", id: t.id, label: t.name, sub: t.phone, date: t.createdAt, score: 0 })), (h) => [h.label, h.sub]],
      ["session", sessions.map((s) => ({ kind: "session", id: s.id, label: s.title, sub: s.attendees.map((a) => a.traineeName).join(", "), date: s.start, score: 0 })), (h) => [h.label, h.sub]],
      ["payment", payments.map((p) => ({ kind: "payment", id: p.id, label: p.traineeName || p.type, sub: p.type, date: p.date, amount: p.amount, flag: p.voidedAt ? "void" : undefined, score: 0 })), (h) => [h.label, h.sub]],
      ["item", inventory.map((i) => ({ kind: "item", id: i.id, label: i.name, sub: i.sku, amount: i.price, score: 0 })), (h) => [h.label, h.sub]],
    ];
    const res: SearchResponse = { query: normalize(q), groups: [] };
    let allTypos = true;
    let best: { score: number; corrected: string } | undefined;
    for (const [kind, pool, fields] of pools) {
      if (kinds.length && !kinds.includes(kind)) continue;
      const ranked = rank(pool, q, fields);
      if (!ranked.length) continue;
      for (const x of ranked) {
        if (!x.typo) allTypos = false;
        if (!best || x.score > best.score) best = x;
      }
      res.groups.push({
        kind, total: ranked.length, hasMore: offset + limit < ranked.length,
        items: ranked.slice(offset, offset + limit).map((x) => ({ ...x.item, score: x.score, typo: x.typo })),
      });
    }
    if (best && allTypos && best.corrected) res.didYouMean = best.corrected;
    return r(res);
  }

  // /trainees
  if (seg[0] === "trainees") {
    if (seg.length === 1) {
      if (method === "POST") { const t: Trainee = { id: genId(), status: "active", monthlyFee: 0, ...body, createdAt: nowISO, updatedAt: nowISO }; trainees.push(t); return r(t); }
      const q = (params.get("q") ?? "").toLowerCase();
      return r(q ? trainees.filter((t) => t.name.toLowerCase().includes(q)) : trainees);
    }
    const id = seg[1];
    if (method === "PUT") { const t = trainees.find((x) => x.id === id); if (t) Object.assign(t, body, { updatedAt: nowISO }); return r(t); }
    if (method === "DELETE") { rm(trainees, id); return r({ ok: true }); }
    return r(trainees.find((t) => t.id === id));
  }

  // /sessions
  if (seg[0] === "sessions") {
    if (seg[1] === "recurring") { return r({ created: 0, seriesId: genId(), sessions: [] }); }
    if (seg.length === 1) {
      if (method === "POST") { const s: Session = { id: genId(), type: "group", status: "scheduled", durationMin: 60, attendees: [], ...body, createdAt: nowISO, updatedAt: nowISO }; sessions.push(s); return r(s); }
      return r(sessions);
    }
    const id = seg[1];
    if (seg[2] === "attendance" && method === "PATCH") {
      const s = sessions.find((x) => x.id === id);
      if (s) { const a = s.attendees.find((x) => x.trainee === body.trainee); if (a) a.status = body.status; else s.attendees.push({ trainee: body.trainee, traineeName: tName(body.trainee), status: body.status }); }
      return r(s);
    }
    if (method === "PUT") { const s = sessions.find((x) => x.id === id); if (s) Object.assign(s, body, { updatedAt: nowISO }); return r(s); }
    if (method === "DELETE") { rm(sessions, id); return r({ ok: true }); }
    return r(sessions.find((s) => s.id === id));
  }

  // /payments
  if (seg[0] === "payments") {
    if (seg.length === 1) {
      if (method === "POST") { const p: Payment = { id: genId(), type: "other", date: nowISO, ...body, traineeName: body.trainee ? tName(body.trainee) : undefined, createdAt: nowISO }; payments.unshift(p); return r(p); }
      const t = params.get("trainee");
      const pm = params.get("periodMonth");
      return r(payments.filter((p) => (!t || p.trainee === t) && (!pm || p.periodMonth === pm)));
    }
    if (seg[1] === "export") {
      const esc = (s: string) => (/[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s);
      const rows = payments.map((p) => [p.date.slice(0, 10), p.traineeName ?? "", p.type, p.periodMonth ?? "", p.amount.toFixed(2), p.note ?? ""].map(esc).join(","));
      return r(["Date,Trainee,Type,Period,Amount,Note", ...rows].join("\n"));
    }
    const id = seg[1];
    if (seg[2] === "receipt") {
      const p = payments.find((x) => x.id === id);
      return r({ studio: "Bronze Boxing Club", studioInfo: { name: "Bronze Boxing Club", currency: "$" }, number: id.slice(-8).toUpperCase(), payment: p, issued: nowISO, void: !!p?.voidedAt, method: p?.method ?? "unspecified", cashDay: p?.date.slice(0, 10) ?? "", timezone: "Asia/Beirut" });
    }
    if (method === "GET" && seg.length === 2) return r(payments.find((x) => x.id === id));
    if (method === "PUT") { const p = payments.find((x) => x.id === id); if (p && !p.voidedAt) { const before = { ...p }; Object.assign(p, body); p.traineeName = body.trainee ? tName(body.trainee) : undefined; logAudit("payment", id, "update", before, p); } return r(p); }
    if (method === "DELETE" || seg[2] === "void") { const p = payments.find((x) => x.id === id); if (p && !p.voidedAt) { const before = { ...p }; p.voidedAt = nowISO; p.voidReason = body.reason ?? params.get("reason") ?? ""; logAudit("payment", id, "void", before, p); } return r({ ok: true, voided: true }); }
  }

  // /reminders
  if (seg[0] === "reminders") {
    if (seg.length === 1) {
      if (method === "POST") { const rem: Reminder = { id: genId(), priority: "normal", done: false, ...body, createdAt: nowISO }; reminders.push(rem); return r(rem); }
      return r(reminders);
    }
    const id = seg[1];
    if (method === "PUT") { const rem = reminders.find((x) => x.id === id); if (rem) Object.assign(rem, body); return r(rem); }
    if (method === "DELETE") { rm(reminders, id); return r({ ok: true }); }
  }

  // /expenses
  if (seg[0] === "expenses") {
    if (seg.length === 1) {
      if (method === "POST") { const e: Expense = { id: genId(), category: "other", date: nowISO, ...body, createdAt: nowISO }; expenses.unshift(e); return r(e); }
      return r(expenses);
    }
    const id = seg[1];
    if (method === "PUT") { const e = expenses.find((x) => x.id === id); if (e && !e.voidedAt) { const before = { ...e }; Object.assign(e, body); logAudit("expense", id, "update", before, e); } return r(e); }
    if (method === "DELETE" || seg[2] === "void") { const e = expenses.find((x) => x.id === id); if (e && !e.voidedAt) { const before = { ...e }; e.voidedAt = nowISO; e.voidReason = body.reason ?? params.get("reason") ?? ""; logAudit("expense", id, "void", before, e); } return r({ ok: true, voided: true }); }
  }

  // /inventory + /sales
  if (seg[0] === "inventory") {
    if (seg.length === 1) {
      if (method === "POST") { const it: InventoryItem = { id: genId(), stock: 0, price: 0, active: true, ...body, createdAt: nowISO, updatedAt: nowISO }; inventory.push(it); return r(it); }
      return r(inventory);
    }
    const id = seg[1];
    if (seg[2] === "sell" && method === "POST") {
      const it = inventory.find((x) => x.id === id);
      const qty = body.qty ?? 1;
      if (it) {
        it.stock = Math.max(0, it.stock - qty);
        const sale: Sale = { id: genId(), item: it.id, itemName: it.name, trainee: body.trainee, traineeName: body.trainee ? tName(body.trainee) : undefined, qty, unitPrice: it.price, total: it.price * qty, date: nowISO, createdAt: nowISO };
        sales.unshift(sale);
        return r(sale);
      }
      return r({});
    }
    if (method === "PUT") { const it = inventory.find((x) => x.id === id); if (it) Object.assign(it, body, { updatedAt: nowISO }); return r(it); }
    if (method === "DELETE") { rm(inventory, id); return r({ ok: true }); }
  }
  if (seg[0] === "sales") {
    if (seg.length === 1) return r(sales);
    const id = seg[1];
    if (method === "DELETE" || seg[2] === "void") {
      const sale = sales.find((x) => x.id === id);
      if (sale && !sale.voidedAt) { const before = { ...sale }; const it = inventory.find((x) => x.id === sale.item); if (it) it.stock += sale.qty; sale.voidedAt = nowISO; sale.voidReason = body.reason ?? params.get("reason") ?? ""; logAudit("sale", id, "void", before, sale); }
      return r({ ok: true, voided: true, restocked: sale?.qty ?? 0 });
    }
    if (method === "PUT") {
      const sale = sales.find((x) => x.id === id);
      if (sale && !sale.voidedAt) {
        const before = { ...sale };
        if (typeof body.qty === "number") {
          const it = inventory.find((x) => x.id === sale.item);
          if (it) it.stock = Math.max(0, it.stock - (body.qty - sale.qty));
          sale.qty = body.qty;
          sale.total = sale.unitPrice * body.qty;
        }
        if (body.trainee !== undefined) { sale.trainee = body.trainee || undefined; sale.traineeName = body.trainee ? tName(body.trainee) : undefined; }
        logAudit("sale", id, "update", before, sale);
      }
      return r(sale);
    }
    return r(sales.find((s) => s.id === id)); // GET /sales/:id
  }

  // Fallback
  return r({ ok: true });
}
