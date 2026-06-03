<script setup lang="ts">
import type { Component } from "vue";
defineProps<{ icon?: Component; title: string; description?: string }>();
</script>

<template>
  <div
    class="flex flex-col items-center gap-3 rounded-2xl border border-dashed border-line px-6 py-10 text-center"
  >
    <span
      v-if="icon"
      class="grid h-14 w-14 place-items-center rounded-2xl bg-bronze/10 text-bronze"
    >
      <component :is="icon" class="h-7 w-7" :stroke-width="1.5" />
    </span>
    <div>
      <p class="font-medium">{{ title }}</p>
      <p v-if="description" class="mt-1 text-sm text-muted">{{ description }}</p>
    </div>
    <slot />
  </div>
</template>
