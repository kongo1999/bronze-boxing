<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { RouterLink } from "vue-router";
import { Plus, Receipt, Download, Pencil } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
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
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

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
const paidCount = computed(() => subs.value.filter((s) => s.state === "paid").length);

const toneMap: Record<string, "paid" | "partial" | "overdue"> = { paid: "paid", partial: "partial", unpaid: "overdue" };
const labelMap: Record<string, string> = { paid: "Paid", partial: "Partial", unpaid: "Unpaid" };
const typeLabel: Record<string, string> = { subscription: "Subscription", private: "Private session", dropin: "Drop-in", sale: "Shop sale", other: "Other" };

// Voids, not deletes: the record stays in the books marked VOID and stops
// counting toward revenue and dues.
async function voidPayment(id: string) {
  if (!confirm("Void this payment? It stays in the books marked VOID and stops counting.")) return;
  try {
    await api.del(`/payments/${id}`);
    load();
    toast("Payment voided.", "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't void payment.", "error");
  }
}

// Inline edit (PUT /payments/:id). Carries the existing trainee link through so
// editing amount/type/note doesn't unlink the trainee.
const editingId = ref<string | null>(null);
const savingEdit = ref(false);
const editForm = reactive({ amount: 0, type: "subscription", periodMonth: "", note: "", trainee: undefined as string | undefined });
function openEdit(p: Payment) {
  editingId.value = editingId.value === p.id ? null : p.id;
  Object.assign(editForm, { amount: p.amount, type: p.type, periodMonth: p.periodMonth ?? "", note: p.note ?? "", trainee: p.trainee });
}
async function saveEdit(id: string) {
  if (savingEdit.value || editForm.amount <= 0) return;
  savingEdit.value = true;
  try {
    await api.put(`/payments/${id}`, {
      amount: editForm.amount,
      type: editForm.type,
      periodMonth: editForm.type === "subscription" ? editForm.periodMonth : "",
      note: editForm.note,
      trainee: editForm.trainee || "",
    });
    editingId.value = null;
    await load();
    toast("Payment updated.", "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't update payment.", "error");
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
        <StatTile label="Subscriptions" :value="`${paidCount}/${subs.length}`" :sub="subs.length - paidCount > 0 ? `${subs.length - paidCount} unpaid` : 'all paid'" />
      </div>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Subscriptions · {{ monthLabel(month) }}</h2>
        <p v-if="subs.length === 0" class="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
          No active subscribers. Set a monthly fee on a trainee to track dues.
        </p>
        <ul v-else class="space-y-2">
          <li v-for="s in subs" :key="s.trainee.id" class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5">
            <RouterLink :to="`/trainees/${s.trainee.id}`" class="flex min-w-0 items-center gap-2.5">
              <Avatar :name="s.trainee.name" class="h-8 w-8 text-xs" />
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">{{ s.trainee.name }}</p>
                <p class="text-xs text-faint tnum">{{ money(s.amountPaid) }} / {{ money(s.due) }}</p>
              </div>
            </RouterLink>
            <div class="flex shrink-0 items-center gap-2">
              <Badge :tone="toneMap[s.state]">{{ labelMap[s.state] }}</Badge>
              <RouterLink
                v-if="s.state !== 'paid'"
                :to="`/payments/new?trainee=${s.trainee.id}&type=subscription&periodMonth=${month}&amount=${s.due - s.amountPaid}`"
                :class="btnClasses('primary', 'sm')"
              >Collect</RouterLink>
            </div>
          </li>
        </ul>
      </section>

      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Transactions</h2>
          <button class="inline-flex items-center gap-1 text-sm font-medium text-bronze hover:underline" @click="exportCsv"><Download class="h-3.5 w-3.5" /> CSV</button>
        </div>
        <EmptyState v-if="payments.length === 0" :icon="Receipt" title="No payments this month" description="Cash you collect will show up here." />
        <ul v-else class="space-y-2">
          <li v-for="p in payments" :key="p.id" class="rounded-xl border border-line bg-surface" :class="p.voidedAt ? 'opacity-60' : ''">
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
                  <button class="grid h-7 w-7 place-items-center rounded-lg text-purple transition-colors hover:bg-purple/10" :aria-label="`Edit payment`" @click="openEdit(p)"><Pencil class="h-4 w-4" /></button>
                  <button class="grid h-7 w-7 place-items-center rounded-lg text-lg leading-none text-faint transition-colors hover:bg-overdue/10 hover:text-overdue" aria-label="Void payment" @click="voidPayment(p.id)">×</button>
                </template>
              </div>
            </div>
            <div v-if="editingId === p.id" class="space-y-2 border-t border-line px-3 py-3">
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
              <label v-if="editForm.type === 'subscription'" class="block"><span class="mb-1 block text-xs text-faint">Period month (YYYY-MM)</span><input v-model="editForm.periodMonth" :class="inputCls" placeholder="2026-06" /></label>
              <label class="block"><span class="mb-1 block text-xs text-faint">Note</span><input v-model="editForm.note" :class="inputCls" /></label>
              <div class="flex gap-2">
                <Button size="sm" :disabled="savingEdit || editForm.amount <= 0" @click="saveEdit(p.id)">{{ savingEdit ? "Saving…" : "Save changes" }}</Button>
                <Button size="sm" variant="ghost" @click="editingId = null">Cancel</Button>
              </div>
            </div>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
