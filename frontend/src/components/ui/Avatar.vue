<script setup lang="ts">
import { computed } from "vue";
import { initials } from "@/lib/format";
const props = defineProps<{ name: string }>();
const text = computed(() => initials(props.name));
</script>

<template>
  <span
    class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-elevated font-display text-sm font-semibold text-muted"
  >
    {{ text }}
  </span>
</template>
