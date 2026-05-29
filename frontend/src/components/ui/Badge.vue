<script setup lang="ts">
import { computed } from "vue";

type Tone = "bronze" | "paid" | "overdue" | "partial" | "info" | "neutral";
const props = withDefaults(defineProps<{ tone?: Tone }>(), { tone: "neutral" });

const tones: Record<Tone, string> = {
  bronze: "text-bronze bg-bronze/15 border-bronze/25",
  paid: "text-paid bg-paid/15 border-paid/25",
  overdue: "text-overdue bg-overdue/15 border-overdue/25",
  partial: "text-partial bg-partial/15 border-partial/25",
  info: "text-info bg-info/15 border-info/25",
  neutral: "text-muted bg-elevated border-line",
};
const cls = computed(() => tones[props.tone]);
</script>

<template>
  <span
    class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium"
    :class="cls"
  >
    <slot />
  </span>
</template>
