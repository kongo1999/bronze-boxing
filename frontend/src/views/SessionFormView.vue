<script setup lang="ts">
import { reactive, ref, computed, onMounted } from "vue";
import { useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Users, User, Search, Check } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Trainee } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import ToggleSwitch from "@/components/ui/ToggleSwitch.vue";
import DateWheel from "@/components/ui/DateWheel.vue";
import TimeWheel from "@/components/ui/TimeWheel.vue";
import { inputCls } from "@/lib/ui";
import { dateKey } from "@/lib/format";

const router = useRouter();
const trainees = ref<Trainee[]>([]);
const saving = ref(false);
const error = ref<string>();
const recurring = ref(false);
const search = ref("");

const form = reactive({
  title: "",
  type: "group" as "group" | "private",
  date: "",
  time: "18:00",
  durationMin: 60,
  attendees: [] as string[],
  // recurring
  weekdays: [] as number[],
  endDate: "",
});

onMounted(async () => {
  trainees.value = (await api.get<Trainee[]>("/trainees")).filter((t) => t.status === "active");
});

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

// Shared pill styling so weekdays, durations and week presets read as one family.
function pill(active: boolean) {
  return [
    "rounded-lg border px-2.5 py-1.5 text-xs transition-colors",
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
  return dateKey(new Date(y, m - 1, d + n * 7 - 1));
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
  else form.attendees.push(id);
}

const filteredTrainees = computed(() => {
  const q = search.value.trim().toLowerCase();
  return q ? trainees.value.filter((t) => t.name.toLowerCase().includes(q)) : trainees.value;
});

// Live count of how many sessions a recurring run will create, so there's no
// surprise before submitting.
const recurringCount = computed(() => {
  if (!recurring.value || !form.date || form.weekdays.length === 0) return 0;
  const [sy, sm, sd] = form.date.split("-").map(Number);
  const start = new Date(sy, sm - 1, sd);
  const [ey, em, ed] = (form.endDate || form.date).split("-").map(Number);
  const end = new Date(ey, em - 1, ed);
  if (end < start) return 0;
  let n = 0;
  for (const dt = new Date(start); dt <= end; dt.setDate(dt.getDate() + 1)) {
    if (form.weekdays.includes(dt.getDay())) n++;
  }
  return n;
});

const createLabel = computed(() => {
  if (saving.value) return "Saving…";
  if (recurring.value) return `Create ${recurringCount.value} session${recurringCount.value === 1 ? "" : "s"}`;
  return "Create session";
});

async function submit() {
  if (!form.title.trim()) return (error.value = "Title is required");
  if (!form.date) return (error.value = "Date is required");
  saving.value = true;
  error.value = undefined;
  const attendees = form.attendees.map((id) => ({ trainee: id, status: "booked" }));
  try {
    if (recurring.value) {
      if (form.weekdays.length === 0) {
        saving.value = false;
        return (error.value = "Pick at least one weekday");
      }
      await api.post("/sessions/recurring", {
        title: form.title, type: form.type, weekdays: form.weekdays, time: form.time,
        durationMin: form.durationMin,
        from: new Date(form.date).toISOString(),
        to: new Date(form.endDate || form.date).toISOString(), attendees,
      });
    } else {
      const start = new Date(`${form.date}T${form.time}`);
      await api.post("/sessions", {
        title: form.title, type: form.type, start: start.toISOString(),
        durationMin: form.durationMin, attendees,
      });
    }
    router.push("/schedule");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Failed to save";
    saving.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/schedule" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Schedule
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">New session</h1>

    <Card class="space-y-4 p-4">
      <Alert v-if="error">{{ error }}</Alert>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Title</span>
        <input v-model="form.title" :class="inputCls" :placeholder="titlePlaceholder" />
      </label>

      <!-- Type: segmented control -->
      <div>
        <span class="mb-1 block text-xs text-faint">Type</span>
        <div class="grid grid-cols-2 gap-1 rounded-xl border border-line bg-elevated p-1">
          <button
            v-for="opt in [{ v: 'group', l: 'Group', icon: Users }, { v: 'private', l: 'Private', icon: User }]"
            :key="opt.v"
            type="button"
            class="flex items-center justify-center gap-1.5 rounded-lg py-2 text-sm font-medium transition-colors"
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
          <input v-model.number="form.durationMin" type="number" min="0" class="ml-1 w-16 rounded-lg border border-line bg-elevated px-2 py-1.5 text-xs text-fg outline-none focus:border-bronze" />
        </div>
      </div>

      <!-- Repeat weekly: iOS switch + reveal -->
      <div class="rounded-xl border border-line bg-elevated/40 p-3">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium">Repeat weekly</p>
            <p class="text-xs text-faint">Generate a recurring series</p>
          </div>
          <ToggleSwitch v-model="recurring" />
        </div>

        <div v-if="recurring" class="mt-3 space-y-3 border-t border-line pt-3">
          <div>
            <span class="mb-1 block text-xs text-faint">On days</span>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="d in dows" :key="d.v" type="button" :class="pill(form.weekdays.includes(d.v))" @click="toggleDay(d.v)">{{ d.l }}</button>
            </div>
          </div>
          <div>
            <span class="mb-1 block text-xs text-faint">For how many weeks</span>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="n in weekPresets" :key="n" type="button" :class="pill(activeWeeks === n)" @click="applyWeeks(n)">{{ n }} weeks</button>
            </div>
          </div>
          <DateWheel v-model="form.endDate" label="End date" placeholder="Select end date" />
          <p v-if="recurringCount > 0" class="rounded-lg bg-bronze/10 px-3 py-2 text-xs text-bronze">
            Creates {{ recurringCount }} session{{ recurringCount === 1 ? "" : "s" }} on the selected days.
          </p>
        </div>
      </div>

      <!-- Attendees / trainees -->
      <div>
        <div class="mb-1 flex items-center justify-between">
          <span class="text-xs text-faint">{{ form.type === "private" ? "Trainees" : "Attendees" }}</span>
          <span v-if="form.attendees.length" class="text-xs text-bronze">{{ form.attendees.length }} selected</span>
        </div>
        <div v-if="trainees.length > 6" class="relative mb-1.5">
          <Search class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
          <input v-model="search" placeholder="Search trainees…" class="w-full rounded-lg border border-line bg-elevated py-1.5 pl-8 pr-3 text-sm outline-none placeholder:text-faint focus:border-bronze" />
        </div>
        <div class="max-h-44 space-y-1 overflow-y-auto rounded-xl border border-line bg-elevated p-2">
          <p v-if="filteredTrainees.length === 0" class="px-2 py-3 text-center text-xs text-faint">
            {{ trainees.length === 0 ? "No active trainees." : "No matches." }}
          </p>
          <button
            v-for="t in filteredTrainees"
            :key="t.id"
            type="button"
            class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors hover:bg-surface"
            :class="form.attendees.includes(t.id) ? 'text-fg' : 'text-muted'"
            @click="toggleAttendee(t.id)"
          >
            <span
              class="grid h-5 w-5 shrink-0 place-items-center rounded-md border transition-colors"
              :class="form.attendees.includes(t.id) ? 'border-bronze bg-bronze/20 text-bronze' : 'border-line text-transparent'"
            ><Check class="h-3 w-3" /></span>
            {{ t.name }}
          </button>
        </div>
      </div>

      <Button :disabled="saving || (recurring && recurringCount === 0)" @click="submit">{{ createLabel }}</Button>
    </Card>
  </div>
</template>
