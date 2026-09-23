import type { SubState, SubStatus } from "./types";

// One vocabulary for dues states on every screen. Partial is its own state —
// never folded into paid or unpaid — and unverified (imported) rows say so.

export const STATE_LABEL: Record<SubState, string> = {
  paid: "Paid",
  partial: "Partial",
  unpaid: "Unpaid",
  waived: "Waived",
  unverified: "Needs review",
  upcoming: "Upcoming",
};

export const STATE_TONE: Record<SubState, "paid" | "partial" | "overdue" | "neutral" | "info"> = {
  paid: "paid",
  partial: "partial",
  unpaid: "overdue",
  waived: "neutral",
  unverified: "info",
  upcoming: "neutral",
};

/** Sort order for a dues roster: what needs collecting first. */
const RANK: Record<SubState, number> = { partial: 0, unpaid: 1, unverified: 2, upcoming: 3, paid: 4, waived: 5 };

export function byUrgency(a: SubStatus, b: SubStatus): number {
  return RANK[a.state] - RANK[b.state] || a.trainee.name.localeCompare(b.trainee.name);
}

/** Can more money be collected toward this row right now? */
export function collectable(s: SubStatus): boolean {
  return (s.state === "partial" || s.state === "unpaid" || s.state === "upcoming") && s.remaining > 0;
}

/** Deep link that opens the payment form pre-filled to settle a row. */
export function collectLink(s: SubStatus, back?: string): string {
  const q = new URLSearchParams({
    trainee: s.trainee.id,
    type: "subscription",
    periodMonth: s.periodMonth,
    amount: String(s.remaining),
  });
  if (back) q.set("back", back);
  return `/payments/new?${q.toString()}`;
}

export interface DuesCounts {
  paid: number;
  partial: number;
  unpaid: number;
  unverified: number;
  other: number;
  /** Still owed across partial + unpaid accounts. */
  outstanding: number;
}

export function countDues(rows: SubStatus[]): DuesCounts {
  const c: DuesCounts = { paid: 0, partial: 0, unpaid: 0, unverified: 0, other: 0, outstanding: 0 };
  for (const r of rows) {
    if (r.state === "paid") c.paid++;
    else if (r.state === "partial") c.partial++;
    else if (r.state === "unpaid") c.unpaid++;
    else if (r.state === "unverified") c.unverified++;
    else c.other++;
    if (r.state === "partial" || r.state === "unpaid") c.outstanding += r.remaining;
  }
  c.outstanding = Math.round(c.outstanding * 100) / 100;
  return c;
}
