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
  type LucideIcon,
} from "lucide-vue-next";
import { api, qs } from "@/lib/api";
import { useCachedAsync } from "@/lib/cache";
import type { Session } from "@/lib/types";
import { formatTime, formatLongDate } from "@/lib/format";

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
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
});
const dateLabel = computed(() =>
  new Date().toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" }),
);

// "Up next": the first scheduled session from now through the next 7 days.
// Cached (stale-while-revalidate) so revisits render instantly; the hero never
// blocks on this — loading shows a same-height shimmer, an error shows nothing.
const from = new Date();
const to = new Date(from.getTime() + 7 * 86_400_000);
const { data: upcoming, loading: nextLoading, error: nextError } = useCachedAsync(
  "home-upcoming",
  () => api.get<Session[]>(`/sessions${qs({ from: from.toISOString(), to: to.toISOString() })}`),
);
const next = computed(() =>
  (upcoming.value ?? []).find((s) => s.status === "scheduled" && new Date(s.start) > new Date()),
);
const nextIsToday = computed(
  () => !!next.value && new Date(next.value.start).toDateString() === new Date().toDateString(),
);
</script>

<template>
  <div class="space-y-7">
    <!-- Hero: the badge on velvet. -->
    <header class="relative pt-3 text-center">
      <div aria-hidden="true" class="hero-veil pointer-events-none absolute -inset-x-8 -top-12 -bottom-4"></div>
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
