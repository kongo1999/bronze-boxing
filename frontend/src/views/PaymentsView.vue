<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { RouterLink } from "vue-router";
import { ChevronLeft, ChevronRight, Plus, Receipt, Download } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
import type { Payment, SubStatus } from "@/lib/types";
import { money, monthKey, monthLabel, shiftMonth, formatLongDate } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { btnClasses } from "@/components/ui/button";

const month = ref(monthKey());
const subs = ref<SubStatus[]>([]);
const payments = ref<Payment[]>([]);

const cacheKey = () => `payments:${month.value}`;
let loadToken = 0;
async function load() {
  const my = ++loadToken;
  const [s, p] = await Promise.all([
    api.get<SubStatus[]>(`/subscriptions?m=${month.value}`),
    api.get<Payment[]>(`/payments?m=${month.value}`),
  ]);
  if (my !== loadToken) return; // ignore stale (out-of-order) responses
  subs.value = s;
  payments.value = p;
  writeCache(cacheKey(), { subs: s, payments: p });
}
function showCached() {
  const hit = readCache<{ subs: SubStatus[]; payments: Payment[] }>(cacheKey());
  if (hit) {
    subs.value = hit.subs;
    payments.value = hit.payments;
  }
}
// On month change/nav: render cache instantly, then revalidate.
watch(month, () => { showCached(); load(); }, { immediate: true });

const revenue = computed(() => payments.value.reduce((s, p) => s + p.amount, 0));
const paidCount = computed(() => subs.value.filter((s) => s.state === "paid").length);
const isCurrent = computed(() => month.value === monthKey());

const toneMap: Record<string, "paid" | "partial" | "overdue"> = { paid: "paid", partial: "partial", unpaid: "overdue" };
const labelMap: Record<string, string> = { paid: "Paid", partial: "Partial", unpaid: "Unpaid" };
const typeLabel: Record<string, string> = { subscription: "Subscription", private: "Private session", dropin: "Drop-in", sale: "Shop sale", other: "Other" };

async function removePayment(id: string) {
  if (!confirm("Delete this payment?")) return;
  await api.del(`/payments/${id}`);
  load();
}
function exportCsv() {
  window.open(`/api/payments/export?m=${month.value}`, "_blank");
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Crew dues & fees" title="Money">
      <template #action>
        <RouterLink to="/payments/new" :class="btnClasses('primary', 'sm')"><Plus class="h-4 w-4" /> Log</RouterLink>
      </template>
    </PageHeader>

    <div class="flex items-center justify-between rounded-2xl border border-line bg-surface p-2">
      <button :class="btnClasses('ghost', 'icon')" aria-label="Previous month" @click="month = shiftMonth(month, -1)"><ChevronLeft class="h-5 w-5" /></button>
      <div class="text-center">
        <p class="font-display font-semibold tracking-tight">{{ monthLabel(month) }}</p>
        <button v-if="!isCurrent" class="text-xs text-bronze" @click="month = monthKey()">Back to this month</button>
        <p v-else class="text-xs text-faint">This month</p>
      </div>
      <button :class="btnClasses('ghost', 'icon')" aria-label="Next month" @click="month = shiftMonth(month, 1)"><ChevronRight class="h-5 w-5" /></button>
    </div>

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
        <button class="inline-flex items-center gap-1 text-sm font-medium text-bronze" @click="exportCsv"><Download class="h-3.5 w-3.5" /> CSV</button>
      </div>
      <EmptyState v-if="payments.length === 0" :icon="Receipt" title="No payments this month" description="Cash you collect will show up here." />
      <ul v-else class="space-y-2">
        <li v-for="p in payments" :key="p.id" class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5">
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{{ p.traineeName || "—" }}</p>
            <p class="truncate text-xs text-faint">{{ typeLabel[p.type] ?? p.type }}{{ p.periodMonth ? ` · ${monthLabel(p.periodMonth)}` : "" }} · {{ formatLongDate(p.date) }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-3">
            <span class="font-display text-sm tnum">{{ money(p.amount) }}</span>
            <button class="text-faint hover:text-overdue" aria-label="Delete" @click="removePayment(p.id)">×</button>
          </div>
        </li>
      </ul>
    </section>
  </div>
</template>
