import { describe, expect, it } from "vitest";
import { demoResolve } from "../demo";
import { isApiError } from "../api-error";
import { addDays, currentMonth, mondayOf, shiftMonthKey, todayKey } from "../studio";
import type { Financials, MonthlyReport, Page, Payment, SearchResponse, SeriesDetail, SessionPlan, SubStatus } from "../types";

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
    expect(get<Payment[]>(`/payments?m=${shiftMonthKey(month, -1)}`)).toHaveLength(0);
    const pg = get<Page<Payment>>(`/payments?m=${month}&limit=2`);
    expect(pg.items).toHaveLength(2);
    expect(pg.total).toBeGreaterThan(2);
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
});
