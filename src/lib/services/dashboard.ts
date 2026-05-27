import { endOfDay, endOfWeek, startOfDay } from "date-fns";
import { monthKey } from "../utils";
import type { ReminderDTO, SessionDTO } from "../dto";
import { listSessions } from "./sessions";
import { getMonthRevenue, getSubscriptionStatuses, type SubscriptionStatus } from "./payments";
import { listReminders } from "./reminders";
import { countActiveTrainees } from "./trainees";

export interface DashboardData {
  today: string;
  month: string;
  monthRevenue: number;
  activeTrainees: number;
  overdueCount: number;
  collectedThisMonth: number;
  expectedThisMonth: number;
  todaySessions: SessionDTO[];
  todayReminders: ReminderDTO[];
  weekReminders: ReminderDTO[];
  overdueSubscriptions: SubscriptionStatus[];
}

export async function getDashboardData(): Promise<DashboardData> {
  const now = new Date();
  const month = monthKey(now);

  const [monthRevenue, activeTrainees, todaySessions, subStatuses, todayReminders, weekReminders] =
    await Promise.all([
      getMonthRevenue(month),
      countActiveTrainees(),
      listSessions({ from: startOfDay(now), to: endOfDay(now) }),
      getSubscriptionStatuses(month),
      listReminders({ from: startOfDay(now), to: endOfDay(now) }),
      listReminders({ from: startOfDay(now), to: endOfWeek(now, { weekStartsOn: 1 }) }),
    ]);

  const overdueSubscriptions = subStatuses.filter((s) => s.state !== "paid");
  const expectedThisMonth = subStatuses.reduce((sum, s) => sum + s.due, 0);
  const collectedThisMonth = subStatuses.reduce((sum, s) => sum + s.amountPaid, 0);

  return {
    today: now.toISOString(),
    month,
    monthRevenue,
    activeTrainees,
    overdueCount: overdueSubscriptions.length,
    collectedThisMonth,
    expectedThisMonth,
    todaySessions,
    todayReminders,
    weekReminders,
    overdueSubscriptions,
  };
}
