import { currentMonth, monthOf, formatMonth, shiftMonthKey } from "./studio";

export { formatTime, formatLongDate, formatDateTime, formatDay, todayKey } from "./studio";

const CURRENCY = (import.meta.env.VITE_CURRENCY as string) || "$";

// Whole amounts read as "$12"; anything with cents shows both digits ("$12.50",
// "$12.55") — the books are kept to the cent, so the screen must be too.
export function money(n: number): string {
  const v = Math.round((n ?? 0) * 100) / 100;
  const cents = Math.round(Math.abs(v) * 100) % 100 !== 0;
  return (
    (v < 0 ? "−" : "") +
    CURRENCY +
    Math.abs(v).toLocaleString("en-US", {
      minimumFractionDigits: cents ? 2 : 0,
      maximumFractionDigits: 2,
    })
  );
}

/** Round a typed amount to the cent (inputs allow step 0.01). */
export function cents(n: number): number {
  return Math.round((Number(n) || 0) * 100) / 100;
}

/** The studio month (YYYY-MM) of an instant — now by default. */
export function monthKey(d?: Date): string {
  return d ? monthOf(d) : currentMonth();
}

/**
 * YYYY-MM-DD of a Date built from calendar parts (new Date(y, m, d)). Pure
 * calendar formatting in the Date's own fields — for "today in the studio"
 * use todayKey(), and for an API instant use dayOf().
 */
export function dateKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function monthLabel(key: string): string {
  return formatMonth(key);
}

export function shiftMonth(key: string, delta: number): string {
  return shiftMonthKey(key, delta);
}

export function initials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? "")
    .join("");
}
