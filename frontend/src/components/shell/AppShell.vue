<script setup lang="ts">
import Navigation from "./Navigation.vue";
import { isDemo } from "@/lib/demo";
</script>

<template>
  <div class="min-h-dvh md:pl-60 print:pl-0">
    <Navigation />
    <!-- In the demo, say so everywhere: nothing done here is saved. -->
    <div
      v-if="isDemo"
      role="status"
      class="sticky top-14 z-30 mt-14 border-b border-info/40 bg-info/15 px-4 py-1.5 text-center text-xs font-medium text-info backdrop-blur md:top-0 md:mt-0"
      data-print-hide
    >
      Demo with sample data — nothing you change here is saved, and it resets when you reload.
    </div>
    <main
      class="mx-auto w-full max-w-md px-4 pb-28 md:max-w-2xl md:px-8 md:pb-12 md:pt-8 print:max-w-none print:p-0"
      :class="isDemo ? 'pt-4' : 'pt-[4.5rem]'"
    >
      <slot />
    </main>
  </div>
</template>
