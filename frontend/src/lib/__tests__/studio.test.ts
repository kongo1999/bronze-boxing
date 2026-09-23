import { afterEach, describe, expect, it } from "vitest";
import {
  addDays, dayOf, formatDay, formatMonth, mondayOf, monthOf, setStudioTZ, shiftMonthKey, studioInstant, timeOf,
} from "../studio";

// The studio is in Beirut (UTC+3 in summer, UTC+2 in winter). Whatever zone
// the browser runs in, days and months must be the studio's.
const BROWSER_ZONES = ["UTC", "America/New_York", "Asia/Tokyo", "Asia/Beirut"];
const originalTZ = process.env.TZ;

afterEach(() => {
  process.env.TZ = originalTZ;
  setStudioTZ("Asia/Beirut");
});

describe.each(BROWSER_ZONES)("studio time with the browser in %s", (zone) => {
  it("files instants under the studio's day and month at midnight", () => {
    process.env.TZ = zone;
    setStudioTZ("Asia/Beirut");
    // 21:30 UTC on Aug 31 is 00:30 on Sep 1 in Beirut (UTC+3).
    expect(dayOf("2026-08-31T21:30:00Z")).toBe("2026-09-01");
    expect(monthOf("2026-08-31T21:30:00Z")).toBe("2026-09");
    // 20:59 UTC is still Aug 31 in Beirut.
    expect(dayOf("2026-08-31T20:59:00Z")).toBe("2026-08-31");
    expect(monthOf("2026-08-31T20:59:00Z")).toBe("2026-08");
  });

  it("turns a studio wall-clock time into the right instant", () => {
    process.env.TZ = zone;
    expect(studioInstant("2026-09-01", "00:00").toISOString()).toBe("2026-08-31T21:00:00.000Z");
    expect(studioInstant("2026-09-23", "18:00").toISOString()).toBe("2026-09-23T15:00:00.000Z");
    expect(timeOf("2026-09-23T15:00:00Z")).toBe("18:00");
  });

  it("keeps 18:00 at 18:00 across the autumn clock change", () => {
    process.env.TZ = zone;
    // Summer (UTC+3) vs winter (UTC+2) — the instant moves, the wall time doesn't.
    expect(studioInstant("2026-10-20", "18:00").toISOString()).toBe("2026-10-20T15:00:00.000Z");
    expect(studioInstant("2026-11-03", "18:00").toISOString()).toBe("2026-11-03T16:00:00.000Z");
    expect(timeOf(studioInstant("2026-11-03", "18:00"))).toBe("18:00");
  });

  it("does calendar arithmetic without drifting across month, year or DST", () => {
    process.env.TZ = zone;
    expect(addDays("2026-09-28", 7)).toBe("2026-10-05");
    expect(addDays("2026-12-29", 5)).toBe("2027-01-03");
    expect(addDays("2026-10-24", 3)).toBe("2026-10-27");
    expect(mondayOf("2026-09-27")).toBe("2026-09-21"); // Sunday → previous Monday
    expect(mondayOf("2026-09-21")).toBe("2026-09-21");
    expect(mondayOf("2027-01-01")).toBe("2026-12-28"); // week rolls over the year
    expect(shiftMonthKey("2026-12", 1)).toBe("2027-01");
    expect(shiftMonthKey("2026-01", -1)).toBe("2025-12");
    expect(formatDay("2026-09-23", { weekday: "short", month: "short", day: "numeric" })).toBe("Wed, Sep 23");
    expect(formatMonth("2026-09")).toBe("September 2026");
  });
});
