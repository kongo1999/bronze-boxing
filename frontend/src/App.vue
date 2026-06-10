<script setup lang="ts">
import { useRoute } from "vue-router";
import AppShell from "@/components/shell/AppShell.vue";
import ToastHost from "@/components/ui/ToastHost.vue";

// The login gate renders bare (no nav shell); everything else gets the shell.
const route = useRoute();
</script>

<template>
  <router-view v-if="route.name === 'login'" />
  <AppShell v-else>
    <router-view v-slot="{ Component, route: r }">
      <transition name="page" mode="out-in">
        <component :is="Component" :key="r.path" />
      </transition>
    </router-view>
  </AppShell>
  <ToastHost />
</template>
