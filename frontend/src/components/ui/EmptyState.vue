<script setup lang="ts">
import type { Component } from "vue";
defineProps<{ icon?: Component; title: string; description?: string }>();
</script>

<template>
  <div
    class="flex flex-col items-center gap-3 rounded-2xl border border-dashed border-line px-6 py-10 text-center"
  >
    <component :is="icon" v-if="icon" class="h-8 w-8 text-faint" :stroke-width="1.5" />
    <div>
      <p class="font-medium">{{ title }}</p>
      <p v-if="description" class="mt-1 text-sm text-muted">{{ description }}</p>
    </div>
    <slot />
  </div>
</template>
