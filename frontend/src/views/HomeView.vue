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
  type LucideIcon,
} from "lucide-vue-next";

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
</script>

<template>
  <div class="space-y-6">
    <header class="pt-2 text-center">
      <img :src="logoUrl" alt="Bronze Boxing Club" class="mx-auto mb-3 h-20 w-auto" />
      <h1 class="font-display text-2xl font-semibold tracking-tight">Bronze Boxing</h1>
      <p class="mt-0.5 text-sm text-muted">{{ greeting }} · {{ dateLabel }}</p>
    </header>

    <nav class="grid grid-cols-2 gap-3">
      <RouterLink
        v-for="item in items"
        :key="item.to"
        :to="item.to"
        class="flex flex-col gap-3 rounded-2xl border border-line bg-surface p-5 shadow-[var(--shadow-soft)] transition-[transform,box-shadow,border-color] duration-150 ease-[var(--ease-out-quart)] hover:-translate-y-0.5 hover:border-bronze/40 hover:shadow-[var(--shadow-lift)] active:translate-y-0 active:scale-[0.98]"
      >
        <span class="grid h-11 w-11 place-items-center rounded-xl bg-bronze/15 text-bronze">
          <component :is="item.icon" class="h-6 w-6" :stroke-width="1.75" />
        </span>
        <div>
          <p class="font-display text-lg font-semibold leading-tight tracking-tight">{{ item.label }}</p>
          <p class="mt-0.5 text-xs text-muted">{{ item.sub }}</p>
        </div>
      </RouterLink>
    </nav>
  </div>
</template>
