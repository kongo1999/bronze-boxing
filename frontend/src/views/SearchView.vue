<script setup lang="ts">
import { ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { Search as SearchIcon } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { SearchResults } from "@/lib/types";
import { money, formatLongDate, formatTime } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Avatar from "@/components/ui/Avatar.vue";

const q = ref("");
const results = ref<SearchResults>();
let timer: ReturnType<typeof setTimeout> | undefined;

watch(q, (val) => {
  clearTimeout(timer);
  if (!val.trim()) {
    results.value = undefined;
    return;
  }
  timer = setTimeout(async () => {
    results.value = await api.get<SearchResults>(`/search?q=${encodeURIComponent(val)}`);
  }, 250);
});

const empty = (r?: SearchResults) =>
  r && r.trainees.length + r.sessions.length + r.payments.length + r.inventory.length === 0;
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Find" title="Search" />
    <div class="relative">
      <SearchIcon class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
      <input
        v-model="q"
        type="search"
        autofocus
        placeholder="Trainees, sessions, payments, items…"
        class="w-full rounded-xl border border-line bg-elevated py-2.5 pl-9 pr-3 text-sm outline-none placeholder:text-faint focus:border-bronze/40"
      />
    </div>

    <p v-if="empty(results)" class="text-sm text-faint">No matches.</p>

    <template v-if="results">
      <section v-if="results.trainees.length" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Trainees</h2>
        <RouterLink
          v-for="t in results.trainees"
          :key="t.id"
          :to="`/trainees/${t.id}`"
          class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
        >
          <Avatar :name="t.name" class="h-8 w-8 text-xs" />
          <span class="text-sm font-medium">{{ t.name }}</span>
        </RouterLink>
      </section>

      <section v-if="results.sessions.length" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Sessions</h2>
        <RouterLink
          v-for="s in results.sessions"
          :key="s.id"
          :to="`/schedule/${s.id}`"
          class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
        >
          <span class="text-sm font-medium">{{ s.title }}</span>
          <span class="text-xs text-faint">{{ formatLongDate(s.start) }} · {{ formatTime(s.start) }}</span>
        </RouterLink>
      </section>

      <section v-if="results.payments.length" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Payments</h2>
        <div v-for="p in results.payments" :key="p.id" class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5">
          <span class="text-sm font-medium">{{ p.traineeName || p.note || "Payment" }}</span>
          <span class="font-display text-sm tnum">{{ money(p.amount) }}</span>
        </div>
      </section>

      <section v-if="results.inventory.length" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Inventory</h2>
        <RouterLink
          v-for="i in results.inventory"
          :key="i.id"
          to="/inventory"
          class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
        >
          <span class="text-sm font-medium">{{ i.name }}</span>
          <span class="text-xs text-faint">{{ i.stock }} in stock</span>
        </RouterLink>
      </section>
    </template>
  </div>
</template>
