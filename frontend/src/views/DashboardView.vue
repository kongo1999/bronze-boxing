<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { ArrowRight, CircleCheck } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";
import type { Dashboard } from "@/lib/types";
import { money, monthLabel, formatTime } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import { btnClasses } from "@/components/ui/button";

const { data, loading, error } = useAsync(() => api.get<Dashboard>("/dashboard"));

const greeting = computed(() => {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
});
const dateLabel = computed(() =>
  new Date().toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" }),
);
</script>

<template>
  <div class="space-y-5">
    <PageHeader :eyebrow="dateLabel" :title="greeting" />

    <p v-if="error" class="rounded-xl border border-overdue/30 bg-overdue/10 px-4 py-3 text-sm text-overdue">
      {{ error }} — is the API running on :8080?
    </p>
    <p v-else-if="loading" class="text-sm text-faint">Loading…</p>

    <template v-else-if="data">
      <div class="grid grid-cols-2 gap-3">
        <StatTile :label="`${monthLabel(data.month)} · cash in`" :value="money(data.monthRevenue)" sub="collected" accent />
        <StatTile
          label="Active crew"
          :value="data.activeTrainees"
          :sub="data.overdueCount > 0 ? `${data.overdueCount} owe dues` : 'all paid up'"
        />
      </div>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Today's sessions</h2>
          <RouterLink to="/schedule" class="inline-flex items-center gap-1 text-sm font-medium text-bronze">
            Schedule <ArrowRight class="h-3.5 w-3.5" />
          </RouterLink>
        </div>
        <Card v-if="data.todaySessions.length === 0" class="px-4 py-5 text-sm text-muted">
          No classes on the calendar today.
        </Card>
        <ul v-else class="space-y-2">
          <li v-for="s in data.todaySessions" :key="s.id">
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
                <p class="truncate text-xs text-muted">
                  {{ s.type === "private" ? (s.attendees[0]?.traineeName ?? "Private") : `${s.attendees.length} booked` }}
                </p>
              </div>
              <Badge :tone="s.type === 'group' ? 'bronze' : 'info'">{{ s.type === "group" ? "Group" : "Private" }}</Badge>
            </RouterLink>
          </li>
        </ul>
      </section>

      <section v-if="data.overdueSubscriptions.length > 0" class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Needs collecting</h2>
          <RouterLink to="/payments" class="inline-flex items-center gap-1 text-sm font-medium text-bronze">
            Money <ArrowRight class="h-3.5 w-3.5" />
          </RouterLink>
        </div>
        <ul class="space-y-2">
          <li
            v-for="s in data.overdueSubscriptions.slice(0, 5)"
            :key="s.trainee.id"
            class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
          >
            <RouterLink :to="`/trainees/${s.trainee.id}`" class="flex min-w-0 items-center gap-2.5">
              <Avatar :name="s.trainee.name" />
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">{{ s.trainee.name }}</p>
                <p class="text-xs text-overdue tnum">owes {{ money(s.due - s.amountPaid) }}</p>
              </div>
            </RouterLink>
            <RouterLink
              :to="`/payments/new?trainee=${s.trainee.id}&type=subscription&periodMonth=${data.month}&amount=${s.due - s.amountPaid}`"
              :class="btnClasses('primary', 'sm')"
            >
              Collect
            </RouterLink>
          </li>
        </ul>
      </section>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Reminders</h2>
          <RouterLink to="/reminders" class="inline-flex items-center gap-1 text-sm font-medium text-bronze">
            All <ArrowRight class="h-3.5 w-3.5" />
          </RouterLink>
        </div>
        <Card v-if="data.weekReminders.length === 0" class="flex items-center gap-2.5 px-4 py-5 text-sm text-muted">
          <CircleCheck class="h-4 w-4 text-paid" /> Nothing due this week.
        </Card>
        <ul v-else class="space-y-2">
          <li
            v-for="r in data.weekReminders"
            :key="r.id"
            class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
            :class="r.done ? 'opacity-50' : ''"
          >
            <span
              class="h-2 w-2 shrink-0 rounded-full"
              :class="r.priority === 'high' ? 'bg-overdue' : r.priority === 'normal' ? 'bg-bronze' : 'bg-faint'"
            />
            <p class="flex-1 truncate text-sm" :class="r.done ? 'line-through' : ''">{{ r.title }}</p>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
