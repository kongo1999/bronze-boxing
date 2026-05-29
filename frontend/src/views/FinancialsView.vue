<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { ChevronLeft, ChevronRight, Plus } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
import type { Financials, Expense } from "@/lib/types";
import { money, monthKey, monthLabel, shiftMonth } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import { btnClasses } from "@/components/ui/button";

const btnGhost = btnClasses("ghost", "icon");
const month = ref(monthKey());
const fin = ref<Financials>();
const expenses = ref<Expense[]>([]);
const showAdd = ref(false);
const busy = ref(false); // guards against duplicate expense on double-click
const form = reactive({ amount: 0, category: "rent", note: "" });
const inputCls = "w-full rounded-xl border border-line bg-elevated px-3 py-2.5 text-sm outline-none focus:border-bronze/40";

const cacheKey = () => `financials:${month.value}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  const [f, e] = await Promise.all([
    api.get<Financials>(`/financials?m=${month.value}`),
    api.get<Expense[]>(`/expenses?m=${month.value}`),
  ]);
  if (my !== loadToken) return; // ignore stale (out-of-order) responses
  fin.value = f;
  expenses.value = e;
  writeCache(cacheKey(), { fin: f, expenses: e });
}
function showCached() {
  const hit = readCache<{ fin: Financials; expenses: Expense[] }>(cacheKey());
  if (hit) {
    fin.value = hit.fin;
    expenses.value = hit.expenses;
  }
}
watch(month, () => { showCached(); load(); }, { immediate: true });

const netPositive = computed(() => (fin.value?.net ?? 0) >= 0);

async function addExpense() {
  if (busy.value || form.amount <= 0) return;
  busy.value = true;
  try {
    await api.post("/expenses", { amount: form.amount, category: form.category, note: form.note });
    form.amount = 0;
    form.note = "";
    showAdd.value = false;
    await load();
  } finally {
    busy.value = false;
  }
}
async function removeExpense(id: string) {
  await api.del(`/expenses/${id}`);
  load();
}
const catLabel: Record<string, string> = {
  rent: "Rent", equipment: "Equipment", utilities: "Utilities", supplies: "Supplies", wages: "Wages", other: "Other",
};
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Books" title="Financials" />

    <div class="flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
      <button :class="btnGhost" aria-label="Previous" @click="month = shiftMonth(month, -1)"><ChevronLeft class="h-5 w-5" /></button>
      <p class="font-display font-semibold tracking-tight">{{ monthLabel(month) }}</p>
      <button :class="btnGhost" aria-label="Next" @click="month = shiftMonth(month, 1)"><ChevronRight class="h-5 w-5" /></button>
    </div>

    <template v-if="fin">
      <div class="grid grid-cols-2 gap-3">
        <StatTile label="Income" :value="money(fin.income)" sub="money in" accent />
        <StatTile label="Outgoings" :value="money(fin.outgoings)" sub="money out" />
      </div>
      <Card class="flex items-center justify-between p-4">
        <div>
          <p class="label-eyebrow text-[0.625rem] text-faint">Net this month</p>
          <p class="font-display text-3xl font-semibold tnum" :class="netPositive ? 'text-paid' : 'text-overdue'">
            {{ netPositive ? "" : "−" }}{{ money(Math.abs(fin.net)) }}
          </p>
        </div>
        <span class="rounded-full px-3 py-1 text-xs font-medium" :class="netPositive ? 'bg-paid/15 text-paid' : 'bg-overdue/15 text-overdue'">
          {{ netPositive ? "Profit" : "Loss" }}
        </span>
      </Card>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Outgoings · {{ monthLabel(month) }}</h2>
          <button class="inline-flex items-center gap-1 text-sm font-medium text-bronze" @click="showAdd = !showAdd"><Plus class="h-3.5 w-3.5" /> Expense</button>
        </div>

        <Card v-if="showAdd" class="space-y-3 p-4">
          <div class="grid grid-cols-2 gap-3">
            <input v-model.number="form.amount" type="number" min="0" placeholder="Amount" :class="inputCls" />
            <select v-model="form.category" :class="inputCls">
              <option v-for="(l, k) in catLabel" :key="k" :value="k">{{ l }}</option>
            </select>
          </div>
          <input v-model="form.note" placeholder="Note (optional)" :class="inputCls" />
          <Button size="sm" :disabled="busy" @click="addExpense">Add expense</Button>
        </Card>

        <ul class="space-y-2">
          <li v-for="e in expenses" :key="e.id" class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5">
            <div class="min-w-0">
              <p class="text-sm font-medium">{{ catLabel[e.category] ?? e.category }}</p>
              <p v-if="e.note" class="truncate text-xs text-faint">{{ e.note }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-3">
              <span class="font-display text-sm text-overdue tnum">−{{ money(e.amount) }}</span>
              <button class="text-faint hover:text-overdue" aria-label="Delete" @click="removeExpense(e.id)">×</button>
            </div>
          </li>
          <li v-if="expenses.length === 0" class="px-1 text-sm text-faint">No expenses logged this month.</li>
        </ul>
      </section>
    </template>
  </div>
</template>

