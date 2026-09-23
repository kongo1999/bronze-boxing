<script setup lang="ts">
// Text with the words that matched a search marked, using the same matcher
// as the ranking (so a typo'd query still shows what it hit).
import { computed } from "vue";
import { highlight } from "@/lib/fuzzy";

const props = defineProps<{ text?: string | null; q?: string }>();
const parts = computed(() => highlight(props.text ?? "", props.q ?? ""));
</script>

<template>
  <template v-for="(p, i) in parts" :key="i"><mark v-if="p.hit" class="rounded-sm bg-bronze/25 px-px text-inherit">{{ p.text }}</mark><template v-else>{{ p.text }}</template></template>
</template>
