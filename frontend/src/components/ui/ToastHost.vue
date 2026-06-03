<script setup lang="ts">
import { CircleCheck, TriangleAlert, Info, X } from "lucide-vue-next";
import { useToasts, dismissToast, type ToastTone } from "@/lib/toast";

const toasts = useToasts();
const icon = { error: TriangleAlert, success: CircleCheck, info: Info };
const tone: Record<ToastTone, string> = {
  error: "border-overdue/40 text-overdue",
  success: "border-paid/40 text-paid",
  info: "border-info/40 text-info",
};
</script>

<template>
  <!-- Fixed above everything; bottom on mobile (clear of the FAB), top-right on desktop. -->
  <div
    class="pointer-events-none fixed inset-x-0 bottom-24 z-[60] flex flex-col items-center gap-2 px-4 md:inset-x-auto md:right-6 md:top-6 md:bottom-auto md:items-end"
  >
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts.items"
        :key="t.id"
        class="pointer-events-auto flex w-full max-w-sm items-start gap-2.5 rounded-xl border bg-elevated px-3.5 py-3 text-sm shadow-[var(--shadow-lift)] backdrop-blur"
        :class="tone[t.tone]"
        role="status"
      >
        <component :is="icon[t.tone]" class="mt-0.5 h-4 w-4 shrink-0" />
        <span class="min-w-0 flex-1 text-fg">{{ t.message }}</span>
        <button class="shrink-0 text-faint hover:text-fg" aria-label="Dismiss" @click="dismissToast(t.id)">
          <X class="h-4 w-4" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.2s var(--ease-out-quart), transform 0.2s var(--ease-out-quart);
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
@media (prefers-reduced-motion: reduce) {
  .toast-enter-active,
  .toast-leave-active {
    transition: none;
  }
}
</style>
