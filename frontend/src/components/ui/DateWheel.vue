<script setup lang="ts">
// iOS-style date spinner. v-model is a local "YYYY-MM-DD" string.
import { ref, computed, watch } from "vue";
import { CalendarDays } from "lucide-vue-next";
import { inputCls } from "@/lib/ui";
import { dateKey } from "@/lib/format";
import WheelColumn from "./WheelColumn.vue";

const props = defineProps<{ modelValue: string; label: string; placeholder?: string }>();
const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const open = ref(false);

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const months = MONTHS.map((label, value) => ({ label, value }));
const thisYear = new Date().getFullYear();
const years = Array.from({ length: 5 }, (_, i) => ({ label: String(thisYear - 1 + i), value: thisYear - 1 + i }));

function parse(v: string) {
  if (!v) {
    const t = new Date();
    return { y: t.getFullYear(), m: t.getMonth(), d: t.getDate() };
  }
  const [y, m, d] = v.split("-").map(Number);
  return { y, m: m - 1, d };
}
const init = parse(props.modelValue);
const y = ref(init.y);
const m = ref(init.m);
const d = ref(init.d);

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
    if (!v) return;
    const p = parse(v);
    y.value = p.y;
    m.value = p.m;
    d.value = p.d;
  },
);
// Commit any user change back to the model.
watch([y, m, d], () => emit("update:modelValue", dateKey(new Date(y.value, m.value, d.value))));

const display = computed(() => {
  if (!props.modelValue) return props.placeholder ?? "Select date";
  const [yy, mm, dd] = props.modelValue.split("-").map(Number);
  return new Date(yy, mm - 1, dd).toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric", year: "numeric" });
});

function done() {
  // If opened on an empty field and left untouched, commit the shown default.
  if (!props.modelValue) emit("update:modelValue", dateKey(new Date(y.value, m.value, d.value)));
  open.value = false;
}
</script>

<template>
  <div class="relative">
    <span class="mb-1 block text-xs text-faint">{{ label }}</span>
    <button type="button" :class="[inputCls, 'flex items-center justify-between text-left']" @click="open = !open">
      <span :class="modelValue ? 'text-fg' : 'text-faint'">{{ display }}</span>
      <CalendarDays class="h-4 w-4 shrink-0 text-faint" />
    </button>

    <template v-if="open">
      <div class="fixed inset-0 z-30" @click="done" />
      <div class="absolute left-0 right-0 z-40 mt-1 rounded-2xl border border-line bg-surface p-3 shadow-xl">
        <div class="relative flex h-[180px] gap-1 overflow-hidden rounded-xl bg-elevated px-2">
          <div class="pointer-events-none absolute inset-x-2 top-1/2 z-10 h-[36px] -translate-y-1/2 rounded-lg bg-bronze/15 ring-1 ring-bronze/40" />
          <WheelColumn :options="months" v-model="m" class="!flex-[1.4]" />
          <WheelColumn :options="dayOptions" v-model="d" />
          <WheelColumn :options="years" v-model="y" />
          <div class="pointer-events-none absolute inset-x-0 top-0 h-[72px] rounded-t-xl bg-gradient-to-b from-elevated to-transparent" />
          <div class="pointer-events-none absolute inset-x-0 bottom-0 h-[72px] rounded-b-xl bg-gradient-to-t from-elevated to-transparent" />
        </div>
        <button type="button" class="mt-2 w-full rounded-lg bg-bronze py-2 text-sm font-semibold text-bronze-ink" @click="done">Done</button>
      </div>
    </template>
  </div>
</template>
