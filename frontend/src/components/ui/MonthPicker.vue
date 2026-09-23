<script setup lang="ts">
import { computed, ref } from "vue";
import { ChevronLeft, ChevronRight, CalendarDays } from "lucide-vue-next";
import { monthKey, monthLabel, shiftMonth } from "@/lib/format";
import { isMonthKey } from "@/lib/studio";
import { btnClasses } from "./button";

// Month navigator shared by Money + Financials. v-model is a "YYYY-MM" key.
// Arrows step one month; tapping the month name jumps straight to any month
// (native month picker), so a year back is one tap, not twelve.
const model = defineModel<string>({ required: true });
const isCurrent = computed(() => model.value === monthKey());
const picker = ref<HTMLInputElement>();

function openPicker() {
  const el = picker.value;
  if (el && typeof el.showPicker === "function") {
    try {
      el.showPicker();
      return;
    } catch {
      /* unsupported for type=month on this browser: fall through */
    }
  }
  const v = window.prompt("Jump to month (YYYY-MM)", model.value);
  if (v && isMonthKey(v.trim())) model.value = v.trim();
}
function onPick(e: Event) {
  const v = (e.target as HTMLInputElement).value;
  if (isMonthKey(v)) model.value = v;
}
</script>

<template>
  <div class="space-y-2">
    <div class="relative flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
      <button
        :class="btnClasses('ghost', 'icon')"
        aria-label="Previous month"
        @click="model = shiftMonth(model, -1)"
      >
        <ChevronLeft class="h-5 w-5" />
      </button>
      <button type="button" class="min-h-10 rounded-xl px-3 hover:bg-elevated" :aria-label="`${monthLabel(model)} — jump to another month`" @click="openPicker">
        <span class="font-display font-semibold tracking-tight">{{ monthLabel(model) }}</span>
      </button>
      <input ref="picker" type="month" :value="model" tabindex="-1" aria-hidden="true" class="pointer-events-none absolute h-0 w-0 opacity-0" @change="onPick" />
      <button
        :class="btnClasses('ghost', 'icon')"
        aria-label="Next month"
        @click="model = shiftMonth(model, 1)"
      >
        <ChevronRight class="h-5 w-5" />
      </button>
    </div>

    <div v-if="!isCurrent" class="flex justify-center">
      <button :class="btnClasses('ghost', 'sm')" @click="model = monthKey()">
        <CalendarDays class="h-4 w-4" /> Jump to this month
      </button>
    </div>
  </div>
</template>
