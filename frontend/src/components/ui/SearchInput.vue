<script setup lang="ts">
// One search field for every data page, so the affordance never drifts.
// Icon inside the field, a clear (×) button once there's a term, and
// type="search" so mobile keyboards show a Search key.
import { Search, X } from "lucide-vue-next";

const model = defineModel<string>({ required: true });
withDefaults(defineProps<{ placeholder?: string }>(), { placeholder: "Search…" });
</script>

<template>
  <div class="relative">
    <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
    <input
      v-model="model"
      type="search"
      :placeholder="placeholder"
      class="w-full rounded-xl border border-line bg-elevated py-2.5 pl-9 pr-9 text-sm outline-none transition-colors placeholder:text-faint focus:border-bronze focus:ring-2 focus:ring-bronze/25 [&::-webkit-search-cancel-button]:appearance-none"
    />
    <button
      v-if="model"
      type="button"
      aria-label="Clear search"
      class="absolute right-2 top-1/2 grid h-6 w-6 -translate-y-1/2 place-items-center rounded-lg text-faint transition-colors hover:bg-surface hover:text-fg"
      @click="model = ''"
    >
      <X class="h-3.5 w-3.5" />
    </button>
  </div>
</template>
