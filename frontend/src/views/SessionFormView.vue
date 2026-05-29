<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Trainee } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";

const router = useRouter();
const trainees = ref<Trainee[]>([]);
const saving = ref(false);
const error = ref<string>();
const recurring = ref(false);

const form = reactive({
  title: "",
  type: "group",
  date: "",
  time: "18:00",
  durationMin: 60,
  location: "",
  capacity: 12,
  fee: 0,
  attendees: [] as string[],
  // recurring
  weekdays: [] as number[],
  endDate: "",
});

onMounted(async () => {
  trainees.value = (await api.get<Trainee[]>("/trainees")).filter((t) => t.status === "active");
});

const inputCls = "w-full rounded-xl border border-line bg-elevated px-3 py-2.5 text-sm outline-none focus:border-bronze/40";
const dows = [
  { v: 1, l: "Mon" }, { v: 2, l: "Tue" }, { v: 3, l: "Wed" }, { v: 4, l: "Thu" },
  { v: 5, l: "Fri" }, { v: 6, l: "Sat" }, { v: 0, l: "Sun" },
];

function toggleDay(v: number) {
  const i = form.weekdays.indexOf(v);
  if (i >= 0) form.weekdays.splice(i, 1);
  else form.weekdays.push(v);
}
function toggleAttendee(id: string) {
  const i = form.attendees.indexOf(id);
  if (i >= 0) form.attendees.splice(i, 1);
  else form.attendees.push(id);
}

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
        durationMin: form.durationMin, location: form.location, capacity: form.capacity,
        fee: form.fee, from: new Date(form.date).toISOString(),
        to: new Date(form.endDate || form.date).toISOString(), attendees,
      });
    } else {
      const start = new Date(`${form.date}T${form.time}`);
      await api.post("/sessions", {
        title: form.title, type: form.type, start: start.toISOString(),
        durationMin: form.durationMin, location: form.location, capacity: form.capacity,
        fee: form.fee, attendees,
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

    <Card class="space-y-3 p-4">
      <p v-if="error" class="rounded-lg bg-overdue/10 px-3 py-2 text-sm text-overdue">{{ error }}</p>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Title</span>
        <input v-model="form.title" :class="inputCls" placeholder="Evening Group Class" />
      </label>

      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Type</span>
          <select v-model="form.type" :class="inputCls">
            <option value="group">Group</option>
            <option value="private">Private</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Duration (min)</span>
          <input v-model.number="form.durationMin" type="number" min="0" :class="inputCls" />
        </label>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">{{ recurring ? "Start date" : "Date" }}</span>
          <input v-model="form.date" type="date" :class="inputCls" />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Time</span>
          <input v-model="form.time" type="time" :class="inputCls" />
        </label>
      </div>

      <label class="flex items-center gap-2 text-sm">
        <input v-model="recurring" type="checkbox" class="h-4 w-4 accent-[oklch(0.72_0.13_64)]" />
        Repeat weekly (generate a series)
      </label>

      <template v-if="recurring">
        <div>
          <span class="mb-1 block text-xs text-faint">On days</span>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="d in dows"
              :key="d.v"
              type="button"
              class="rounded-lg border px-2.5 py-1.5 text-xs transition-colors"
              :class="form.weekdays.includes(d.v) ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
              @click="toggleDay(d.v)"
            >{{ d.l }}</button>
          </div>
        </div>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">End date</span>
          <input v-model="form.endDate" type="date" :class="inputCls" />
        </label>
      </template>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Location</span>
        <input v-model="form.location" :class="inputCls" placeholder="Main floor" />
      </label>

      <div>
        <span class="mb-1 block text-xs text-faint">Attendees</span>
        <div class="max-h-44 space-y-1 overflow-y-auto rounded-xl border border-line bg-elevated p-2">
          <label v-for="t in trainees" :key="t.id" class="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-surface">
            <input type="checkbox" :checked="form.attendees.includes(t.id)" class="h-4 w-4 accent-[oklch(0.72_0.13_64)]" @change="toggleAttendee(t.id)" />
            {{ t.name }}
          </label>
        </div>
      </div>

      <Button :disabled="saving" @click="submit">{{ saving ? "Saving…" : "Create" }}</Button>
    </Card>
  </div>
</template>
