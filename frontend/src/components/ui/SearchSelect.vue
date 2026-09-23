<script setup lang="ts">
// A picker you can type into — for any list longer than a handful (trainees,
// buyers, items). Opens with a search field focused; ranks with the shared
// fuzzy matcher (typos, accents, phone digits, aliases) and highlights what
// matched; shows the top 20 with "show more". Keyboard: arrows move, Enter
// picks, Escape closes and returns focus to the button. ARIA combobox +
// listbox so screen readers announce the active option.
import { computed, nextTick, ref, watch, onBeforeUnmount, onMounted, useId } from "vue";
import { Check, ChevronDown, Search } from "lucide-vue-next";
import { rank } from "@/lib/fuzzy";
import Highlight from "./Highlight.vue";

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
    /** Accessible name for the picker (defaults to the placeholder). */
    label?: string;
  }>(),
  { placeholder: "Select…", searchPlaceholder: "Search…" },
);

const model = defineModel<string>({ required: true });

const PAGE = 20;
const uid = useId();
const listId = `${uid}-list`;
const open = ref(false);
const term = ref("");
const shown = ref(PAGE);
const active = ref(0);
const root = ref<HTMLElement>();
const trigger = ref<HTMLButtonElement>();
const searchEl = ref<HTMLInputElement>();
const listEl = ref<HTMLElement>();

const selected = computed(() => props.options.find((o) => o.id === model.value));
const buttonLabel = computed(() => selected.value?.label ?? (model.value ? "…" : (props.emptyLabel ?? props.placeholder)));

// Rows in the list: the "nothing" choice (when allowed and not searching),
// then the ranked matches up to the page size.
interface Row {
  id: string;
  label: string;
  sub?: string;
}
const matches = computed(() => rank(props.options, term.value, (o) => [o.label, o.sub]).map((r) => r.item));
const rows = computed<Row[]>(() => {
  const head: Row[] = props.emptyLabel !== undefined && !term.value.trim() ? [{ id: "", label: props.emptyLabel }] : [];
  return [...head, ...matches.value.slice(0, shown.value)];
});
const more = computed(() => Math.max(0, matches.value.length - shown.value));
watch(term, () => {
  shown.value = PAGE;
  active.value = 0;
});

async function openList() {
  if (props.disabled) return;
  open.value = true;
  term.value = "";
  shown.value = PAGE;
  active.value = Math.max(0, rows.value.findIndex((r) => r.id === model.value));
  await nextTick();
  searchEl.value?.focus();
  scrollActive();
}
function close(refocus = true) {
  open.value = false;
  if (refocus) nextTick(() => trigger.value?.focus());
}
function pick(id: string) {
  model.value = id;
  close();
}
function scrollActive() {
  nextTick(() => listEl.value?.querySelector<HTMLElement>(`[data-i="${active.value}"]`)?.scrollIntoView({ block: "nearest" }));
}
function onInputKey(e: KeyboardEvent) {
  const n = rows.value.length;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    if (n) active.value = (active.value + 1) % n;
    scrollActive();
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    if (n) active.value = (active.value - 1 + n) % n;
    scrollActive();
  } else if (e.key === "Home" && n) {
    e.preventDefault();
    active.value = 0;
    scrollActive();
  } else if (e.key === "End" && n) {
    e.preventDefault();
    active.value = n - 1;
    scrollActive();
  } else if (e.key === "Enter") {
    e.preventDefault();
    const r = rows.value[active.value];
    if (r) pick(r.id);
  } else if (e.key === "Escape") {
    e.preventDefault();
    e.stopPropagation();
    close();
  } else if (e.key === "Tab") {
    close(false);
  }
}
function onTriggerKey(e: KeyboardEvent) {
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    openList();
  }
}
function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) close(false);
}
onMounted(() => document.addEventListener("mousedown", onDocClick));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocClick));
const optId = (i: number) => `${uid}-opt-${i}`;
</script>

<template>
  <div ref="root" class="relative">
    <button
      ref="trigger"
      type="button"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-label="`${label ?? placeholder}: ${buttonLabel}`"
      class="flex min-h-10 w-full items-center justify-between gap-2 rounded-xl border border-line bg-elevated px-3 py-2 text-left text-sm outline-none transition-colors focus:border-bronze focus:ring-2 focus:ring-bronze/25 disabled:opacity-50"
      :class="open ? 'border-bronze' : ''"
      @click="open ? close() : openList()"
      @keydown="onTriggerKey"
    >
      <span class="truncate" :class="selected ? 'text-fg' : 'text-faint'">{{ buttonLabel }}</span>
      <ChevronDown class="h-4 w-4 shrink-0 text-faint transition-transform duration-150 motion-reduce:transition-none" :class="open ? 'rotate-180' : ''" />
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
          role="combobox"
          aria-autocomplete="list"
          :aria-expanded="true"
          :aria-controls="listId"
          :aria-activedescendant="rows.length ? optId(active) : undefined"
          :aria-label="searchPlaceholder"
          :placeholder="searchPlaceholder"
          autocomplete="off"
          class="min-h-10 w-full rounded-lg border border-line bg-elevated py-1.5 pl-7 pr-2 text-sm outline-none placeholder:text-faint focus:border-bronze"
          @keydown="onInputKey"
        />
      </div>
      <ul :id="listId" ref="listEl" role="listbox" :aria-label="label ?? placeholder" class="max-h-60 overflow-y-auto p-1">
        <li
          v-for="(o, i) in rows"
          :id="optId(i)"
          :key="o.id || '__none'"
          :data-i="i"
          role="option"
          :aria-selected="model === o.id"
          class="flex min-h-10 cursor-pointer items-center gap-2 rounded-lg px-2 text-left text-sm"
          :class="[i === active ? 'bg-elevated' : '', model === o.id ? 'text-bronze' : o.id ? 'text-fg' : 'text-muted']"
          @mousemove="active = i"
          @mousedown.prevent
          @click="pick(o.id)"
        >
          <Check class="h-3.5 w-3.5 shrink-0" :class="model === o.id ? '' : 'text-transparent'" />
          <span class="min-w-0 flex-1 truncate"><Highlight :text="o.label" :q="term" /></span>
          <span v-if="o.sub" class="shrink-0 text-xs text-faint tnum"><Highlight :text="o.sub" :q="term" /></span>
        </li>
        <li v-if="!matches.length && term.trim()" class="px-2 py-3 text-center text-xs text-faint" role="presentation">
          No matches for “{{ term.trim() }}”.
        </li>
        <li v-else-if="!options.length" class="px-2 py-3 text-center text-xs text-faint" role="presentation">Nothing to pick yet.</li>
      </ul>
      <button
        v-if="more"
        type="button"
        class="min-h-10 w-full border-t border-line text-xs font-medium text-bronze hover:bg-elevated"
        @mousedown.prevent
        @click="shown += 20"
      >Show {{ Math.min(more, 20) }} more of {{ more }}</button>
    </div>
  </div>
</template>
