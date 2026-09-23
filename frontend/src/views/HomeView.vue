<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import logoUrl from "@/assets/logo.png";
import {
  CalendarDays,
  Users,
  Wallet,
  LineChart,
  Package,
  Bell,
  ChevronRight,
  FileBarChart,
  type LucideIcon,
} from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync } from "@/lib/cache";
import type { Dashboard } from "@/lib/types";
import { formatTime, formatLongDate, money } from "@/lib/format";
import { studioParts, todayKey, dayOf, formatDay, currentMonth } from "@/lib/studio";

interface MenuItem {
  to: string;
  label: string;
  sub: string;
  icon: LucideIcon;
}

// 2×3 launcher menu — every main section, one tap away.
const items: MenuItem[] = [
  { to: "/schedule", label: "Schedule", sub: "Classes & calendar", icon: CalendarDays },
  { to: "/trainees", label: "Crew", sub: "Roster & dues", icon: Users },
  { to: "/payments", label: "Money", sub: "Payments & subs", icon: Wallet },
  { to: "/financials", label: "Financials", sub: "Income & outgoings", icon: LineChart },
  { to: "/inventory", label: "Inventory", sub: "Stock & sales", icon: Package },
  { to: "/reminders", label: "Reminders", sub: "Things to do", icon: Bell },
];

const greeting = computed(() => {
  const h = studioParts(new Date()).hh;
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
});
const dateLabel = computed(() =>
  formatDay(todayKey(), { weekday: "long", month: "long", day: "numeric" }),
);

// One call for the hero's "Up next" and the "Needs attention" strip. Cached
// (stale-while-revalidate) so revisits render instantly; the hero never
// blocks on it — loading shows a same-height shimmer, an error shows nothing.
const { data: dash, loading: nextLoading, error: nextError } = useCachedAsync("dashboard", () => api.get<Dashboard>("/dashboard"));
const next = computed(() => dash.value?.nextSession ?? undefined);

// Exceptions only, each opening the filtered place to deal with it.
interface Attention {
  to: string;
  text: string;
  tone: "partial" | "overdue" | "info";
}
const month = currentMonth();
const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
const attention = computed<Attention[]>(() => {
  const d = dash.value;
  if (!d) return [];
  const out: Attention[] = [];
  if (d.partialCount) out.push({ to: `/payments?m=${month}&show=partial`, text: `${plural(d.partialCount, "trainee")} partly paid`, tone: "partial" });
  if (d.unpaidCount) out.push({ to: `/payments?m=${month}&show=unpaid`, text: `${plural(d.unpaidCount, "trainee")} unpaid · ${money(d.outstanding)} still owed in all`, tone: "overdue" });
  if (d.remindersOverdue) out.push({ to: "/reminders", text: `${plural(d.remindersOverdue, "overdue reminder")}`, tone: "overdue" });
  for (const p of d.plansNearing.slice(0, 3)) {
    const left = p.remaining === 0 ? "plan complete" : `${plural(p.remaining, "session")} left`;
    const ends = p.endDate ? ` · ends ${formatDay(p.endDate, { month: "short", day: "numeric" })}` : "";
    out.push({ to: `/trainees/${p.trainee}`, text: `${p.traineeName}: ${left}${ends}`, tone: "info" });
  }
  if (d.plansNearing.length > 3) out.push({ to: "/trainees", text: `${d.plansNearing.length - 3} more plans nearly done`, tone: "info" });
  if (d.lowStock || d.outOfStock) {
    const bits = [d.outOfStock ? `${d.outOfStock} out of stock` : "", d.lowStock ? `${d.lowStock} running low` : ""].filter(Boolean);
    out.push({ to: "/inventory?show=short", text: bits.join(" · "), tone: d.outOfStock ? "overdue" : "partial" });
  }
  return out;
});
const dot: Record<Attention["tone"], string> = { partial: "bg-partial", overdue: "bg-overdue", info: "bg-bronze" };
const nextIsToday = computed(
  () => !!next.value && dayOf(next.value.start) === todayKey(),
);
</script>

<template>
  <div class="space-y-7">
    <!-- Hero: the badge floating on the page's backlit canvas. -->
    <header class="relative pt-3 text-center">
      <div class="relative">
        <img
          :src="logoUrl"
          alt="Bronze Boxing Club"
          class="mx-auto h-32 w-auto drop-shadow-[0_14px_28px_oklch(0_0_0/0.55)] md:h-36"
        />
        <p class="mt-5 label-eyebrow text-[0.625rem] text-bronze">{{ greeting }}</p>
        <h1 class="mt-1 font-display text-[1.75rem] font-semibold leading-tight tracking-tight">
          {{ dateLabel }}
        </h1>

        <!-- Up-next ticket: time + title, taps through to the session. -->
        <div class="mt-4 flex justify-center">
          <div
            v-if="nextLoading"
            class="h-11 w-64 animate-pulse rounded-xl border border-line bg-surface"
          ></div>
          <RouterLink
            v-else-if="next"
            :to="`/schedule/${next.id}`"
            class="group inline-flex h-11 items-center gap-3 rounded-xl border border-line bg-surface/90 pl-4 pr-2.5 shadow-[var(--shadow-soft)] transition-colors hover:border-bronze/40"
          >
            <span class="label-eyebrow text-[0.5625rem] text-faint">Up next</span>
            <span class="font-display text-sm font-semibold text-bronze tnum">
              {{ nextIsToday ? "" : `${formatLongDate(next.start)} · ` }}{{ formatTime(next.start) }}
            </span>
            <span class="max-w-44 truncate text-sm">{{ next.title }}</span>
            <ChevronRight
              class="h-4 w-4 text-faint transition-transform duration-150 ease-[var(--ease-out-quart)] group-hover:translate-x-0.5"
            />
          </RouterLink>
          <RouterLink
            v-else-if="!nextError"
            to="/schedule"
            class="group inline-flex h-11 items-center gap-2 rounded-xl px-3 text-sm text-faint transition-colors hover:text-muted"
          >
            No upcoming sessions · plan one
            <ChevronRight class="h-4 w-4 transition-transform duration-150 ease-[var(--ease-out-quart)] group-hover:translate-x-0.5" />
          </RouterLink>
        </div>
      </div>
    </header>

    <!-- Needs attention: only exceptions, each one tap from its fix. -->
    <section v-if="dash" class="rounded-2xl border border-line bg-surface/80 p-2" aria-labelledby="attention-title">
      <h2 id="attention-title" class="px-2 pb-1 pt-1 label-eyebrow text-[0.625rem] text-faint">Needs attention</h2>
      <p v-if="!attention.length" class="px-2 pb-2 text-sm text-muted">All clear: nothing owed, overdue or running low.</p>
      <RouterLink
        v-for="a in attention"
        :key="a.to + a.text"
        :to="a.to"
        class="flex min-h-11 items-center gap-3 rounded-xl px-2 text-sm transition-colors hover:bg-elevated"
      >
        <span class="h-2 w-2 shrink-0 rounded-full" :class="dot[a.tone]" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">{{ a.text }}</span>
        <ChevronRight class="h-4 w-4 shrink-0 text-faint" />
      </RouterLink>
      <RouterLink :to="`/reports?m=${month}`" class="mt-1 flex min-h-11 items-center gap-3 rounded-xl border-t border-line/60 px-2 text-sm font-medium text-bronze transition-colors hover:bg-elevated">
        <FileBarChart class="h-4 w-4 shrink-0" /> <span class="flex-1">This month's report</span>
        <ChevronRight class="h-4 w-4 shrink-0 text-faint" />
      </RouterLink>
    </section>

    <!-- Launcher: same six destinations, tiles with real depth. -->
    <nav class="grid grid-cols-2 gap-3 md:grid-cols-3">
      <RouterLink
        v-for="item in items"
        :key="item.to"
        :to="item.to"
        class="group relative flex flex-col gap-3.5 rounded-2xl border border-line bg-gradient-to-b from-elevated/70 to-surface p-5 shadow-[var(--shadow-btn)] transition-[transform,box-shadow,border-color] duration-150 ease-[var(--ease-out-quart)] hover:-translate-y-0.5 hover:border-bronze/40 hover:shadow-[var(--shadow-btn-hover)] active:translate-y-0 active:scale-[0.98]"
      >
        <span
          class="grid h-11 w-11 place-items-center rounded-xl bg-bronze/12 text-bronze ring-1 ring-bronze/20 ring-inset transition-colors duration-150 group-hover:bg-bronze/20"
        >
          <component :is="item.icon" class="h-6 w-6" :stroke-width="1.75" />
        </span>
        <div>
          <p class="font-display text-lg font-semibold leading-tight tracking-tight">{{ item.label }}</p>
          <p class="mt-0.5 text-xs text-muted">{{ item.sub }}</p>
        </div>
        <ChevronRight
          class="absolute right-4 top-5 h-4 w-4 text-faint opacity-0 transition-opacity duration-150 group-hover:opacity-100"
        />
      </RouterLink>
    </nav>
  </div>
</template>
