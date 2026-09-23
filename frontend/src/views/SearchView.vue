<script setup lang="ts">
// Global search: one box over trainees, sessions, payments, sales, items,
// expenses and reminders — typo-tolerant, in typed groups, each result
// opening the exact record. Suggestions arrive as you type (no Enter), the
// query lives in the URL, and "searching" is never shown as "no matches".
import { computed, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { api } from "@/lib/api";
import type { SearchGroup, SearchHit, SearchKind, SearchResponse } from "@/lib/types";
import { money, formatLongDate, formatTime } from "@/lib/format";
import { useQueryState, withBack } from "@/lib/route-state";
import { useRemoteSearch } from "@/lib/remote-search";
import PageHeader from "@/components/ui/PageHeader.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Highlight from "@/components/ui/Highlight.vue";
import Badge from "@/components/ui/Badge.vue";
import Alert from "@/components/ui/Alert.vue";

const route = useRoute();
const q = useQueryState<string>("q", () => "");
const term = ref(q.value);
watch(term, (v) => (q.value = v.trim()));
const here = computed(() => route.fullPath);

const { result, searching, error, retry } = useRemoteSearch(term, (s) =>
  api.get<SearchResponse>(`/search?q=${encodeURIComponent(s)}&limit=5`),
);

// "Show all" per group appends that group's next page.
const extra = ref<Record<string, SearchHit[]>>({});
const loadingMore = ref<SearchKind | "">("");
watch(result, () => (extra.value = {}));
const groups = computed<SearchGroup[]>(() =>
  (result.value?.groups ?? []).map((g) => {
    const items = [...g.items, ...(extra.value[g.kind] ?? [])];
    return { ...g, items, hasMore: items.length < g.total };
  }),
);
async function showMore(g: SearchGroup) {
  loadingMore.value = g.kind;
  try {
    const res = await api.get<SearchResponse>(
      `/search?q=${encodeURIComponent(term.value.trim())}&kinds=${g.kind}&limit=20&offset=${g.items.length}`,
    );
    extra.value = { ...extra.value, [g.kind]: [...(extra.value[g.kind] ?? []), ...(res.groups[0]?.items ?? [])] };
  } finally {
    loadingMore.value = "";
  }
}
const total = computed(() => (result.value?.groups ?? []).reduce((n, g) => n + g.total, 0));

const TITLE: Record<SearchKind, string> = {
  trainee: "Crew",
  session: "Sessions",
  payment: "Payments",
  sale: "Shop sales",
  item: "Items",
  expense: "Expenses",
  reminder: "Reminders",
};
const FLAG: Record<string, [string, "neutral" | "overdue" | "paid" | "info"]> = {
  void: ["Void", "overdue"],
  archived: ["Archived", "neutral"],
  inactive: ["Inactive", "neutral"],
  cancelled: ["Cancelled", "overdue"],
  completed: ["Done", "paid"],
  done: ["Done", "paid"],
};

// Where each result opens: the record itself, or the filtered page for
// kinds without a detail page (an expense opens its month's ledger, found
// by the same search and highlighted).
function linkOf(h: SearchHit): string {
  switch (h.kind) {
    case "trainee":
      return `/trainees/${h.id}`;
    case "session":
      return `/schedule/${h.id}`;
    case "payment":
      return `/payments/${h.id}`;
    case "sale":
      return `/sales/${h.id}`;
    case "item":
      return `/inventory/${h.id}`;
    case "reminder":
      return `/reminders/${h.id}/edit`;
    case "expense":
      return `/financials?m=${h.month}&kind=expense&q=${encodeURIComponent(term.value.trim())}&focus=${h.id}`;
  }
}
function detail(h: SearchHit): string {
  const parts: string[] = [];
  if (h.kind === "session" && h.date) parts.push(`${formatLongDate(h.date)} · ${formatTime(h.date)}`);
  else if (h.date && h.kind !== "trainee" && h.kind !== "item") parts.push(formatLongDate(h.date));
  return parts.join(" · ");
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Find" title="Search" />
    <SearchInput
      v-model="term"
      label="Search everything"
      placeholder="Names, phone, sessions, items, notes…"
      :searching="searching"
      :matches="term.trim() && !searching && result ? total : undefined"
      autofocus
    />

    <Alert v-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="retry">Retry</button>
    </Alert>

    <p v-if="result?.didYouMean" class="text-sm text-muted">
      Did you mean
      <button class="font-medium text-bronze underline-offset-2 hover:underline" @click="term = result.didYouMean">{{ result.didYouMean }}</button>?
      <span v-if="total" class="text-faint">Showing the closest matches.</span>
    </p>

    <p v-if="!term.trim()" class="text-sm text-faint">
      Try a name, part of a phone number, a session, an item or SKU, or a note. Typos are fine.
    </p>
    <p v-else-if="result && !searching && total === 0 && !error" class="text-sm text-faint">
      Nothing matches “{{ term.trim() }}”. Check the spelling or try fewer words.
    </p>

    <section v-for="g in groups" :key="g.kind" class="space-y-2">
      <h2 class="flex items-baseline justify-between px-1 label-eyebrow text-[0.625rem] text-faint">
        <span>{{ TITLE[g.kind] }}</span><span class="tnum">{{ g.total }}</span>
      </h2>
      <RouterLink
        v-for="h in g.items"
        :key="h.id"
        :to="withBack(linkOf(h), here)"
        class="flex min-h-12 items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2 transition-colors hover:border-bronze/30"
      >
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium" :class="h.flag === 'void' ? 'line-through decoration-overdue/60' : ''">
            <Highlight :text="h.label" :q="term" />
          </p>
          <p v-if="h.sub || detail(h)" class="truncate text-xs text-faint">
            <Highlight v-if="h.sub" :text="h.sub" :q="term" /><template v-if="h.sub && detail(h)"> · </template>{{ detail(h) }}
          </p>
        </div>
        <Badge v-if="h.flag && FLAG[h.flag]" :tone="FLAG[h.flag][1]">{{ FLAG[h.flag][0] }}</Badge>
        <span v-if="h.amount !== undefined && h.kind !== 'trainee'" class="shrink-0 font-display text-sm tnum">{{ money(h.amount) }}</span>
      </RouterLink>
      <button
        v-if="g.hasMore"
        class="min-h-10 px-1 text-sm font-medium text-bronze hover:underline disabled:opacity-50"
        :disabled="loadingMore === g.kind"
        @click="showMore(g)"
      >{{ loadingMore === g.kind ? "Loading…" : `Show more ${TITLE[g.kind].toLowerCase()} (${g.total - g.items.length})` }}</button>
    </section>
  </div>
</template>
