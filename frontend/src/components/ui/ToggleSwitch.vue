<script setup lang="ts">
// iOS-style on/off switch. v-model:boolean.
defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{ (e: "update:modelValue", v: boolean): void }>();
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    class="relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors duration-200 ease-[var(--ease-out-quart)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-bronze focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
    :class="modelValue ? 'bg-bronze' : 'bg-line'"
    @click="emit('update:modelValue', !modelValue)"
  >
    <span
      class="inline-block h-5 w-5 rounded-full bg-white shadow-sm transition-transform duration-200 ease-[var(--ease-out-quart)]"
      :class="modelValue ? 'translate-x-[1.375rem]' : 'translate-x-0.5'"
    />
  </button>
</template>
