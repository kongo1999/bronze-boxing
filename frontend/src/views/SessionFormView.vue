<script setup lang="ts">
import { reactive, ref, computed, onMounted, watch } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Users, User, Search, Check, TriangleAlert } from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { Session, Trainee, AttendanceStatus, SessionStatus, SessionPlan, RecurringPreview } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import ToggleSwitch from "@/components/ui/ToggleSwitch.vue";
import DateWheel from "@/components/ui/DateWheel.vue";
import TimeWheel from "@/components/ui/TimeWheel.vue";
import { inputCls } from "@/lib/ui";
import { dateKey } from "@/lib/format";
import { dayOf, timeOf, studioInstant, isDayKey, formatDay, formatTime } from "@/lib/studio";
import { backTarget } from "@/lib/route-state";
import { toast } from "@/lib/toast";

const route = useRoute();
const router = useRouter();

// One form, three jobs: /schedule/new creates (optionally ?duplicate=<id> to
// start from another session), /schedule/:id/edit updates one occurrence.
// Changing a whole series happens from the session page ("this & future").
const editId = (route.params.id as string | undefined) ?? "";
const isEdit = !!editId;
const duplicateId = !isEdit && typeof route.query.duplicate === "string" ? route.query.duplicate : "";

const trainees = ref<Trainee[]>([]);
const saving = ref(false);
const loading = ref(isEdit || !!duplicateId);
const error = ref<string>();
const conflictDates = ref<string[]>([]);
const recurring = ref(false);
const search = ref("");

// A new session opens on the day it was started from (?day=), at ?time= if
// given. Dates are studio days; times are the studio's wall clock.
const prefillDay = isDayKey(route.query.day) ? route.query.day : "";
const prefillTime = typeof route.query.time === "string" && /^\d{2}:\d{2}$/.test(route.query.time) ? route.query.time : "18:00";
const form = reactive({
  title: "",
  type: "group" as "group" | "private",
  date: prefillDay,
  time: prefillTime,
  durationMin: 60,
  location: "",
  capacity: 0, // 0 = unlimited
  status: "scheduled" as SessionStatus,
  attendees: [] as string[],
  // recurring
  weekdays: [] as number[],
  endDate: "",
});
// Which session plan each booking counts toward ("" = none).
const attendeePlan = reactive<Record<string, string>>({});
const planOverride = ref(false);
// Started from a trainee's profile ("Book session" / "Book toward plan").
if (!isEdit && !duplicateId && typeof route.query.attendee === "string") {
  form.attendees.push(route.query.attendee);
  attendeePlan[route.query.attendee] = typeof route.query.plan === "string" ? route.query.plan : "";
}

// Attendance already marked on this session, kept so re-saving the roster
// doesn't quietly reset everyone marked attended/no-show back to "booked"
// (the update endpoint replaces the whole attendee array).
const priorStatus = new Map<string, AttendanceStatus>();

onMounted(async () => {
  try {
    const source = editId || duplicateId;
    const [all, existing] = await Promise.all([
      api.get<Trainee[]>("/trainees"),
      source ? api.get<Session>(`/sessions/${source}`) : Promise.resolve(undefined),
    ]);
    // An inactive trainee already on this session stays pickable, so editing
    // doesn't silently drop them from the roster.
    const keep = new Set(existing?.attendees.map((a) => a.trainee) ?? []);
    trainees.value = all.filter((t) => t.status === "active" || keep.has(t.id));

    if (existing) {
      Object.assign(form, {
        title: existing.title,
        type: existing.type,
        date: isEdit ? dayOf(existing.start) : form.date,
        time: timeOf(existing.start),
        durationMin: existing.durationMin,
        location: existing.location ?? "",
        capacity: existing.capacity ?? 0,
        status: isEdit ? existing.status : "scheduled",
        attendees: existing.attendees.map((a) => a.trainee),
      });
      for (const a of existing.attendees) {
        if (isEdit) priorStatus.set(a.trainee, a.status);
        attendeePlan[a.trainee] = isEdit ? (a.planId ?? "") : "";
      }
    }
  } catch (e) {
    error.value = errMsg(e, "Couldn't load this session.");
  } finally {
    loading.value = false;
  }
});

// Active plans of whoever is ticked, so a booking can count toward one.
const plans = ref<SessionPlan[]>([]);
watch(
  () => form.attendees.slice().sort().join(","),
  async (ids) => {
    if (!ids) return (plans.value = []);
    try {
      plans.value = await api.get<SessionPlan[]>(`/session-plans?trainees=${ids}&status=active`);
    } catch {
      /* plans are optional */
    }
  },
  // Immediate: an attendee pre-filled from a profile must show its plans too.
  { immediate: true },
);
const plansFor = (tid: string) => plans.value.filter((p) => p.trainee === tid && (!p.sessionType || p.sessionType === form.type));

// Placeholder speaks the chosen type — no stray "group" wording on a private.
const titlePlaceholder = computed(() =>
  form.type === "private" ? "Private Session — trainee name" : "Evening Group Class",
);

const dows = [
  { v: 1, l: "Mon" }, { v: 2, l: "Tue" }, { v: 3, l: "Wed" }, { v: 4, l: "Thu" },
  { v: 5, l: "Fri" }, { v: 6, l: "Sat" }, { v: 0, l: "Sun" },
];
const durationPresets = [45, 60, 90];
const weekPresets = [2, 4, 8, 12];
const statuses: { v: SessionStatus; l: string }[] = [
  { v: "scheduled", l: "Scheduled" },
  { v: "completed", l: "Done" },
  { v: "cancelled", l: "Cancelled" },
];

// Shared pill styling so weekdays, durations and week presets read as one family.
function pill(active: boolean) {
  return [
    "min-h-10 rounded-lg border px-2.5 text-xs transition-colors",
    active ? "border-bronze bg-bronze/15 text-bronze" : "border-line text-faint hover:text-fg",
  ];
}

function toggleDay(v: number) {
  const i = form.weekdays.indexOf(v);
  if (i >= 0) form.weekdays.splice(i, 1);
  else form.weekdays.push(v);
}

// End date for an N-week run (Mon start → the last day of week N). Derived, not
// stored, so the matching preset highlights and edits to End date deselect it.
function weeksEndDate(n: number): string {
  const [y, m, d] = form.date.split("-").map(Number);
  return dateKey(new Date(y, m - 1, d + n * 7 - 1)); // calendar arithmetic only
}
const activeWeeks = computed(() =>
  form.date ? (weekPresets.find((n) => form.endDate === weeksEndDate(n)) ?? null) : null,
);
function applyWeeks(n: number) {
  if (!form.date) return;
  form.endDate = weeksEndDate(n);
}

// Both group and private sessions can hold several people.
function toggleAttendee(id: string) {
  const i = form.attendees.indexOf(id);
  if (i >= 0) form.attendees.splice(i, 1);
  else {
    form.attendees.push(id);
    if (!(id in attendeePlan)) attendeePlan[id] = "";
  }
}

const filteredTrainees = computed(() => {
  const q = search.value.trim().toLowerCase();
  return q ? trainees.value.filter((t) => t.name.toLowerCase().includes(q)) : trainees.value;
});
const overCapacity = computed(() => form.capacity > 0 && form.attendees.length > form.capacity);

// Keep each attendee's recorded attendance; anyone newly ticked starts booked.
const attendeePayload = () =>
  form.attendees.map((id) => ({ trainee: id, status: priorStatus.get(id) ?? "booked", planId: attendeePlan[id] ?? "" }));

const recurringBody = () => ({
  title: form.title, type: form.type, weekdays: form.weekdays, time: form.time,
  durationMin: form.durationMin, location: form.location, capacity: form.capacity,
  fromDay: form.date, toDay: form.endDate || form.date, attendees: attendeePayload(), planOverride: planOverride.value,
});

// ── Recurring preview: every date, with clashes, before anything is saved ──
const preview = ref<RecurringPreview>();
const previewError = ref<string>();
const previewing = ref(false);
let previewTimer: ReturnType<typeof setTimeout> | undefined;
let previewToken = 0;
watch(
  () => [recurring.value, form.title, form.type, form.date, form.endDate, form.time, form.durationMin, form.weekdays.join(","),
    form.attendees.join(","), JSON.stringify(attendeePlan), planOverride.value, form.capacity],
  () => {
    clearTimeout(previewTimer);
    preview.value = undefined;
    previewError.value = undefined;
    if (isEdit || !recurring.value || !form.date || !form.weekdays.length || !form.title.trim()) return;
    previewTimer = setTimeout(async () => {
      const my = ++previewToken;
      previewing.value = true;
      try {
        const p = await api.post<RecurringPreview>("/sessions/recurring/preview", recurringBody());
        if (my === previewToken) preview.value = p;
      } catch (e) {
        if (my === previewToken) previewError.value = errMsg(e, "Couldn't preview the series.");
      } finally {
        if (my === previewToken) previewing.value = false;
      }
    }, 400);
  },
);
const canCreateSeries = computed(() =>
  !!preview.value && preview.value.count > 0 && preview.value.conflicts === 0 && (!preview.value.planProblem || planOverride.value),
);

const submitLabel = computed(() => {
  if (saving.value) return "Saving…";
  if (isEdit) return "Save changes";
  if (recurring.value) return preview.value ? `Create ${preview.value.count} session${preview.value.count === 1 ? "" : "s"}` : "Create series";
  return "Create session";
});

const back = () => backTarget(route.query, isEdit ? `/schedule/${editId}` : "/schedule");
// The studio's wall-clock time on the chosen day, as an exact instant — so
// 18:00 means 18:00 at the studio whatever zone this browser is in.
const startISO = () => studioInstant(form.date, form.time).toISOString();

async function submit(override = false) {
  error.value = undefined;
  conflictDates.value = [];
  if (!form.title.trim()) return (error.value = "Title is required");
  if (!form.date) return (error.value = "Date is required");
  if (!form.durationMin || form.durationMin <= 0) return (error.value = "Duration must be at least 1 minute");
  if (overCapacity.value) return (error.value = `This class holds ${form.capacity}; ${form.attendees.length} are ticked.`);
  saving.value = true;
  try {
    if (isEdit) {
      await api.put(`/sessions/${editId}`, {
        title: form.title,
        type: form.type,
        start: startISO(),
        durationMin: form.durationMin,
        location: form.location,
        capacity: form.capacity,
        status: form.status,
        attendees: attendeePayload(),
        planOverride: override,
      });
      invalidate("sessions", "home-upcoming", "trainees");
      toast("Session updated.", "success");
      router.push(back());
      return;
    }
    if (recurring.value) {
      if (form.weekdays.length === 0) {
        saving.value = false;
        return (error.value = "Pick at least one weekday");
      }
      await api.post("/sessions/recurring", recurringBody());
    } else {
      await api.post("/sessions", {
        title: form.title, type: form.type, start: startISO(), durationMin: form.durationMin,
        location: form.location, capacity: form.capacity, attendees: attendeePayload(), planOverride: override,
      });
    }
    invalidate("sessions", "home-upcoming", "trainees");
    router.push(back());
  } catch (e) {
    saving.value = false;
    if (isApiError(e, "PLAN_FULL") && !recurring.value && confirm(e.message)) return submit(true);
    if (isApiError(e, "SCHEDULE_CONFLICT") && Array.isArray(e.details?.conflicts)) conflictDates.value = e.details!.conflicts as string[];
    error.value = errMsg(e, "Failed to save");
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back()" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> {{ isEdit ? "Session" : "Schedule" }}
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">{{ isEdit ? "Edit session" : duplicateId ? "Duplicate session" : "New session" }}</h1>

    <Skeleton v-if="loading" variant="detail" />

    <Card v-else class="space-y-4 p-4">
      <Alert v-if="error">
        {{ error }}
        <ul v-if="conflictDates.length" class="mt-1 list-disc pl-4">
          <li v-for="d in conflictDates" :key="d">{{ formatDay(dayOf(d), { weekday: "short", month: "short", day: "numeric" }) }} · {{ formatTime(d) }}</li>
        </ul>
      </Alert>
      <p v-if="duplicateId && !form.date" class="text-xs text-faint">Copied from the original — pick a date for the new session.</p>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Title <span class="text-overdue">*</span></span>
        <input v-model="form.title" :class="inputCls" :placeholder="titlePlaceholder" />
      </label>

      <!-- Type: segmented control -->
      <div>
        <span class="mb-1 block text-xs text-faint">Type</span>
        <div class="grid grid-cols-2 gap-1 rounded-xl border border-line bg-elevated p-1" role="radiogroup" aria-label="Type">
          <button
            v-for="opt in [{ v: 'group', l: 'Group', icon: Users }, { v: 'private', l: 'Private', icon: User }]"
            :key="opt.v"
            type="button"
            role="radio"
            :aria-checked="form.type === opt.v"
            class="flex min-h-10 items-center justify-center gap-1.5 rounded-lg text-sm font-medium transition-colors"
            :class="form.type === opt.v ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
            @click="form.type = opt.v as 'group' | 'private'"
          >
            <component :is="opt.icon" class="h-4 w-4" /> {{ opt.l }}
          </button>
        </div>
      </div>

      <!-- Date + Time: iOS-style wheel spinners -->
      <div class="grid grid-cols-2 gap-3">
        <DateWheel v-model="form.date" :label="recurring ? 'Start date' : 'Date'" placeholder="Select date" />
        <TimeWheel v-model="form.time" label="Time" />
      </div>

      <!-- Duration: presets + custom -->
      <div>
        <span class="mb-1 block text-xs text-faint">Duration</span>
        <div class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="n in durationPresets"
            :key="n"
            type="button"
            :class="pill(form.durationMin === n)"
            @click="form.durationMin = n"
          >{{ n }} min</button>
          <input v-model.number="form.durationMin" type="number" min="1" aria-label="Duration in minutes" class="ml-1 min-h-10 w-16 rounded-lg border border-line bg-elevated px-2 text-xs text-fg outline-none focus:border-bronze" />
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Location <span class="text-faint/70">(optional)</span></span>
          <input v-model="form.location" :class="inputCls" placeholder="Main floor" />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Capacity <span class="text-faint/70">(0 = no limit)</span></span>
          <input v-model.number="form.capacity" type="number" min="0" inputmode="numeric" :class="inputCls" :aria-invalid="overCapacity" />
        </label>
      </div>

      <!-- Status: only meaningful once the session exists. -->
      <div v-if="isEdit">
        <span class="mb-1 block text-xs text-faint">Status</span>
        <div class="grid grid-cols-3 gap-1 rounded-xl border border-line bg-elevated p-1" role="radiogroup" aria-label="Status">
          <button
            v-for="s in statuses"
            :key="s.v"
            type="button"
            role="radio"
            :aria-checked="form.status === s.v"
            class="min-h-10 rounded-lg text-sm font-medium transition-colors"
            :class="form.status === s.v ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
            @click="form.status = s.v"
          >{{ s.l }}</button>
        </div>
        <p v-if="form.status === 'cancelled'" class="mt-1.5 text-xs text-faint">
          A cancelled session stays on the schedule, greyed out, and stops blocking the slot.
        </p>
      </div>

      <!-- Repeat weekly: iOS switch + reveal (creating only) -->
      <div v-if="!isEdit" class="rounded-xl border border-line bg-elevated/40 p-3">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium">Repeat weekly</p>
            <p class="text-xs text-faint">Generate a recurring series</p>
          </div>
          <ToggleSwitch v-model="recurring" aria-label="Repeat weekly" />
        </div>

        <div v-if="recurring" class="mt-3 space-y-3 border-t border-line pt-3">
          <div>
            <span class="mb-1 block text-xs text-faint">On days</span>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="d in dows" :key="d.v" type="button" :aria-pressed="form.weekdays.includes(d.v)" :class="pill(form.weekdays.includes(d.v))" @click="toggleDay(d.v)">{{ d.l }}</button>
            </div>
          </div>
          <div>
            <span class="mb-1 block text-xs text-faint">For how many weeks</span>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="n in weekPresets" :key="n" type="button" :aria-pressed="activeWeeks === n" :class="pill(activeWeeks === n)" @click="applyWeeks(n)">{{ n }} weeks</button>
            </div>
          </div>
          <DateWheel v-model="form.endDate" label="End date" placeholder="Select end date" />

          <!-- Preview: the exact dates, clashes flagged, before saving. -->
          <div v-if="previewing && !preview" class="text-xs text-faint">Checking dates…</div>
          <Alert v-else-if="previewError">{{ previewError }}</Alert>
          <div v-else-if="preview" class="space-y-2 rounded-lg bg-bronze/10 p-3 text-xs">
            <p class="text-bronze">
              {{ preview.count }} session{{ preview.count === 1 ? "" : "s" }}
              <template v-if="preview.conflicts"> · <span class="font-semibold text-overdue">{{ preview.conflicts }} clash{{ preview.conflicts === 1 ? "" : "es" }} — adjust those dates or times</span></template>
            </p>
            <ul class="max-h-48 space-y-0.5 overflow-y-auto">
              <li v-for="o in preview.occurrences" :key="o.start" class="flex justify-between gap-2" :class="o.conflict ? 'text-overdue' : 'text-muted'">
                <span>{{ formatDay(o.day, { weekday: "short", month: "short", day: "numeric" }) }} · {{ formatTime(o.start) }}</span>
                <span v-if="o.conflict" class="truncate">clashes with {{ o.conflict.title }}</span>
              </li>
            </ul>
            <div v-if="preview.planProblem" class="rounded-lg border border-partial/40 p-2 text-partial">
              <p class="flex items-center gap-1"><TriangleAlert class="h-3.5 w-3.5" /> {{ preview.planProblem.error }}</p>
              <label v-if="preview.planProblem.code === 'PLAN_FULL'" class="mt-1 flex min-h-10 items-center gap-2 text-muted">
                <input v-model="planOverride" type="checkbox" class="h-4 w-4" /> Book past the plan's target anyway
              </label>
            </div>
          </div>
        </div>
      </div>

      <!-- Attendees / trainees, each optionally counted toward a session plan -->
      <div>
        <div class="mb-1 flex items-center justify-between">
          <span class="text-xs text-faint">{{ form.type === "private" ? "Trainees" : "Attendees" }}</span>
          <span v-if="form.attendees.length" class="text-xs tnum" :class="overCapacity ? 'text-overdue' : 'text-bronze'">
            {{ form.attendees.length }}{{ form.capacity ? ` of ${form.capacity}` : "" }} selected
          </span>
        </div>
        <div v-if="trainees.length > 6" class="relative mb-1.5">
          <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
          <input v-model="search" placeholder="Search trainees…" aria-label="Search trainees" class="min-h-10 w-full rounded-lg border border-line bg-elevated pl-8 pr-3 text-sm outline-none placeholder:text-faint focus:border-bronze" />
        </div>
        <div class="max-h-60 space-y-1 overflow-y-auto rounded-xl border border-line bg-elevated p-2">
          <p v-if="filteredTrainees.length === 0" class="px-2 py-3 text-center text-xs text-faint">
            {{ trainees.length === 0 ? "No active trainees." : "No matches." }}
          </p>
          <div v-for="t in filteredTrainees" :key="t.id">
            <button
              type="button"
              class="flex min-h-10 w-full items-center gap-2 rounded-lg px-2 text-left text-sm transition-colors hover:bg-surface"
              :class="form.attendees.includes(t.id) ? 'text-fg' : 'text-muted'"
              :aria-pressed="form.attendees.includes(t.id)"
              @click="toggleAttendee(t.id)"
            >
              <span
                class="grid h-5 w-5 shrink-0 place-items-center rounded-md border transition-colors"
                :class="form.attendees.includes(t.id) ? 'border-bronze bg-bronze/20 text-bronze' : 'border-line text-transparent'"
              ><Check class="h-3 w-3" /></span>
              {{ t.name }}
            </button>
            <div v-if="form.attendees.includes(t.id) && plansFor(t.id).length" class="mb-1 ml-9 flex flex-wrap gap-1">
              <button
                type="button"
                class="min-h-9 rounded-md border px-2 text-[0.6875rem]"
                :class="!attendeePlan[t.id] ? 'border-bronze/60 text-bronze' : 'border-line text-faint'"
                @click="attendeePlan[t.id] = ''"
              >No plan</button>
              <button
                v-for="p in plansFor(t.id)"
                :key="p.id"
                type="button"
                class="min-h-9 rounded-md border px-2 text-[0.6875rem]"
                :class="attendeePlan[t.id] === p.id ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
                @click="attendeePlan[t.id] = p.id"
              >{{ p.title }} · {{ p.progress.remaining }} left</button>
            </div>
          </div>
        </div>
      </div>

      <Button :disabled="saving || (!isEdit && recurring && !canCreateSeries)" @click="submit()">{{ submitLabel }}</Button>
    </Card>
  </div>
</template>
