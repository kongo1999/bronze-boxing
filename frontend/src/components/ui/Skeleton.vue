<script setup lang="ts">
// Loading placeholder that mirrors the shape of the content it replaces, so
// pages don't jump when real data lands. Variants cover the layouts we use.
withDefaults(
  defineProps<{ variant?: "list" | "stats" | "card" | "detail"; rows?: number }>(),
  { variant: "list", rows: 5 },
);
</script>

<template>
  <div class="animate-pulse" aria-hidden="true">
    <!-- Two stat tiles side by side -->
    <div v-if="variant === 'stats'" class="grid grid-cols-2 gap-3">
      <div class="h-[5.75rem] rounded-2xl border border-line bg-surface" />
      <div class="h-[5.75rem] rounded-2xl border border-line bg-surface" />
    </div>

    <!-- A single detail card -->
    <div v-else-if="variant === 'card' || variant === 'detail'" class="space-y-4">
      <div class="rounded-2xl border border-line bg-surface p-4">
        <div class="flex items-center gap-3">
          <div class="h-14 w-14 shrink-0 rounded-full bg-elevated" />
          <div class="flex-1 space-y-2">
            <div class="h-4 w-1/2 rounded bg-elevated" />
            <div class="h-3 w-1/3 rounded bg-elevated/70" />
          </div>
        </div>
        <div class="mt-4 grid grid-cols-2 gap-3">
          <div class="h-14 rounded-xl bg-elevated" />
          <div class="h-14 rounded-xl bg-elevated" />
        </div>
      </div>
    </div>

    <!-- A list of rows -->
    <div v-else class="space-y-2">
      <div
        v-for="n in rows"
        :key="n"
        class="flex items-center gap-3 rounded-2xl border border-line bg-surface p-3"
      >
        <div class="h-10 w-10 shrink-0 rounded-full bg-elevated" />
        <div class="flex-1 space-y-2">
          <div class="h-3 w-1/3 rounded bg-elevated" />
          <div class="h-2.5 w-1/2 rounded bg-elevated/70" />
        </div>
        <div class="h-3 w-10 rounded bg-elevated/70" />
      </div>
    </div>
  </div>
</template>
