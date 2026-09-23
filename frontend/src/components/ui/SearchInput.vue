<script setup lang="ts">
// One search field for every data list, so the affordance never drifts.
// Icon inside the field, a live match count and a clear (×) button once
// there's a term, and type="search" so mobile keyboards show a Search key.
// The count is announced politely to screen readers.
import { computed, ref, useAttrs } from "vue";
import { Search, X, LoaderCircle } from "lucide-vue-next";

defineOptions({ inheritAttrs: false });
const model = defineModel<string>({ required: true });
const props = withDefaults(
  defineProps<{
    placeholder?: string;
    /** Accessible name; defaults to the placeholder. */
    label?: string;
    /** Matches for the current term (hidden while the field is empty). */
    matches?: number;
    /** A remote search is in flight. */
    searching?: boolean;
  }>(),
  { placeholder: "Search…" },
);
const attrs = useAttrs();
const input = ref<HTMLInputElement>();
function clear() {
  model.value = "";
  input.value?.focus();
}
const countText = computed(() =>
  props.matches === undefined ? "" : props.matches === 1 ? "1 match" : `${props.matches} matches`,
);
</script>

<template>
  <div class="relative" :class="attrs.class">
    <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
    <input
      :id="attrs.id as string | undefined"
      ref="input"
      v-model="model"
      :autofocus="'autofocus' in attrs"
      type="search"
      :placeholder="placeholder"
      :aria-label="label ?? placeholder"
      autocomplete="off"
      class="min-h-11 w-full rounded-xl border border-line bg-elevated py-2.5 pl-9 text-sm outline-none transition-colors placeholder:text-faint focus:border-bronze focus:ring-2 focus:ring-bronze/25 [&::-webkit-search-cancel-button]:appearance-none"
      :class="model ? 'pr-28' : 'pr-9'"
    />
    <div v-if="model" class="absolute right-0.5 top-1/2 flex -translate-y-1/2 items-center gap-1">
      <LoaderCircle v-if="searching" class="h-3.5 w-3.5 animate-spin text-faint" aria-hidden="true" />
      <span v-else-if="countText" class="whitespace-nowrap text-[0.6875rem] text-faint tnum">{{ countText }}</span>
      <button
        type="button"
        aria-label="Clear search"
        class="grid h-10 w-10 place-items-center rounded-lg text-faint transition-colors hover:bg-surface hover:text-fg"
        @click="clear"
      >
        <X class="h-3.5 w-3.5" />
      </button>
    </div>
    <span class="sr-only" aria-live="polite">{{ model ? (searching ? "Searching…" : countText) : "" }}</span>
  </div>
</template>
