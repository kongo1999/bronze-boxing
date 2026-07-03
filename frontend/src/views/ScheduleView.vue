<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { RouterLink } from "vue-router";
import { ChevronLeft, ChevronRight, CalendarPlus, Check, CalendarDays } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
import { toast } from "@/lib/toast";
import type { Session } from "@/lib/types";
import { formatTime, dateKey } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Badge from "@/components/ui/Badge.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { btnClasses } from "@/components/ui/button";

const cursor = ref(new Date()); // any date within the visible week

function mondayOf(d: Date): Date {
  const offset = (d.getDay() + 6) % 7; // days since Monday
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() - offset);
}
function sameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

const weekStart = computed(() => mondayOf(cursor.value));
const weekEndExclusive = computed(
  () => new Date(weekStart.value.getFullYear(), weekStart.value.getMonth(), weekStart.value.getDate() + 7),
);
const isCurrentWeek = computed(() => sameDay(mondayOf(new Date()), weekStart.value));

const weekLabel = computed(() => {
  const start = weekStart.value;
  const end = new Date(start.getFullYear(), start.getMonth(), start.getDate() + 6);
  const fmt = (d: Date) => d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  return `${fmt(start)} – ${fmt(end)}, ${end.getFullYear()}`;
});

const sessions = ref<Session[]>([]);
const cacheKey = () => `sessions:week:${dateKey(weekStart.value)}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  const from = dateKey(weekStart.value);
  const to = dateKey(weekEndExclusive.value);
  try {
    const res = await api.get<Session[]>(`/sessions?from=${from}&to=${to}`);
    if (my !== loadToken) return; // ignore stale (out-of-order) responses
    sessions.value = res;
    writeCache(cacheKey(), res);
  } catch {
    if (my === loadToken) toast("Couldn't load the schedule.", "error");
  }
}
function showCached() {
  const hit = readCache<Session[]>(cacheKey());
  if (hit) sessions.value = hit;
}
watch(weekStart, () => { showCached(); load(); }, { immediate: true });

// Group the visible week's sessions by day (Monday first); skip empty days so
// the list reads like a to-do list, not a mostly-blank grid.
const days = computed(() => {
  const today = new Date();
  const out: { key: string; label: string; isToday: boolean; sessions: Session[] }[] = [];
  for (let i = 0; i < 7; i++) {
    const date = new Date(weekStart.value.getFullYear(), weekStart.value.getMonth(), weekStart.value.getDate() + i);
    const daySessions = sessions.value
      .filter((s) => sameDay(new Date(s.start), date))
      .sort((a, b) => +new Date(a.start) - +new Date(b.start));
    if (daySessions.length === 0) continue;
    out.push({
      key: date.toISOString(),
      label: date.toLocaleDateString("en-US", { weekday: "long", month: "short", day: "numeric" }),
      isToday: sameDay(date, today),
      sessions: daySessions,
    });
  }
  return out;
});
const isEmpty = computed(() => days.value.length === 0);

function shift(delta: number) {
  cursor.value = new Date(weekStart.value.getFullYear(), weekStart.value.getMonth(), weekStart.value.getDate() + delta * 7);
}
function goToday() {
  cursor.value = new Date();
}

// Optimistic: flip the checkbox immediately, fire one request, revert on failure.
// Mirrors the Reminders list interaction — no detour through a calendar/attendance
// screen just to mark a class done.
async function toggleDone(s: Session) {
  if (s.status === "cancelled") return;
  const prev = s.status;
  const next = s.status === "completed" ? "scheduled" : "completed";
  s.status = next;
  try {
    await api.put(`/sessions/${s.id}`, { status: next });
  } catch {
    s.status = prev;
    toast("Couldn't update that session.", "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Classes" title="Schedule">
      <template #action>
        <RouterLink to="/schedule/new" :class="btnClasses('primary', 'sm')"><CalendarPlus class="h-4 w-4" /> New</RouterLink>
      </template>
    </PageHeader>

    <div class="flex items-center justify-between rounded-2xl border border-line bg-surface p-3">
      <button :class="btnClasses('ghost', 'icon')" aria-label="Previous week" @click="shift(-1)"><ChevronLeft class="h-5 w-5" /></button>
      <p class="font-display font-semibold tracking-tight">{{ weekLabel }}</p>
      <button :class="btnClasses('ghost', 'icon')" aria-label="Next week" @click="shift(1)"><ChevronRight class="h-5 w-5" /></button>
    </div>

    <div v-if="!isCurrentWeek" class="flex justify-center">
      <button :class="btnClasses('ghost', 'sm')" @click="goToday"><CalendarDays class="h-4 w-4" /> Jump to this week</button>
    </div>

    <EmptyState v-if="isEmpty" :icon="CalendarDays" title="No classes this week" description="Add a session to fill the week." />

    <section v-for="day in days" :key="day.key" class="space-y-2">
      <h2 class="flex items-center gap-2 px-1 font-display text-sm font-semibold tracking-tight text-fg">
        {{ day.label }}
        <span v-if="day.isToday" class="rounded-full bg-bronze px-1.5 py-0.5 text-[0.5625rem] font-semibold uppercase tracking-wide text-bronze-ink">Today</span>
      </h2>
      <ul class="space-y-2">
        <li
          v-for="s in day.sessions"
          :key="s.id"
          class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
          :class="s.status === 'cancelled' ? 'opacity-50' : ''"
        >
          <button
            class="grid h-6 w-6 shrink-0 place-items-center rounded-full border transition-colors"
            :class="s.status === 'completed' ? 'border-paid bg-paid/20 text-paid' : 'border-line text-transparent hover:border-bronze'"
            aria-label="Mark session done"
            @click="toggleDone(s)"
          >
            <Check class="h-3.5 w-3.5" />
          </button>
          <RouterLink :to="`/schedule/${s.id}`" class="flex min-w-0 flex-1 items-center gap-3">
            <div class="w-14 shrink-0 text-center">
              <p class="font-display text-sm font-semibold tnum">{{ formatTime(s.start) }}</p>
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium" :class="s.status === 'completed' ? 'line-through opacity-70' : ''">{{ s.title }}</p>
              <p class="truncate text-xs text-muted">{{ s.type === "private" ? (s.attendees.length > 1 ? `${s.attendees.length} booked` : (s.attendees[0]?.traineeName ?? "Private")) : `${s.attendees.length} booked` }}</p>
            </div>
            <Badge :tone="s.type === 'group' ? 'bronze' : 'info'">{{ s.type === "group" ? "Group" : "Private" }}</Badge>
          </RouterLink>
        </li>
      </ul>
    </section>
  </div>
</template>
