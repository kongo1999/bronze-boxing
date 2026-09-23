<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import {
  ChevronLeft, Trash2, MapPin, Clock, Pencil, Copy, Bell, Ban, Check, TriangleAlert, Repeat, UserPlus, RotateCcw,
} from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { Session, Attendee, SessionPlan, SeriesDetail, Trainee, AttendanceStatus } from "@/lib/types";
import { formatTime, formatLongDate } from "@/lib/format";
import { formatDay, dayOf, timeOf, addDays } from "@/lib/studio";
import { backTarget, withBack } from "@/lib/route-state";
import { askReason } from "@/lib/prompt";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import Button from "@/components/ui/Button.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Highlight from "@/components/ui/Highlight.vue";
import { fuzzyFilter } from "@/lib/fuzzy";
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { traineeOption } from "@/lib/options";

const route = useRoute();
const router = useRouter();
const id = computed(() => route.params.id as string);
const session = ref<Session>();
const loading = ref(true);
const error = ref<string>();
const plans = ref<SessionPlan[]>([]);
const series = ref<SeriesDetail>();

async function load() {
  error.value = undefined;
  try {
    const s = await api.get<Session>(`/sessions/${id.value}`);
    session.value = s;
    loadPlans(s);
    if (s.seriesId) loadSeries(s.seriesId);
  } catch (e) {
    error.value = errMsg(e, "Couldn't load this session.");
  } finally {
    loading.value = false;
  }
}
async function loadPlans(s: Session) {
  if (!s.attendees.length) return (plans.value = []);
  try {
    plans.value = await api.get<SessionPlan[]>(`/session-plans?trainees=${s.attendees.map((a) => a.trainee).join(",")}`);
  } catch {
    /* plan badges are a hint */
  }
}
async function loadSeries(seriesId: string) {
  try {
    series.value = await api.get<SeriesDetail>(`/sessions/series/${seriesId}/progress`);
  } catch {
    series.value = undefined;
  }
}
watch(id, () => { loading.value = true; load(); }, { immediate: true });

const back = computed(() => backTarget(route.query, "/schedule"));
const here = computed(() => route.fullPath);
const dateLabel = (iso: string) => formatDay(dayOf(iso), { weekday: "long", month: "long", day: "numeric" });
function changed() {
  invalidate("sessions", "home-upcoming", "trainees");
}

// ── Attendance ────────────────────────────────────────────────────────────
const booked = computed(() => session.value?.attendees.filter((a) => a.status === "booked") ?? []);
const needsAttendance = computed(() => session.value?.status === "completed" && booked.value.length > 0);
// Outcomes (done, attended, no-show) wait for the class to begin; the server
// allows taking attendance up to 30 minutes early, at the door.
const started = computed(() => !!session.value && new Date(session.value.start).getTime() - 30 * 60_000 <= Date.now());
const planById = computed(() => Object.fromEntries(plans.value.map((p) => [p.id, p])));
const plansFor = (trainee: string) => plans.value.filter((p) => p.trainee === trainee && p.status === "active");

// Optimistic: update the chip immediately, fire one request, revert on failure.
async function setAttendance(a: Attendee, status: AttendanceStatus) {
  if (a.status === status) return;
  const prev = a.status;
  a.status = status;
  try {
    session.value = await api.patch<Session>(`/sessions/${id.value}/attendance`, { trainee: a.trainee, status });
    changed();
    loadPlans(session.value);
    if (session.value.seriesId) loadSeries(session.value.seriesId);
  } catch (e) {
    a.status = prev;
    toast(errMsg(e, "Couldn't save attendance."), "error");
  }
}

// After a class: review who was still "booked", then mark them attended in
// one go. No-shows already marked are never touched.
const reviewing = ref(false);
const reviewPick = reactive<Record<string, boolean>>({});
function startReview() {
  for (const a of booked.value) reviewPick[a.trainee] = true;
  reviewing.value = true;
}
async function confirmReview() {
  const trainees = booked.value.filter((a) => reviewPick[a.trainee]).map((a) => a.trainee);
  if (!trainees.length) return (reviewing.value = false);
  try {
    session.value = await api.post<Session>(`/sessions/${id.value}/attendance/bulk`, { status: "attended", trainees });
    reviewing.value = false;
    changed();
    loadPlans(session.value);
    toast(`Marked ${trainees.length} attended.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't save attendance."), "error");
  }
}

async function setStatus(status: Session["status"]) {
  const s = session.value;
  if (!s) return;
  try {
    session.value = await api.put<Session>(`/sessions/${id.value}`, { status });
    changed();
    if (session.value.seriesId) loadSeries(session.value.seriesId);
    if (status === "completed" && booked.value.length) startReview();
  } catch (e) {
    toast(errMsg(e, "Couldn't update the session."), "error");
  }
}
async function cancelClass() {
  const reason = await askReason({
    title: "Cancel this class?",
    message: "It stays on the schedule, greyed out, stops blocking the slot, and doesn't count toward anyone's plan.",
    confirmLabel: "Cancel class",
    required: false,
    suggestions: ["Coach away", "Holiday", "Not enough bookings"],
  });
  if (reason === null) return;
  await setStatus("cancelled");
}

// Linking a booking to one of the trainee's plans (or unlinking it).
const planning = ref<string>();
async function setPlan(a: Attendee, planId: string, override = false) {
  try {
    session.value = await api.patch<Session>(`/sessions/${id.value}/attendance`, {
      trainee: a.trainee, status: a.status, planId, override,
    });
    planning.value = undefined;
    changed();
    loadPlans(session.value);
  } catch (e) {
    if (isApiError(e, "PLAN_FULL") && confirm(`${e.message}`)) return setPlan(a, planId, true);
    toast(errMsg(e, "Couldn't link the plan."), "error");
  }
}

// Booking someone into the class.
const adding = ref(false);
const addPick = ref("");
const roster = ref<Trainee[]>([]);
async function openAdd() {
  adding.value = !adding.value;
  if (adding.value && !roster.value.length) {
    try {
      roster.value = (await api.get<Trainee[]>("/trainees")).filter((t) => t.status === "active");
    } catch (e) {
      toast(errMsg(e, "Couldn't load trainees."), "error");
    }
  }
}
const addOptions = computed(() => {
  const inClass = new Set(session.value?.attendees.map((a) => a.trainee));
  return roster.value.filter((t) => !inClass.has(t.id)).map(traineeOption);
});
watch(addPick, async (tid) => {
  if (!tid) return;
  try {
    session.value = await api.patch<Session>(`/sessions/${id.value}/attendance`, { trainee: tid, status: "booked" });
    addPick.value = "";
    changed();
    loadPlans(session.value);
  } catch (e) {
    addPick.value = "";
    toast(errMsg(e, "Couldn't book that trainee."), "error");
  }
});

// Searching the class list and the series' dates, once they're long enough
// to scroll. The same matcher as everywhere (typos, accents, "no show").
const ATTENDANCE_SEARCH_AT = 6;
const attQ = ref("");
const statusWord: Record<string, string> = { booked: "booked", attended: "attended", no_show: "no-show" };
const shownAttendees = computed(() =>
  fuzzyFilter(session.value?.attendees ?? [], attQ.value, (a) => [a.traineeName, statusWord[a.status]]),
);
const seriesQ = ref("");
const occLabel = (o: { start: string }) => `${formatDay(dayOf(o.start), { weekday: "short", month: "short", day: "numeric" })} · ${formatTime(o.start)}`;
const shownOccurrences = computed(() =>
  fuzzyFilter(series.value?.occurrences ?? [], seriesQ.value, (o) => [occLabel(o), o.attendanceNeeded ? "needs attendance" : o.status]),
);

// ── Series ────────────────────────────────────────────────────────────────
const seriesOpen = ref(false);
const futureEdit = ref(false);
const futureForm = reactive({ title: "", time: "", durationMin: 60, location: "" });
function openFutureEdit() {
  const s = session.value;
  if (!s) return;
  Object.assign(futureForm, { title: s.title, time: timeOf(s.start), durationMin: s.durationMin, location: s.location ?? "" });
  futureEdit.value = true;
}
async function saveFuture() {
  const s = session.value;
  if (!s?.seriesId) return;
  const changes: Record<string, unknown> = {};
  if (futureForm.title.trim() && futureForm.title.trim() !== s.title) changes.title = futureForm.title.trim();
  if (futureForm.time && futureForm.time !== timeOf(s.start)) changes.time = futureForm.time;
  if (futureForm.durationMin !== s.durationMin) changes.durationMin = futureForm.durationMin;
  if (futureForm.location.trim() !== (s.location ?? "")) changes.location = futureForm.location.trim();
  if (!Object.keys(changes).length) return (futureEdit.value = false);
  const reason = await askReason({
    title: "Change this and every later class?",
    message: "Classes already completed keep their time and attendance.",
    confirmLabel: "Apply to future",
    tone: "primary",
    suggestions: ["New time", "Room change"],
  });
  if (reason === null) return;
  try {
    const res = await api.patch<{ changed: number }>(`/sessions/series/${s.seriesId}`, {
      scope: "future", fromOccurrence: s.id, changes, reason,
    });
    futureEdit.value = false;
    changed();
    await load();
    toast(`Updated ${res.changed} class${res.changed === 1 ? "" : "es"}.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't change the series."), "error");
  }
}
async function cancelFuture() {
  const s = session.value;
  if (!s?.seriesId) return;
  const reason = await askReason({
    title: "End the series from this class?",
    message: "This class and every later one still scheduled are cancelled (kept, greyed out). The series' planned count drops by that many.",
    confirmLabel: "End series here",
    suggestions: ["Summer break", "Moved to another day", "Trainee stopped"],
  });
  if (reason === null) return;
  try {
    const res = await api.post<{ cancelled: number }>(`/sessions/series/${s.seriesId}/end`, { fromOccurrence: s.id, reason });
    changed();
    await load();
    toast(`Cancelled ${res.cancelled} class${res.cancelled === 1 ? "" : "es"}.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't end the series."), "error");
  }
}
const extendTo = ref("");
async function extend() {
  const s = session.value;
  if (!s?.seriesId || !extendTo.value) return;
  const reason = await askReason({ title: "Extend the series?", confirmLabel: "Extend", tone: "primary", suggestions: ["Renewed"] });
  if (reason === null) return;
  try {
    const res = await api.post<{ added: number }>(`/sessions/series/${s.seriesId}/extend`, { toDay: extendTo.value, reason });
    changed();
    await load();
    toast(`Added ${res.added} class${res.added === 1 ? "" : "es"}.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't extend the series."), "error");
  }
}
const lastSeriesDay = computed(() => {
  const occ = series.value?.occurrences;
  return occ?.length ? dayOf(occ[occ.length - 1].start) : "";
});

// ── Other actions ─────────────────────────────────────────────────────────
const hasRecordedAttendance = computed(() => session.value?.attendees.some((a) => a.status !== "booked") ?? false);
const deleting = ref(false);
async function remove() {
  if (deleting.value || !confirm("Delete this session? Nothing has been recorded on it yet. (Cancel it instead to keep it on the schedule.)")) return;
  deleting.value = true;
  try {
    await api.del(`/sessions/${id.value}`);
    changed();
    router.push(back.value);
  } catch (e) {
    deleting.value = false;
    toast(errMsg(e, "Couldn't delete session."), "error");
  }
}
const remindLink = computed(() => {
  const s = session.value;
  if (!s) return "";
  const q = new URLSearchParams({ relatedType: "session", relatedId: s.id, relatedLabel: `${s.title} · ${formatLongDate(s.start)}`, day: addDays(dayOf(s.start), -1) });
  return `/reminders/new?${q.toString()}`;
});
const statusButtons: { v: AttendanceStatus; l: string }[] = [
  { v: "attended", l: "Attended" },
  { v: "no_show", l: "No-show" },
  { v: "booked", l: "Booked" },
];
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Schedule
    </RouterLink>
    <Skeleton v-if="loading" variant="detail" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else-if="session">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <h1 class="font-display text-xl font-semibold" :class="session.status === 'cancelled' ? 'line-through opacity-70' : ''">
              {{ session.title }}
            </h1>
            <p class="text-sm text-muted">{{ dateLabel(session.start) }}</p>
          </div>
          <div class="flex shrink-0 flex-col items-end gap-1.5">
            <Badge :tone="session.type === 'group' ? 'bronze' : 'info'">{{ session.type === "group" ? "Group" : "Private" }}</Badge>
            <Badge v-if="session.status === 'cancelled'" tone="overdue">Cancelled</Badge>
            <Badge v-else-if="session.status === 'completed'" tone="paid">Done</Badge>
          </div>
        </div>
        <div class="mt-3 flex flex-wrap gap-3 text-sm text-muted">
          <span class="inline-flex items-center gap-1"><Clock class="h-4 w-4" /> {{ formatTime(session.start) }} · {{ session.durationMin }}min</span>
          <span v-if="session.location" class="inline-flex items-center gap-1"><MapPin class="h-4 w-4" /> {{ session.location }}</span>
          <span v-if="session.capacity" class="tnum">{{ session.attendees.length }}/{{ session.capacity }} booked</span>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <Button v-if="session.status === 'scheduled' && started" size="sm" @click="setStatus('completed')"><Check class="h-4 w-4" /> Mark done</Button>
          <button v-else-if="session.status === 'completed'" :class="btnClasses('ghost', 'sm')" @click="setStatus('scheduled')"><RotateCcw class="h-4 w-4" /> Reopen</button>
          <RouterLink :to="withBack(`/schedule/${id}/edit`, here)" :class="btnClasses('ghost', 'sm')"><Pencil class="h-4 w-4" /> Edit</RouterLink>
          <RouterLink :to="withBack('/schedule/new', here, { duplicate: id })" :class="btnClasses('ghost', 'sm')"><Copy class="h-4 w-4" /> Duplicate</RouterLink>
          <RouterLink :to="withBack(remindLink, here)" :class="btnClasses('ghost', 'sm')"><Bell class="h-4 w-4" /> Remind me</RouterLink>
          <button v-if="session.status === 'scheduled'" :class="btnClasses('ghost', 'sm')" @click="cancelClass"><Ban class="h-4 w-4" /> Cancel class</button>
          <button v-if="!hasRecordedAttendance && session.status !== 'completed'" :class="btnClasses('danger', 'sm')" :disabled="deleting" @click="remove">
            <Trash2 class="h-4 w-4" /> {{ deleting ? "Deleting…" : "Delete" }}
          </button>
        </div>
      </Card>

      <!-- Attendance needed: a completed class with people still "booked". -->
      <Card v-if="needsAttendance" class="border-partial/40 p-4">
        <p class="flex items-center gap-2 text-sm font-medium text-partial">
          <TriangleAlert class="h-4 w-4" /> Attendance needed — {{ booked.length }} still marked booked
        </p>
        <template v-if="reviewing">
          <p class="mt-2 text-xs text-faint">Untick anyone who didn't come; mark them no-show below.</p>
          <ul class="mt-2 space-y-1">
            <li v-for="a in booked" :key="a.trainee">
              <label class="flex min-h-10 items-center gap-3 rounded-lg px-2 hover:bg-elevated">
                <input v-model="reviewPick[a.trainee]" type="checkbox" class="h-5 w-5 accent-[oklch(0.74_0.14_162)]" />
                <span class="text-sm">{{ a.traineeName }}</span>
              </label>
            </li>
          </ul>
          <div class="mt-3 flex gap-2">
            <Button size="sm" @click="confirmReview">Mark {{ booked.filter((a) => reviewPick[a.trainee]).length }} attended</Button>
            <Button size="sm" variant="ghost" @click="reviewing = false">Not now</Button>
          </div>
        </template>
        <Button v-else size="sm" class="mt-3" @click="startReview">Review & mark attended</Button>
      </Card>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Attendance · {{ session.attendees.length }}{{ session.capacity ? ` of ${session.capacity}` : "" }}</h2>
          <button class="inline-flex min-h-10 items-center gap-1 px-1 text-sm font-medium text-bronze hover:underline" @click="openAdd"><UserPlus class="h-4 w-4" /> Book someone</button>
        </div>
        <div v-if="adding">
          <SearchSelect v-model="addPick" :options="addOptions" placeholder="Pick a trainee to book" search-placeholder="Search trainees…" />
        </div>
        <SearchInput
          v-if="session.attendees.length >= ATTENDANCE_SEARCH_AT"
          v-model="attQ"
          label="Search this class"
          placeholder="Find someone in this class…"
          :matches="attQ ? shownAttendees.length : undefined"
        />
        <ul class="space-y-2">
          <li v-for="a in shownAttendees" :key="a.trainee" class="rounded-xl border border-line bg-surface px-3 py-2">
            <div class="flex items-center gap-3">
              <Avatar :name="a.traineeName ?? '?'" class="h-8 w-8 text-xs" />
              <div class="min-w-0 flex-1">
                <RouterLink :to="withBack(`/trainees/${a.trainee}`, here)" class="block truncate text-sm font-medium hover:underline"><Highlight :text="a.traineeName" :q="attQ" /></RouterLink>
                <button
                  type="button"
                  class="text-left text-xs"
                  :class="a.planId ? 'text-bronze' : 'text-faint'"
                  :aria-expanded="planning === a.trainee"
                  @click="planning = planning === a.trainee ? undefined : a.trainee"
                >
                  <template v-if="a.planId && planById[a.planId]">
                    {{ planById[a.planId].title }} · {{ planById[a.planId].progress.completed }}/{{ planById[a.planId].progress.target }}
                  </template>
                  <template v-else-if="plansFor(a.trainee).length">No plan — link one</template>
                  <template v-else-if="a.planId">On a plan</template>
                </button>
              </div>
            </div>
            <div class="mt-2 grid grid-cols-3 gap-1" role="radiogroup" :aria-label="`${a.traineeName} attendance`">
              <button
                v-for="st in statusButtons"
                :key="st.v"
                type="button"
                role="radio"
                :aria-checked="a.status === st.v"
                :disabled="!started && st.v !== 'booked'"
                :title="!started && st.v !== 'booked' ? 'Available once the class starts' : undefined"
                class="min-h-10 rounded-lg border text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40"
                :class="a.status === st.v
                  ? st.v === 'attended' ? 'border-paid bg-paid/15 text-paid' : st.v === 'no_show' ? 'border-overdue bg-overdue/15 text-overdue' : 'border-bronze bg-bronze/15 text-bronze'
                  : 'border-line text-faint hover:text-fg'"
                @click="setAttendance(a, st.v)"
              >{{ st.l }}</button>
            </div>
            <div v-if="planning === a.trainee" class="mt-2 flex flex-wrap gap-1.5 border-t border-line/60 pt-2">
              <button
                v-for="p in plansFor(a.trainee)"
                :key="p.id"
                type="button"
                class="min-h-10 rounded-lg border px-3 text-xs"
                :class="a.planId === p.id ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted hover:text-fg'"
                @click="setPlan(a, p.id)"
              >{{ p.title }} · {{ p.progress.completed }}/{{ p.progress.target }}</button>
              <button v-if="a.planId" type="button" class="min-h-10 rounded-lg border border-line px-3 text-xs text-faint hover:text-fg" @click="setPlan(a, '')">No plan</button>
              <p v-if="!plansFor(a.trainee).length" class="text-xs text-faint">
                No active plans. <RouterLink :to="withBack(`/trainees/${a.trainee}`, here)" class="text-bronze hover:underline">Create one on their profile</RouterLink>.
              </p>
            </div>
          </li>
          <li v-if="session.attendees.length === 0" class="px-1 text-sm text-faint">No one booked yet.</li>
          <li v-else-if="!shownAttendees.length" class="px-1 text-sm text-faint">No one in this class matches “{{ attQ.trim() }}”.</li>
        </ul>
      </section>

      <!-- Series: completed / planned, and "this and future" changes. -->
      <Card v-if="session.seriesId && series" class="space-y-3 p-4">
        <div class="flex items-start justify-between gap-2">
          <div>
            <p class="label-eyebrow flex items-center gap-1 text-[0.625rem] text-faint"><Repeat class="h-3 w-3" /> Series</p>
            <p class="font-display text-2xl font-semibold tnum">{{ series.progress.completed }}/{{ series.progress.planned }} <span class="text-sm font-normal text-muted">completed</span></p>
            <p class="text-xs text-faint tnum">
              {{ series.progress.scheduled }} scheduled · {{ series.progress.cancelled }} cancelled<template v-if="series.progress.attendanceNeeded"> · <span class="text-partial">{{ series.progress.attendanceNeeded }} need attendance</span></template>
              <template v-if="series.progress.inferred"> · planned count inferred from existing classes</template>
            </p>
          </div>
          <button class="min-h-10 px-2 text-sm font-medium text-bronze hover:underline" :aria-expanded="seriesOpen" @click="seriesOpen = !seriesOpen">{{ seriesOpen ? "Hide" : "All dates" }}</button>
        </div>
        <SearchInput
          v-if="seriesOpen && series.occurrences.length >= ATTENDANCE_SEARCH_AT"
          v-model="seriesQ"
          label="Search this series' dates"
          placeholder="e.g. oct 14, cancelled, needs attendance"
          :matches="seriesQ ? shownOccurrences.length : undefined"
        />
        <ul v-if="seriesOpen" class="max-h-72 space-y-1 overflow-y-auto">
          <li v-if="!shownOccurrences.length" class="px-2 text-sm text-faint">No dates match.</li>
          <li v-for="o in shownOccurrences" :key="o.id">
            <RouterLink
              :to="withBack(`/schedule/${o.id}`, route.query.back as string || '/schedule')"
              class="flex min-h-10 items-center justify-between rounded-lg px-2 text-sm hover:bg-elevated"
              :class="o.id === session.id ? 'bg-bronze/10 text-bronze' : ''"
            >
              <span :class="o.status === 'cancelled' ? 'line-through text-faint' : ''"><Highlight :text="occLabel(o)" :q="seriesQ" /></span>
              <span class="text-xs" :class="o.attendanceNeeded ? 'text-partial' : o.status === 'completed' ? 'text-paid' : 'text-faint'">
                {{ o.attendanceNeeded ? "needs attendance" : o.status }}
              </span>
            </RouterLink>
          </li>
        </ul>
        <div v-if="session.status !== 'completed'" class="flex flex-wrap gap-2 border-t border-line pt-3">
          <button :class="btnClasses('ghost', 'sm')" @click="openFutureEdit">Change this & future</button>
          <button :class="btnClasses('ghost', 'sm')" @click="cancelFuture">End series here</button>
        </div>
        <div v-if="futureEdit" class="space-y-2 rounded-xl bg-elevated p-3">
          <label class="block"><span class="mb-1 block text-xs text-faint">Title</span><input v-model="futureForm.title" :class="inputCls" /></label>
          <div class="grid grid-cols-2 gap-2">
            <label class="block"><span class="mb-1 block text-xs text-faint">Time</span><input v-model="futureForm.time" type="time" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Minutes</span><input v-model.number="futureForm.durationMin" type="number" min="1" :class="inputCls" /></label>
          </div>
          <label class="block"><span class="mb-1 block text-xs text-faint">Location</span><input v-model="futureForm.location" :class="inputCls" /></label>
          <p class="text-xs text-faint">Applies from {{ formatLongDate(session.start) }} on; completed classes keep their time and attendance.</p>
          <div class="flex gap-2">
            <Button size="sm" @click="saveFuture">Apply to future</Button>
            <Button size="sm" variant="ghost" @click="futureEdit = false">Cancel</Button>
          </div>
        </div>
        <div class="flex flex-wrap items-end gap-2 border-t border-line pt-3">
          <label class="block"><span class="mb-1 block text-xs text-faint">Extend until</span><input v-model="extendTo" type="date" :min="lastSeriesDay" :class="inputCls" /></label>
          <Button size="sm" variant="ghost" :disabled="!extendTo" @click="extend">Extend</Button>
        </div>
      </Card>
    </template>

    <Alert v-else>Session not found.</Alert>
  </div>
</template>
