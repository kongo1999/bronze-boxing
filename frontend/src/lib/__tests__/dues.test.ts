import { describe, expect, it } from "vitest";
import { byUrgency, collectLink, collectable, countDues } from "../dues";
import { money } from "../format";
import type { SubStatus } from "../types";

function row(name: string, state: SubStatus["state"], due: number, paid: number): SubStatus {
  return {
    trainee: { id: name.toLowerCase(), name, monthlyFee: due, status: "active", createdAt: "", updatedAt: "" },
    periodMonth: "2026-09",
    due,
    amountPaid: paid,
    remaining: Math.max(0, due - paid),
    state,
    source: "normal",
    paymentCount: paid > 0 ? 1 : 0,
  };
}

describe("money", () => {
  it("keeps whole amounts whole and shows cents when there are any", () => {
    expect(money(12)).toBe("$12");
    expect(money(12.5)).toBe("$12.50");
    expect(money(12.55)).toBe("$12.55");
    expect(money(1200)).toBe("$1,200");
    expect(money(-5.5)).toBe("−$5.50");
  });
  it("rounds float drift to the cent", () => {
    expect(money(0.1 + 0.2)).toBe("$0.30");
  });
});

describe("dues", () => {
  const rows = [
    row("Paid", "paid", 50, 50),
    row("Partial", "partial", 50, 20),
    row("Unpaid", "unpaid", 80, 0),
    row("Imported", "unverified", 40, 40),
  ];

  it("counts partial separately from paid and unpaid", () => {
    const c = countDues(rows);
    expect(c).toMatchObject({ paid: 1, partial: 1, unpaid: 1, unverified: 1 });
    // Only partial + unpaid are owed; imported rows stay out until reviewed.
    expect(c.outstanding).toBe(30 + 80);
  });

  it("sorts what needs collecting first", () => {
    expect([...rows].sort(byUrgency).map((r) => r.state)).toEqual(["partial", "unpaid", "unverified", "paid"]);
  });

  it("offers Collect for exactly the remaining balance", () => {
    const partial = rows[1];
    expect(collectable(partial)).toBe(true);
    expect(collectable(rows[0])).toBe(false);
    expect(collectable(rows[3])).toBe(false);
    const link = new URL(collectLink(partial), "http://x");
    expect(link.pathname).toBe("/payments/new");
    expect(link.searchParams.get("amount")).toBe("30");
    expect(link.searchParams.get("periodMonth")).toBe("2026-09");
  });
});
