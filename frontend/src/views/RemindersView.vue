<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { Plus, Bell, Check, Trash2, Pencil, Repeat, AlarmClock, Link2 } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { useCachedAsync, invalidate, writeCache } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { Priority, Reminder } from "@/lib/types";
import { addDays, formatDay, todayKey } from "@/lib/studio";
import { useQueryState, withBack } from "@/lib/route-state";
import PageHeader from "@/components/ui/PageHeader.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import { fuzzyFilter } from "@/lib/fuzzy";
import Highlight from "@/components/ui/Highlight.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import { btnClasses } from "@/components/ui/button";
import { toast } from "@/lib/toast";
import { refreshBadges } from "@/lib/badges";

const route = useRoute();
const { data, loading, error, reload } = useCachedAsync("reminders", () => api.get<Reminder[]>("/reminders"));

// A reminder wants attention on its due day, or later if it was snoozed.
const effectiveDay = (r: Reminder) => (r.snoozedUntil && r.snoozedUntil > r.dueDay ? r.snoozedUntil : r.dueDay);

type PriFilter = "all" | Priority;
const priority = useQueryState<PriFilter>("priority", () => "all", (v): v is PriFilter => ["all", "high", "normal", "low"].includes(v as string));
const showDone = useQueryState<"0" | "1">("done", () => "0", (v): v is "0" | "1" => v === "0" || v === "1");

const q = ref("");
const filtered = computed(() =>
  fuzzyFilter(
    (data.value ?? []).filter((r) => priority.value === "all" || r.priority === priority.value),
    q.value,
    (r) => [r.title, r.relatedLabel],
  ),
);

// Open reminders grouped by when they need attention; done ones kept apart.
const RANK: Record<Priority, number> = { high: 0, normal: 1, low: 2 };
const byDue = (a: Reminder, b: Reminder) =>
  effectiveDay(a).localeCompare(effectiveDay(b)) || RANK[a.priority] - RANK[b.priority];
const groups = computed(() => {
  const today = todayKey();
  const open = filtered.value.filter((r) => !r.done).sort(byDue);
  return [
    { key: "overdue", label: "Overdue", tone: "text-overdue", items: open.filter((r) => effectiveDay(r) < today) },
    { key: "today", label: "Today", tone: "text-bronze", items: open.filter((r) => effectiveDay(r) === today) },
    { key: "upcoming", label: "Upcoming", tone: "text-faint", items: open.filter((r) => effectiveDay(r) > today) },
  ].filter((g) => g.items.length > 0);
});
const done = computed(() =>
  filtered.value.filter((r) => r.done).sort((a, b) => (b.doneAt ?? b.dueDay).localeCompare(a.doneAt ?? a.dueDay)),
);
const { page, pageCount, items: doneItems, total, from, to } = usePaged(done, 10);
const openCount = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0));

function dueLabel(r: Reminder): string {
  const day = effectiveDay(r);
  const today = todayKey();
  if (day === today) return "Today";
  if (day === addDays(today, 1)) return "Tomorrow";
  if (day === addDays(today, -1)) return "Yesterday";
  return formatDay(day, { weekday: "short", month: "short", day: "numeric" });
}
const relatedHref = (r: Reminder) =>
  r.relatedType === "trainee" ? `/trainees/${r.relatedId}`
  : r.relatedType === "session" ? `/schedule/${r.relatedId}`
  : r.relatedType === "item" ? `/inventory/${r.relatedId}`
  : "";

// Optimistic: flip the UI immediately, fire one request, revert only on
// failure. Completing a repeating reminder brings its next instance in.
async function toggle(r: Reminder) {
  const next = !r.done;
  r.done = next;
  try {
    await api.put(`/reminders/${r.id}`, { done: next });
    refreshBadges(true);
    invalidate("dashboard");
    if (r.recurrence) reload();
    else if (data.value) writeCache("reminders", data.value);
  } catch (e) {
    r.done = !next; // revert
    toast(errMsg(e, "Couldn't update that reminder."), "error");
  }
}
async function snooze(r: Reminder, days: number) {
  try {
    const updated = await api.post<Reminder>(`/reminders/${r.id}/snooze`, { days });
    refreshBadges(true);
    Object.assign(r, updated);
    if (data.value) writeCache("reminders", data.value);
    toast(`Snoozed until ${formatDay(updated.snoozedUntil ?? "", { weekday: "short", month: "short", day: "numeric" })}.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't snooze that reminder."), "error");
  }
}
async function remove(r: Reminder) {
  const list = data.value;
  if (!list) return;
  if (!confirm(`Delete "${r.title}"?`)) return;
  const idx = list.indexOf(r);
  if (idx >= 0) list.splice(idx, 1); // optimistic removal
  try {
    await api.del(`/reminders/${r.id}`);
    refreshBadges(true);
    writeCache("reminders", list);
  } catch {
    reload(); // restore true state on failure
    toast("Couldn't delete that reminder.", "error");
  }
}
const dotColor = (p: string) => (p === "high" ? "bg-overdue" : p === "normal" ? "bg-bronze" : "bg-faint");
const chips: { v: PriFilter; l: string }[] = [
  { v: "all", l: "All" },
  { v: "high", l: "High" },
  { v: "normal", l: "Normal" },
  { v: "low", l: "Low" },
];
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="To-do" title="Reminders">
      <template #action>
        <RouterLink :to="withBack('/reminders/new', route.fullPath)" :class="btnClasses('primary', 'sm')"><Plus class="h-4 w-4" /> New</RouterLink>
      </template>
    </PageHeader>

    <SearchInput v-model="q" placeholder="Search reminders…" />

    <div class="flex flex-wrap items-center gap-1.5" role="radiogroup" aria-label="Priority">
      <button
        v-for="c in chips"
        :key="c.v"
        type="button"
        role="radio"
        :aria-checked="priority === c.v"
        class="min-h-10 rounded-lg border px-3 text-xs font-medium transition-colors"
        :class="priority === c.v ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
        @click="priority = c.v"
      >{{ c.l }}</button>
      <button
        type="button"
        class="ml-auto min-h-10 rounded-lg px-3 text-xs font-medium text-faint hover:text-fg"
        :aria-pressed="showDone === '1'"
        @click="showDone = showDone === '1' ? '0' : '1'"
      >{{ showDone === "1" ? "Hide done" : `Show done (${done.length})` }}</button>
    </div>

    <Skeleton v-if="loading" :rows="4" />
    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="reload">Retry</button>
    </Alert>
    <EmptyState
      v-else-if="openCount === 0 && (showDone === '0' || done.length === 0)"
      :icon="Bell"
      :title="q || priority !== 'all' ? 'No matches' : 'Nothing to do'"
      :description="q || priority !== 'all' ? 'Try a different word or priority.' : 'Add things you need to remember this week.'"
    />

    <template v-else>
      <section v-for="g in groups" :key="g.key" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem]" :class="g.tone">{{ g.label }} · {{ g.items.length }}</h2>
        <ul class="space-y-2">
          <li v-for="r in g.items" :key="r.id" class="rounded-xl border border-line bg-surface px-1 py-1">
            <div class="flex items-center gap-1">
              <button
                class="grid h-10 w-10 shrink-0 place-items-center rounded-full"
                :aria-label="`Mark “${r.title}” done`"
                @click="toggle(r)"
              >
                <span class="grid h-6 w-6 place-items-center rounded-full border border-line text-transparent transition-colors hover:border-bronze">
                  <Check class="h-3.5 w-3.5" />
                </span>
              </button>
              <span class="h-2 w-2 shrink-0 rounded-full" :class="dotColor(r.priority)" :aria-label="`${r.priority} priority`" />
              <div class="min-w-0 flex-1 px-2">
                <p class="truncate text-sm"><Highlight :text="r.title" :q="q" /></p>
                <p class="flex flex-wrap items-center gap-x-2 text-xs text-faint">
                  <span :class="g.key === 'overdue' ? 'text-overdue' : ''">{{ dueLabel(r) }}</span>
                  <span v-if="r.snoozedUntil && r.snoozedUntil > r.dueDay" class="inline-flex items-center gap-0.5"><AlarmClock class="h-3 w-3" /> snoozed</span>
                  <span v-if="r.recurrence" class="inline-flex items-center gap-0.5 capitalize"><Repeat class="h-3 w-3" /> {{ r.recurrence }}</span>
                  <RouterLink v-if="r.relatedId && relatedHref(r)" :to="relatedHref(r)" class="inline-flex items-center gap-0.5 text-bronze hover:underline">
                    <Link2 class="h-3 w-3" /> {{ r.relatedLabel || r.relatedType }}
                  </RouterLink>
                </p>
              </div>
              <RouterLink
                :to="withBack(`/reminders/${r.id}/edit`, route.fullPath)"
                class="grid h-10 w-10 shrink-0 place-items-center rounded-lg text-purple transition-colors hover:bg-purple/10"
                :aria-label="`Edit “${r.title}”`"
              ><Pencil class="h-4 w-4" /></RouterLink>
            </div>
            <div class="flex items-center gap-1 border-t border-line/60 px-2 pt-1">
              <span class="text-[0.6875rem] text-faint">Snooze</span>
              <button class="min-h-10 rounded-lg px-2.5 text-xs font-medium text-muted hover:bg-elevated hover:text-fg" @click="snooze(r, 1)">Tomorrow</button>
              <button class="min-h-10 rounded-lg px-2.5 text-xs font-medium text-muted hover:bg-elevated hover:text-fg" @click="snooze(r, 7)">Next week</button>
              <button class="ml-auto grid h-10 w-10 place-items-center rounded-lg text-faint hover:bg-overdue/10 hover:text-overdue" :aria-label="`Delete “${r.title}”`" @click="remove(r)"><Trash2 class="h-4 w-4" /></button>
            </div>
          </li>
        </ul>
      </section>

      <section v-if="showDone === '1' && done.length" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Done · {{ done.length }}</h2>
        <ul class="space-y-2">
          <li v-for="r in doneItems" :key="r.id" class="flex items-center gap-1 rounded-xl border border-line bg-surface px-1 py-1 opacity-60">
            <button class="grid h-10 w-10 shrink-0 place-items-center rounded-full" :aria-label="`Mark “${r.title}” not done`" @click="toggle(r)">
              <span class="grid h-6 w-6 place-items-center rounded-full border border-paid bg-paid/20 text-paid"><Check class="h-3.5 w-3.5" /></span>
            </button>
            <div class="min-w-0 flex-1 px-2">
              <p class="truncate text-sm line-through"><Highlight :text="r.title" :q="q" /></p>
              <p class="text-xs text-faint">
                Due {{ formatDay(r.dueDay, { month: "short", day: "numeric" }) }}<template v-if="r.recurrence"> · repeats {{ r.recurrence }}</template>
              </p>
            </div>
            <button class="grid h-10 w-10 shrink-0 place-items-center rounded-lg text-faint hover:bg-overdue/10 hover:text-overdue" :aria-label="`Delete “${r.title}”`" @click="remove(r)"><Trash2 class="h-4 w-4" /></button>
          </li>
        </ul>
        <Pagination v-model="page" :page-count="pageCount" :total="total" :from="from" :to="to" label="done" />
      </section>
    </template>
  </div>
</template>
