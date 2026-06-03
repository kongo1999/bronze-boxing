<script setup lang="ts">
import { computed } from "vue";
import { ChevronLeft, ChevronRight } from "lucide-vue-next";
import { monthKey, monthLabel, shiftMonth } from "@/lib/format";
import { btnClasses } from "./button";

// Month navigator shared by Money + Financials. v-model is a "YYYY-MM" key.
const model = defineModel<string>({ required: true });
const isCurrent = computed(() => model.value === monthKey());
</script>

<template>
  <div class="flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
    <button
      :class="btnClasses('ghost', 'icon')"
      aria-label="Previous month"
      @click="model = shiftMonth(model, -1)"
    >
      <ChevronLeft class="h-5 w-5" />
    </button>
    <div class="text-center">
      <p class="font-display font-semibold tracking-tight">{{ monthLabel(model) }}</p>
      <button v-if="!isCurrent" class="text-xs text-bronze hover:underline" @click="model = monthKey()">
        Back to this month
      </button>
      <p v-else class="text-xs text-faint">This month</p>
    </div>
    <button
      :class="btnClasses('ghost', 'icon')"
      aria-label="Next month"
      @click="model = shiftMonth(model, 1)"
    >
      <ChevronRight class="h-5 w-5" />
    </button>
  </div>
</template>
