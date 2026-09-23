<script setup lang="ts">
// Range readout + prev/next. Deliberately not a numbered page strip: on a
// phone the coach only ever wants "more" or "back", and two big targets beat
// eight small ones. Renders nothing when everything fits on one page.
import { computed } from "vue";
import { ChevronLeft, ChevronRight } from "lucide-vue-next";

const props = defineProps<{
  pageCount: number;
  total: number;
  from: number;
  to: number;
  label?: string;
}>();
const page = defineModel<number>({ required: true });

const noun = computed(() => props.label ?? "items");
const canPrev = computed(() => page.value > 1);
const canNext = computed(() => page.value < props.pageCount);
const btn =
  "grid h-8 w-8 place-items-center rounded-lg border border-line text-muted transition-colors " +
  "hover:border-bronze/40 hover:text-fg disabled:pointer-events-none disabled:opacity-35";
</script>

<template>
  <div v-if="pageCount > 1" class="flex items-center justify-between gap-3 px-1 pt-1">
    <p class="text-xs text-faint tnum">{{ from }}–{{ to }} of {{ total }} {{ noun }}</p>
    <div class="flex items-center gap-1.5">
      <button :class="btn" :disabled="!canPrev" aria-label="Previous page" @click="page--">
        <ChevronLeft class="h-4 w-4" />
      </button>
      <span class="px-1 text-xs text-muted tnum">{{ page }} / {{ pageCount }}</span>
      <button :class="btn" :disabled="!canNext" aria-label="Next page" @click="page++">
        <ChevronRight class="h-4 w-4" />
      </button>
    </div>
  </div>
</template>
