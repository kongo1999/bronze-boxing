<script setup lang="ts">
// The month in one page: cash in and out, dues for the period (with who
// still owes), trainees, classes, attendance and the shop. Every figure
// comes from one server aggregation shared with the CSV, and links to the
// rows behind it. Printable; any past month by the picker or a direct jump.
import { computed, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { Download, Printer, RefreshCw, TrendingDown, TrendingUp } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import type { MonthlyReport } from "@/lib/types";
import { money, monthLabel, formatDateTime } from "@/lib/format";
import { currentMonth, isMonthKey, mondayOf } from "@/lib/studio";
import { useQueryState, withBack } from "@/lib/route-state";
import { categoryLabel, methodLabel, payTypeLabel } from "@/lib/labels";
import { toast } from "@/lib/toast";
import PageHeader from "@/components/ui/PageHeader.vue";
import MonthPicker from "@/components/ui/MonthPicker.vue";
import Card from "@/components/ui/Card.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import Badge from "@/components/ui/Badge.vue";
import { btnClasses } from "@/components/ui/button";

const route = useRoute();
const here = computed(() => route.fullPath);
const month = useQueryState("m", () => currentMonth(), (v): v is string => isMonthKey(v) && (v as string) <= currentMonth());

const rep = ref<MonthlyReport>();
const loading = ref(true);
const error = ref("");
let token = 0;
async function load() {
  const my = ++token;
  loading.value = !rep.value || rep.value.month !== month.value;
  error.value = "";
  try {
    const r = await api.get<MonthlyReport>(`/reports/monthly?m=${month.value}`);
    if (my === token) rep.value = r;
  } catch (e) {
    if (my === token) error.value = errMsg(e, "Couldn't build the report.");
  } finally {
    if (my === token) loading.value = false;
  }
}
watch(month, load, { immediate: true });

async function exportCSV() {
  try {
    const csv = await api.get<string>(`/reports/monthly/export?m=${month.value}`);
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `report-${month.value}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    toast(errMsg(e, "Couldn't export the report."), "error");
  }
}
function printIt() {
  window.print();
}

const f = computed(() => rep.value?.financial);
const s = computed(() => rep.value?.subscriptions);
const ss = computed(() => rep.value?.sessions);
const at = computed(() => rep.value?.attendance);
const inv = computed(() => rep.value?.inventory);
const entries = (m?: Record<string, number>) => Object.entries(m ?? {}).filter(([, v]) => v !== 0).sort((a, b) => Math.abs(b[1]) - Math.abs(a[1]));
const pctText = (v: number | null | undefined) => (v === null || v === undefined ? "—" : `${v}%`);
const change = (v: number | null | undefined) => (v === null || v === undefined ? "" : `${v > 0 ? "+" : ""}${v}% vs ${monthLabel(prevMonth.value)}`);
const prevMonth = computed(() => {
  const [y, m] = month.value.split("-").map(Number);
  return m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, "0")}`;
});
const collectLink = (p: { traineeId: string; remaining: number }) =>
  withBack("/payments/new", here.value, {
    trainee: p.traineeId,
    type: "subscription",
    periodMonth: month.value,
    amount: String(p.remaining),
    m: month.value,
  });
const weekOfMonth = computed(() => mondayOf(`${month.value}-01`));
</script>

<template>
  <div class="report space-y-4">
    <PageHeader eyebrow="Financials · report" title="Monthly report">
      <template #action>
        <div class="flex gap-2" data-print-hide>
          <button :class="btnClasses('ghost', 'sm')" :disabled="!rep" @click="exportCSV"><Download class="h-4 w-4" /> CSV</button>
          <button :class="btnClasses('primary', 'sm')" :disabled="!rep" @click="printIt"><Printer class="h-4 w-4" /> Print</button>
        </div>
      </template>
    </PageHeader>

    <div data-print-hide><MonthPicker v-model="month" /></div>
    <h2 class="hidden font-display text-xl font-semibold print:block">{{ monthLabel(month) }}</h2>

    <Skeleton v-if="loading" :rows="6" />
    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else-if="rep && f && s && ss && at && inv">
      <p class="flex flex-wrap items-center gap-x-2 text-xs text-faint">
        <span>Generated {{ formatDateTime(rep.generatedAt) }} · {{ rep.timezone }}</span>
        <Badge v-if="rep.partial" tone="info">Month in progress — figures so far</Badge>
        <button class="inline-flex min-h-10 items-center gap-1 font-medium text-bronze hover:underline" data-print-hide @click="load"><RefreshCw class="h-3.5 w-3.5" /> Refresh</button>
      </p>

      <!-- Cash -->
      <Card class="space-y-3 p-4">
        <div class="flex items-baseline justify-between gap-2">
          <h3 class="font-display text-lg font-semibold">Cash</h3>
          <RouterLink :to="withBack(`/financials?m=${month}`, here)" class="min-h-10 content-center text-sm font-medium text-bronze hover:underline" data-print-hide>Statement</RouterLink>
        </div>
        <p class="text-xs text-faint">Money by the day it moved. Matches the Financials statement and its CSV; voided entries never count.</p>
        <div class="grid grid-cols-3 gap-2">
          <div>
            <p class="label-eyebrow text-[0.6rem] text-faint">In</p>
            <p class="font-display text-xl font-semibold tnum">{{ money(f.income) }}</p>
            <p v-if="change(f.incomeChangePct)" class="flex items-center gap-0.5 text-[0.6875rem] text-faint"><TrendingUp v-if="(f.incomeChangePct ?? 0) >= 0" class="h-3 w-3" /><TrendingDown v-else class="h-3 w-3" />{{ change(f.incomeChangePct) }}</p>
          </div>
          <div>
            <p class="label-eyebrow text-[0.6rem] text-faint">Out</p>
            <p class="font-display text-xl font-semibold tnum">{{ money(f.expenses) }}</p>
            <p v-if="change(f.expensesChangePct)" class="text-[0.6875rem] text-faint">{{ change(f.expensesChangePct) }}</p>
          </div>
          <div>
            <p class="label-eyebrow text-[0.6rem] text-faint">Net cash</p>
            <p class="font-display text-xl font-semibold tnum" :class="f.net < 0 ? 'text-overdue' : 'text-paid'">{{ money(f.net) }}</p>
            <p class="text-[0.6875rem] text-faint">{{ monthLabel(prevMonth) }}: {{ money(f.previous.net) }}</p>
          </div>
        </div>
        <dl class="space-y-1 border-t border-line pt-3 text-sm">
          <div class="flex justify-between"><dt class="text-muted">Trainee payments</dt><dd class="tnum">{{ money(f.traineePayments) }}</dd></div>
          <div class="flex justify-between"><dt class="text-muted">Shop sales</dt><dd class="tnum">{{ money(f.shopSales) }}</dd></div>
          <div v-if="f.refunds" class="flex justify-between"><dt class="text-muted">Shop refunds</dt><dd class="tnum text-overdue">−{{ money(f.refunds) }}</dd></div>
        </dl>
        <details class="group text-sm">
          <summary class="min-h-10 cursor-pointer content-center font-medium text-bronze">Breakdown by type, category and method</summary>
          <div class="grid gap-3 pt-2 sm:grid-cols-3">
            <div>
              <p class="label-eyebrow mb-1 text-[0.6rem] text-faint">Income by type</p>
              <RouterLink v-for="[k, v] in entries(f.byType)" :key="k" :to="withBack(`/financials?m=${month}&kind=income&type=${k}`, here)" class="flex min-h-8 justify-between hover:text-bronze">
                <span>{{ payTypeLabel(k) }}</span><span class="tnum">{{ money(v) }}</span>
              </RouterLink>
            </div>
            <div>
              <p class="label-eyebrow mb-1 text-[0.6rem] text-faint">Expenses by category</p>
              <p v-if="!entries(f.byCategory).length" class="text-faint">None</p>
              <RouterLink v-for="[k, v] in entries(f.byCategory)" :key="k" :to="withBack(`/financials?m=${month}&kind=expense&type=${k}`, here)" class="flex min-h-8 justify-between hover:text-bronze">
                <span>{{ categoryLabel(k) }}</span><span class="tnum">{{ money(v) }}</span>
              </RouterLink>
            </div>
            <div>
              <p class="label-eyebrow mb-1 text-[0.6rem] text-faint">Income by method</p>
              <div v-for="[k, v] in entries(f.byMethod)" :key="k" class="flex min-h-8 justify-between"><span>{{ methodLabel(k) }}</span><span class="tnum">{{ money(v) }}</span></div>
            </div>
          </div>
        </details>
      </Card>

      <!-- Dues for the period -->
      <Card class="space-y-3 p-4">
        <div class="flex items-baseline justify-between gap-2">
          <h3 class="font-display text-lg font-semibold">Subscriptions for {{ monthLabel(month) }}</h3>
          <RouterLink :to="withBack(`/payments?m=${month}`, here)" class="min-h-10 content-center text-sm font-medium text-bronze hover:underline" data-print-hide>Money</RouterLink>
        </div>
        <p class="text-xs text-faint">About the fee period: a payment made later for {{ monthLabel(month) }} counts here, and under Cash in the month it was received.</p>
        <div class="grid grid-cols-3 gap-2">
          <div><p class="label-eyebrow text-[0.6rem] text-faint">Billed</p><p class="font-display text-xl font-semibold tnum">{{ money(s.totalBilled) }}</p><p class="text-[0.6875rem] text-faint">{{ s.billed }} trainee{{ s.billed === 1 ? "" : "s" }}</p></div>
          <div><p class="label-eyebrow text-[0.6rem] text-faint">Paid toward it</p><p class="font-display text-xl font-semibold tnum">{{ money(s.paidTowardPeriod) }}</p></div>
          <div><p class="label-eyebrow text-[0.6rem] text-faint">Outstanding</p><p class="font-display text-xl font-semibold tnum" :class="s.outstanding ? 'text-partial' : ''">{{ money(s.outstanding) }}</p></div>
        </div>
        <div class="flex flex-wrap gap-1.5 text-xs">
          <Badge tone="paid">{{ s.paid }} paid</Badge>
          <Badge tone="partial">{{ s.partial }} partial</Badge>
          <Badge tone="overdue">{{ s.unpaid }} unpaid</Badge>
          <Badge v-if="s.waived" tone="neutral">{{ s.waived }} waived</Badge>
          <Badge v-if="s.unverified" tone="info">{{ s.unverified }} unverified (imported, outside these totals)</Badge>
        </div>
        <div v-if="s.partialPayers.length" class="space-y-1">
          <p class="label-eyebrow text-[0.6rem] text-faint">Partly paid — not counted as paid</p>
          <div v-for="p in s.partialPayers" :key="p.traineeId" class="flex items-center gap-2 text-sm">
            <RouterLink :to="withBack(`/trainees/${p.traineeId}`, here)" class="min-w-0 flex-1 truncate hover:underline">{{ p.name }}</RouterLink>
            <span class="shrink-0 text-xs text-faint tnum">{{ money(p.paid) }} of {{ money(p.due) }}</span>
            <RouterLink :to="collectLink(p)" class="min-h-10 shrink-0 content-center px-1 text-sm font-medium text-bronze hover:underline" data-print-hide>Collect {{ money(p.remaining) }}</RouterLink>
            <span class="hidden shrink-0 tnum print:inline">{{ money(p.remaining) }} left</span>
          </div>
        </div>
        <details v-if="s.unpaidPayers.length" class="text-sm">
          <summary class="min-h-10 cursor-pointer content-center font-medium text-bronze">Unpaid · {{ s.unpaidPayers.length }}</summary>
          <div v-for="p in s.unpaidPayers" :key="p.traineeId" class="flex items-center gap-2">
            <RouterLink :to="withBack(`/trainees/${p.traineeId}`, here)" class="min-w-0 flex-1 truncate hover:underline">{{ p.name }}</RouterLink>
            <RouterLink :to="collectLink(p)" class="min-h-10 shrink-0 content-center px-1 font-medium text-bronze hover:underline" data-print-hide>Collect {{ money(p.remaining) }}</RouterLink>
          </div>
        </details>
      </Card>

      <!-- People, classes, attendance -->
      <div class="grid gap-4 md:grid-cols-2">
        <Card class="space-y-2 p-4">
          <h3 class="font-display text-lg font-semibold">Trainees</h3>
          <dl class="space-y-1 text-sm">
            <div class="flex justify-between"><dt class="text-muted">Active at month end</dt><dd class="tnum">{{ rep.trainees.activeAtMonthEnd }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Joined</dt><dd class="tnum">{{ rep.trainees.joined }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Went inactive or archived</dt><dd class="tnum">{{ rep.trainees.inactivated }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Came to at least one class</dt><dd class="tnum">{{ rep.trainees.attended }}</dd></div>
          </dl>
          <p class="text-[0.6875rem] text-faint">Active is read from each trainee's dated membership record, not today's profile.</p>
        </Card>

        <Card class="space-y-2 p-4">
          <div class="flex items-baseline justify-between gap-2">
            <h3 class="font-display text-lg font-semibold">Classes</h3>
            <RouterLink :to="withBack(`/schedule?week=${weekOfMonth}`, here)" class="min-h-10 content-center text-sm font-medium text-bronze hover:underline" data-print-hide>Schedule</RouterLink>
          </div>
          <dl class="space-y-1 text-sm">
            <div class="flex justify-between"><dt class="text-muted">Classes this month</dt><dd class="tnum">{{ ss.total }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Done · scheduled · cancelled</dt><dd class="tnum">{{ ss.completed }} · {{ ss.scheduled }} · {{ ss.cancelled }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Group · private</dt><dd class="tnum">{{ ss.group }} · {{ ss.private }}</dd></div>
            <div class="flex justify-between">
              <dt class="text-muted">Marked done</dt>
              <dd class="tnum">{{ pctText(ss.completionPct) }} <span class="text-xs text-faint">({{ ss.completed }} of {{ ss.started }} begun)</span></dd>
            </div>
            <div v-if="ss.notMarkedDone" class="flex justify-between text-partial"><dt>Begun but not marked done</dt><dd class="tnum">{{ ss.notMarkedDone }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Series started</dt><dd class="tnum">{{ ss.seriesCreated }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Active plans at month end</dt><dd class="tnum">{{ ss.plansActive }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Plan sessions earned · still owed</dt><dd class="tnum">{{ ss.planCredits }} · {{ ss.planRemaining }}</dd></div>
          </dl>
          <details v-if="ss.series.length" class="text-sm">
            <summary class="min-h-10 cursor-pointer content-center font-medium text-bronze">Series · {{ ss.series.length }}</summary>
            <div v-for="se in ss.series" :key="se.seriesId" class="flex min-h-8 justify-between gap-2">
              <span class="min-w-0 truncate">{{ se.title }}</span>
              <span class="shrink-0 tnum">{{ se.completed }}/{{ se.planned || "?" }} <span class="text-xs text-faint">({{ se.inMonth }} this month)</span></span>
            </div>
          </details>
          <p class="text-[0.6875rem] text-faint">Cancelled classes are left out of every other figure. Nothing is assumed done just because its time passed.</p>
        </Card>

        <Card class="space-y-2 p-4">
          <h3 class="font-display text-lg font-semibold">Attendance</h3>
          <dl class="space-y-1 text-sm">
            <div class="flex justify-between"><dt class="text-muted">Attended · no-show</dt><dd class="tnum">{{ at.attended }} · {{ at.noShow }}</dd></div>
            <div class="flex justify-between" :class="at.unmarked ? 'text-partial' : ''"><dt :class="at.unmarked ? '' : 'text-muted'">Not marked yet</dt><dd class="tnum">{{ at.unmarked }}</dd></div>
            <div v-if="at.bookedAhead" class="flex justify-between"><dt class="text-muted">Booked for later this month</dt><dd class="tnum">{{ at.bookedAhead }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">People who came · who missed one</dt><dd class="tnum">{{ at.people }} · {{ at.noShowPeople }}</dd></div>
            <div class="flex justify-between">
              <dt class="text-muted">Attendance rate</dt>
              <dd class="tnum">{{ pctText(at.ratePct) }} <span class="text-xs text-faint">({{ at.attended }} of {{ at.attended + at.noShow }} decided)</span></dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-muted">Occupancy</dt>
              <dd class="tnum">
                <template v-if="at.cappedClasses">{{ pctText(at.occupancyPct) }} <span class="text-xs text-faint">({{ at.cappedBooked }} of {{ at.cappedCapacity }} places, {{ at.cappedClasses }} capped classes)</span></template>
                <span v-else class="text-xs text-faint">no class had a capacity</span>
              </dd>
            </div>
          </dl>
          <p class="text-[0.6875rem] text-faint">The rate counts only decided attendance; unmarked bookings are shown beside it. Occupancy uses only classes with a set capacity.</p>
        </Card>

        <Card class="space-y-2 p-4">
          <div class="flex items-baseline justify-between gap-2">
            <h3 class="font-display text-lg font-semibold">Shop</h3>
            <RouterLink :to="withBack('/inventory', here)" class="min-h-10 content-center text-sm font-medium text-bronze hover:underline" data-print-hide>Inventory</RouterLink>
          </div>
          <dl class="space-y-1 text-sm">
            <div class="flex justify-between"><dt class="text-muted">Units sold</dt><dd class="tnum">{{ inv.unitsSold }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Sales revenue</dt><dd class="tnum">{{ money(inv.salesRevenue) }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Returned · refunded</dt><dd class="tnum">{{ inv.unitsReturned }} · {{ money(inv.refunds) }}</dd></div>
            <div class="flex justify-between"><dt class="text-muted">Low · out of stock <span class="text-[0.6875rem] text-faint">(now, not at month end)</span></dt><dd class="tnum">{{ inv.lowNow }} · {{ inv.outNow }}</dd></div>
          </dl>
          <div v-if="inv.topByUnits.length" class="text-sm">
            <p class="label-eyebrow mb-1 text-[0.6rem] text-faint">Top sellers</p>
            <RouterLink v-for="it in inv.topByUnits" :key="it.itemId" :to="withBack(`/inventory/${it.itemId}`, here)" class="flex min-h-8 justify-between gap-2 hover:text-bronze">
              <span class="min-w-0 truncate">{{ it.name }}</span><span class="shrink-0 tnum">{{ it.units }} · {{ money(it.revenue) }}</span>
            </RouterLink>
          </div>
          <details v-if="inv.stock.length" class="text-sm">
            <summary class="min-h-10 cursor-pointer content-center font-medium text-bronze">Stock at month end</summary>
            <div v-for="st in inv.stock" :key="st.itemId" class="flex min-h-8 justify-between gap-2">
              <span class="min-w-0 truncate">{{ st.name }}</span>
              <span class="shrink-0 tnum">{{ st.atMonthEnd ?? "—" }} <span class="text-xs text-faint">(now {{ st.now }})</span></span>
            </div>
            <p class="pt-1 text-[0.6875rem] text-faint">From the stock history; “—” means tracking began after this month.</p>
          </details>
        </Card>
      </div>
    </template>
  </div>
</template>
