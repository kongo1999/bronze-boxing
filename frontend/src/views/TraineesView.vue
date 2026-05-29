<script setup lang="ts">
import { ref, computed } from "vue";
import { RouterLink } from "vue-router";
import { ChevronRight, UserPlus, Users } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";
import type { Trainee } from "@/lib/types";
import { money } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import { btnClasses } from "@/components/ui/button";

const q = ref("");
const { data, loading } = useAsync(() => api.get<Trainee[]>(`/trainees`));

const filtered = computed(() => {
  const list = data.value ?? [];
  const term = q.value.trim().toLowerCase();
  return term ? list.filter((t) => t.name.toLowerCase().includes(term)) : list;
});
const activeCount = computed(() => filtered.value.filter((t) => t.status === "active").length);
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Roster" title="Crew">
      <template #action>
        <RouterLink to="/trainees/new" :class="btnClasses('primary', 'sm')">
          <UserPlus class="h-4 w-4" /> Add
        </RouterLink>
      </template>
    </PageHeader>

    <input
      v-model="q"
      type="search"
      placeholder="Search by name…"
      class="w-full rounded-xl border border-line bg-elevated px-3 py-2.5 text-sm outline-none placeholder:text-faint focus:border-bronze/40"
    />

    <p v-if="loading" class="text-sm text-faint">Loading…</p>

    <EmptyState
      v-else-if="filtered.length === 0"
      :icon="Users"
      :title="q ? 'No matches' : 'No trainees yet'"
      :description="q ? 'Try a different name.' : 'Add your first trainee to start tracking dues and attendance.'"
    >
      <Button v-if="!q" to="/trainees/new" size="sm"><UserPlus class="h-4 w-4" /> Add trainee</Button>
    </EmptyState>

    <template v-else>
      <p class="px-1 text-sm text-faint">
        {{ activeCount }} active{{ filtered.length !== activeCount ? ` · ${filtered.length - activeCount} inactive` : "" }}
      </p>
      <ul class="space-y-2">
        <li v-for="t in filtered" :key="t.id">
          <RouterLink
            :to="`/trainees/${t.id}`"
            class="flex items-center gap-3 rounded-2xl border border-line bg-surface p-3 transition-colors hover:border-bronze/30"
          >
            <Avatar :name="t.name" :class="t.status === 'inactive' ? 'opacity-50' : ''" />
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium">{{ t.name }}</p>
              <p class="truncate text-sm text-muted">{{ t.phone || (t.skillLevel ? t.skillLevel : "No phone") }}</p>
            </div>
            <div class="flex flex-col items-end gap-1">
              <span v-if="t.monthlyFee > 0" class="font-display text-sm tnum">
                {{ money(t.monthlyFee) }}<span class="text-xs text-faint">/mo</span>
              </span>
              <Badge v-if="t.status === 'inactive'" tone="neutral">Inactive</Badge>
            </div>
            <ChevronRight class="h-4 w-4 shrink-0 text-faint" />
          </RouterLink>
        </li>
      </ul>
    </template>
  </div>
</template>
