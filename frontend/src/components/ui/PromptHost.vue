<script setup lang="ts">
// The app-wide reason prompt (see lib/prompt.ts). A modal dialog: focus moves
// into the field when it opens and back to whatever opened it when it closes;
// Escape cancels; Enter (without Shift) confirms.
import { computed, nextTick, ref, watch } from "vue";
import { usePromptState, settlePrompt } from "@/lib/prompt";
import { inputCls } from "@/lib/ui";
import { btnClasses } from "./button";

const s = usePromptState();
const field = ref<HTMLTextAreaElement>();
let returnFocus: HTMLElement | null = null;

const canConfirm = computed(() => !s.required || s.value.trim().length > 0);

watch(
  () => s.open,
  async (open) => {
    if (open) {
      returnFocus = document.activeElement as HTMLElement | null;
      await nextTick();
      field.value?.focus();
    } else {
      returnFocus?.focus?.();
      returnFocus = null;
    }
  },
);

function confirm() {
  if (canConfirm.value) settlePrompt(s.value);
}
function onKey(e: KeyboardEvent) {
  if (e.key === "Escape") {
    e.preventDefault();
    settlePrompt(null);
  } else if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    confirm();
  }
}
</script>

<template>
  <div v-if="s.open" class="fixed inset-0 z-[60] flex items-end justify-center p-4 sm:items-center" @keydown="onKey">
    <div class="absolute inset-0 bg-canvas/70 backdrop-blur-sm" @click="settlePrompt(null)" />
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="prompt-title"
      class="relative w-full max-w-sm space-y-3 rounded-2xl border border-line bg-surface p-4 shadow-[var(--shadow-lift)]"
    >
      <div>
        <h2 id="prompt-title" class="font-display text-lg font-semibold">{{ s.title }}</h2>
        <p v-if="s.message" class="mt-1 text-sm text-muted">{{ s.message }}</p>
      </div>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Reason{{ s.required ? "" : " (optional)" }}</span>
        <textarea
          ref="field"
          v-model="s.value"
          rows="2"
          :placeholder="s.placeholder ?? 'Why?'"
          :class="inputCls"
          maxlength="500"
        />
      </label>
      <div v-if="s.suggestions?.length" class="flex flex-wrap gap-1.5">
        <button
          v-for="sug in s.suggestions"
          :key="sug"
          type="button"
          class="min-h-10 rounded-lg border border-line px-3 text-xs text-muted transition-colors hover:border-bronze/40 hover:text-fg"
          :class="s.value === sug ? 'border-bronze bg-bronze/15 text-bronze' : ''"
          @click="s.value = sug"
        >{{ sug }}</button>
      </div>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" :class="btnClasses('ghost', 'sm')" @click="settlePrompt(null)">Cancel</button>
        <button type="button" :class="btnClasses(s.tone === 'primary' ? 'primary' : 'danger', 'sm')" :disabled="!canConfirm" @click="confirm">
          {{ s.confirmLabel ?? "Confirm" }}
        </button>
      </div>
    </div>
  </div>
</template>
