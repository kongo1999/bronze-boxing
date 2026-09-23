<script setup lang="ts">
// A <select> replacement you can type into. Once the roster passes a handful
// of names, scrolling a native dropdown on a phone is the slowest part of
// logging a payment or a sale — so the picker opens with a search field
// focused and filters as you type.
import { computed, nextTick, ref, onBeforeUnmount, onMounted } from "vue";
import { Check, ChevronDown, Search } from "lucide-vue-next";

export interface SelectOption {
  id: string;
  label: string;
  sub?: string;
}

const props = withDefaults(
  defineProps<{
    options: SelectOption[];
    placeholder?: string;
    /** Label for the "nothing selected" choice. Omit to make a pick required. */
    emptyLabel?: string;
    searchPlaceholder?: string;
    disabled?: boolean;
  }>(),
  { placeholder: "Select…", searchPlaceholder: "Search…" },
);

const model = defineModel<string>({ required: true });

const open = ref(false);
const term = ref("");
const root = ref<HTMLElement>();
const searchEl = ref<HTMLInputElement>();

const selected = computed(() => props.options.find((o) => o.id === model.value));
const buttonLabel = computed(
  () => selected.value?.label ?? (model.value ? "…" : (props.emptyLabel ?? props.placeholder)),
);
const filtered = computed(() => {
  const q = term.value.trim().toLowerCase();
  if (!q) return props.options;
  return props.options.filter(
    (o) => o.label.toLowerCase().includes(q) || (o.sub ?? "").toLowerCase().includes(q),
  );
});

async function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
  if (open.value) {
    term.value = "";
    await nextTick();
    searchEl.value?.focus();
  }
}
function pick(id: string) {
  model.value = id;
  open.value = false;
}
function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false;
}
function onKey(e: KeyboardEvent) {
  if (e.key === "Escape" && open.value) {
    open.value = false;
    e.stopPropagation();
  }
}
onMounted(() => document.addEventListener("mousedown", onDocClick));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocClick));
</script>

<template>
  <div ref="root" class="relative" @keydown="onKey">
    <button
      type="button"
      :disabled="disabled"
      class="flex w-full items-center justify-between gap-2 rounded-xl border border-line bg-elevated px-3 py-2.5 text-left text-sm outline-none transition-colors focus:border-bronze focus:ring-2 focus:ring-bronze/25 disabled:opacity-50"
      :class="open ? 'border-bronze' : ''"
      @click="toggle"
    >
      <span class="truncate" :class="selected ? 'text-fg' : 'text-faint'">{{ buttonLabel }}</span>
      <ChevronDown class="h-4 w-4 shrink-0 text-faint transition-transform duration-150" :class="open ? 'rotate-180' : ''" />
    </button>

    <div
      v-if="open"
      class="absolute left-0 right-0 top-[calc(100%+0.25rem)] z-30 overflow-hidden rounded-xl border border-line bg-surface shadow-[var(--shadow-lift)]"
    >
      <div class="relative border-b border-line p-2">
        <Search class="pointer-events-none absolute left-4 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
        <input
          ref="searchEl"
          v-model="term"
          type="text"
          :placeholder="searchPlaceholder"
          class="w-full rounded-lg border border-line bg-elevated py-1.5 pl-7 pr-2 text-sm outline-none placeholder:text-faint focus:border-bronze"
        />
      </div>
      <div class="max-h-52 overflow-y-auto p-1">
        <button
          v-if="emptyLabel !== undefined && !term"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-elevated"
          :class="model === '' ? 'text-bronze' : 'text-muted'"
          @click="pick('')"
        >
          <Check class="h-3.5 w-3.5 shrink-0" :class="model === '' ? '' : 'text-transparent'" />
          {{ emptyLabel }}
        </button>
        <p v-if="filtered.length === 0" class="px-2 py-3 text-center text-xs text-faint">
          {{ options.length === 0 ? "Nothing to pick yet." : "No matches." }}
        </p>
        <button
          v-for="o in filtered"
          :key="o.id"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-elevated"
          :class="model === o.id ? 'text-bronze' : 'text-fg'"
          @click="pick(o.id)"
        >
          <Check class="h-3.5 w-3.5 shrink-0" :class="model === o.id ? '' : 'text-transparent'" />
          <span class="min-w-0 flex-1 truncate">{{ o.label }}</span>
          <span v-if="o.sub" class="shrink-0 text-xs text-faint tnum">{{ o.sub }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
