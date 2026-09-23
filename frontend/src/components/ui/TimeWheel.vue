<script setup lang="ts">
// iOS-style time spinner. v-model is a 24h "HH:mm" studio wall-clock string.
// The wheel moves in 5-minute steps (twelve values, not sixty); the time
// field above it takes any exact minute and is the keyboard path.
import { ref, computed, watch, nextTick, useId } from "vue";
import { Clock } from "lucide-vue-next";
import { inputCls } from "@/lib/ui";
import WheelColumn from "./WheelColumn.vue";

const props = defineProps<{ modelValue: string; label: string }>();
const emit = defineEmits<{ (e: "update:modelValue", v: string): void }>();

const open = ref(false);
const id = useId();
const typed = ref<HTMLInputElement>();
const pad = (n: number) => String(n).padStart(2, "0");

const hours = Array.from({ length: 12 }, (_, i) => ({ label: String(i + 1), value: i + 1 }));
const meridiem = [
  { label: "AM", value: 0 },
  { label: "PM", value: 1 },
];

function parse(v: string) {
  const [hh, mm] = (/^\d{2}:\d{2}$/.test(v) ? v : "18:00").split(":").map(Number);
  return { h12: hh % 12 || 12, min: mm, pm: hh >= 12 ? 1 : 0 };
}
const init = parse(props.modelValue);
const h12 = ref(init.h12);
const min = ref(init.min);
const pm = ref(init.pm);

// 5-minute steps, plus the current value if it's an odd minute typed exactly.
const minutes = computed(() => {
  const set = new Set(Array.from({ length: 12 }, (_, i) => i * 5));
  set.add(min.value);
  return [...set].sort((a, b) => a - b).map((v) => ({ label: pad(v), value: v }));
});

function to24() {
  const h = pm.value ? (h12.value % 12) + 12 : h12.value % 12;
  return `${pad(h)}:${pad(min.value)}`;
}
watch(
  () => props.modelValue,
  (v) => {
    const p = parse(v);
    h12.value = p.h12;
    min.value = p.min;
    pm.value = p.pm;
  },
);
watch([h12, min, pm], () => {
  if (to24() !== props.modelValue) emit("update:modelValue", to24());
});

const display = computed(() => {
  const p = parse(props.modelValue);
  return `${p.h12}:${pad(p.min)} ${p.pm ? "PM" : "AM"}`;
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
  if (/^\d{2}:\d{2}$/.test(v)) emit("update:modelValue", v);
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
      <span class="text-fg tabular-nums">{{ display }}</span>
      <Clock class="h-4 w-4 shrink-0 text-faint" />
    </button>

    <template v-if="open">
      <div class="fixed inset-0 z-30" @click="open = false" />
      <div
        role="dialog"
        :aria-label="label"
        class="absolute left-0 right-0 z-40 mt-1 min-w-56 space-y-2 rounded-2xl border border-line bg-surface p-3 shadow-xl"
        @keydown.escape.prevent="open = false"
      >
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Type an exact time</span>
          <input ref="typed" type="time" :value="modelValue" :class="inputCls" @change="onTyped" />
        </label>
        <div class="relative flex h-[180px] gap-1 overflow-hidden rounded-xl bg-elevated px-2" aria-hidden="true">
          <div class="pointer-events-none absolute inset-x-2 top-1/2 z-10 h-[36px] -translate-y-1/2 rounded-lg bg-bronze/15 ring-1 ring-bronze/40" />
          <WheelColumn :options="hours" v-model="h12" />
          <WheelColumn :options="minutes" v-model="min" />
          <WheelColumn :options="meridiem" v-model="pm" />
          <div class="pointer-events-none absolute inset-x-0 top-0 h-[72px] rounded-t-xl bg-gradient-to-b from-elevated to-transparent" />
          <div class="pointer-events-none absolute inset-x-0 bottom-0 h-[72px] rounded-b-xl bg-gradient-to-t from-elevated to-transparent" />
        </div>
        <button type="button" class="min-h-10 w-full rounded-lg bg-bronze py-2 text-sm font-semibold text-bronze-ink" @click="open = false">Done</button>
      </div>
    </template>
  </div>
</template>
