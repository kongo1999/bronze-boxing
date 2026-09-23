<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { Plus, Download, Scale, X, ChevronRight, TrendingUp, TrendingDown } from "lucide-vue-next";
import { api, errMsg, qs } from "@/lib/api";
import { readCache, writeCache, invalidate } from "@/lib/cache";
import type { Financials, LedgerPage, LedgerRow } from "@/lib/types";
import { money, monthKey, monthLabel, formatLongDate } from "@/lib/format";
import { addDays, currentMonth, formatDay, isDayKey, isMonthKey, todayKey } from "@/lib/studio";
import { useQueryState, withBack } from "@/lib/route-state";
import { CATEGORY_LABEL, categoryLabel, methodLabel, payTypeLabel } from "@/lib/labels";
import { askReason, VOID_REASONS, CORRECTION_REASONS } from "@/lib/prompt";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Highlight from "@/components/ui/Highlight.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

const route = useRoute();
const here = computed(() => route.fullPath);

// ── Period: a month, or a custom range of studio days (inclusive) ────────
type Mode = "month" | "range";
const mode = useQueryState<Mode>("period", () => "month", (v): v is Mode => v === "month" || v === "range");
const month = useQueryState("m", () => monthKey(), isMonthKey);
const fromDay = useQueryState("from", () => `${currentMonth()}-01`, isDayKey);
const toDay = useQueryState("to", () => todayKey(), isDayKey);
const periodQS = computed(() =>
  mode.value === "month" ? `m=${month.value}` : `from=${fromDay.value}&to=${addDays(toDay.value, 1)}`,
);
const periodLabel = computed(() =>
  mode.value === "month"
    ? monthLabel(month.value)
    : `${formatDay(fromDay.value, { month: "short", day: "numeric" })} – ${formatDay(toDay.value, { month: "short", day: "numeric", year: "numeric" })}`,
);
const rangeInvalid = computed(() => mode.value === "range" && toDay.value < fromDay.value);

// ── Summary ──────────────────────────────────────────────────────────────
const fin = ref<Financials>();
const loading = ref(false);
const error = ref<string>();
let loadToken = 0;
async function loadSummary() {
  if (rangeInvalid.value) return;
  const my = ++loadToken;
  const key = `financials:${periodQS.value}`;
  const hit = readCache<Financials>(key);
  if (hit) fin.value = hit;
  loading.value = !hit;
  error.value = undefined;
  try {
    const f = await api.get<Financials>(`/financials?${periodQS.value}`);
    if (my !== loadToken) return;
    fin.value = f;
    writeCache(key, f);
  } catch (e) {
    if (my !== loadToken) return;
    if (hit) toast("Couldn't refresh — showing saved data.", "error");
    else error.value = errMsg(e, "Couldn't load financials.");
  } finally {
    if (my === loadToken) loading.value = false;
  }
}

const netPositive = computed(() => (fin.value?.net ?? 0) >= 0);
const change = computed(() => {
  const f = fin.value;
  if (!f) return null;
  const d = Math.round((f.net - f.previous.net) * 100) / 100;
  const pct = f.previous.net !== 0 ? Math.round((d / Math.abs(f.previous.net)) * 100) : null;
  return { d, pct };
});
const prevLabel = computed(() => (mode.value === "month" ? monthLabel(shiftPrev(month.value)) : "the previous period"));
function shiftPrev(m: string) {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(Date.UTC(y, mo - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

const sortedEntries = (m?: Record<string, number>) =>
  Object.entries(m ?? {}).filter(([, v]) => v !== 0).sort((a, b) => Math.abs(b[1]) - Math.abs(a[1]));
const inTypes = computed(() => sortedEntries(fin.value?.byType));
const outCats = computed(() => sortedEntries(fin.value?.byCategory));
const methods = computed(() => sortedEntries(fin.value?.byMethod));
const share = (v: number, total: number) => (total > 0 ? Math.max(2, Math.round((Math.abs(v) / total) * 100)) : 0);

// ── Ledger (filtered on the server, before paging) ───────────────────────
type Kind = "all" | "income" | "expense";
const kind = useQueryState<Kind>("kind", () => "all", (v): v is Kind => ["all", "income", "expense"].includes(v as string));
const typeFilter = useQueryState<string>("type", () => "");
// The search lives in the URL, so a result opened from global Search (or a
// shared link) lands on the same filtered ledger, the record highlighted.
const q = useQueryState<string>("q", () => "");
const focusId = computed(() => (typeof route.query.focus === "string" ? route.query.focus : ""));
const ledger = ref<LedgerRow[]>([]);
const ledgerTotal = ref(0);
const ledgerMore = ref(false);
const ledgerTotals = ref<LedgerPage["totals"]>();
const ledgerLoading = ref(false);
const ledgerError = ref<string>();
const ledgerQS = (offset = 0) =>
  `${periodQS.value}${qs({
    kind: kind.value === "all" ? "" : kind.value,
    type: typeFilter.value,
    q: q.value.trim(),
    limit: 30,
    offset,
  }).replace("?", "&")}`;
let ledgerToken = 0;
async function loadLedger(append = false) {
  if (rangeInvalid.value) return;
  const my = ++ledgerToken;
  ledgerLoading.value = true;
  ledgerError.value = undefined;
  try {
    const pg = await api.get<LedgerPage>(`/ledger?${ledgerQS(append ? ledger.value.length : 0)}`);
    if (my !== ledgerToken) return;
    ledger.value = append ? [...ledger.value, ...pg.items] : pg.items;
    ledgerTotal.value = pg.total;
    ledgerMore.value = pg.hasMore;
    ledgerTotals.value = pg.totals;
  } catch (e) {
    if (my === ledgerToken) ledgerError.value = errMsg(e, "Couldn't load the ledger.");
  } finally {
    if (my === ledgerToken) ledgerLoading.value = false;
  }
}
let debounce: ReturnType<typeof setTimeout> | undefined;
watch(q, () => {
  clearTimeout(debounce);
  debounce = setTimeout(() => loadLedger(), 200);
});
watch([periodQS, kind, typeFilter], () => loadLedger(), { immediate: true });
watch(periodQS, loadSummary, { immediate: true });

function filterBy(k: Kind, t: string) {
  kind.value = k;
  typeFilter.value = t;
  document.getElementById("ledger")?.scrollIntoView({ behavior: "smooth", block: "start" });
}
const typeChipLabel = computed(() =>
  kind.value === "expense" ? categoryLabel(typeFilter.value) : payTypeLabel(typeFilter.value),
);
function rowHref(r: LedgerRow): string | null {
  if (r.kind === "payment") return `/payments/${r.id}`;
  if (r.kind === "sale") return `/sales/${r.id}`;
  if (r.kind === "return" && r.sale) return `/sales/${r.sale}`;
  return null;
}
function rowSub(r: LedgerRow): string {
  if (r.kind === "expense") return categoryLabel(r.type) + (r.note ? ` · ${r.note}` : "");
  const parts = [payTypeLabel(r.type)];
  if (r.periodMonth) parts.push(`for ${monthLabel(r.periodMonth)}`);
  if (r.kind === "payment") parts.push(methodLabel(r.method));
  return parts.join(" · ");
}

// CSV of exactly what the ledger shows (same period and filters).
async function exportStatement() {
  try {
    const csv = await api.get<string>(`/financials/export?${ledgerQS().replace(/&limit=\d+|&offset=\d+/g, "")}`);
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `statement-${mode.value === "month" ? month.value : `${fromDay.value}_${toDay.value}`}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    toast(errMsg(e, "Couldn't export the statement."), "error");
  }
}

function afterChange() {
  invalidate("financials", "expenses");
  loadSummary();
  loadLedger();
}

// ── Expenses: add, and edit / void inline from the ledger ────────────────
const showAdd = ref(false);
const busy = ref(false);
const form = reactive({ amount: 0, category: "rent", note: "", day: todayKey() });
// Adding while viewing an old month defaults the date into that month (the
// 1st), so it lands where the owner is looking — and the form says where.
watch([month, mode], () => {
  form.day = mode.value === "month" && month.value !== currentMonth() ? `${month.value}-01` : todayKey();
}, { immediate: true });
async function addExpense() {
  if (busy.value || form.amount <= 0 || !form.day) return;
  busy.value = true;
  try {
    await api.post("/expenses", { amount: form.amount, category: form.category, note: form.note, day: form.day });
    form.amount = 0;
    form.note = "";
    showAdd.value = false;
    afterChange();
    toast("Expense added.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't add expense."), "error");
  } finally {
    busy.value = false;
  }
}

const openExpense = ref<string | null>(null);
const editing = ref(false);
const savingEdit = ref(false);
const editForm = reactive({ amount: 0, category: "rent", note: "", day: "" });
function toggleExpense(r: LedgerRow) {
  editing.value = false;
  openExpense.value = openExpense.value === r.id ? null : r.id;
  Object.assign(editForm, { amount: r.out, category: r.type, note: r.note ?? "", day: r.day });
}
async function saveExpense(r: LedgerRow) {
  if (savingEdit.value || editForm.amount <= 0) return;
  let reason = "";
  if (Math.round(r.out * 100) !== Math.round(editForm.amount * 100)) {
    const res = await askReason({
      title: "Correct the amount?",
      message: `${money(r.out)} → ${money(editForm.amount)}. The change is kept in this expense's history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: CORRECTION_REASONS,
    });
    if (res === null) return;
    reason = res;
  }
  savingEdit.value = true;
  try {
    await api.put(`/expenses/${r.id}`, {
      amount: editForm.amount, category: editForm.category, note: editForm.note,
      day: editForm.day !== r.day ? editForm.day : undefined, reason,
    });
    editing.value = false;
    afterChange();
    toast("Expense updated.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't update expense."), "error");
  } finally {
    savingEdit.value = false;
  }
}
async function voidExpense(r: LedgerRow) {
  const reason = await askReason({
    title: "Void this expense?",
    message: `${money(r.out)} (${categoryLabel(r.type)}) stays in the books marked VOID and stops counting toward outgoings.`,
    confirmLabel: "Void expense",
    suggestions: VOID_REASONS,
  });
  if (reason === null) return;
  try {
    await api.post(`/expenses/${r.id}/void`, { reason });
    openExpense.value = null;
    afterChange();
    toast("Expense voided.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't void expense."), "error");
  }
}
const kindChips = [
  { v: "all" as const, l: "All" },
  { v: "income" as const, l: "Money in" },
  { v: "expense" as const, l: "Money out" },
];
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Whole business · money in & out" title="Financials">
      <template #action>
        <RouterLink :to="withBack('/financials/reconcile', here, { m: mode === 'month' ? month : undefined })" :class="btnClasses('ghost', 'sm')">
          <Scale class="h-4 w-4" /> Count cash
        </RouterLink>
      </template>
    </PageHeader>

    <ChipGroup v-model="mode" :options="[{ v: 'month', l: 'Month' }, { v: 'range', l: 'Custom range' }]" label="Period" />
    <MonthPicker v-if="mode === 'month'" v-model="month" />
    <div v-else class="grid grid-cols-2 gap-2 rounded-2xl border border-line bg-surface p-3">
      <label class="block"><span class="mb-1 block text-xs text-faint">From</span><input v-model="fromDay" type="date" :class="inputCls" /></label>
      <label class="block"><span class="mb-1 block text-xs text-faint">To (inclusive)</span><input v-model="toDay" type="date" :class="inputCls" /></label>
      <p v-if="rangeInvalid" class="col-span-2 text-xs text-overdue">The end date is before the start date.</p>
    </div>

    <div v-if="loading" class="space-y-3">
      <Skeleton variant="stats" />
      <Skeleton :rows="3" />
    </div>

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="loadSummary">Retry</button>
    </Alert>

    <template v-else-if="fin">
      <div class="grid grid-cols-2 gap-3">
        <StatTile label="Money in" :value="money(fin.income)" sub="fees, sessions, shop − returns" accent />
        <StatTile label="Money out" :value="money(fin.outgoings)" sub="recorded expenses" />
      </div>
      <Card class="p-4">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="label-eyebrow text-[0.625rem] text-faint">Net cash · {{ periodLabel }}</p>
            <p class="font-display text-3xl font-semibold tnum" :class="netPositive ? 'text-paid' : 'text-overdue'">{{ money(fin.net) }}</p>
          </div>
          <div v-if="change" class="text-right text-xs">
            <p class="inline-flex items-center gap-1 font-medium tnum" :class="change.d >= 0 ? 'text-paid' : 'text-overdue'">
              <component :is="change.d >= 0 ? TrendingUp : TrendingDown" class="h-3.5 w-3.5" />
              {{ change.d >= 0 ? "+" : "" }}{{ money(change.d) }}<template v-if="change.pct !== null"> ({{ change.pct >= 0 ? "+" : "" }}{{ change.pct }}%)</template>
            </p>
            <p class="text-faint">vs {{ prevLabel }} ({{ money(fin.previous.net) }})</p>
          </div>
        </div>
        <p class="mt-2 text-xs text-faint">
          Cash received minus recorded expenses, by the date the money moved. Not profit — stock purchases and the cost of goods sold aren't counted separately.
        </p>
      </Card>

      <Card v-if="inTypes.length || outCats.length" class="space-y-4 p-4">
        <div v-if="inTypes.length">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Money in by type</h2>
          <ul class="mt-2 space-y-1">
            <li v-for="[t, v] in inTypes" :key="t">
              <button type="button" class="block min-h-10 w-full rounded-lg px-2 py-1 text-left hover:bg-elevated" @click="filterBy('income', t)">
                <span class="flex justify-between text-sm"><span>{{ payTypeLabel(t) }}</span><span class="tnum">{{ money(v) }}</span></span>
                <span class="mt-1 block h-1.5 rounded-full bg-elevated"><span class="block h-1.5 rounded-full" :class="v < 0 ? 'bg-overdue/70' : 'bg-bronze/70'" :style="{ width: `${share(v, fin.income || 1)}%` }" /></span>
              </button>
            </li>
          </ul>
          <p v-if="methods.length" class="mt-2 px-2 text-xs text-faint">
            By method: <template v-for="([m, v], i) in methods" :key="m">{{ i ? " · " : "" }}{{ methodLabel(m) }} {{ money(v) }}</template>
          </p>
        </div>
        <div v-if="outCats.length">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Money out by category</h2>
          <ul class="mt-2 space-y-1">
            <li v-for="[c, v] in outCats" :key="c">
              <button type="button" class="block min-h-10 w-full rounded-lg px-2 py-1 text-left hover:bg-elevated" @click="filterBy('expense', c)">
                <span class="flex justify-between text-sm"><span>{{ categoryLabel(c) }}</span><span class="tnum">{{ money(v) }}</span></span>
                <span class="mt-1 block h-1.5 rounded-full bg-elevated"><span class="block h-1.5 rounded-full bg-overdue/60" :style="{ width: `${share(v, fin.outgoings || 1)}%` }" /></span>
              </button>
            </li>
          </ul>
        </div>
      </Card>
    </template>

    <!-- Ledger: every row of the period, filtered on the server before paging. -->
    <section id="ledger" class="scroll-mt-20 space-y-2">
      <div class="flex items-center justify-between px-1">
        <h2 class="label-eyebrow text-[0.625rem] text-faint">Ledger · {{ periodLabel }}</h2>
        <div class="flex items-center gap-1">
          <button class="inline-flex min-h-10 items-center gap-1 px-2 text-sm font-medium text-bronze hover:underline" @click="exportStatement"><Download class="h-3.5 w-3.5" /> CSV</button>
          <button :class="btnClasses('primary', 'sm')" @click="showAdd = !showAdd"><Plus class="h-4 w-4" /> {{ showAdd ? "Close" : "Expense" }}</button>
        </div>
      </div>

      <Card v-if="showAdd" class="space-y-3 p-4">
        <div class="grid grid-cols-2 gap-3">
          <label class="block"><span class="mb-1 block text-xs text-faint">Amount</span><input v-model.number="form.amount" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" /></label>
          <label class="block"><span class="mb-1 block text-xs text-faint">Category</span>
            <select v-model="form.category" :class="inputCls">
              <option v-for="(l, k) in CATEGORY_LABEL" :key="k" :value="k">{{ l }}</option>
            </select>
          </label>
        </div>
        <label class="block"><span class="mb-1 block text-xs text-faint">Note</span><input v-model="form.note" placeholder="Optional" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Paid on</span><input v-model="form.day" type="date" :class="inputCls" /></label>
        <p class="text-xs text-faint">Counts toward {{ monthLabel(form.day.slice(0, 7)) }}.</p>
        <Button size="sm" :disabled="busy || form.amount <= 0 || !form.day" @click="addExpense">{{ busy ? "Adding…" : "Add expense" }}</Button>
      </Card>

      <ChipGroup v-model="kind" :options="kindChips" label="Show" />
      <div v-if="typeFilter" class="flex">
        <button class="inline-flex min-h-10 items-center gap-1 rounded-lg border border-bronze/40 bg-bronze/10 px-2.5 text-xs text-bronze" @click="typeFilter = ''">
          {{ typeChipLabel }} <X class="h-3.5 w-3.5" /><span class="sr-only">Remove filter</span>
        </button>
      </div>
      <SearchInput v-model="q" label="Search the ledger" placeholder="Search — name, note, reference…" :matches="ledgerTotal" :searching="ledgerLoading && !!q" />

      <Alert v-if="ledgerError">
        {{ ledgerError }}
        <button class="ml-1 font-medium underline" @click="loadLedger()">Retry</button>
      </Alert>
      <p v-else-if="!ledgerLoading && ledger.length === 0" class="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
        {{ q || typeFilter || kind !== "all" ? "Nothing matches in this period." : "No money moved in this period." }}
      </p>
      <template v-else>
        <p v-if="ledgerTotals" class="px-1 text-xs text-faint tnum">
          {{ ledgerTotal }} row{{ ledgerTotal === 1 ? "" : "s" }} · in {{ money(ledgerTotals.income) }} · out {{ money(ledgerTotals.outgoings) }} · net {{ money(ledgerTotals.net) }} (voided rows shown, not counted)
        </p>
        <ul class="space-y-2">
          <li
            v-for="r in ledger"
            :key="`${r.kind}-${r.id}`"
            class="rounded-xl border bg-surface"
            :class="[r.voided ? 'opacity-60' : '', r.id === focusId ? 'border-bronze ring-2 ring-bronze/30' : 'border-line']"
          >
            <component
              :is="rowHref(r) ? RouterLink : 'button'"
              v-bind="rowHref(r) ? { to: withBack(rowHref(r)!, here) } : { type: 'button', 'aria-expanded': openExpense === r.id }"
              class="flex w-full items-center justify-between gap-3 px-3 py-2.5 text-left"
              @click="!rowHref(r) && toggleExpense(r)"
            >
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">
                  <Highlight :text="r.kind === 'expense' ? categoryLabel(r.detail) : r.detail" :q="q" />
                  <span v-if="r.voided" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                </p>
                <p class="truncate text-xs text-faint">{{ formatLongDate(r.date) }} · <Highlight :text="rowSub(r)" :q="q" /></p>
              </div>
              <span class="flex shrink-0 items-center gap-1">
                <span class="font-display text-sm tnum" :class="[r.voided ? 'line-through text-faint' : r.out > 0 || r.in < 0 ? 'text-overdue' : 'text-fg']">
                  {{ r.out > 0 ? `−${money(r.out)}` : r.in < 0 ? money(r.in) : `+${money(r.in)}` }}
                </span>
                <ChevronRight v-if="rowHref(r)" class="h-4 w-4 text-faint" />
              </span>
            </component>
            <div v-if="r.kind === 'expense' && openExpense === r.id" class="space-y-2 border-t border-line px-3 py-3">
              <p v-if="r.voided" class="text-xs text-faint">Voided<template v-if="r.voidReason"> — “{{ r.voidReason }}”</template>.</p>
              <template v-else-if="editing">
                <div class="grid grid-cols-2 gap-2">
                  <label class="block"><span class="mb-1 block text-xs text-faint">Amount</span><input v-model.number="editForm.amount" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" /></label>
                  <label class="block"><span class="mb-1 block text-xs text-faint">Category</span>
                    <select v-model="editForm.category" :class="inputCls">
                      <option v-for="(l, k) in CATEGORY_LABEL" :key="k" :value="k">{{ l }}</option>
                    </select>
                  </label>
                </div>
                <label class="block"><span class="mb-1 block text-xs text-faint">Note</span><input v-model="editForm.note" :class="inputCls" /></label>
                <label class="block"><span class="mb-1 block text-xs text-faint">Paid on</span><input v-model="editForm.day" type="date" :class="inputCls" /></label>
                <div class="flex gap-2">
                  <Button size="sm" :disabled="savingEdit || editForm.amount <= 0" @click="saveExpense(r)">{{ savingEdit ? "Saving…" : "Save changes" }}</Button>
                  <Button size="sm" variant="ghost" @click="editing = false">Cancel</Button>
                </div>
              </template>
              <div v-else class="flex gap-2">
                <Button size="sm" variant="ghost" @click="editing = true">Edit</Button>
                <button :class="btnClasses('danger', 'sm')" @click="voidExpense(r)">Void</button>
              </div>
              <AuditTrail entity="expense" :id="r.id" />
            </div>
          </li>
        </ul>
        <div v-if="ledgerMore" class="flex justify-center">
          <button :class="btnClasses('ghost', 'sm')" :disabled="ledgerLoading" @click="loadLedger(true)">
            {{ ledgerLoading ? "Loading…" : `Show more (${ledgerTotal - ledger.length} left)` }}
          </button>
        </div>
      </template>
    </section>
  </div>
</template>
