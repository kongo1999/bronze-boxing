<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string;
    value: string | number;
    sub?: string;
    accent?: boolean;
  }>(),
  { accent: false },
);
</script>

<template>
  <div
    class="rounded-2xl border p-4"
    :class="
      accent
        ? 'border-bronze/30 bg-bronze/10'
        : 'border-line bg-surface shadow-[var(--shadow-soft)]'
    "
  >
    <p class="label-eyebrow text-[0.625rem]" :class="accent ? 'text-bronze' : 'text-faint'">
      {{ label }}
    </p>
    <p class="mt-1 font-display text-3xl font-semibold tnum" :class="accent ? 'text-bronze' : 'text-fg'">
      {{ value }}
    </p>
    <p v-if="sub" class="mt-0.5 text-xs text-muted">{{ sub }}</p>
  </div>
</template>
