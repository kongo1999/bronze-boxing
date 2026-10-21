<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ChevronLeft, ChevronRight, CalendarPlus, Check, CalendarDays, Plus, TriangleAlert } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { readCache, writeCache, invalidate } from "@/lib/cache";
import { toast } from "@/lib/toast";
import type { Session, SeriesProgress, SessionPlan, SeriesDetail } from "@/lib/types";
import { askReason } from "@/lib/prompt";
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
    loadProgress(res, my);
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
const studioCompleted = ref<number>();
const traineeCompleted = ref<Record<string, number>>({});
const activePlans = ref<SessionPlan[]>([]);
const studentQ = ref("");
async function loadProgress(list: Session[], token: number) {
  const ids = [...new Set(list.flatMap((s) => s.attendees.map((a) => a.trainee)))];
  try {
    const chunks = ids.length ? Array.from({ length: Math.ceil(ids.length / 100) }, (_, i) => ids.slice(i * 100, (i + 1) * 100)) : [[]];
    const [statRows, planRows] = await Promise.all([
      Promise.all(chunks.map((batch) => api.get<{ studioCompleted: number; traineeCompleted: Record<string, number> }>(`/sessions/attendance-stats?trainees=${batch.join(",")}`))),
      Promise.all(chunks.filter((batch) => batch.length).map((batch) => api.get<SessionPlan[]>(`/session-plans?trainees=${batch.join(",")}&status=active`))),
    ]);
    if (token !== loadToken) return;
    studioCompleted.value = statRows[0]?.studioCompleted;
    traineeCompleted.value = Object.assign({}, ...statRows.map((x) => x.traineeCompleted));
    activePlans.value = planRows.flat();
  } catch { /* Progress is an extra hint; the schedule remains usable. */ }
}
const coachWeekDone = computed(() => sessions.value.filter((s) => s.status === "completed").length);
const studentWeekVisits = computed(() => sessions.value.reduce((n, s) => n + (s.status === "completed" ? s.attendees.filter((a) => a.status === "attended").length : 0), 0));
const studentRows = computed(() => {
  const byID = new Map<string, { id: string; name: string; week: number; total: number; plan?: SessionPlan }>();
  for (const s of sessions.value) for (const a of s.attendees) {
    const row: { id: string; name: string; week: number; total: number; plan?: SessionPlan } =
      byID.get(a.trainee) ?? { id: a.trainee, name: a.traineeName ?? "Member", week: 0, total: traineeCompleted.value[a.trainee] ?? 0 };
    if (s.status === "completed" && a.status === "attended") row.week++;
    row.plan ??= activePlans.value.find((p) => p.trainee === a.trainee);
    byID.set(a.trainee, row);
  }
  return fuzzyFilter([...byID.values()].sort((a, b) => a.name.localeCompare(b.name)), studentQ.value, (r) => [r.name, r.plan?.title]);
});
const planFor = (id?: string) => activePlans.value.find((p) => p.trainee === id);
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
    traineeCompleted.value = {};
    activePlans.value = [];
  } else {
    sessions.value = [];
    loadedWeek = "";
    traineeCompleted.value = {};
    activePlans.value = [];
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

// Bulk selection survives week navigation, so scattered classes can be
// gathered without opening and editing each one. The backend rechecks all
// selected IDs before committing an action.
type SelectedClass = { id: string; title: string; start: string; seriesId?: string };
const bulkMode = ref(false);
const selected = ref<Record<string, SelectedClass>>({});
const selectedRows = computed(() => Object.values(selected.value).sort((a, b) => a.start.localeCompare(b.start)));
const selectedQ = ref("");
const shownSelected = computed(() => fuzzyFilter(selectedRows.value, selectedQ.value, (s) => [s.title, formatDay(dayOf(s.start)), s.seriesId]));
const bulkBusy = ref(false);
const bulkEditOpen = ref(false);
const bulkFields = reactive({ title: false, time: false, durationMin: false, location: false, capacity: false, fee: false });
const bulkForm = reactive({ title: "", time: "18:00", durationMin: 60, location: "", capacity: 0, fee: 0 });
function selectClass(s: SelectedClass) {
  if (selected.value[s.id]) delete selected.value[s.id];
  else if (selectedRows.value.length >= 200) toast("Select at most 200 classes at once.", "error");
  else selected.value[s.id] = { id: s.id, title: s.title, start: s.start, seriesId: s.seriesId };
}
function selectShown() {
  for (const s of matching.value.filter((s) => s.status === "scheduled")) {
    if (selected.value[s.id]) continue;
    if (selectedRows.value.length >= 200) return toast("Select at most 200 classes at once.", "error");
    selected.value[s.id] = { id: s.id, title: s.title, start: s.start, seriesId: s.seriesId };
  }
}
async function selectSeries(s: Session) {
  if (!s.seriesId) return;
  try {
    const detail = await api.get<SeriesDetail>(`/sessions/series/${s.seriesId}/progress`);
    const rows = detail.occurrences.filter((o) => o.status === "scheduled");
    const newRows = rows.filter((o) => !selected.value[o.id]);
    if (selectedRows.value.length + newRows.length > 200) return toast("This would exceed the 200-class selection limit.", "error");
    for (const o of rows) selected.value[o.id] = { id: o.id, title: detail.progress.title || s.title, start: o.start, seriesId: s.seriesId };
    toast(`Selected ${rows.length} unfinished classes in this series.`, "success");
  } catch (e) { toast(errMsg(e, "Couldn't select the series."), "error"); }
}
function stopBulk() {
  bulkMode.value = false;
  bulkEditOpen.value = false;
  selected.value = {};
  selectedQ.value = "";
}
async function bulkEdit() {
  const ids = selectedRows.value.map((s) => s.id);
  if (!ids.length || bulkBusy.value) return;
  const changes: Record<string, unknown> = {};
  for (const key of Object.keys(bulkFields) as (keyof typeof bulkFields)[]) if (bulkFields[key]) changes[key] = bulkForm[key];
  if (!Object.keys(changes).length) return toast("Choose at least one field to change.", "error");
  const reason = await askReason({ title: `Edit ${ids.length} selected classes?`, message: "Only scheduled classes can be changed. All selected classes must pass the checks or none will change.", confirmLabel: "Apply to selected", tone: "primary", suggestions: ["Schedule correction", "Room change"] });
  if (reason === null) return;
  bulkBusy.value = true;
  try {
    const result = await api.patch<{ changed: number }>("/sessions/bulk", { ids, changes, reason });
    invalidate("sessions", "dashboard", "trainees");
    await load();
    toast(`Updated ${result.changed} classes.`, "success");
    stopBulk();
  } catch (e) { toast(errMsg(e, "Couldn't edit selected classes."), "error"); }
  finally { bulkBusy.value = false; }
}
async function bulkDelete() {
  const ids = selectedRows.value.map((s) => s.id);
  if (!ids.length || bulkBusy.value) return;
  const reason = await askReason({ title: `Delete ${ids.length} selected classes?`, message: "Classes with attendance, linked payments, waitlists, cancellations or notes cannot be deleted. If one is protected, none will be deleted. Series counters adjust for deleted classes.", confirmLabel: "Delete selected", tone: "danger", suggestions: ["Created by mistake", "Schedule replaced"] });
  if (reason === null) return;
  bulkBusy.value = true;
  try {
    const result = await api.post<{ deleted: number }>("/sessions/bulk/delete", { ids, reason });
    invalidate("sessions", "dashboard", "trainees");
    await load();
    toast(`Deleted ${result.deleted} classes.`, "success");
    stopBulk();
  } catch (e) { toast(errMsg(e, "Couldn't delete selected classes."), "error"); }
  finally { bulkBusy.value = false; }
}

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
    loadProgress(sessions.value, loadToken);
    if (s.seriesId) loadSeries(sessions.value);
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
        <button :class="btnClasses('ghost', 'sm')" @click="bulkMode ? stopBulk() : (bulkMode = true)">{{ bulkMode ? 'Done selecting' : 'Manage classes' }}</button>
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

    <div class="grid grid-cols-3 gap-2 text-center"><div class="rounded-xl border border-line bg-surface p-2"><p class="font-display text-xl font-semibold tnum">{{ coachWeekDone }}</p><p class="text-[0.6875rem] text-faint">Coach classes done this week</p></div><div class="rounded-xl border border-line bg-surface p-2"><p class="font-display text-xl font-semibold tnum">{{ studioCompleted ?? '—' }}</p><p class="text-[0.6875rem] text-faint">Coach classes done overall</p></div><div class="rounded-xl border border-line bg-surface p-2"><p class="font-display text-xl font-semibold tnum">{{ studentWeekVisits }}</p><p class="text-[0.6875rem] text-faint">Student attendances this week</p></div></div>
    <details v-if="sessions.some((s) => s.attendees.length)" class="rounded-xl border border-line bg-surface p-3"><summary class="cursor-pointer text-sm font-medium text-bronze">Student class totals for this week’s roster</summary><div class="mt-3 space-y-2"><SearchInput v-model="studentQ" label="Search student totals" placeholder="Find a student…" :matches="studentQ ? studentRows.length : undefined" /><p v-if="!studentRows.length" class="text-xs text-faint">No students match.</p><ul class="max-h-72 space-y-1 overflow-y-auto"><li v-for="r in studentRows" :key="r.id" class="flex flex-wrap justify-between gap-1 rounded-lg border border-line p-2 text-xs"><RouterLink :to="withBack(`/trainees/${r.id}`, here)" class="font-medium text-bronze">{{ r.name }}</RouterLink><span class="tnum">{{ r.week }} this week · {{ r.total }} attended overall<template v-if="r.plan"> · {{ r.plan.progress.completed }}/{{ r.plan.progress.target }} in {{ r.plan.title }} ({{ r.plan.progress.remaining }} left)</template></span></li></ul></div></details>

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

    <div v-if="bulkMode" class="space-y-3 rounded-xl border border-bronze/40 bg-bronze/10 p-3">
      <div class="flex flex-wrap items-center justify-between gap-2"><div><p class="font-medium">{{ selectedRows.length }} classes selected</p><p class="text-xs text-faint">Select random classes across weeks, or add every unfinished class in a series. Only scheduled classes can be selected.</p></div><div class="flex gap-2"><button :class="btnClasses('ghost', 'sm')" @click="selectShown">Select shown</button><button v-if="selectedRows.length" :class="btnClasses('ghost', 'sm')" @click="selected = {}">Clear</button></div></div>
      <SearchInput v-if="selectedRows.length >= 8" v-model="selectedQ" label="Search selected classes" placeholder="Find a selected class…" :matches="selectedQ ? shownSelected.length : undefined" />
      <ul v-if="selectedRows.length" class="max-h-36 space-y-1 overflow-y-auto"><li v-if="selectedQ && !shownSelected.length" class="text-xs text-faint">No selected classes match.</li><li v-for="s in shownSelected" :key="s.id" class="flex items-center justify-between gap-2 rounded-lg bg-surface px-2 py-1 text-xs"><span class="truncate">{{ formatDay(dayOf(s.start)) }} · {{ formatTime(s.start) }} · {{ s.title }}</span><button type="button" class="shrink-0 text-bronze" :aria-label="`Remove ${s.title} from selection`" @click="delete selected[s.id]">Remove</button></li></ul>
      <div v-if="selectedRows.length" class="flex flex-wrap gap-2"><button :class="btnClasses('ghost', 'sm')" @click="bulkEditOpen = !bulkEditOpen">{{ bulkEditOpen ? 'Hide edit fields' : 'Edit selected' }}</button><button :class="btnClasses('danger', 'sm')" :disabled="bulkBusy" @click="bulkDelete">Delete selected</button></div>
      <div v-if="bulkEditOpen && selectedRows.length" class="space-y-2 rounded-lg bg-surface p-3 text-sm"><p class="text-xs text-faint">Tick only the fields you want to change in every selected class. Time keeps each class on its current date.</p><label class="flex items-center gap-2"><input v-model="bulkFields.title" type="checkbox" /> Title <input v-model="bulkForm.title" aria-label="New title" :disabled="!bulkFields.title" class="min-w-0 flex-1 rounded-lg border border-line bg-elevated p-2" /></label><label class="flex items-center gap-2"><input v-model="bulkFields.time" type="checkbox" /> Time <input v-model="bulkForm.time" aria-label="New time" :disabled="!bulkFields.time" type="time" class="ml-auto rounded-lg border border-line bg-elevated p-2" /></label><label class="flex items-center gap-2"><input v-model="bulkFields.durationMin" type="checkbox" /> Duration (minutes) <input v-model.number="bulkForm.durationMin" aria-label="New duration in minutes" :disabled="!bulkFields.durationMin" type="number" min="1" class="ml-auto w-24 rounded-lg border border-line bg-elevated p-2" /></label><label class="flex items-center gap-2"><input v-model="bulkFields.location" type="checkbox" /> Location <input v-model="bulkForm.location" aria-label="New location" :disabled="!bulkFields.location" class="min-w-0 flex-1 rounded-lg border border-line bg-elevated p-2" /></label><label class="flex items-center gap-2"><input v-model="bulkFields.capacity" type="checkbox" /> Capacity <input v-model.number="bulkForm.capacity" aria-label="New capacity" :disabled="!bulkFields.capacity" type="number" min="0" class="ml-auto w-24 rounded-lg border border-line bg-elevated p-2" /></label><label class="flex items-center gap-2"><input v-model="bulkFields.fee" type="checkbox" /> Class fee <input v-model.number="bulkForm.fee" aria-label="New class fee" :disabled="!bulkFields.fee" type="number" min="0" step="0.01" class="ml-auto w-24 rounded-lg border border-line bg-elevated p-2" /></label><button :class="btnClasses('primary', 'sm')" :disabled="bulkBusy" @click="bulkEdit">Apply changes to {{ selectedRows.length }} classes</button></div>
    </div>

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
          <button v-if="bulkMode"
            type="button" class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line"
            :disabled="s.status !== 'scheduled'" :aria-label="selected[s.id] ? `Deselect ${s.title}` : `Select ${s.title}`" :aria-pressed="!!selected[s.id]"
            @click="selectClass(s)"
          ><Check class="h-4 w-4" :class="selected[s.id] ? 'text-bronze' : 'text-transparent'" /></button>
          <button v-else
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
              <p v-if="s.type === 'private' && s.attendees.length === 1 && planFor(s.attendees[0]?.trainee)" class="mt-0.5 text-[0.6875rem] text-bronze tnum">Student plan {{ planFor(s.attendees[0]?.trainee)!.progress.completed }}/{{ planFor(s.attendees[0]?.trainee)!.progress.target }} done · {{ planFor(s.attendees[0]?.trainee)!.progress.remaining }} left</p>
              <p v-if="needsAttendance(s)" class="mt-0.5 flex items-center gap-1 text-xs font-medium text-partial">
                <TriangleAlert class="h-3 w-3" /> Attendance needed
              </p>
            </div>
            <Badge :tone="s.type === 'group' ? 'bronze' : 'info'">{{ s.type === "group" ? "Group" : "Private" }}</Badge>
          </RouterLink>
          <button v-if="bulkMode && s.seriesId && s.status === 'scheduled'" type="button" class="shrink-0 rounded-lg border border-line px-2 py-1 text-xs text-bronze" :aria-label="`Select all unfinished classes in ${s.title} series`" @click="selectSeries(s)">Series</button>
        </li>
      </ul>
    </section>
  </div>
</template>
