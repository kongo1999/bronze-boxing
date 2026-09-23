<script setup lang="ts">
// iOS-style date spinner. v-model is a studio "YYYY-MM-DD" string (a calendar
// day — no timezone involved). The popover also has a plain date field, which
// is the keyboard and screen-reader path and the fast way to type a far date.
import { ref, computed, watch, nextTick, useId } from "vue";
import { CalendarDays } from "lucide-vue-next";
import { inputCls } from "@/lib/ui";
import { dateKey } from "@/lib/format";
import { isDayKey, todayKey } from "@/lib/studio";
import WheelColumn from "./WheelColumn.vue";

const props = defineProps<{ modelValue: string; label: string; placeholder?: string }>();
const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const open = ref(false);
const id = useId();
const typed = ref<HTMLInputElement>();

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const months = MONTHS.map((label, value) => ({ label, value }));

function parse(v: string) {
  const [y, m, d] = (isDayKey(v) ? v : todayKey()).split("-").map(Number);
  return { y, m: m - 1, d };
}
const init = parse(props.modelValue);
const y = ref(init.y);
const m = ref(init.m);
const d = ref(init.d);

// Old records reach back years: offer 2015 → five years ahead, always
// including whatever year the value itself is in.
const thisYear = Number(todayKey().slice(0, 4));
const years = computed(() => {
  const lo = Math.min(2015, y.value);
  const hi = Math.max(thisYear + 5, y.value);
  return Array.from({ length: hi - lo + 1 }, (_, i) => ({ label: String(lo + i), value: lo + i }));
});

function daysInMonth(yy: number, mm: number) {
  return new Date(yy, mm + 1, 0).getDate();
}
const dayOptions = computed(() =>
  Array.from({ length: daysInMonth(y.value, m.value) }, (_, i) => ({ label: String(i + 1), value: i + 1 })),
);

// Keep the day valid when the month/year (and thus day count) changes.
watch([y, m], () => {
  const max = daysInMonth(y.value, m.value);
  if (d.value > max) d.value = max;
});
// Reflect external changes (e.g. the "for N weeks" preset filling End date).
watch(
  () => props.modelValue,
  (v) => {
    if (!isDayKey(v)) return;
    const p = parse(v);
    y.value = p.y;
    m.value = p.m;
    d.value = p.d;
  },
);
// Commit any wheel change back to the model (calendar arithmetic only).
watch([y, m, d], () => {
  if (open.value) emit("update:modelValue", dateKey(new Date(y.value, m.value, d.value)));
});

const display = computed(() => {
  if (!props.modelValue) return props.placeholder ?? "Select date";
  const [yy, mm, dd] = props.modelValue.split("-").map(Number);
  return new Date(yy, mm - 1, dd).toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric", year: "numeric" });
});

async function toggle() {
  open.value = !open.value;
  if (open.value) {
    await nextTick();
    typed.value?.focus();
  }
}
function onTyped(e: Event) {
  const v = (e.target as HTMLInputElement).value;
  if (isDayKey(v)) emit("update:modelValue", v);
}
function done() {
  // If opened on an empty field and left untouched, commit the shown default.
  if (!props.modelValue) emit("update:modelValue", dateKey(new Date(y.value, m.value, d.value)));
  open.value = false;
}
</script>

<template>
  <div class="relative">
    <span :id="`${id}-label`" class="mb-1 block text-xs text-faint">{{ label }}</span>
    <button
      type="button"
      :class="[inputCls, 'flex items-center justify-between text-left']"
      :aria-labelledby="`${id}-label`"
      aria-haspopup="dialog"
      :aria-expanded="open"
      @click="toggle"
    >
      <span :class="modelValue ? 'text-fg' : 'text-faint'">{{ display }}</span>
      <CalendarDays class="h-4 w-4 shrink-0 text-faint" />
    </button>

    <template v-if="open">
      <div class="fixed inset-0 z-30" @click="done" />
      <div
        role="dialog"
        :aria-label="label"
        class="absolute left-0 right-0 z-40 mt-1 min-w-64 space-y-2 rounded-2xl border border-line bg-surface p-3 shadow-xl"
        @keydown.escape.prevent="done"
      >
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Type or pick a date</span>
          <input ref="typed" type="date" :value="modelValue" :class="inputCls" @change="onTyped" />
        </label>
        <div class="relative flex h-[180px] gap-1 overflow-hidden rounded-xl bg-elevated px-2" aria-hidden="true">
          <div class="pointer-events-none absolute inset-x-2 top-1/2 z-10 h-[36px] -translate-y-1/2 rounded-lg bg-bronze/15 ring-1 ring-bronze/40" />
          <WheelColumn :options="months" v-model="m" class="!flex-[1.4]" />
          <WheelColumn :options="dayOptions" v-model="d" />
          <WheelColumn :options="years" v-model="y" />
          <div class="pointer-events-none absolute inset-x-0 top-0 h-[72px] rounded-t-xl bg-gradient-to-b from-elevated to-transparent" />
          <div class="pointer-events-none absolute inset-x-0 bottom-0 h-[72px] rounded-b-xl bg-gradient-to-t from-elevated to-transparent" />
        </div>
        <button type="button" class="min-h-10 w-full rounded-lg bg-bronze py-2 text-sm font-semibold text-bronze-ink" @click="done">Done</button>
      </div>
    </template>
  </div>
</template>
