// Picker options built one way everywhere, so every trainee picker can be
// searched by name or phone digits.
import type { Trainee } from "./types";

export function traineeOption(t: Trainee): { id: string; label: string; sub?: string } {
  const bits = [t.archivedAt ? "archived" : t.status === "inactive" ? "inactive" : "", t.phone ?? ""].filter(Boolean);
  return { id: t.id, label: t.name, sub: bits.join(" · ") || undefined };
}
