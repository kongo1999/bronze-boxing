<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ChevronLeft, ChevronRight, CalendarPlus, Check, CalendarDays, Plus, TriangleAlert } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
import { toast } from "@/lib/toast";
import type { Session, SeriesProgress } from "@/lib/types";
import {
  addDays, browserIsElsewhere, dayOf, dayStartISO, formatDay, formatTime, isDayKey, mondayOf, studioTZ, todayKey,
} from "@/lib/studio";
import { useQueryState, withBack } from "@/lib/route-state";
import PageHeader from "@/components/ui/PageHeader.vue";
import Badge from "@/components/ui/Badge.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Alert from "@/components/ui/Alert.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import { btnClasses } from "@/components/ui/button";
import { fuzzyFilter } from "@/lib/fuzzy";
import Highlight from "@/components/ui/Highlight.vue";

const route = useRoute();

// The visible week is its Monday, kept in the URL (?week=YYYY-MM-DD) so a
// trip to a session and back — or a reload — lands on the same week.
const week = useQueryState("week", () => mondayOf(todayKey()), isDayKey);
const weekStart = computed(() => mondayOf(week.value));
const isCurrentWeek = computed(() => weekStart.value === mondayOf(todayKey()));
const weekLabel = computed(() => {
  const end = addDays(weekStart.value, 6);
  const s = formatDay(weekStart.value, { month: "short", day: "numeric" });
  const e = formatDay(end, { month: "short", day: "numeric", year: "numeric" });
  return `${s} – ${e}`;
});
const here = computed(() => route.fullPath);

const sessions = ref<Session[]>([]);
const loading = ref(false);
const failed = ref<string>();
let loadedWeek = "";
const cacheKey = () => `sessions:week:${weekStart.value}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  failed.value = undefined;
  const from = dayStartISO(weekStart.value);
  const to = dayStartISO(addDays(weekStart.value, 7));
  try {
    const res = await api.get<Session[]>(`/sessions?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`);
    if (my !== loadToken) return; // ignore stale (out-of-order) responses
    sessions.value = res;
    loadedWeek = weekStart.value;
    writeCache(cacheKey(), res);
    loadSeries(res);
  } catch (e) {
    if (my !== loadToken) return;
    // Never show a failed load as an empty week: say so, keep a retry.
    failed.value = errMsg(e, "Couldn't load the schedule.");
    if (loadedWeek === weekStart.value) toast("Couldn't refresh — showing saved data.", "error");
  } finally {
    if (my === loadToken) loading.value = false;
  }
}
// Series counters ("9/12") for every series visible this week, in one call.
const series = ref<Record<string, SeriesProgress>>({});
async function loadSeries(list: Session[]) {
  const ids = [...new Set(list.map((s) => s.seriesId).filter(Boolean))] as string[];
  if (!ids.length) return;
  try {
    const rows = await api.get<SeriesProgress[]>(`/sessions/series?ids=${ids.join(",")}`);
    const next = { ...series.value };
    for (const r of rows) next[r.seriesId] = r;
    series.value = next;
  } catch {
    /* counters are a hint; the week still shows */
  }
}
function showCached(): boolean {
  const hit = readCache<Session[]>(cacheKey());
  if (hit) {
    sessions.value = hit;
    loadedWeek = weekStart.value;
  } else {
    sessions.value = [];
    loadedWeek = "";
  }
  return !!hit;
}
watch(weekStart, () => { loading.value = !showCached(); load(); }, { immediate: true });

// Filters: a tiny fixed set, so visible chips rather than a dropdown.
type Filter = "all" | "scheduled" | "completed" | "cancelled" | "attention";
const FILTERS: Filter[] = ["all", "scheduled", "completed", "cancelled", "attention"];
const filter = useQueryState<Filter>("show", () => "all", (v): v is Filter => FILTERS.includes(v as Filter));
const filters: { v: Filter; l: string }[] = [
  { v: "all", l: "All" },
  { v: "scheduled", l: "Scheduled" },
  { v: "attention", l: "Needs attendance" },
  { v: "completed", l: "Done" },
  { v: "cancelled", l: "Cancelled" },
];

/** Done, but someone is still marked booked: attendance hasn't been taken. */
const needsAttendance = (s: Session) => s.status === "completed" && s.attendees.some((a) => a.status === "booked");
const booked = (s: Session) => s.attendees.length;
const isFull = (s: Session) => !!s.capacity && s.status !== "cancelled" && booked(s) >= s.capacity;

// Search narrows the visible week by class title, location, or who's booked —
// "who is Rami in this week?" without leaving the schedule.
const q = ref("");
const matching = computed(() => {
  const f = filter.value;
  const inFilter = sessions.value.filter((s) => (f === "attention" ? needsAttendance(s) : f === "all" || s.status === f));
  // Days stay in time order; the search only decides which classes show.
  const hit = new Set(fuzzyFilter(inFilter, q.value, (s) => [s.title, s.attendees.map((a) => a.traineeName).join(" "), s.location, s.type]));
  return inFilter.filter((s) => hit.has(s));
});
const attentionCount = computed(() => sessions.value.filter(needsAttendance).length);

// Group the visible week's sessions by studio day (Monday first); skip empty
// days so the list reads like a to-do list, not a mostly-blank grid.
const days = computed(() => {
  const today = todayKey();
  const out: { key: string; label: string; isToday: boolean; sessions: Session[] }[] = [];
  for (let i = 0; i < 7; i++) {
    const day = addDays(weekStart.value, i);
    const daySessions = matching.value
      .filter((s) => dayOf(s.start) === day)
      .sort((a, b) => +new Date(a.start) - +new Date(b.start));
    if (daySessions.length === 0) continue;
    out.push({ key: day, label: formatDay(day), isToday: day === today, sessions: daySessions });
  }
  return out;
});
const isEmpty = computed(() => days.value.length === 0);

function shift(delta: number) {
  week.value = addDays(weekStart.value, delta * 7);
}
function goToday() {
  week.value = mondayOf(todayKey());
}
const picker = ref<HTMLInputElement>();
function openPicker() {
  const el = picker.value;
  if (!el) return;
  // Native picker: keyboard- and screen-reader-friendly on every platform.
  if (typeof el.showPicker === "function") {
    try {
      el.showPicker();
      return;
    } catch {
      /* fall through */
    }
  }
  const v = window.prompt("Jump to date (YYYY-MM-DD)", weekStart.value);
  if (v && isDayKey(v.trim())) week.value = mondayOf(v.trim());
}
function jump(e: Event) {
  const v = (e.target as HTMLInputElement).value;
  if (isDayKey(v)) week.value = mondayOf(v);
}
// A new session opens on the day being looked at: today in the current week,
// the week's Monday otherwise.
const newDay = computed(() => (isCurrentWeek.value ? todayKey() : weekStart.value));

// Optimistic: flip the checkbox immediately, fire one request, revert on failure.
async function toggleDone(s: Session) {
  if (s.status === "cancelled") return;
  const prev = s.status;
  const next = s.status === "completed" ? "scheduled" : "completed";
  s.status = next;
  try {
    await api.put(`/sessions/${s.id}`, { status: next });
    writeCache(cacheKey(), sessions.value);
  } catch (e) {
    s.status = prev;
    toast(errMsg(e, "Couldn't update that session."), "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Classes" title="Schedule">
      <template #action>
        <RouterLink :to="withBack('/schedule/new', here, { day: newDay })" :class="btnClasses('primary', 'sm')"><CalendarPlus class="h-4 w-4" /> New</RouterLink>
      </template>
    </PageHeader>

    <div class="relative flex items-center justify-between gap-2 rounded-2xl border border-line bg-surface p-2">
      <button :class="btnClasses('ghost', 'icon')" aria-label="Previous week" @click="shift(-1)"><ChevronLeft class="h-5 w-5" /></button>
      <button type="button" class="flex min-h-10 min-w-0 flex-1 flex-col items-center rounded-xl px-2 hover:bg-elevated" aria-label="Jump to a date" @click="openPicker">
        <span class="font-display font-semibold tracking-tight">{{ weekLabel }}</span>
        <span class="text-[0.6875rem] text-faint">Tap to jump to a date</span>
      </button>
      <input
        ref="picker"
        type="date"
        :value="weekStart"
        tabindex="-1"
        aria-hidden="true"
        class="pointer-events-none absolute h-0 w-0 opacity-0"
        @change="jump"
      />
      <button :class="btnClasses('ghost', 'icon')" aria-label="Next week" @click="shift(1)"><ChevronRight class="h-5 w-5" /></button>
    </div>

    <div v-if="!isCurrentWeek" class="flex justify-center">
      <button :class="btnClasses('ghost', 'sm')" @click="goToday"><CalendarDays class="h-4 w-4" /> Jump to this week</button>
    </div>
    <p v-if="browserIsElsewhere()" class="px-1 text-xs text-faint">Times shown in studio time ({{ studioTZ() }}).</p>

    <div class="-mx-1 flex gap-1.5 overflow-x-auto px-1 pb-1" role="radiogroup" aria-label="Show">
      <button
        v-for="f in filters"
        :key="f.v"
        type="button"
        role="radio"
        :aria-checked="filter === f.v"
        class="min-h-10 shrink-0 rounded-lg border px-3 text-xs font-medium transition-colors"
        :class="filter === f.v ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
        @click="filter = f.v"
      >
        {{ f.l }}<template v-if="f.v === 'attention' && attentionCount"> · {{ attentionCount }}</template>
      </button>
    </div>

    <SearchInput v-model="q" placeholder="Search this week — class, trainee, place…" />

    <Alert v-if="failed">
      {{ failed }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <Skeleton v-if="loading" :rows="4" />

    <EmptyState
      v-else-if="isEmpty && !failed"
      :icon="CalendarDays"
      :title="q || filter !== 'all' ? 'No matches this week' : 'No classes this week'"
      :description="q || filter !== 'all' ? 'Try another word or filter, or step to a different week.' : 'Add a session to fill the week.'"
    />

    <section v-for="day in days" :key="day.key" class="space-y-2">
      <h2 class="flex items-center gap-2 px-1 font-display text-sm font-semibold tracking-tight text-fg">
        {{ day.label }}
        <span v-if="day.isToday" class="rounded-full bg-bronze px-1.5 py-0.5 text-[0.5625rem] font-semibold uppercase tracking-wide text-bronze-ink">Today</span>
        <RouterLink
          :to="withBack('/schedule/new', here, { day: day.key })"
          class="ml-auto grid h-10 w-10 place-items-center rounded-lg text-faint transition-colors hover:bg-elevated hover:text-bronze"
          :aria-label="`New session on ${day.label}`"
        ><Plus class="h-4 w-4" /></RouterLink>
      </h2>
      <ul class="space-y-2">
        <li
          v-for="s in day.sessions"
          :key="s.id"
          class="flex items-center gap-3 rounded-xl border bg-surface px-3 py-2.5"
          :class="[
            s.status === 'cancelled' ? 'opacity-50' : '',
            needsAttendance(s) ? 'border-partial/50' : 'border-line',
          ]"
        >
          <button
            class="grid h-10 w-10 shrink-0 place-items-center rounded-full"
            :aria-label="s.status === 'completed' ? 'Mark session not done' : 'Mark session done'"
            :disabled="s.status === 'cancelled'"
            @click="toggleDone(s)"
          >
            <span
              class="grid h-6 w-6 place-items-center rounded-full border transition-colors"
              :class="s.status === 'completed' ? 'border-paid bg-paid/20 text-paid' : 'border-line text-transparent hover:border-bronze'"
            ><Check class="h-3.5 w-3.5" /></span>
          </button>
          <RouterLink :to="withBack(`/schedule/${s.id}`, here)" class="flex min-w-0 flex-1 items-center gap-3">
            <div class="w-14 shrink-0 text-center">
              <p class="font-display text-sm font-semibold tnum">{{ formatTime(s.start) }}</p>
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium" :class="s.status === 'completed' ? 'line-through opacity-70' : ''"><Highlight :text="s.title" :q="q" /></p>
              <p class="flex items-center gap-1 truncate text-xs text-muted">
                <template v-if="s.type === 'private' && s.attendees.length === 1">{{ s.attendees[0]?.traineeName ?? "Private" }}</template>
                <template v-else-if="s.capacity">
                  <span class="tnum" :class="isFull(s) ? 'font-semibold text-partial' : ''">{{ booked(s) }}/{{ s.capacity }} booked</span>
                  <span v-if="isFull(s)" class="text-partial">· full</span>
                </template>
                <template v-else>{{ booked(s) }} booked</template>
              </p>
              <p v-if="s.seriesId && series[s.seriesId]" class="mt-0.5 text-[0.6875rem] text-faint tnum">
                Series {{ series[s.seriesId].completed }}/{{ series[s.seriesId].planned }} done<template v-if="series[s.seriesId].cancelled"> · {{ series[s.seriesId].cancelled }} cancelled</template>
              </p>
              <p v-if="needsAttendance(s)" class="mt-0.5 flex items-center gap-1 text-xs font-medium text-partial">
                <TriangleAlert class="h-3 w-3" /> Attendance needed
              </p>
            </div>
            <Badge :tone="s.type === 'group' ? 'bronze' : 'info'">{{ s.type === "group" ? "Group" : "Private" }}</Badge>
          </RouterLink>
        </li>
      </ul>
    </section>
  </div>
</template>
