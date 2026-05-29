<script setup lang="ts">
defineProps<{ eyebrow?: string; title: string }>();
</script>

<template>
  <header class="flex items-end justify-between gap-3">
    <div>
      <p v-if="eyebrow" class="label-eyebrow text-[0.625rem] text-faint">{{ eyebrow }}</p>
      <h1 class="font-display text-2xl font-semibold tracking-tight">{{ title }}</h1>
    </div>
    <div class="shrink-0">
      <slot name="action" />
    </div>
  </header>
</template>
