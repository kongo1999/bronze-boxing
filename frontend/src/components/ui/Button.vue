<script setup lang="ts">
import { computed } from "vue";
import { btnClasses, type BtnVariant, type BtnSize } from "./button";

const props = withDefaults(
  defineProps<{
    variant?: BtnVariant;
    size?: BtnSize;
    to?: string;
    type?: "button" | "submit";
    disabled?: boolean;
  }>(),
  { variant: "primary", size: "md", type: "button" },
);

const cls = computed(() => btnClasses(props.variant, props.size));
</script>

<template>
  <router-link v-if="to" :to="to" :class="cls">
    <slot />
  </router-link>
  <button v-else :type="type" :disabled="disabled" :class="cls">
    <slot />
  </button>
</template>
