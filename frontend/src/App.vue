<script setup lang="ts">
import AppShell from "@/components/shell/AppShell.vue";
import ToastHost from "@/components/ui/ToastHost.vue";
</script>

<template>
  <AppShell>
    <router-view v-slot="{ Component, route }">
      <transition name="page" mode="out-in">
        <component :is="Component" :key="route.path" />
      </transition>
    </router-view>
  </AppShell>
  <ToastHost />
</template>
