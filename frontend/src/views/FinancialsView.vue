<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { Plus, Pencil, Download } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache, invalidate } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { Financials, Expense } from "@/lib/types";
import { money, monthKey, monthLabel } from "@/lib/format";
import { useQueryState } from "@/lib/route-state";
import { isMonthKey, todayKey, currentMonth } from "@/lib/studio";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import { btnClasses } from "@/components/ui/button";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { askReason, VOID_REASONS, CORRECTION_REASONS } from "@/lib/prompt";
import { errMsg } from "@/lib/api";

const month = useQueryState("m", () => monthKey(), isMonthKey);
const fin = ref<Financials>();
const expenses = ref<Expense[]>([]);
const showAdd = ref(false);
const busy = ref(false); // guards against duplicate expense on double-click
const loading = ref(false);
const error = ref<string>();
let loaded = false;
const form = reactive({ amount: 0, category: "rent", note: "", day: todayKey() });
// Adding an expense while viewing an old month defaults its date into that
// month (the 1st), so it lands where the owner is looking — and says so.
watch(month, (m) => {
  form.day = m === currentMonth() ? todayKey() : `${m}-01`;
}, { immediate: true });

const cacheKey = () => `financials:${month.value}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  error.value = undefined;
  try {
    const [f, e] = await Promise.all([
      api.get<Financials>(`/financials?m=${month.value}`),
      api.get<Expense[]>(`/expenses?m=${month.value}`),
    ]);
    if (my !== loadToken) return; // ignore stale (out-of-order) responses
    fin.value = f;
    expenses.value = e;
    writeCache(cacheKey(), { fin: f, expenses: e });
    loaded = true;
  } catch (err) {
    if (my !== loadToken) return;
    if (loaded) toast("Couldn't refresh — showing saved data.", "error");
    else error.value = err instanceof Error && err.message ? err.message : "Couldn't load financials.";
  } finally {
    if (my === loadToken) loading.value = false;
  }
}
function showCached(): boolean {
  const hit = readCache<{ fin: Financials; expenses: Expense[] }>(cacheKey());
  if (hit) {
    fin.value = hit.fin;
    expenses.value = hit.expenses;
    loaded = true;
  }
  return !!hit;
}
watch(month, () => { loading.value = !showCached(); load(); }, { immediate: true });

const netPositive = computed(() => (fin.value?.net ?? 0) >= 0);

// Search this month s outgoings by category, note, or amount. catLabel is
// declared further down; the computed only reads it when it first evaluates,
// which is after setup has run.
const q = ref("");
const filteredExpenses = computed(() => {
  const term = q.value.trim().toLowerCase();
  if (!term) return expenses.value;
  return expenses.value.filter(
    (e) =>
      (catLabel[e.category] ?? e.category).toLowerCase().includes(term) ||
      (e.note ?? "").toLowerCase().includes(term) ||
      String(e.amount).includes(term),
  );
});
const { page, pageCount, items, total, from, to } = usePaged(filteredExpenses, 10);

async function addExpense() {
  if (busy.value || form.amount <= 0) return;
  busy.value = true;
  try {
    await api.post("/expenses", { amount: form.amount, category: form.category, note: form.note, day: form.day });
    form.amount = 0;
    form.note = "";
    invalidate("financials", "expenses");
    showAdd.value = false;
    await load();
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't add expense.", "error");
  } finally {
    busy.value = false;
  }
}
// Voids, not deletes: the record stays in the books marked VOID and stops
// counting toward outgoings.
async function voidExpense(e: Expense) {
  const reason = await askReason({
    title: "Void this expense?",
    message: `${money(e.amount)} (${catLabel[e.category] ?? e.category}) stays in the books marked VOID and stops counting toward outgoings.`,
    confirmLabel: "Void expense",
    suggestions: VOID_REASONS,
  });
  if (reason === null) return;
  try {
    await api.post(`/expenses/${e.id}/void`, { reason });
    invalidate("financials", "expenses");
    load();
    toast("Expense voided.", "success");
  } catch (err) {
    toast(errMsg(err, "Couldn't void expense."), "error");
  }
}

// Full month ledger (payments + sales + expenses + totals) as CSV, fetched
// through the api client so the auth header rides along.
async function exportStatement() {
  try {
    const csv = await api.get<string>(`/financials/export?m=${month.value}`);
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `statement-${month.value}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't export statement.", "error");
  }
}

// Inline edit (PUT /expenses/:id).
const editingId = ref<string | null>(null);
const savingEdit = ref(false);
const editForm = reactive({ amount: 0, category: "rent", note: "" });
function openEditExpense(e: Expense) {
  editingId.value = editingId.value === e.id ? null : e.id;
  Object.assign(editForm, { amount: e.amount, category: e.category, note: e.note ?? "" });
}
async function saveExpenseEdit(id: string) {
  if (savingEdit.value || editForm.amount <= 0) return;
  const original = expenses.value.find((x) => x.id === id);
  let reason = "";
  if (original && Math.round(original.amount * 100) !== Math.round(editForm.amount * 100)) {
    const r = await askReason({
      title: "Correct the amount?",
      message: `${money(original.amount)} → ${money(editForm.amount)}. The change is kept in this expense's history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: CORRECTION_REASONS,
    });
    if (r === null) return;
    reason = r;
  }
  savingEdit.value = true;
  try {
    await api.put(`/expenses/${id}`, { amount: editForm.amount, category: editForm.category, note: editForm.note, reason });
    editingId.value = null;
    invalidate("financials", "expenses");
    await load();
    toast("Expense updated.", "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't update expense.", "error");
  } finally {
    savingEdit.value = false;
  }
}
const catLabel: Record<string, string> = {
  rent: "Rent", equipment: "Equipment", utilities: "Utilities", supplies: "Supplies", wages: "Wages", other: "Other",
};
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Whole business · income & expenses" title="Financials" />

    <MonthPicker v-model="month" />

    <div v-if="loading" class="space-y-3">
      <Skeleton variant="stats" />
      <Skeleton :rows="3" />
    </div>

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else-if="fin">
      <div class="grid grid-cols-2 gap-3">
        <StatTile label="Income" :value="money(fin.income)" sub="fees + shop sales" accent />
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

      <button class="inline-flex items-center gap-1 px-1 text-sm font-medium text-bronze hover:underline" @click="exportStatement">
        <Download class="h-3.5 w-3.5" /> Monthly statement (CSV)
      </button>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Outgoings · {{ monthLabel(month) }}</h2>
          <button :class="btnClasses('primary', 'sm')" @click="showAdd = !showAdd"><Plus class="h-4 w-4" /> {{ showAdd ? "Close" : "Add expense" }}</button>
        </div>

        <Card v-if="showAdd" class="space-y-3 p-4">
          <div class="grid grid-cols-2 gap-3">
            <input v-model.number="form.amount" type="number" min="0" step="0.01" inputmode="decimal" placeholder="Amount" aria-label="Amount" :class="inputCls" />
            <select v-model="form.category" :class="inputCls">
              <option v-for="(l, k) in catLabel" :key="k" :value="k">{{ l }}</option>
            </select>
          </div>
          <input v-model="form.note" placeholder="Note (optional)" aria-label="Note" :class="inputCls" />
          <label class="block">
            <span class="mb-1 block text-xs text-faint">Paid on</span>
            <input v-model="form.day" type="date" :class="inputCls" />
          </label>
          <p class="text-xs text-faint">Counts toward {{ monthLabel(form.day.slice(0, 7)) }}{{ form.day.slice(0, 7) !== month ? " — not the month you're viewing" : "" }}.</p>
          <Button size="sm" :disabled="busy || form.amount <= 0 || !form.day" @click="addExpense">{{ busy ? "Adding…" : "Add expense" }}</Button>
        </Card>

        <SearchInput v-model="q" placeholder="Search outgoings — category, note, amount…" />

        <ul class="space-y-2">
          <li v-for="e in items" :key="e.id" class="rounded-xl border border-line bg-surface" :class="e.voidedAt ? 'opacity-60' : ''">
            <div class="flex items-center justify-between gap-3 px-3 py-2.5">
              <div class="min-w-0">
                <p class="text-sm font-medium">
                  {{ catLabel[e.category] ?? e.category }}
                  <span v-if="e.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                </p>
                <p v-if="e.note" class="truncate text-xs text-faint">{{ e.note }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <span class="font-display text-sm text-overdue tnum" :class="e.voidedAt ? 'line-through text-faint' : ''">−{{ money(e.amount) }}</span>
                <template v-if="!e.voidedAt">
                  <button class="grid h-7 w-7 place-items-center rounded-lg text-purple transition-colors hover:bg-purple/10" aria-label="Edit expense" @click="openEditExpense(e)"><Pencil class="h-4 w-4" /></button>
                  <button class="grid h-7 w-7 place-items-center rounded-lg text-lg leading-none text-faint transition-colors hover:bg-overdue/10 hover:text-overdue" aria-label="Void expense" @click="voidExpense(e)">×</button>
                </template>
              </div>
            </div>
            <div v-if="editingId === e.id" class="space-y-2 border-t border-line px-3 py-3">
              <div class="grid grid-cols-2 gap-2">
                <input v-model.number="editForm.amount" type="number" min="0" placeholder="Amount" :class="inputCls" />
                <select v-model="editForm.category" :class="inputCls">
                  <option v-for="(l, k) in catLabel" :key="k" :value="k">{{ l }}</option>
                </select>
              </div>
              <input v-model="editForm.note" placeholder="Note (optional)" :class="inputCls" />
              <div class="flex gap-2">
                <Button size="sm" :disabled="savingEdit || editForm.amount <= 0" @click="saveExpenseEdit(e.id)">{{ savingEdit ? "Saving…" : "Save changes" }}</Button>
                <Button size="sm" variant="ghost" @click="editingId = null">Cancel</Button>
              </div>
            </div>
            <div class="border-t border-line px-3 py-2">
              <AuditTrail entity="expense" :id="e.id" />
            </div>
          </li>
          <li v-if="filteredExpenses.length === 0" class="px-1 text-sm text-faint">
            {{ q ? "No outgoings match that." : "No expenses logged this month." }}
          </li>
        </ul>
        <Pagination v-model="page" :page-count="pageCount" :total="total" :from="from" :to="to" label="expenses" />
      </section>
    </template>
  </div>
</template>
