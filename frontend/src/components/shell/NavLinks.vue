<script setup lang="ts">
import { RouterLink, useRoute } from "vue-router";
import { Search, type LucideIcon } from "lucide-vue-next";

// Vertical nav list shared by the desktop side-rail and the mobile drawer, so
// the link markup + active styling live in exactly one place.
interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
}
defineProps<{ items: NavItem[]; showSearch?: boolean }>();
const emit = defineEmits<{ navigate: [] }>();

const route = useRoute();
const isActive = (to: string) => (to === "/" ? route.path === "/" : route.path.startsWith(to));
const linkCls = (active: boolean) =>
  active ? "bg-bronze/15 text-bronze" : "text-muted hover:bg-elevated hover:text-fg";
</script>

<template>
  <nav class="flex flex-col gap-1">
    <RouterLink
      v-for="item in items"
      :key="item.to"
      :to="item.to"
      class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
      :class="linkCls(isActive(item.to))"
      @click="emit('navigate')"
    >
      <component :is="item.icon" class="h-5 w-5" :stroke-width="isActive(item.to) ? 2.25 : 1.75" />
      {{ item.label }}
    </RouterLink>
    <RouterLink
      v-if="showSearch"
      to="/search"
      class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors"
      :class="linkCls(isActive('/search'))"
      @click="emit('navigate')"
    >
      <Search class="h-5 w-5" :stroke-width="1.75" />
      Search
    </RouterLink>
  </nav>
</template>
