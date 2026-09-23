<script setup lang="ts">
// A trainee's session plan at a glance: attended over target, what's left,
// what's booked, and the exceptions (no-shows, classes not yet marked).
import { computed } from "vue";
import type { SessionPlan } from "@/lib/types";
import { formatDay } from "@/lib/studio";

const props = defineProps<{ plan: SessionPlan }>();
const p = computed(() => props.plan.progress);
const pct = computed(() => Math.min(100, Math.round((p.value.completed / Math.max(1, p.value.target)) * 100)));
const booked = computed(() => Math.min(100 - pct.value, Math.round((p.value.upcomingBooked / Math.max(1, p.value.target)) * 100)));
const dates = computed(() => {
  const s = formatDay(props.plan.startDate, { month: "short", day: "numeric" });
  return props.plan.endDate ? `${s} – ${formatDay(props.plan.endDate, { month: "short", day: "numeric" })}` : `from ${s}`;
});
</script>

<template>
  <div class="space-y-2">
    <div class="flex items-baseline justify-between gap-2">
      <p class="truncate text-sm font-medium">
        {{ plan.title }}
        <span v-if="plan.status !== 'active'" class="ml-1 text-xs font-normal capitalize text-faint">· {{ plan.status }}</span>
      </p>
      <p class="shrink-0 font-display text-lg font-semibold tnum">
        {{ p.completed }}/{{ p.target }} <span class="text-xs font-normal text-muted">attended</span>
      </p>
    </div>
    <div class="flex h-2 overflow-hidden rounded-full bg-elevated" role="progressbar" :aria-valuenow="p.completed" :aria-valuemax="p.target" :aria-label="`${p.completed} of ${p.target} sessions attended`">
      <span class="h-2 bg-paid/80" :style="{ width: `${pct}%` }" />
      <span class="h-2 bg-bronze/40" :style="{ width: `${booked}%` }" />
    </div>
    <p class="text-xs text-faint tnum">
      <span class="font-medium text-fg">{{ p.remaining }} remaining</span>
      · {{ p.upcomingBooked }} booked
      <template v-if="p.unassignedSlots"> · {{ p.unassignedSlots }} not yet booked</template>
      <template v-if="p.noShows"> · <span class="text-overdue">{{ p.noShows }} no-show{{ p.noShows === 1 ? "" : "s" }}</span></template>
      <template v-if="p.attendanceNeeded"> · <span class="text-partial">{{ p.attendanceNeeded }} to mark</span></template>
      <template v-if="p.cancelled"> · {{ p.cancelled }} cancelled</template>
      · {{ plan.sessionType ? `${plan.sessionType} · ` : "" }}{{ dates }}
    </p>
  </div>
</template>
