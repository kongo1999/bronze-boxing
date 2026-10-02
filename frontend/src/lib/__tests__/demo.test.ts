import { describe, expect, it } from "vitest";
import { demoResolve } from "../demo";
import { isApiError } from "../api-error";
import { addDays, currentMonth, mondayOf, shiftMonthKey, todayKey } from "../studio";
import type { Financials, MonthlyReport, Page, Payment, SearchResponse, SeriesDetail, SessionPlan, SubStatus, PaymentPromise, TrialLead, FollowUp, ProgressNote, MonthReview } from "../types";

// The demo answers with the live API's shapes and rules, so a preview can't
// teach anyone something the real app won't do.
const get = <T>(p: string) => demoResolve<T>(p, "GET", null);
const post = <T>(p: string, body: unknown) => demoResolve<T>(p, "POST", JSON.stringify(body));
function code(fn: () => unknown): string | undefined {
  try {
    fn();
  } catch (e) {
    return isApiError(e) ? e.code : "not an ApiError";
  }
  return undefined;
}
const month = currentMonth();

describe("demo contract", () => {
  it("keeps a partial payer partial, refuses overpayment, then settles", () => {
    const rami = () => get<SubStatus[]>(`/subscriptions?m=${month}`).find((s) => s.trainee.id === "t2")!;
    expect(rami().state).toBe("partial");
    expect(rami().remaining).toBe(50);
    expect(code(() => post("/payments", { trainee: "t2", amount: 60, type: "subscription", periodMonth: month }))).toBe("OVERPAYMENT");
    post("/payments", { trainee: "t2", amount: 50, type: "subscription", periodMonth: month });
    expect(rami().state).toBe("paid");
  });

  it("needs a reason to void, and a voided payment stops counting", () => {
    const p = post<Payment>("/payments", { trainee: "t6", amount: 15, type: "dropin" });
    const before = get<Financials>(`/financials?m=${month}`).income;
    expect(code(() => post(`/payments/${p.id}/void`, {}))).toBe("REASON_REQUIRED");
    post(`/payments/${p.id}/void`, { reason: "entered twice" });
    expect(get<Financials>(`/financials?m=${month}`).income).toBe(before - 15);
  });

  it("creates a recurring series with its planned count", () => {
    const from = addDays(mondayOf(todayKey()), 7);
    const res = post<{ seriesId: string; plannedCount: number }>("/sessions/recurring", {
      title: "PT — Nour", type: "private", weekdays: [2, 4], time: "07:00", durationMin: 45, fromDay: from, toDay: addDays(from, 13),
      attendees: [{ trainee: "t4" }],
    });
    expect(res.plannedCount).toBe(4);
    const detail = get<SeriesDetail>(`/sessions/series/${res.seriesId}/progress`);
    expect(detail.progress.planned).toBe(4);
    expect(detail.occurrences).toHaveLength(4);
  });

  it("computes plan progress from attended, completed classes", () => {
    const [plan] = get<SessionPlan[]>("/trainees/t3/session-plans");
    const p = plan.progress;
    expect(p.target).toBe(12);
    expect(p.completed + p.upcomingBooked + p.attendanceNeeded + p.unassignedSlots).toBe(12);
  });

  it("filters by studio month and pages like the API", () => {
    expect(get<Payment[]>(`/payments?m=${shiftMonthKey(month, -2)}`)).toHaveLength(0);
    const rows = get<Payment[]>(`/payments?m=${month}`);
    const pg = get<Page<Payment>>(`/payments?m=${month}&limit=2`);
    expect(pg.items).toHaveLength(Math.min(2, rows.length));
    expect(pg.total).toBe(rows.length);
  });

  it("searches with the shared matcher, typos and all", () => {
    const res = get<SearchResponse>("/search?q=jda");
    expect(res.groups[0].kind).toBe("trainee");
    expect(res.groups[0].items[0].label).toBe("Jad Saliba");
    expect(res.didYouMean).toBe("jad");
  });

  it("builds a monthly report that matches its own financials", () => {
    const rep = get<MonthlyReport>(`/reports/monthly?m=${month}`);
    const fin = get<Financials>(`/financials?m=${month}`);
    expect(rep.financial.income).toBe(fin.income);
    expect(rep.financial.net).toBe(fin.net);
    expect(rep.subscriptions.partial + rep.subscriptions.paid + rep.subscriptions.unpaid).toBe(rep.subscriptions.billed);
    expect(code(() => get(`/reports/monthly?m=${shiftMonthKey(month, 1)}`))).toBe("VALIDATION");
  });

  it("tracks a promised balance and keeps trial follow-ups actionable", () => {
    const t = post<{ id: string }>("/trainees", { name: "Promise demo", monthlyFee: 100 });
    const p = post<PaymentPromise>("/payment-promises", { trainee: t.id, periodMonth: month, amount: 100, dueDay: addDays(todayKey(), 1) });
    expect(get<PaymentPromise[]>(`/payment-promises?m=${month}`).find((x) => x.id === p.id)?.state).toBe("open");
    post("/payments", { trainee: t.id, type: "subscription", periodMonth: month, amount: 40 });
    expect(get<PaymentPromise[]>(`/payment-promises?m=${month}`).find((x) => x.id === p.id)?.outstanding).toBe(60);
    const lead = post<TrialLead>("/trial-leads", { name: "Demo visitor", followUpDay: todayKey() });
    const item = get<FollowUp[]>("/follow-ups").find((x) => x.key === `trial:${lead.id}`);
    expect(item).toBeTruthy();
    post("/follow-ups/action", { key: item!.key, action: "snooze", days: 7 });
    expect(get<FollowUp[]>("/follow-ups").some((x) => x.key === item!.key)).toBe(false);
    expect(get<FollowUp[]>("/follow-ups?show=snoozed").some((x) => x.key === item!.key)).toBe(true);
  });

  it("saves boxing notes and month review metadata", () => {
    post<ProgressNote>("/trainees/t3/progress", { goal: "Better guard", nextFocus: "Slip" });
    expect(get<ProgressNote[]>("/trainees/t3/progress")[0].nextFocus).toBe("Slip");
    post("/month-review", { month, note: "Checked" });
    expect(get<MonthReview>(`/month-review?m=${month}`).review?.note).toBe("Checked");
  });
});
