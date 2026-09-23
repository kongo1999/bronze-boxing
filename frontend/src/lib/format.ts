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

export function monthKey(d = new Date()): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

// Local-timezone YYYY-MM-DD. Unlike Date.toISOString(), this never shifts the
// calendar day for browsers east of UTC (e.g. the studio's Beirut timezone).
export function dateKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function monthLabel(key: string): string {
  const [y, m] = key.split("-").map(Number);
  return new Date(y, m - 1, 1).toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  });
}

export function shiftMonth(key: string, delta: number): string {
  const [y, m] = key.split("-").map(Number);
  return monthKey(new Date(y, m - 1 + delta, 1));
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
  });
}

export function formatLongDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
}

export function initials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? "")
    .join("");
}
