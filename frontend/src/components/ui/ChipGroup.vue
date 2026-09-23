<script setup lang="ts" generic="T extends string">
// A small fixed set of choices shown as visible chips (a radio group) — for
// filters and tabs where a dropdown would only hide four options. Arrow keys
// move the selection, as in a native radio group. Chips are 40px tall.
import { ref } from "vue";

export interface Chip<V extends string = string> {
  v: V;
  l: string;
  /** Small trailing figure, e.g. a count or "3 · $90". */
  n?: string | number;
  /** Tone for the count, e.g. to flag money owed. */
  tone?: "partial" | "overdue" | "paid" | "info";
}

const props = withDefaults(defineProps<{ options: Chip<T>[]; label: string; scroll?: boolean }>(), { scroll: false });
const model = defineModel<T>({ required: true });
const refs = ref<HTMLButtonElement[]>([]);

function onKey(e: KeyboardEvent, i: number) {
  const n = props.options.length;
  let j = -1;
  if (e.key === "ArrowRight" || e.key === "ArrowDown") j = (i + 1) % n;
  else if (e.key === "ArrowLeft" || e.key === "ArrowUp") j = (i - 1 + n) % n;
  else if (e.key === "Home") j = 0;
  else if (e.key === "End") j = n - 1;
  if (j < 0) return;
  e.preventDefault();
  model.value = props.options[j].v;
  refs.value[j]?.focus();
}
const toneCls: Record<string, string> = {
  partial: "text-partial",
  overdue: "text-overdue",
  paid: "text-paid",
  info: "text-info",
};
</script>

<template>
  <div
    role="radiogroup"
    :aria-label="label"
    class="flex gap-1.5"
    :class="scroll ? '-mx-1 overflow-x-auto px-1 pb-1' : 'flex-wrap'"
  >
    <button
      v-for="(o, i) in options"
      :key="o.v"
      :ref="(el) => (refs[i] = el as HTMLButtonElement)"
      type="button"
      role="radio"
      :aria-checked="model === o.v"
      :tabindex="model === o.v ? 0 : -1"
      class="inline-flex min-h-10 shrink-0 items-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition-colors"
      :class="model === o.v ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
      @click="model = o.v"
      @keydown="onKey($event, i)"
    >
      {{ o.l }}
      <span v-if="o.n !== undefined && o.n !== ''" class="tnum" :class="model === o.v ? '' : o.tone ? toneCls[o.tone] : 'text-muted'">{{ o.n }}</span>
    </button>
  </div>
</template>
