<script setup lang="ts">
// One column of an iOS-style wheel picker: a vertically scroll-snapping drum.
// The parent draws the selection band + fades; this renders only the scroller.
import { ref, watch, onMounted, nextTick } from "vue";

type Opt = { label: string; value: number };
const props = defineProps<{ options: Opt[]; modelValue: number }>();
const emit = defineEmits<{ (e: "update:modelValue", v: number): void }>();

const ITEM_H = 36; // must match the h-[36px] on items and the parent band height
const scroller = ref<HTMLElement>();
let settle: number | undefined;

function indexOf(v: number) {
  const i = props.options.findIndex((o) => o.value === v);
  return i < 0 ? 0 : i;
}
function scrollToIndex(i: number, smooth = false) {
  scroller.value?.scrollTo({ top: i * ITEM_H, behavior: smooth ? "smooth" : "auto" });
}

onMounted(async () => {
  await nextTick();
  scrollToIndex(indexOf(props.modelValue));
});

// External value change (e.g. day clamped when month flips) — realign.
watch(
  () => props.modelValue,
  (v) => {
    if (!scroller.value) return;
    if (Math.round(scroller.value.scrollTop / ITEM_H) !== indexOf(v)) scrollToIndex(indexOf(v), true);
  },
);

function onScroll() {
  window.clearTimeout(settle);
  settle = window.setTimeout(() => {
    if (!scroller.value) return;
    const i = Math.min(Math.max(Math.round(scroller.value.scrollTop / ITEM_H), 0), props.options.length - 1);
    const val = props.options[i].value;
    if (val !== props.modelValue) emit("update:modelValue", val);
    if (Math.abs(scroller.value.scrollTop - i * ITEM_H) > 1) scrollToIndex(i, true); // snap
  }, 110);
}

function pick(v: number) {
  emit("update:modelValue", v);
  scrollToIndex(indexOf(v), true);
}
</script>

<template>
  <div ref="scroller" class="wheel h-full flex-1 snap-y snap-mandatory overflow-y-scroll py-[72px]" @scroll="onScroll">
    <button
      v-for="o in options"
      :key="o.value"
      type="button"
      class="flex h-[36px] w-full snap-center items-center justify-center text-[0.9375rem] tabular-nums transition-colors"
      :class="o.value === modelValue ? 'font-semibold text-fg' : 'text-faint'"
      @click="pick(o.value)"
    >{{ o.label }}</button>
  </div>
</template>

<style scoped>
.wheel {
  scrollbar-width: none; /* Firefox */
  -webkit-overflow-scrolling: touch;
  overscroll-behavior: contain;
}
.wheel::-webkit-scrollbar {
  display: none; /* WebKit */
}
</style>
