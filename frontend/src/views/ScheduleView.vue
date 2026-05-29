<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { RouterLink } from "vue-router";
import { ChevronLeft, ChevronRight, CalendarPlus } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Session } from "@/lib/types";
import { formatTime } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Badge from "@/components/ui/Badge.vue";
import { btnClasses } from "@/components/ui/button";

const cursor = ref(new Date());
const selected = ref(new Date());
const sessions = ref<Session[]>([]);

const monthLabel = computed(() =>
  cursor.value.toLocaleDateString("en-US", { month: "long", year: "numeric" }),
);

function sameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

async function load() {
  const y = cursor.value.getFullYear();
  const m = cursor.value.getMonth();
  const from = `${y}-${String(m + 1).padStart(2, "0")}-01`;
  const next = new Date(y, m + 1, 1);
  const to = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, "0")}-01`;
  sessions.value = await api.get<Session[]>(`/sessions?from=${from}&to=${to}`);
}
watch(cursor, load, { immediate: true });

// Build a Monday-start grid covering the visible month.
const weeks = computed(() => {
  const y = cursor.value.getFullYear();
  const m = cursor.value.getMonth();
  const first = new Date(y, m, 1);
  const startOffset = (first.getDay() + 6) % 7; // days since Monday
  const gridStart = new Date(y, m, 1 - startOffset);
  const out: { date: Date; inMonth: boolean; count: number }[][] = [];
  for (let w = 0; w < 6; w++) {
    const row: { date: Date; inMonth: boolean; count: number }[] = [];
    for (let d = 0; d < 7; d++) {
      const date = new Date(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + w * 7 + d);
      const count = sessions.value.filter((s) => sameDay(new Date(s.start), date)).length;
      row.push({ date, inMonth: date.getMonth() === m, count });
    }
    out.push(row);
  }
  return out;
});

const daySessions = computed(() =>
  sessions.value
    .filter((s) => sameDay(new Date(s.start), selected.value))
    .sort((a, b) => +new Date(a.start) - +new Date(b.start)),
);
const selectedLabel = computed(() =>
  selected.value.toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" }),
);

function shift(delta: number) {
  cursor.value = new Date(cursor.value.getFullYear(), cursor.value.getMonth() + delta, 1);
}
const today = new Date();
const dows = ["M", "T", "W", "T", "F", "S", "S"];
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Calendar" title="Schedule">
      <template #action>
        <RouterLink to="/schedule/new" :class="btnClasses('primary', 'sm')"><CalendarPlus class="h-4 w-4" /> New</RouterLink>
      </template>
    </PageHeader>

    <div class="rounded-2xl border border-line bg-surface p-3">
      <div class="mb-2 flex items-center justify-between">
        <button :class="btnClasses('ghost', 'icon')" aria-label="Previous month" @click="shift(-1)"><ChevronLeft class="h-5 w-5" /></button>
        <p class="font-display font-semibold tracking-tight">{{ monthLabel }}</p>
        <button :class="btnClasses('ghost', 'icon')" aria-label="Next month" @click="shift(1)"><ChevronRight class="h-5 w-5" /></button>
      </div>
      <div class="grid grid-cols-7 gap-1 text-center">
        <span v-for="(d, i) in dows" :key="i" class="label-eyebrow py-1 text-[0.6rem] text-faint">{{ d }}</span>
      </div>
      <div v-for="(week, wi) in weeks" :key="wi" class="grid grid-cols-7 gap-1">
        <button
          v-for="(cell, ci) in week"
          :key="ci"
          class="relative aspect-square rounded-lg text-sm transition-colors"
          :class="[
            cell.inMonth ? 'text-fg' : 'text-faint/40',
            sameDay(cell.date, selected) ? 'bg-bronze text-bronze-ink font-semibold' : 'hover:bg-elevated',
          ]"
          @click="selected = cell.date"
        >
          <span :class="sameDay(cell.date, today) && !sameDay(cell.date, selected) ? 'text-bronze font-semibold' : ''">
            {{ cell.date.getDate() }}
          </span>
          <span
            v-if="cell.count > 0"
            class="absolute bottom-1 left-1/2 h-1 w-1 -translate-x-1/2 rounded-full"
            :class="sameDay(cell.date, selected) ? 'bg-bronze-ink' : 'bg-bronze'"
          />
        </button>
      </div>
    </div>

    <section class="space-y-2">
      <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">{{ selectedLabel }}</h2>
      <p v-if="daySessions.length === 0" class="rounded-xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
        No sessions this day.
      </p>
      <ul v-else class="space-y-2">
        <li v-for="s in daySessions" :key="s.id">
          <RouterLink
            :to="`/schedule/${s.id}`"
            class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
            :class="s.status === 'cancelled' ? 'opacity-50' : ''"
          >
            <div class="w-14 shrink-0 text-center">
              <p class="font-display text-sm font-semibold tnum">{{ formatTime(s.start) }}</p>
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium" :class="s.status === 'cancelled' ? 'line-through' : ''">{{ s.title }}</p>
              <p class="truncate text-xs text-muted">{{ s.type === "private" ? (s.attendees[0]?.traineeName ?? "Private") : `${s.attendees.length} booked` }}</p>
            </div>
            <Badge :tone="s.type === 'group' ? 'bronze' : 'info'">{{ s.type === "group" ? "Group" : "Private" }}</Badge>
          </RouterLink>
        </li>
      </ul>
    </section>
  </div>
</template>
