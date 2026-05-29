<script setup lang="ts">
import AppShell from "@/components/shell/AppShell.vue";
</script>

<template>
  <AppShell>
    <router-view v-slot="{ Component }">
      <component :is="Component" />
    </router-view>
  </AppShell>
</template>
