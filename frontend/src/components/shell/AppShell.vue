<script setup lang="ts">
import Navigation from "./Navigation.vue";
</script>

<template>
  <div class="min-h-dvh md:pl-60">
    <Navigation />
    <main
      class="mx-auto w-full max-w-md px-4 pb-28 pt-[4.5rem] md:max-w-2xl md:px-8 md:pb-12 md:pt-8"
    >
      <slot />
    </main>
  </div>
</template>
