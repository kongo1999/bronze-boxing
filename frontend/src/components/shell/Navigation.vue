<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, RouterLink } from "vue-router";
import {
  House,
  CalendarDays,
  Users,
  Wallet,
  Bell,
  LineChart,
  Package,
  Plus,
  X,
  CalendarPlus,
  Banknote,
  UserPlus,
  BellPlus,
  Menu,
  Search,
  type LucideIcon,
} from "lucide-vue-next";

const route = useRoute();
const addOpen = ref(false);
const menuOpen = ref(false);

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
}

const tabs: NavItem[] = [
  { to: "/", label: "Home", icon: House },
  { to: "/schedule", label: "Schedule", icon: CalendarDays },
  { to: "/trainees", label: "Crew", icon: Users },
  { to: "/payments", label: "Money", icon: Wallet },
];

const railItems: NavItem[] = [
  ...tabs,
  { to: "/reminders", label: "Reminders", icon: Bell },
  { to: "/financials", label: "Financials", icon: LineChart },
  { to: "/inventory", label: "Inventory", icon: Package },
];

const addActions: NavItem[] = [
  { to: "/schedule/new", label: "New session", icon: CalendarPlus },
  { to: "/payments/new", label: "Log payment", icon: Banknote },
  { to: "/trainees/new", label: "Add trainee", icon: UserPlus },
  { to: "/reminders/new", label: "New reminder", icon: BellPlus },
];

const isActive = (to: string) =>
  to === "/" ? route.path === "/" : route.path.startsWith(to);

const onKey = (e: KeyboardEvent) => {
  if (e.key === "Escape") {
    addOpen.value = false;
    menuOpen.value = false;
  }
};
onMounted(() => window.addEventListener("keydown", onKey));
onUnmounted(() => window.removeEventListener("keydown", onKey));

const leftTabs = computed(() => tabs.slice(0, 2));
const rightTabs = computed(() => tabs.slice(2));
</script>

<template>
  <!-- Mobile top bar with hamburger -->
  <header
    class="fixed inset-x-0 top-0 z-40 flex h-14 items-center justify-between border-b border-line bg-surface/95 px-2 backdrop-blur md:hidden"
  >
    <button
      type="button"
      aria-label="Open menu"
      class="grid h-10 w-10 place-items-center rounded-xl text-muted transition-colors hover:bg-elevated hover:text-fg"
      @click="menuOpen = true"
    >
      <Menu class="h-6 w-6" />
    </button>
    <RouterLink to="/" class="flex items-center gap-2">
      <span
        class="grid h-7 w-7 place-items-center rounded-lg border border-bronze-strong/50 bg-gradient-to-b from-bronze-strong to-bronze font-display text-sm font-bold text-bronze-ink"
        >B</span
      >
      <span class="font-display text-base font-semibold tracking-tight">Bronze Boxing</span>
    </RouterLink>
    <RouterLink
      to="/search"
      aria-label="Search"
      class="grid h-10 w-10 place-items-center rounded-xl text-muted transition-colors hover:bg-elevated hover:text-fg"
    >
      <Search class="h-5 w-5" />
    </RouterLink>
  </header>

  <!-- Desktop side rail -->
  <aside
    class="fixed inset-y-0 left-0 z-40 hidden w-60 flex-col border-r border-line bg-surface px-4 py-6 md:flex"
  >
    <RouterLink to="/" class="mb-8 flex items-center gap-2 px-2">
      <span
        class="grid h-8 w-8 place-items-center rounded-lg bg-bronze font-display text-base font-bold text-bronze-ink"
        >B</span
      >
      <span class="font-display text-lg font-semibold tracking-tight">Bronze Boxing</span>
    </RouterLink>
    <nav class="flex flex-col gap-1">
      <RouterLink
        v-for="item in railItems"
        :key="item.to"
        :to="item.to"
        class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
        :class="
          isActive(item.to)
            ? 'bg-purple/20 text-purple-strong'
            : 'text-muted hover:bg-elevated hover:text-fg'
        "
      >
        <component :is="item.icon" class="h-5 w-5" :stroke-width="isActive(item.to) ? 2.25 : 1.75" />
        {{ item.label }}
      </RouterLink>
    </nav>
    <button
      type="button"
      class="mt-6 inline-flex h-11 items-center justify-center gap-2 rounded-xl border border-bronze-strong/50 bg-gradient-to-b from-bronze-strong to-bronze font-medium text-bronze-ink shadow-[var(--shadow-btn)] transition-[transform,box-shadow] duration-150 ease-[var(--ease-out-quart)] hover:-translate-y-0.5 hover:shadow-[var(--shadow-btn-hover)] active:translate-y-0 active:scale-[0.97]"
      @click="addOpen = true"
    >
      <Plus class="h-5 w-5" :stroke-width="2.5" /> Quick add
    </button>
  </aside>

  <!-- Mobile bottom nav -->
  <nav
    class="fixed inset-x-0 bottom-0 z-40 border-t border-line bg-surface/95 backdrop-blur md:hidden"
  >
    <div
      class="mx-auto grid max-w-md grid-cols-5 items-center px-2"
      style="padding-bottom: env(safe-area-inset-bottom)"
    >
      <RouterLink
        v-for="item in leftTabs"
        :key="item.to"
        :to="item.to"
        class="flex flex-col items-center gap-1 py-2.5 text-[0.6875rem] font-medium transition-colors"
        :class="isActive(item.to) ? 'text-purple-strong' : 'text-faint hover:text-muted'"
      >
        <component :is="item.icon" class="h-[1.35rem] w-[1.35rem]" :stroke-width="isActive(item.to) ? 2.25 : 1.75" />
        {{ item.label }}
      </RouterLink>
      <div class="flex justify-center">
        <button
          type="button"
          aria-label="Quick add"
          class="-mt-6 grid h-14 w-14 place-items-center rounded-full border border-bronze-strong/50 bg-gradient-to-b from-bronze-strong to-bronze text-bronze-ink shadow-[var(--shadow-lift)] transition-[transform,box-shadow] duration-150 ease-[var(--ease-out-quart)] hover:-translate-y-0.5 active:translate-y-0 active:scale-95"
          @click="addOpen = true"
        >
          <Plus class="h-7 w-7" :stroke-width="2.5" />
        </button>
      </div>
      <RouterLink
        v-for="item in rightTabs"
        :key="item.to"
        :to="item.to"
        class="flex flex-col items-center gap-1 py-2.5 text-[0.6875rem] font-medium transition-colors"
        :class="isActive(item.to) ? 'text-purple-strong' : 'text-faint hover:text-muted'"
      >
        <component :is="item.icon" class="h-[1.35rem] w-[1.35rem]" :stroke-width="isActive(item.to) ? 2.25 : 1.75" />
        {{ item.label }}
      </RouterLink>
    </div>
  </nav>

  <!-- Quick-add sheet -->
  <div
    class="fixed inset-0 z-50 transition-opacity duration-200"
    :class="addOpen ? 'opacity-100' : 'pointer-events-none opacity-0'"
    :aria-hidden="!addOpen"
  >
    <div class="absolute inset-0 bg-canvas/70 backdrop-blur-sm" @click="addOpen = false" />
    <div
      class="absolute inset-x-0 bottom-0 rounded-t-2xl border-t border-line bg-surface p-4 shadow-[var(--shadow-lift)] transition-transform duration-200 md:inset-auto md:bottom-6 md:left-6 md:w-64 md:rounded-2xl md:border"
      :class="addOpen ? 'translate-y-0' : 'translate-y-6'"
      style="padding-bottom: calc(1rem + env(safe-area-inset-bottom))"
    >
      <div class="mb-3 flex items-center justify-between">
        <p class="label-eyebrow text-[0.625rem] text-faint">Quick add</p>
        <button type="button" aria-label="Close" class="text-faint hover:text-fg" @click="addOpen = false">
          <X class="h-5 w-5" />
        </button>
      </div>
      <div class="grid grid-cols-2 gap-2 md:grid-cols-1">
        <RouterLink
          v-for="a in addActions"
          :key="a.to"
          :to="a.to"
          class="flex items-center gap-3 rounded-xl border border-line bg-elevated px-3 py-3 text-sm font-medium transition-colors hover:border-bronze/40 hover:text-bronze"
          @click="addOpen = false"
        >
          <component :is="a.icon" class="h-5 w-5 text-bronze" :stroke-width="2" />
          {{ a.label }}
        </RouterLink>
      </div>
    </div>
  </div>

  <!-- Mobile menu drawer (full navigation, incl. Financials/Inventory) -->
  <div
    class="fixed inset-0 z-50 transition-opacity duration-200 md:hidden"
    :class="menuOpen ? 'opacity-100' : 'pointer-events-none opacity-0'"
    :aria-hidden="!menuOpen"
  >
    <div class="absolute inset-0 bg-canvas/70 backdrop-blur-sm" @click="menuOpen = false" />
    <aside
      class="absolute inset-y-0 left-0 flex w-72 max-w-[80%] flex-col border-r border-line bg-surface px-4 py-5 shadow-[var(--shadow-lift)] transition-transform duration-200 ease-[var(--ease-out-quart)]"
      :class="menuOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="mb-6 flex items-center justify-between px-1">
        <RouterLink to="/" class="flex items-center gap-2" @click="menuOpen = false">
          <span
            class="grid h-8 w-8 place-items-center rounded-lg border border-bronze-strong/50 bg-gradient-to-b from-bronze-strong to-bronze font-display text-base font-bold text-bronze-ink"
            >B</span
          >
          <span class="font-display text-lg font-semibold tracking-tight">Bronze Boxing</span>
        </RouterLink>
        <button type="button" aria-label="Close menu" class="text-faint hover:text-fg" @click="menuOpen = false">
          <X class="h-5 w-5" />
        </button>
      </div>
      <nav class="flex flex-col gap-1">
        <RouterLink
          v-for="item in railItems"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
          :class="
            isActive(item.to)
              ? 'bg-purple/20 text-purple-strong'
              : 'text-muted hover:bg-elevated hover:text-fg'
          "
          @click="menuOpen = false"
        >
          <component :is="item.icon" class="h-5 w-5" :stroke-width="isActive(item.to) ? 2.25 : 1.75" />
          {{ item.label }}
        </RouterLink>
        <RouterLink
          to="/search"
          class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
          :class="
            isActive('/search')
              ? 'bg-purple/20 text-purple-strong'
              : 'text-muted hover:bg-elevated hover:text-fg'
          "
          @click="menuOpen = false"
        >
          <Search class="h-5 w-5" :stroke-width="1.75" />
          Search
        </RouterLink>
      </nav>
    </aside>
  </div>
</template>
