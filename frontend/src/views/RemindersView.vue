<script setup lang="ts">
import { RouterLink } from "vue-router";
import { Plus, Bell, Check, Trash2 } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";
import type { Reminder } from "@/lib/types";
import { formatLongDate } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { btnClasses } from "@/components/ui/button";

const { data, loading, reload } = useAsync(() => api.get<Reminder[]>("/reminders"));

async function toggle(r: Reminder) {
  await api.put(`/reminders/${r.id}`, { done: !r.done });
  reload();
}
async function remove(r: Reminder) {
  await api.del(`/reminders/${r.id}`);
  reload();
}
const dotColor = (p: string) => (p === "high" ? "bg-overdue" : p === "normal" ? "bg-bronze" : "bg-faint");
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="To-do" title="Reminders">
      <template #action>
        <RouterLink to="/reminders/new" :class="btnClasses('primary', 'sm')"><Plus class="h-4 w-4" /> New</RouterLink>
      </template>
    </PageHeader>

    <p v-if="loading" class="text-sm text-faint">Loading…</p>
    <EmptyState v-else-if="(data ?? []).length === 0" :icon="Bell" title="No reminders" description="Add things you need to remember this week." />
    <ul v-else class="space-y-2">
      <li
        v-for="r in data"
        :key="r.id"
        class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
        :class="r.done ? 'opacity-50' : ''"
      >
        <button
          class="grid h-6 w-6 shrink-0 place-items-center rounded-full border transition-colors"
          :class="r.done ? 'border-paid bg-paid/20 text-paid' : 'border-line text-transparent hover:border-bronze'"
          @click="toggle(r)"
        >
          <Check class="h-3.5 w-3.5" />
        </button>
        <span class="h-2 w-2 shrink-0 rounded-full" :class="dotColor(r.priority)" />
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm" :class="r.done ? 'line-through' : ''">{{ r.title }}</p>
          <p class="text-xs text-faint">{{ formatLongDate(r.dueDate) }}</p>
        </div>
        <button class="text-faint hover:text-overdue" aria-label="Delete" @click="remove(r)"><Trash2 class="h-4 w-4" /></button>
      </li>
    </ul>
  </div>
</template>
