<script setup lang="ts">
import { TriangleAlert, Info } from "lucide-vue-next";
withDefaults(defineProps<{ tone?: "error" | "info" }>(), { tone: "error" });
</script>

<template>
  <div
    class="flex items-start gap-2 rounded-xl border px-3 py-2.5 text-sm"
    :class="tone === 'error' ? 'border-overdue/30 bg-overdue/10 text-overdue' : 'border-info/30 bg-info/10 text-info'"
    role="alert"
  >
    <component :is="tone === 'error' ? TriangleAlert : Info" class="mt-0.5 h-4 w-4 shrink-0" />
    <span class="min-w-0 flex-1"><slot /></span>
  </div>
</template>
