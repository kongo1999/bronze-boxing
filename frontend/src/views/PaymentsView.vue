<script setup lang="ts">
import { ref, computed, watch, nextTick } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { Plus, Receipt, Download, ChevronRight, SlidersHorizontal, FileText } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { readCache, writeCache, invalidate } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { Payment, SubStatus, SubState } from "@/lib/types";
import { money, monthKey, monthLabel, formatLongDate } from "@/lib/format";
import { isMonthKey } from "@/lib/studio";
import { useQueryState, usePatchQuery, withBack } from "@/lib/route-state";
import { STATE_LABEL, STATE_TONE, byUrgency, collectable, collectLink, countDues } from "@/lib/dues";
import { methodLabel, payTypeLabel } from "@/lib/labels";
import { askAmountAndReason } from "@/lib/prompt";
import PageHeader from "@/components/ui/PageHeader.vue";
import StatTile from "@/components/ui/StatTile.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import { fuzzyFilter } from "@/lib/fuzzy";
import Highlight from "@/components/ui/Highlight.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import ChipGroup, { type Chip } from "@/components/ui/ChipGroup.vue";
import { btnClasses } from "@/components/ui/button";
import { toast } from "@/lib/toast";

const route = useRoute();
const patchQuery = usePatchQuery();

// Period, section and dues filter live in the URL, so collecting a payment,
// opening a trainee or reloading lands back on exactly this view.
const month = useQueryState("m", () => monthKey(), isMonthKey);
type Tab = "dues" | "payments";
const tab = useQueryState<Tab>("tab", () => "dues", (v): v is Tab => v === "dues" || v === "payments");
type DuesFilter = "all" | "partial" | "unpaid" | "paid" | "review";
const filter = useQueryState<DuesFilter>("show", () => "all", (v): v is DuesFilter =>
  ["all", "partial", "unpaid", "paid", "review"].includes(v as string));
const here = computed(() => route.fullPath);

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
    else error.value = errMsg(e, "Couldn't load this month.");
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

const collected = computed(() => payments.value.filter((p) => !p.voidedAt).reduce((s, p) => s + p.amount, 0));
const counts = computed(() => countDues(subs.value));

// One search over the month, labelled as such: it narrows the dues roster and
// the payments list together, so "Rami" answers both "did he pay?" and "what
// has he paid?" at once — across the whole month, before paging.
const q = ref("");
const term = computed(() => q.value.trim());
const searchedSubs = computed(() => fuzzyFilter(subs.value, term.value, (s) => [s.trainee.name, s.trainee.phone]));
const FILTER_STATES: Record<DuesFilter, SubState[] | null> = {
  all: null,
  partial: ["partial"],
  unpaid: ["unpaid", "upcoming"],
  paid: ["paid", "waived"],
  review: ["unverified"],
};
const filteredSubs = computed(() => {
  const want = FILTER_STATES[filter.value];
  const rows = searchedSubs.value.filter((s) => !want || want.includes(s.state));
  return term.value ? rows : rows.slice().sort(byUrgency); // a search keeps best-match order
});
const filteredPayments = computed(() =>
  fuzzyFilter(payments.value, term.value, (p) => [
    p.traineeName || payTypeLabel(p.type), p.note, p.reference, payTypeLabel(p.type), p.periodMonth, methodLabel(p.method),
  ]),
);
// Destructured (not kept as objects) so the template reads the refs directly.
const { page: subPage, pageCount: subPages, items: subItems, total: subTotal, from: subFrom, to: subTo } = usePaged(filteredSubs, 8);
const { page: payPage, pageCount: payPages, items: payItems, total: payTotal, from: payFrom, to: payTo } = usePaged(filteredPayments, 10);

// Chips with counts and what's owed, computed over the searched month. The
// Partial chip is always there — even for one person — so nobody half-paid
// hides among the unpaid.
const chips = computed<Chip<DuesFilter>[]>(() => {
  const c = countDues(searchedSubs.value);
  const owed = (st: SubState) => searchedSubs.value.filter((s) => s.state === st).reduce((n, s) => n + s.remaining, 0);
  const out: Chip<DuesFilter>[] = [
    { v: "all", l: "All", n: searchedSubs.value.length },
    { v: "partial", l: "Partial", n: c.partial ? `${c.partial} · ${money(owed("partial"))}` : 0, tone: "partial" },
    { v: "unpaid", l: "Unpaid", n: c.unpaid ? `${c.unpaid} · ${money(owed("unpaid"))}` : searchedSubs.value.filter((s) => s.state === "upcoming").length || 0, tone: "overdue" },
    { v: "paid", l: "Paid", n: c.paid + searchedSubs.value.filter((s) => s.state === "waived").length, tone: "paid" },
  ];
  if (c.unverified) out.push({ v: "review", l: "Needs review", n: c.unverified, tone: "info" });
  return out;
});

// Returning from Collect (?focus=<trainee>): show that trainee's row,
// scrolled into view and briefly highlighted.
const focusId = ref<string>();
watch(
  () => [route.query.focus, subs.value.length] as const,
  async ([f]) => {
    if (typeof f !== "string" || !subs.value.length) return;
    focusId.value = f;
    tab.value = "dues";
    filter.value = "all";
    q.value = "";
    const idx = filteredSubs.value.findIndex((s) => s.trainee.id === f);
    if (idx >= 0) subPage.value = Math.floor(idx / 8) + 1;
    await nextTick();
    const calm = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    document.getElementById(`due-${f}`)?.scrollIntoView({ block: "center", behavior: calm ? "auto" : "smooth" });
    patchQuery({ focus: undefined });
    setTimeout(() => (focusId.value = undefined), 2400);
  },
  { immediate: true },
);

// Owner decisions on a month's dues: every one is a recorded adjustment
// with a reason, never a silent edit.
async function adjust(s: SubStatus, mode: "adjust" | "waive" | "confirm") {
  if (!s.chargeId) return;
  const titles = {
    adjust: `Adjust ${s.trainee.name}'s ${monthLabel(s.periodMonth)} dues`,
    waive: `Waive the rest of ${s.trainee.name}'s ${monthLabel(s.periodMonth)} dues?`,
    confirm: `Confirm ${s.trainee.name}'s ${monthLabel(s.periodMonth)} dues`,
  };
  const messages = {
    adjust: `Currently ${money(s.due)} due, ${money(s.amountPaid)} paid. The change is recorded with your reason.`,
    waive: `${money(s.amountPaid)} paid of ${money(s.due)} — the remaining ${money(s.remaining)} stops being owed.`,
    confirm: `Imported from older records: ${money(s.amountPaid)} was paid, but what was due isn't known. Enter the amount that was actually due.`,
  };
  const res = await askAmountAndReason({
    title: titles[mode],
    message: messages[mode],
    confirmLabel: mode === "waive" ? "Waive rest" : "Save",
    tone: mode === "waive" ? "danger" : "primary",
    suggestions: mode === "waive" ? ["Agreed discount", "Joined late", "Hardship"] : mode === "confirm" ? ["Checked old records"] : ["Agreed discount", "Wrong fee entered"],
    amount: {
      label: "Amount due",
      initial: mode === "waive" ? s.amountPaid : s.due,
      min: s.amountPaid,
      hint: `At least ${money(s.amountPaid)} — what's already been paid.`,
    },
  });
  if (!res) return;
  try {
    await api.post(`/subscription-charges/${s.chargeId}/adjust`, { due: res.amount, reason: res.reason });
    invalidate("dues", "subs", "dashboard", "trainees");
    await load();
    toast("Dues updated.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't adjust the dues."), "error");
  }
}
const openActions = ref<string>();

// Fetch through the api client (carries the session cookie; window.open
// can't), then hand the CSV to the browser as a download.
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
    toast(errMsg(e, "Couldn't export CSV."), "error");
  }
}
const lastPaid = (s: SubStatus) => (s.lastPaymentDate ? `last paid ${formatLongDate(s.lastPaymentDate)}` : "");
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Dues & payments" title="Money">
      <template #action>
        <RouterLink :to="withBack('/payments/new', here, { m: month })" :class="btnClasses('primary', 'sm')"><Plus class="h-4 w-4" /> Log</RouterLink>
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
        <StatTile label="Collected" :value="money(collected)" :sub="`cash received in ${monthLabel(month).split(' ')[0]}`" accent />
        <StatTile
          label="Still owed"
          :value="money(counts.outstanding)"
          :sub="`${counts.paid}/${subs.length} fully paid · ${counts.partial} partial · ${counts.unpaid} unpaid`"
        />
      </div>

      <div class="grid grid-cols-2 gap-1 rounded-xl border border-line bg-elevated p-1" role="tablist" aria-label="Money sections">
        <button
          v-for="t in [{ v: 'dues', l: `Dues · ${monthLabel(month).split(' ')[0]}` }, { v: 'payments', l: `Payments (${payments.length})` }]"
          :key="t.v"
          type="button"
          role="tab"
          :aria-selected="tab === t.v"
          class="min-h-10 rounded-lg text-sm font-medium transition-colors"
          :class="tab === t.v ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
          @click="tab = t.v as Tab"
        >{{ t.l }}</button>
      </div>

      <div>
        <SearchInput
          v-model="q"
          label="Search dues and payments this month"
          placeholder="Search dues & payments — name, note, reference…"
          :matches="tab === 'dues' ? filteredSubs.length : filteredPayments.length"
        />
        <p class="mt-1 px-1 text-[0.6875rem] text-faint">Searches both dues and payments for {{ monthLabel(month) }}.</p>
      </div>

      <!-- Dues: what each trainee owes for this month (the fee period). -->
      <section v-if="tab === 'dues'" class="space-y-2">
        <ChipGroup v-model="filter" :options="chips" label="Show dues" scroll />
        <p v-if="counts.unverified" class="px-1 text-xs text-info">
          {{ counts.unverified }} older month{{ counts.unverified === 1 ? "" : "s" }} imported from past records need{{ counts.unverified === 1 ? "s" : "" }} review — what was due isn't known yet.
        </p>
        <p v-if="filteredSubs.length === 0" class="rounded-2xl border border-dashed border-line px-4 py-6 text-center text-sm text-muted">
          {{ q ? "No one matches that." : filter !== "all" ? "No one in this list for this month." : "No dues this month. Set a monthly fee on a trainee to track dues." }}
        </p>
        <template v-else>
          <ul class="space-y-2">
            <li
              v-for="s in subItems"
              :id="`due-${s.trainee.id}`"
              :key="s.trainee.id"
              class="rounded-xl border bg-surface transition-colors duration-700"
              :class="focusId === s.trainee.id ? 'border-bronze bg-bronze/10' : 'border-line'"
            >
              <div class="flex items-center justify-between gap-2 px-3 py-2.5">
                <RouterLink :to="withBack(`/trainees/${s.trainee.id}`, here)" class="flex min-w-0 flex-1 items-center gap-2.5">
                  <Avatar :name="s.trainee.name" class="h-8 w-8 text-xs" />
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium">
                      <Highlight :text="s.trainee.name" :q="term" />
                      <span v-if="s.trainee.status !== 'active'" class="text-xs font-normal text-faint">· {{ s.trainee.status }}</span>
                    </p>
                    <p class="text-xs text-faint tnum">
                      <span class="text-muted">{{ money(s.amountPaid) }} / {{ money(s.due) }}</span>
                      <template v-if="s.state === 'partial' || s.state === 'unpaid'"> · <span class="font-medium text-fg">{{ money(s.remaining) }} left</span></template>
                      <template v-if="lastPaid(s)"> · {{ lastPaid(s) }}</template>
                    </p>
                  </div>
                </RouterLink>
                <Badge :tone="STATE_TONE[s.state]">{{ STATE_LABEL[s.state] }}</Badge>
                <button
                  v-if="s.chargeId"
                  type="button"
                  class="grid h-10 w-9 shrink-0 place-items-center rounded-lg text-faint hover:bg-elevated hover:text-fg"
                  :aria-label="`More for ${s.trainee.name}`"
                  :aria-expanded="openActions === s.trainee.id"
                  @click="openActions = openActions === s.trainee.id ? undefined : s.trainee.id"
                ><SlidersHorizontal class="h-4 w-4" /></button>
              </div>
              <div v-if="collectable(s) || s.state === 'unverified' || openActions === s.trainee.id" class="flex flex-wrap items-center gap-2 border-t border-line/60 px-3 py-2">
                <RouterLink v-if="collectable(s)" :to="collectLink(s, here)" :class="btnClasses('primary', 'sm')">
                  Collect {{ money(s.remaining) }}
                </RouterLink>
                <button v-if="s.state === 'unverified'" :class="btnClasses('ghost', 'sm')" @click="adjust(s, 'confirm')">Confirm amount due</button>
                <template v-if="openActions === s.trainee.id && s.state !== 'unverified'">
                  <button :class="btnClasses('ghost', 'sm')" @click="adjust(s, 'adjust')">Adjust due</button>
                  <button v-if="s.remaining > 0" :class="btnClasses('ghost', 'sm')" @click="adjust(s, 'waive')">Waive rest</button>
                </template>
                <span v-if="s.source === 'adjusted'" class="ml-auto text-[0.6875rem] text-faint">adjusted</span>
              </div>
            </li>
          </ul>
          <Pagination v-model="subPage" :page-count="subPages" :total="subTotal" :from="subFrom" :to="subTo" label="trainees" />
        </template>
      </section>

      <!-- Payments: money received this month (by cash date). -->
      <section v-else class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Received in {{ monthLabel(month) }}</h2>
          <button class="inline-flex min-h-10 items-center gap-1 px-1 text-sm font-medium text-bronze hover:underline" @click="exportCsv"><Download class="h-3.5 w-3.5" /> CSV</button>
        </div>
        <EmptyState
          v-if="filteredPayments.length === 0"
          :icon="Receipt"
          :title="q ? 'No matching payments' : 'No payments this month'"
          :description="q ? 'Try a different name, note or reference.' : 'Cash you collect will show up here.'"
        />
        <template v-else>
          <ul class="space-y-2">
            <li v-for="p in payItems" :key="p.id">
              <RouterLink
                :to="withBack(`/payments/${p.id}`, here)"
                class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
                :class="p.voidedAt ? 'opacity-60' : ''"
              >
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">
                    <Highlight :text="p.traineeName || '—'" :q="term" />
                    <span v-if="p.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                  </p>
                  <p class="truncate text-xs text-faint">
                    {{ payTypeLabel(p.type) }}{{ p.periodMonth ? ` for ${monthLabel(p.periodMonth)}` : "" }} · {{ formatLongDate(p.date) }} · {{ methodLabel(p.method) }}
                  </p>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <span class="font-display text-sm tnum" :class="p.voidedAt ? 'line-through text-faint' : ''">{{ money(p.amount) }}</span>
                  <ChevronRight class="h-4 w-4 text-faint" />
                </div>
              </RouterLink>
            </li>
          </ul>
          <Pagination v-model="payPage" :page-count="payPages" :total="payTotal" :from="payFrom" :to="payTo" label="payments" />
          <p class="flex items-center gap-1 px-1 text-[0.6875rem] text-faint"><FileText class="h-3 w-3" /> Tap a payment for its receipt, edits and history.</p>
        </template>
      </section>
    </template>
  </div>
</template>
