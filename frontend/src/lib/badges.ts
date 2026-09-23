// Counts shown on the navigation (e.g. overdue reminders). Refreshed as the
// user moves around — throttled, since every screen change triggers it — and
// forced right after a change that affects a count.
import { reactive } from "vue";
import { api } from "./api";

export const badges = reactive({ remindersOverdue: 0, remindersToday: 0 });

let last = 0;
let inflight: Promise<void> | null = null;

export function refreshBadges(force = false): Promise<void> {
  if (inflight) return inflight;
  if (!force && Date.now() - last < 15_000) return Promise.resolve();
  last = Date.now();
  inflight = api
    .get<{ overdue: number; today: number }>("/reminders/counts")
    .then((c) => {
      badges.remindersOverdue = c?.overdue ?? 0;
      badges.remindersToday = c?.today ?? 0;
    })
    .catch(() => {
      /* a badge is a hint; never an error on screen */
    })
    .finally(() => {
      inflight = null;
    });
  return inflight;
}
