<script setup lang="ts">
// End-of-day cash count: what the books say came in as cash each day, next
// to what was actually counted in the till. Days where they differ stand out.
import { computed, reactive, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ChevronLeft, Check, TriangleAlert } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import type { ClosingDay } from "@/lib/types";
import { money, monthKey, monthLabel } from "@/lib/format";
import { formatDay, isMonthKey, todayKey } from "@/lib/studio";
import { backTarget, useQueryState } from "@/lib/route-state";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import Button from "@/components/ui/Button.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

const route = useRoute();
const month = useQueryState("m", () => monthKey(), isMonthKey);
type Show = "all" | "mismatch" | "uncounted";
const show = useQueryState<Show>("show", () => "all", (v): v is Show => ["all", "mismatch", "uncounted"].includes(v as string));

const days = ref<ClosingDay[]>([]);
const loading = ref(true);
const error = ref<string>();
let token = 0;
async function load() {
  const my = ++token;
  error.value = undefined;
  try {
    const res = await api.get<ClosingDay[]>(`/cash-closings?m=${month.value}`);
    if (my === token) days.value = res;
  } catch (e) {
    if (my === token) error.value = errMsg(e, "Couldn't load the cash counts.");
  } finally {
    if (my === token) loading.value = false;
  }
}
watch(month, () => { loading.value = true; load(); }, { immediate: true });

const mismatched = (d: ClosingDay) => d.difference !== undefined && Math.abs(d.difference) >= 0.01;
const visible = computed(() =>
  days.value.filter((d) => (show.value === "mismatch" ? mismatched(d) : show.value === "uncounted" ? d.counted === undefined : true)),
);
const chips = computed(() => [
  { v: "all" as const, l: "All days", n: days.value.length },
  { v: "mismatch" as const, l: "Don't match", n: days.value.filter(mismatched).length, tone: "overdue" as const },
  { v: "uncounted" as const, l: "Not counted", n: days.value.filter((d) => d.counted === undefined).length },
]);

const editing = ref<string>();
const form = reactive({ counted: 0, note: "" });
const saving = ref(false);
function edit(d: ClosingDay) {
  editing.value = editing.value === d.day ? undefined : d.day;
  Object.assign(form, { counted: d.counted ?? d.expected, note: d.note ?? "" });
}
async function save(d: ClosingDay) {
  if (saving.value) return;
  saving.value = true;
  try {
    await api.put(`/cash-closings/${d.day}`, { counted: form.counted, note: form.note });
    editing.value = undefined;
    await load();
    toast("Count saved.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't save the count."), "error");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="backTarget(route.query, `/financials?m=${month}`)" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Financials
    </RouterLink>
    <div>
      <h1 class="font-display text-2xl font-semibold">Count cash</h1>
      <p class="mt-1 text-sm text-muted">
        For each day: the cash the books expect — cash payments and shop sales, less refunds — against what you counted in the till.
      </p>
    </div>

    <MonthPicker v-model="month" />
    <ChipGroup v-model="show" :options="chips" label="Show days" />

    <Skeleton v-if="loading" :rows="4" />
    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>
    <p v-else-if="visible.length === 0" class="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
      {{ days.length ? "No days in this list." : `No money came in during ${monthLabel(month)}.` }}
    </p>

    <ul v-else class="space-y-2">
      <li v-for="d in visible" :key="d.day" class="rounded-xl border bg-surface" :class="mismatched(d) ? 'border-overdue/50' : 'border-line'">
        <button type="button" class="flex w-full items-center justify-between gap-3 px-3 py-2.5 text-left" :aria-expanded="editing === d.day" @click="edit(d)">
          <div class="min-w-0">
            <p class="text-sm font-medium">{{ formatDay(d.day, { weekday: "short", month: "short", day: "numeric" }) }}</p>
            <p class="text-xs text-faint tnum">
              Expected cash {{ money(d.expected) }}
              <template v-if="d.unspecified"> (incl. {{ money(d.unspecified) }} with no method recorded)</template>
              <template v-if="d.card || d.transfer"> · card {{ money(d.card) }} · transfer {{ money(d.transfer) }}</template>
            </p>
          </div>
          <div class="shrink-0 text-right">
            <template v-if="d.counted !== undefined">
              <p class="font-display text-sm tnum">{{ money(d.counted) }}</p>
              <p class="inline-flex items-center gap-1 text-xs tnum" :class="mismatched(d) ? 'text-overdue' : 'text-paid'">
                <component :is="mismatched(d) ? TriangleAlert : Check" class="h-3 w-3" />
                {{ mismatched(d) ? `${d.difference! > 0 ? "+" : ""}${money(d.difference!)}` : "matches" }}
              </p>
            </template>
            <p v-else class="text-xs text-faint">{{ d.day <= todayKey() ? "Tap to count" : "" }}</p>
          </div>
        </button>
        <div v-if="editing === d.day" class="space-y-2 border-t border-line px-3 py-3">
          <label class="block"><span class="mb-1 block text-xs text-faint">Counted in the till</span>
            <input v-model.number="form.counted" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" />
          </label>
          <label class="block"><span class="mb-1 block text-xs text-faint">Note <span class="text-faint/70">(optional)</span></span>
            <input v-model="form.note" :class="inputCls" placeholder="e.g. paid the cleaner from the till" />
          </label>
          <p v-if="d.note && !form.note" class="text-xs text-faint">Previous note: {{ d.note }}</p>
          <Button size="sm" :disabled="saving" @click="save(d)">{{ saving ? "Saving…" : "Save count" }}</Button>
        </div>
      </li>
    </ul>
  </div>
</template>
