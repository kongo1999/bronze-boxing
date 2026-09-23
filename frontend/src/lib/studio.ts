// Studio time. Every calendar decision — which day a session is on, which
// month a payment falls in, what "today" is — is made in the studio's
// timezone (STUDIO_TZ on the API, Asia/Beirut by default), never the
// browser's. A coach checking the schedule from abroad sees the studio's
// days and times, and a class at 18:00 Beirut is created at 18:00 Beirut.
//
// Calendar values are plain strings: a day is "YYYY-MM-DD", a month
// "YYYY-MM". Instants are ISO strings from the API. Calendar arithmetic on
// day strings is done in UTC so no local offset can shift it.

let tz: string = (import.meta.env.VITE_STUDIO_TZ as string) || "Asia/Beirut";

/** The studio timezone (updated from /api/health on boot). */
export function studioTZ(): string {
  return tz;
}
export function setStudioTZ(zone: string | undefined): void {
  if (!zone) return;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: zone });
    tz = zone;
  } catch {
    /* keep the default on an unknown zone */
  }
}

/** True when this browser is in a different zone than the studio. */
export function browserIsElsewhere(): boolean {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone !== tz;
  } catch {
    return false;
  }
}

export interface Parts {
  y: number;
  m: number; // 1-12
  d: number;
  hh: number;
  mm: number;
  ss: number;
  weekday: number; // 0 = Sunday
}

const fmtCache = new Map<string, Intl.DateTimeFormat>();
function partsFormatter(zone: string): Intl.DateTimeFormat {
  let f = fmtCache.get(zone);
  if (!f) {
    f = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      weekday: "short",
    });
    fmtCache.set(zone, f);
  }
  return f;
}

const WEEKDAYS: Record<string, number> = { Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6 };

/** Wall-clock parts of an instant in the studio timezone. */
export function studioParts(at: Date | string, zone = tz): Parts {
  const date = typeof at === "string" ? new Date(at) : at;
  const out: Record<string, string> = {};
  for (const p of partsFormatter(zone).formatToParts(date)) out[p.type] = p.value;
  return {
    y: Number(out.year),
    m: Number(out.month),
    d: Number(out.day),
    hh: Number(out.hour) % 24,
    mm: Number(out.minute),
    ss: Number(out.second),
    weekday: WEEKDAYS[out.weekday] ?? 0,
  };
}

const pad = (n: number) => String(n).padStart(2, "0");

/** Studio day (YYYY-MM-DD) of an instant. */
export function dayOf(at: Date | string): string {
  const p = studioParts(at);
  return `${p.y}-${pad(p.m)}-${pad(p.d)}`;
}
/** Studio month (YYYY-MM) of an instant. */
export function monthOf(at: Date | string): string {
  return dayOf(at).slice(0, 7);
}
/** "HH:MM" wall time of an instant in the studio. */
export function timeOf(at: Date | string): string {
  const p = studioParts(at);
  return `${pad(p.hh)}:${pad(p.mm)}`;
}
export function todayKey(): string {
  return dayOf(new Date());
}
export function currentMonth(): string {
  return monthOf(new Date());
}

const DAY_RE = /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;
const MONTH_RE = /^\d{4}-(0[1-9]|1[0-2])$/;
export const isDayKey = (s: unknown): s is string => typeof s === "string" && DAY_RE.test(s);
export const isMonthKey = (s: unknown): s is string => typeof s === "string" && MONTH_RE.test(s);

function utcOfDay(day: string): Date {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
}
function keyOfUTC(dt: Date): string {
  return `${dt.getUTCFullYear()}-${pad(dt.getUTCMonth() + 1)}-${pad(dt.getUTCDate())}`;
}

/** Calendar arithmetic on day strings. */
export function addDays(day: string, n: number): string {
  const dt = utcOfDay(day);
  dt.setUTCDate(dt.getUTCDate() + n);
  return keyOfUTC(dt);
}
export function weekdayOfDay(day: string): number {
  return utcOfDay(day).getUTCDay();
}
/** The Monday on or before a day (weeks run Monday–Sunday). */
export function mondayOf(day: string): string {
  return addDays(day, -((weekdayOfDay(day) + 6) % 7));
}
export function daysInMonth(month: string): number {
  const [y, m] = month.split("-").map(Number);
  return new Date(Date.UTC(y, m, 0)).getUTCDate();
}
export function shiftMonthKey(month: string, n: number): string {
  const [y, m] = month.split("-").map(Number);
  const dt = new Date(Date.UTC(y, m - 1 + n, 1));
  return `${dt.getUTCFullYear()}-${pad(dt.getUTCMonth() + 1)}`;
}

/**
 * The instant at which the studio's wall clock reads `time` on `day`. Solves
 * for the zone offset (twice, so a DST change between the guess and the
 * answer still lands on the right hour).
 */
export function studioInstant(day: string, time = "00:00", zone = tz): Date {
  const [y, m, d] = day.split("-").map(Number);
  const [hh, mm] = time.split(":").map(Number);
  const wall = Date.UTC(y, m - 1, d, hh || 0, mm || 0);
  const offset = (ts: number) => {
    const p = studioParts(new Date(ts), zone);
    return Date.UTC(p.y, p.m - 1, p.d, p.hh, p.mm, p.ss) - ts;
  };
  let ts = wall - offset(wall);
  const second = offset(ts);
  if (wall - second !== ts) ts = wall - second;
  return new Date(ts);
}

/** Start of a studio day as an ISO instant (for API range queries). */
export function dayStartISO(day: string): string {
  return studioInstant(day, "00:00").toISOString();
}

// ── Display ────────────────────────────────────────────────────────────────

function fmt(at: Date | string, opts: Intl.DateTimeFormatOptions): string {
  const date = typeof at === "string" ? new Date(at) : at;
  return date.toLocaleString("en-US", { timeZone: tz, ...opts });
}
/** "6:30 PM" in studio time. */
export function formatTime(iso: string): string {
  return fmt(iso, { hour: "numeric", minute: "2-digit" });
}
/** "Sep 23" in studio time. */
export function formatLongDate(iso: string): string {
  return fmt(iso, { month: "short", day: "numeric" });
}
/** "Sep 23, 2026, 6:30 PM" in studio time. */
export function formatDateTime(iso: string): string {
  return fmt(iso, { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
}
/** Label a calendar day ("Wednesday, Sep 23"). Pure calendar — no zone. */
export function formatDay(day: string, opts: Intl.DateTimeFormatOptions = { weekday: "long", month: "short", day: "numeric" }): string {
  return utcOfDay(day).toLocaleDateString("en-US", { timeZone: "UTC", ...opts });
}
/** "September 2026". */
export function formatMonth(month: string): string {
  const [y, m] = month.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString("en-US", { timeZone: "UTC", month: "long", year: "numeric" });
}
