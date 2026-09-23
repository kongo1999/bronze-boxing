<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { RouterLink } from "vue-router";
import { Plus, Receipt, Download, Pencil } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache, clearCache } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { Payment, SubStatus } from "@/lib/types";
import { money, monthKey, monthLabel, formatLongDate } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import Button from "@/components/ui/Button.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { askReason, VOID_REASONS, CORRECTION_REASONS } from "@/lib/prompt";
import { STATE_LABEL, STATE_TONE, byUrgency, collectable, collectLink, countDues } from "@/lib/dues";
import { errMsg } from "@/lib/api";

const month = ref(monthKey());
const subs = ref<SubStatus[]>([]);
const payments = ref<Payment[]>([]);
const loading = ref(false);
const error = ref<string>();
let loaded = false; // have we ever shown real data? distinguishes empty from not-yet-loaded

const cacheKey = () => `payments:${month.value}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  error.value = undefined;
  try {
    const [s, p] = await Promise.all([
      api.get<SubStatus[]>(`/subscriptions?m=${month.value}`),
      api.get<Payment[]>(`/payments?m=${month.value}`),
    ]);
    if (my !== loadToken) return; // ignore stale (out-of-order) responses
    subs.value = s;
    payments.value = p;
    writeCache(cacheKey(), { subs: s, payments: p });
    loaded = true;
  } catch (e) {
    if (my !== loadToken) return;
    if (loaded) toast("Couldn't refresh — showing saved data.", "error");
    else error.value = e instanceof Error && e.message ? e.message : "Couldn't load this month.";
  } finally {
    if (my === loadToken) loading.value = false;
  }
}
function showCached(): boolean {
  const hit = readCache<{ subs: SubStatus[]; payments: Payment[] }>(cacheKey());
  if (hit) {
    subs.value = hit.subs;
    payments.value = hit.payments;
    loaded = true;
  }
  return !!hit;
}
// On month change/nav: render cache instantly (skeleton only when nothing cached), then revalidate.
watch(month, () => { loading.value = !showCached(); load(); }, { immediate: true });

const revenue = computed(() => payments.value.filter((p) => !p.voidedAt).reduce((s, p) => s + p.amount, 0));
// Fully paid, partial and unpaid are counted separately — a partial account
// is neither paid nor untouched.
const counts = computed(() => countDues(subs.value));
const duesSub = computed(() => {
  const c = counts.value;
  const parts = [];
  if (c.partial) parts.push(`${c.partial} partial`);
  if (c.unpaid) parts.push(`${c.unpaid} unpaid`);
  if (c.unverified) parts.push(`${c.unverified} to review`);
  return parts.length ? parts.join(" · ") : "all paid";
});
const typeLabel: Record<string, string> = { subscription: "Subscription", private: "Private session", dropin: "Drop-in", sale: "Shop sale", other: "Other" };

// One search box over the whole month: it narrows the dues roster and the
// transaction ledger together, so "Rami" answers both "did he pay?" and
// "what has he paid?" at once.
const q = ref("");
const term = computed(() => q.value.trim().toLowerCase());
const filteredSubs = computed(() =>
  (term.value ? subs.value.filter((s) => s.trainee.name.toLowerCase().includes(term.value)) : subs.value)
    .slice()
    .sort(byUrgency),
);
const filteredPayments = computed(() => {
  if (!term.value) return payments.value;
  return payments.value.filter(
    (p) =>
      (p.traineeName ?? "").toLowerCase().includes(term.value) ||
      (p.note ?? "").toLowerCase().includes(term.value) ||
      (typeLabel[p.type] ?? p.type).toLowerCase().includes(term.value) ||
      (p.periodMonth ?? "").includes(term.value),
  );
});
// Destructured (not kept as objects) so the template reads the refs directly.
const { page: subPage, pageCount: subPages, items: subItems, total: subTotal, from: subFrom, to: subTo } = usePaged(filteredSubs, 8);
const { page: payPage, pageCount: payPages, items: payItems, total: payTotal, from: payFrom, to: payTo } = usePaged(filteredPayments, 10);

// Trainee options for the inline editor, drawn from the dues roster plus
// anyone who already appears on a payment this month.
const traineeOptions = computed(() => {
  const byId = new Map<string, string>();
  for (const s of subs.value) byId.set(s.trainee.id, s.trainee.name);
  for (const p of payments.value) if (p.trainee && p.traineeName) byId.set(p.trainee, p.traineeName);
  return [...byId].map(([id, label]) => ({ id, label })).sort((a, b) => a.label.localeCompare(b.label));
});

// Voids, not deletes: the record stays in the books marked VOID and stops
// counting toward revenue and dues.
async function voidPayment(p: Payment) {
  const reason = await askReason({
    title: "Void this payment?",
    message: `${money(p.amount)} from ${p.traineeName || "—"} stays in the books marked VOID and stops counting toward revenue and dues.`,
    confirmLabel: "Void payment",
    suggestions: VOID_REASONS,
  });
  if (reason === null) return;
  try {
    await api.post(`/payments/${p.id}/void`, { reason });
    clearCache(); // trainee pages hold their own copy of the payment list
    load();
    toast("Payment voided.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't void payment."), "error");
  }
}

// Inline edit (PUT /payments/:id). Carries the existing trainee link through so
// editing amount/type/note doesn't unlink the trainee.
const editingId = ref<string | null>(null);
const savingEdit = ref(false);
const editForm = reactive({ amount: 0, type: "subscription", periodMonth: "", note: "", trainee: "" });
function openEdit(p: Payment) {
  editingId.value = editingId.value === p.id ? null : p.id;
  Object.assign(editForm, { amount: p.amount, type: p.type, periodMonth: p.periodMonth ?? "", note: p.note ?? "", trainee: p.trainee ?? "" });
}
async function saveEdit(id: string) {
  if (savingEdit.value || editForm.amount <= 0) return;
  // Changing an amount is a correction of the books: say why.
  const original = payments.value.find((p) => p.id === id);
  let reason = "";
  if (original && Math.round(original.amount * 100) !== Math.round(editForm.amount * 100)) {
    const r = await askReason({
      title: "Correct the amount?",
      message: `${money(original.amount)} → ${money(editForm.amount)}. The change is kept in this payment's history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: CORRECTION_REASONS,
    });
    if (r === null) return;
    reason = r;
  }
  savingEdit.value = true;
  try {
    await api.put(`/payments/${id}`, {
      amount: editForm.amount,
      type: editForm.type,
      periodMonth: editForm.type === "subscription" ? editForm.periodMonth : "",
      note: editForm.note,
      trainee: editForm.trainee || "",
      reason,
    });
    editingId.value = null;
    clearCache();
    await load();
    toast("Payment updated.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't update payment."), "error");
  } finally {
    savingEdit.value = false;
  }
}
// Fetch through the api client (carries the auth header — window.open can't),
// then hand the CSV to the browser as a download.
async function exportCsv() {
  try {
    const csv = await api.get<string>(`/payments/export?m=${month.value}`);
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `payments-${month.value}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't export CSV.", "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Crew dues & fees" title="Money">
      <template #action>
        <RouterLink to="/payments/new" :class="btnClasses('primary', 'sm')"><Plus class="h-4 w-4" /> Log</RouterLink>
      </template>
    </PageHeader>

    <MonthPicker v-model="month" />

    <div v-if="loading" class="space-y-3">
      <Skeleton variant="stats" />
      <Skeleton :rows="4" />
    </div>

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else>
      <div class="grid grid-cols-2 gap-3">
        <StatTile label="Collected" :value="money(revenue)" sub="fees this month" accent />
        <StatTile label="Fully paid" :value="`${counts.paid}/${subs.length}`" :sub="duesSub" />
      </div>

      <SearchInput v-model="q" placeholder="Search a name, note or type…" />

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Subscriptions · {{ monthLabel(month) }}</h2>
        <p v-if="filteredSubs.length === 0" class="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
          {{ q ? "No subscriber matches that." : "No active subscribers. Set a monthly fee on a trainee to track dues." }}
        </p>
        <template v-else>
          <ul class="space-y-2">
            <li v-for="s in subItems" :key="s.trainee.id" class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5">
              <RouterLink :to="`/trainees/${s.trainee.id}`" class="flex min-w-0 items-center gap-2.5">
                <Avatar :name="s.trainee.name" class="h-8 w-8 text-xs" />
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">{{ s.trainee.name }}</p>
                  <p class="text-xs text-faint tnum">
                    {{ money(s.amountPaid) }} / {{ money(s.due) }}<template v-if="s.state === 'partial' || s.state === 'unpaid'"> · <span class="text-muted">{{ money(s.remaining) }} left</span></template>
                  </p>
                </div>
              </RouterLink>
              <div class="flex shrink-0 items-center gap-2">
                <Badge :tone="STATE_TONE[s.state]">{{ STATE_LABEL[s.state] }}</Badge>
                <RouterLink
                  v-if="collectable(s)"
                  :to="collectLink(s)"
                  :class="btnClasses('primary', 'sm')"
                >Collect</RouterLink>
              </div>
            </li>
          </ul>
          <Pagination v-model="subPage" :page-count="subPages" :total="subTotal" :from="subFrom" :to="subTo" label="subscribers" />
        </template>
      </section>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Transactions</h2>
          <button class="inline-flex items-center gap-1 text-sm font-medium text-bronze hover:underline" @click="exportCsv"><Download class="h-3.5 w-3.5" /> CSV</button>
        </div>
        <EmptyState
          v-if="filteredPayments.length === 0"
          :icon="Receipt"
          :title="q ? 'No matching payments' : 'No payments this month'"
          :description="q ? 'Try a different name or note.' : 'Cash you collect will show up here.'"
        />
        <template v-else>
          <ul class="space-y-2">
            <li v-for="p in payItems" :key="p.id" class="rounded-xl border border-line bg-surface" :class="p.voidedAt ? 'opacity-60' : ''">
              <div class="flex items-center justify-between gap-3 px-3 py-2.5">
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">
                    {{ p.traineeName || "—" }}
                    <span v-if="p.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                  </p>
                  <p class="truncate text-xs text-faint">{{ typeLabel[p.type] ?? p.type }}{{ p.periodMonth ? ` · ${monthLabel(p.periodMonth)}` : "" }} · {{ formatLongDate(p.date) }}</p>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <span class="font-display text-sm tnum" :class="p.voidedAt ? 'line-through text-faint' : ''">{{ money(p.amount) }}</span>
                  <template v-if="!p.voidedAt">
                    <button class="grid h-7 w-7 place-items-center rounded-lg text-purple transition-colors hover:bg-purple/10" aria-label="Edit payment" @click="openEdit(p)"><Pencil class="h-4 w-4" /></button>
                    <button class="grid h-7 w-7 place-items-center rounded-lg text-lg leading-none text-faint transition-colors hover:bg-overdue/10 hover:text-overdue" aria-label="Void payment" @click="voidPayment(p)">×</button>
                  </template>
                </div>
              </div>
              <div v-if="editingId === p.id" class="space-y-2 border-t border-line px-3 py-3">
                <label class="block"><span class="mb-1 block text-xs text-faint">Trainee</span>
                  <SearchSelect v-model="editForm.trainee" :options="traineeOptions" empty-label="— (none)" placeholder="— (none)" search-placeholder="Search trainees…" />
                </label>
                <div class="grid grid-cols-2 gap-2">
                  <label class="block"><span class="mb-1 block text-xs text-faint">Amount</span><input v-model.number="editForm.amount" type="number" min="0" :class="inputCls" /></label>
                  <label class="block"><span class="mb-1 block text-xs text-faint">Type</span>
                    <select v-model="editForm.type" :class="inputCls">
                      <option value="subscription">Subscription</option>
                      <option value="private">Private session</option>
                      <option value="dropin">Drop-in</option>
                      <option value="other">Other</option>
                    </select>
                  </label>
                </div>
                <label v-if="editForm.type === 'subscription'" class="block"><span class="mb-1 block text-xs text-faint">Period month</span><input v-model="editForm.periodMonth" type="month" :class="inputCls" /></label>
                <label class="block"><span class="mb-1 block text-xs text-faint">Note</span><input v-model="editForm.note" :class="inputCls" /></label>
                <div class="flex gap-2">
                  <Button size="sm" :disabled="savingEdit || editForm.amount <= 0" @click="saveEdit(p.id)">{{ savingEdit ? "Saving…" : "Save changes" }}</Button>
                  <Button size="sm" variant="ghost" @click="editingId = null">Cancel</Button>
                </div>
              </div>
              <div class="border-t border-line px-3 py-2">
                <AuditTrail entity="payment" :id="p.id" />
              </div>
            </li>
          </ul>
          <Pagination v-model="payPage" :page-count="payPages" :total="payTotal" :from="payFrom" :to="payTo" label="payments" />
        </template>
      </section>
    </template>
  </div>
</template>
