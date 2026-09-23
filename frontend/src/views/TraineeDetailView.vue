<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import {
  ChevronLeft, Pencil, Phone, MessageCircle, Wallet, CalendarPlus, Archive, ArchiveRestore, Trash2, Plus, FileText, ChevronRight,
} from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { useCachedAsync, invalidate } from "@/lib/cache";
import type { Trainee, Payment, SubStatus, Charge, Session, Sale, SessionPlan, Page } from "@/lib/types";
import { money, formatLongDate, formatTime, monthKey, monthLabel } from "@/lib/format";
import { addDays, dayOf, formatDay, todayKey } from "@/lib/studio";
import { backTarget, withBack } from "@/lib/route-state";
import { STATE_LABEL, STATE_TONE, collectable, collectLink } from "@/lib/dues";
import { methodLabel, payTypeLabel } from "@/lib/labels";
import { telHref, whatsappHref } from "@/lib/phone";
import { askReason } from "@/lib/prompt";
import Card from "@/components/ui/Card.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Badge from "@/components/ui/Badge.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import Button from "@/components/ui/Button.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import PlanCard from "@/components/PlanCard.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Highlight from "@/components/ui/Highlight.vue";
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;
const here = computed(() => route.fullPath);
const back = computed(() => backTarget(route.query, "/trainees"));

const { data: trainee, loading, error, reload } = useCachedAsync(`trainees:${id}`, () => api.get<Trainee>(`/trainees/${id}`));
const call = computed(() => telHref(trainee.value?.phone));
const whatsapp = computed(() => whatsappHref(trainee.value?.phone));

// ── Session plans ─────────────────────────────────────────────────────────
const plans = ref<SessionPlan[]>([]);
const plansError = ref<string>();
async function loadPlans() {
  plansError.value = undefined;
  try {
    plans.value = await api.get<SessionPlan[]>(`/trainees/${id}/session-plans`);
  } catch (e) {
    plansError.value = errMsg(e, "Couldn't load session plans.");
  }
}
loadPlans();
const activePlans = computed(() => plans.value.filter((p) => p.status === "active"));
const pastPlans = computed(() => plans.value.filter((p) => p.status !== "active"));
const showPastPlans = ref(false);

const newPlan = ref(false);
const planForm = reactive({ title: "", targetCount: 12, startDate: todayKey(), endDate: "", sessionType: "" });
const planErr = ref<Record<string, string>>({});
async function createPlan() {
  planErr.value = {};
  try {
    await api.post(`/trainees/${id}/session-plans`, { ...planForm, title: planForm.title || `${planForm.targetCount} sessions` });
    newPlan.value = false;
    Object.assign(planForm, { title: "", targetCount: 12, startDate: todayKey(), endDate: "", sessionType: "" });
    loadPlans();
    toast("Plan created.", "success");
  } catch (e) {
    if (isApiError(e) && e.field) planErr.value = { [e.field]: e.message };
    else toast(errMsg(e, "Couldn't create the plan."), "error");
  }
}
async function closePlan(p: SessionPlan, status: "completed" | "cancelled" | "active") {
  const reason = await askReason({
    title: status === "active" ? `Reopen “${p.title}”?` : `Mark “${p.title}” ${status}?`,
    message: status === "active" ? "It counts bookings again." : "Its history stays; it stops taking new bookings.",
    confirmLabel: status === "active" ? "Reopen" : status === "completed" ? "Mark completed" : "Cancel plan",
    tone: status === "cancelled" ? "danger" : "primary",
    suggestions: status === "completed" ? ["All sessions used"] : status === "cancelled" ? ["Refunded", "Stopped training"] : ["Continuing"],
  });
  if (reason === null) return;
  try {
    await api.patch(`/session-plans/${p.id}`, { status, reason });
    loadPlans();
  } catch (e) {
    toast(errMsg(e, "Couldn't update the plan."), "error");
  }
}

// Assign sessions: this trainee's bookings in the plan's window that don't
// count toward any plan yet — tick to credit them.
const assigning = ref<string>();
const candidates = ref<Session[]>([]);
const picked = reactive<Record<string, boolean>>({});
async function openAssign(p: SessionPlan) {
  if (assigning.value === p.id) return (assigning.value = undefined);
  assigning.value = p.id;
  candidates.value = [];
  try {
    const q = new URLSearchParams({ trainee: id, from: p.startDate });
    if (p.endDate) q.set("to", addDays(p.endDate, 1)); // inclusive end → exclusive bound
    const list = await api.get<Session[]>(`/sessions?${q.toString()}`);
    candidates.value = list.filter((s) => {
      const a = s.attendees.find((x) => x.trainee === id);
      return a && !a.planId && s.status !== "cancelled" && a.status !== "no_show" && (!p.sessionType || s.type === p.sessionType) && (!p.endDate || dayOf(s.start) <= p.endDate);
    });
    for (const s of candidates.value) picked[s.id] = true;
  } catch (e) {
    toast(errMsg(e, "Couldn't load sessions to assign."), "error");
  }
}
async function assign(p: SessionPlan) {
  const chosen = candidates.value.filter((s) => picked[s.id]);
  let override = false;
  let done = 0;
  for (const s of chosen) {
    const a = s.attendees.find((x) => x.trainee === id)!;
    try {
      await api.patch(`/sessions/${s.id}/attendance`, { trainee: id, status: a.status, planId: p.id, override });
      done++;
    } catch (e) {
      if (isApiError(e, "PLAN_FULL") && !override && confirm(`${e.message}`)) {
        override = true;
        await api.patch(`/sessions/${s.id}/attendance`, { trainee: id, status: a.status, planId: p.id, override }).then(() => done++).catch(() => {});
        continue;
      }
      toast(errMsg(e, "Couldn't assign a session."), "error");
      break;
    }
  }
  assigning.value = undefined;
  invalidate("sessions");
  loadPlans();
  loadSessions();
  if (done) toast(`Assigned ${done} session${done === 1 ? "" : "s"} to ${p.title}.`, "success");
}

// ── Dues, by month ────────────────────────────────────────────────────────
const duesMonth = ref(monthKey());
const charges = ref<Charge[]>([]);
async function loadCharges() {
  try {
    charges.value = await api.get<Charge[]>(`/subscription-charges?trainee=${id}&limit=24`);
  } catch {
    /* the current month still loads below */
  }
}
loadCharges();
const monthChips = computed(() => {
  const months = new Set([monthKey(), ...charges.value.map((c) => c.periodMonth)]);
  return [...months].sort().reverse().slice(0, 6).map((m) => {
    const c = charges.value.find((x) => x.periodMonth === m);
    return { v: m, l: monthLabel(m).replace(/ \d{4}$/, (y) => (m.slice(0, 4) === monthKey().slice(0, 4) ? "" : y)), n: c && c.state !== "paid" && c.remaining > 0 ? money(c.remaining) : "", tone: c?.state === "partial" ? "partial" as const : "overdue" as const };
  });
});
const dues = ref<SubStatus | null>(null);
const duesPayments = ref<Payment[]>([]);
let duesToken = 0;
watch(
  duesMonth,
  async (m) => {
    const my = ++duesToken;
    try {
      const [subs, pays] = await Promise.all([
        api.get<SubStatus[]>(`/subscriptions?m=${m}`),
        api.get<Payment[]>(`/payments?trainee=${id}&periodMonth=${m}&type=subscription`),
      ]);
      if (my !== duesToken) return;
      dues.value = subs.find((s) => s.trainee.id === id) ?? null;
      duesPayments.value = pays;
    } catch {
      if (my === duesToken) dues.value = null;
    }
  },
  { immediate: true },
);

// ── Sessions: upcoming + attendance history ───────────────────────────────
// One search over this trainee's history (attendance, payments, purchases),
// run by the API over everything they have — not just the rows on screen.
const histQ = ref("");
const hq = () => (histQ.value.trim() ? `&q=${encodeURIComponent(histQ.value.trim())}` : "");
let histTimer: ReturnType<typeof setTimeout> | undefined;
watch(histQ, () => {
  clearTimeout(histTimer);
  histTimer = setTimeout(() => {
    loadSessions();
    loadPayments();
    loadPurchases();
  }, 200);
});

const upcoming = ref<Session[]>([]);
const history = ref<Session[]>([]);
const historyTotal = ref(0);
const historyMore = ref(false);
let sessToken = 0;
async function loadSessions(append = false) {
  const now = new Date().toISOString();
  const my = ++sessToken;
  try {
    const [up, past] = await Promise.all([
      append ? Promise.resolve(upcoming.value) : api.get<Session[]>(`/sessions?trainee=${id}&from=${encodeURIComponent(now)}`),
      api.get<Page<Session>>(`/sessions?trainee=${id}&to=${encodeURIComponent(now)}&order=desc&limit=10&offset=${append ? history.value.length : 0}${hq()}`),
    ]);
    if (my !== sessToken) return;
    upcoming.value = up.filter((s) => s.status !== "cancelled").slice(0, 5);
    history.value = append ? [...history.value, ...past.items] : past.items;
    historyTotal.value = past.total;
    historyMore.value = past.hasMore;
  } catch {
    /* sections show their own empty state */
  }
}
loadSessions();
const myStatus = (s: Session) => s.attendees.find((a) => a.trainee === id)?.status;
const attended = computed(() => history.value.filter((s) => s.status === "completed" && myStatus(s) === "attended").length);
const noShows = computed(() => history.value.filter((s) => myStatus(s) === "no_show").length);

// ── Payments (paged on the server) and purchases ──────────────────────────
const payments = ref<Payment[]>([]);
const paymentsTotal = ref(0);
const paymentsMore = ref(false);
let payToken = 0;
async function loadPayments(append = false) {
  const my = ++payToken;
  try {
    const pg = await api.get<Page<Payment>>(`/payments?trainee=${id}&limit=8&offset=${append ? payments.value.length : 0}${hq()}`);
    if (my !== payToken) return;
    payments.value = append ? [...payments.value, ...pg.items] : pg.items;
    paymentsTotal.value = pg.total;
    paymentsMore.value = pg.hasMore;
  } catch {
    /* empty state below */
  }
}
loadPayments();
const purchases = ref<Sale[]>([]);
const purchasesTotal = ref(0);
let buyToken = 0;
function loadPurchases() {
  const my = ++buyToken;
  api
    .get<Page<Sale>>(`/sales?trainee=${id}&limit=10${hq()}`)
    .then((pg) => {
      if (my !== buyToken) return;
      purchases.value = pg.items;
      purchasesTotal.value = pg.total;
    })
    .catch(() => {});
}
loadPurchases();

// ── Archive / delete ──────────────────────────────────────────────────────
const busy = ref(false);
async function archive() {
  const t = trainee.value;
  if (!t || busy.value) return;
  const reason = await askReason({
    title: `Archive ${t.name}?`,
    message: "They leave the roster and stop being billed from next month (this month's dues stand). Payments, attendance and purchases stay on record.",
    confirmLabel: "Archive",
    required: false,
    suggestions: ["Stopped training", "Moved away"],
  });
  if (reason === null) return;
  busy.value = true;
  try {
    await api.post(`/trainees/${id}/archive`, { reason });
    invalidate("trainees", "dues", "subs");
    await reload();
    toast(`${t.name} archived.`, "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't archive."), "error");
  } finally {
    busy.value = false;
  }
}
async function unarchive() {
  busy.value = true;
  try {
    await api.post(`/trainees/${id}/unarchive`);
    invalidate("trainees");
    await reload();
  } catch (e) {
    toast(errMsg(e, "Couldn't restore."), "error");
  } finally {
    busy.value = false;
  }
}
async function remove() {
  const t = trainee.value;
  if (!t || busy.value) return;
  busy.value = true;
  try {
    const l = await api.get<Record<string, number>>(`/trainees/${id}/links`);
    const linked = Object.entries(l).filter(([, n]) => n > 0).map(([k, n]) => `${n} ${k}`);
    let confirmName = "";
    if (linked.length) {
      const typed = window.prompt(
        `${t.name} is on ${linked.join(", ")}. Archiving keeps all of that — deleting removes the trainee (records keep the name they were saved with).\n\nType their full name to delete anyway:`,
      );
      if (typed === null) return;
      confirmName = typed;
    } else if (!confirm(`Delete ${t.name}? Nothing is recorded against them yet.`)) {
      return;
    }
    await api.del(`/trainees/${id}?confirmName=${encodeURIComponent(confirmName)}`);
    invalidate("trainees", "dues", "subs");
    router.push("/trainees");
  } catch (e) {
    toast(errMsg(e, "Couldn't delete."), "error");
  } finally {
    busy.value = false;
  }
}
const attendanceTone = (st?: string) => (st === "attended" ? "paid" : st === "no_show" ? "overdue" : "neutral") as "paid" | "overdue" | "neutral";
const attendanceLabel = (st?: string) => (st === "attended" ? "Attended" : st === "no_show" ? "No-show" : "Booked");
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Crew
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="reload">Retry</button>
    </Alert>

    <template v-else-if="trainee">
      <Card class="p-4">
        <div class="flex items-center gap-3">
          <Avatar :name="trainee.name" class="h-14 w-14 text-lg" />
          <div class="min-w-0 flex-1">
            <h1 class="font-display text-xl font-semibold">{{ trainee.name }}</h1>
            <p class="flex flex-wrap items-center gap-1.5 text-sm text-muted">
              <span v-if="trainee.skillLevel" class="capitalize">{{ trainee.skillLevel }}</span>
              <Badge v-if="trainee.archivedAt" tone="neutral">Archived</Badge>
              <Badge v-else-if="trainee.status === 'inactive'" tone="neutral">Inactive</Badge>
              <span v-if="trainee.monthlyFee > 0" class="tnum">· {{ money(trainee.monthlyFee) }}/mo<template v-if="trainee.feeFromMonth && trainee.feeFromMonth > monthKey()"> from {{ monthLabel(trainee.feeFromMonth) }}</template></span>
            </p>
          </div>
        </div>
        <div v-if="trainee.phone" class="mt-3 flex flex-wrap items-center gap-2">
          <a v-if="call" :href="call" :class="btnClasses('ghost', 'sm')"><Phone class="h-4 w-4 text-bronze" /> {{ trainee.phone }}</a>
          <span v-else class="text-sm text-muted">{{ trainee.phone }}</span>
          <a v-if="whatsapp" :href="whatsapp" target="_blank" rel="noopener" :class="btnClasses('ghost', 'sm')"><MessageCircle class="h-4 w-4 text-paid" /> WhatsApp</a>
        </div>
        <p v-if="trainee.notes" class="mt-3 rounded-xl bg-elevated px-3 py-2 text-sm text-muted">{{ trainee.notes }}</p>
        <div class="mt-4 flex flex-wrap gap-2">
          <RouterLink :to="withBack('/schedule/new', here, { attendee: id, day: todayKey() })" :class="btnClasses('primary', 'sm')"><CalendarPlus class="h-4 w-4" /> Book session</RouterLink>
          <RouterLink :to="dues ? collectLink(dues, here) : withBack('/payments/new', here, { trainee: id })" :class="btnClasses('ghost', 'sm')"><Wallet class="h-4 w-4" /> Collect</RouterLink>
          <RouterLink :to="withBack(`/trainees/${id}/edit`, here)" :class="btnClasses('ghost', 'sm')"><Pencil class="h-4 w-4" /> Edit</RouterLink>
          <button v-if="!trainee.archivedAt" :class="btnClasses('ghost', 'sm')" :disabled="busy" @click="archive"><Archive class="h-4 w-4" /> Archive</button>
          <button v-else :class="btnClasses('ghost', 'sm')" :disabled="busy" @click="unarchive"><ArchiveRestore class="h-4 w-4" /> Restore</button>
          <button class="inline-flex min-h-10 items-center gap-1 px-2 text-xs text-faint hover:text-overdue" :disabled="busy" @click="remove"><Trash2 class="h-3.5 w-3.5" /> Delete</button>
        </div>
      </Card>

      <!-- Session plans: the trainee's own attended / target counters. -->
      <section class="space-y-2">
        <div class="flex items-center justify-between px-1">
          <h2 class="label-eyebrow text-[0.625rem] text-faint">Session plans</h2>
          <button class="inline-flex min-h-10 items-center gap-1 px-1 text-sm font-medium text-bronze hover:underline" @click="newPlan = !newPlan"><Plus class="h-4 w-4" /> {{ newPlan ? "Close" : "New plan" }}</button>
        </div>
        <Card v-if="newPlan" class="space-y-3 p-4">
          <div class="grid grid-cols-2 gap-2">
            <label class="block"><span class="mb-1 block text-xs text-faint">Sessions</span>
              <input v-model.number="planForm.targetCount" type="number" min="1" max="500" inputmode="numeric" :class="inputCls" :aria-invalid="!!planErr.targetCount" />
            </label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Name <span class="text-faint/70">(optional)</span></span>
              <input v-model="planForm.title" :placeholder="`${planForm.targetCount} sessions`" :class="inputCls" />
            </label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Starts</span><input v-model="planForm.startDate" type="date" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Ends <span class="text-faint/70">(optional)</span></span><input v-model="planForm.endDate" type="date" :class="inputCls" /></label>
          </div>
          <div>
            <span class="mb-1 block text-xs text-faint">Counts</span>
            <ChipGroup v-model="planForm.sessionType" :options="[{ v: '', l: 'Any session' }, { v: 'private', l: 'Private only' }, { v: 'group', l: 'Group only' }]" label="Which sessions count" />
          </div>
          <p v-for="(m, k) in planErr" :key="k" class="text-xs text-overdue">{{ m }}</p>
          <Button size="sm" @click="createPlan">Create plan</Button>
        </Card>
        <Alert v-if="plansError">{{ plansError }}</Alert>
        <p v-else-if="!activePlans.length && !newPlan" class="rounded-2xl border border-dashed border-line px-4 py-4 text-center text-sm text-muted">
          No active plan. Create one to count sessions against an allowance, e.g. 12 private sessions.
        </p>
        <Card v-for="p in activePlans" :key="p.id" class="space-y-3 p-4">
          <PlanCard :plan="p" />
          <div class="flex flex-wrap gap-2">
            <button :class="btnClasses('ghost', 'sm')" :aria-expanded="assigning === p.id" @click="openAssign(p)">Assign sessions</button>
            <RouterLink :to="withBack('/schedule/new', here, { attendee: id, plan: p.id, day: todayKey() })" :class="btnClasses('ghost', 'sm')">Book toward it</RouterLink>
            <button class="min-h-10 px-2 text-xs text-faint hover:text-fg" @click="closePlan(p, 'completed')">Mark completed</button>
          </div>
          <div v-if="assigning === p.id" class="space-y-2 rounded-xl bg-elevated p-3">
            <p v-if="!candidates.length" class="text-xs text-faint">No unassigned bookings of {{ trainee.name }} fall in this plan's dates.</p>
            <template v-else>
              <p class="text-xs text-faint">Their bookings not yet counted toward any plan:</p>
              <label v-for="s in candidates" :key="s.id" class="flex min-h-10 items-center gap-3 rounded-lg px-1 text-sm">
                <input v-model="picked[s.id]" type="checkbox" class="h-5 w-5" />
                <span class="min-w-0 flex-1 truncate">{{ formatDay(dayOf(s.start), { weekday: "short", month: "short", day: "numeric" }) }} · {{ formatTime(s.start) }} · {{ s.title }}</span>
                <span class="shrink-0 text-xs text-faint">{{ s.status === "completed" ? attendanceLabel(myStatus(s)) : "upcoming" }}</span>
              </label>
              <Button size="sm" @click="assign(p)">Assign {{ candidates.filter((s) => picked[s.id]).length }}</Button>
            </template>
          </div>
        </Card>
        <button v-if="pastPlans.length" class="min-h-10 px-1 text-xs text-faint hover:text-fg" @click="showPastPlans = !showPastPlans">
          {{ showPastPlans ? "Hide" : "Show" }} {{ pastPlans.length }} past plan{{ pastPlans.length === 1 ? "" : "s" }}
        </button>
        <Card v-for="p in showPastPlans ? pastPlans : []" :key="p.id" class="space-y-2 p-4 opacity-80">
          <PlanCard :plan="p" />
          <button class="min-h-10 px-1 text-xs text-faint hover:text-fg" @click="closePlan(p, 'active')">Reopen</button>
        </Card>
      </section>

      <!-- Dues: any recent month, with its payments and receipts. -->
      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Dues</h2>
        <ChipGroup v-model="duesMonth" :options="monthChips" label="Dues month" scroll />
        <Card class="p-4">
          <div class="flex items-center justify-between gap-3">
            <div class="min-w-0">
              <p class="label-eyebrow text-[0.6rem] text-faint">{{ monthLabel(duesMonth) }}</p>
              <p v-if="dues" class="mt-0.5 font-display text-lg tnum">{{ money(dues.amountPaid) }} <span class="text-faint">/ {{ money(dues.due) }}</span></p>
              <p v-else class="mt-0.5 text-sm text-muted">No dues for this month.</p>
              <p v-if="dues && dues.remaining > 0 && dues.state !== 'unverified'" class="text-xs text-faint">{{ money(dues.remaining) }} still owed</p>
              <p v-if="dues?.state === 'unverified'" class="text-xs text-faint">Imported from older records — confirm the amount due on the Money page.</p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <Badge v-if="dues" :tone="STATE_TONE[dues.state]">{{ STATE_LABEL[dues.state] }}</Badge>
              <RouterLink v-if="dues && collectable(dues)" :to="collectLink(dues, here)" :class="btnClasses('primary', 'sm')">Collect</RouterLink>
            </div>
          </div>
          <ul v-if="duesPayments.length" class="mt-3 space-y-1 border-t border-line pt-3">
            <li v-for="p in duesPayments" :key="p.id">
              <RouterLink :to="withBack(`/payments/${p.id}/receipt`, here)" class="flex min-h-10 items-center justify-between gap-2 rounded-lg px-1 text-sm hover:bg-elevated" :class="p.voidedAt ? 'line-through opacity-60' : ''">
                <span class="text-muted">{{ formatLongDate(p.date) }} · {{ methodLabel(p.method) }}</span>
                <span class="flex items-center gap-1 tnum">{{ money(p.amount) }} <FileText class="h-3.5 w-3.5 text-faint" /></span>
              </RouterLink>
            </li>
          </ul>
        </Card>
      </section>

      <!-- Coming up -->
      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Coming up</h2>
        <p v-if="!upcoming.length" class="px-1 text-sm text-faint">Nothing booked.</p>
        <RouterLink
          v-for="s in upcoming"
          :key="s.id"
          :to="withBack(`/schedule/${s.id}`, here)"
          class="flex items-center justify-between gap-2 rounded-xl border border-line bg-surface px-3 py-2.5 hover:border-bronze/30"
        >
          <span class="min-w-0 truncate text-sm">{{ s.title }}</span>
          <span class="shrink-0 text-xs text-faint">{{ formatDay(dayOf(s.start), { weekday: "short", month: "short", day: "numeric" }) }} · {{ formatTime(s.start) }}</span>
        </RouterLink>
      </section>

      <SearchInput v-model="histQ" label="Search this trainee's history" placeholder="Search history — sessions, payments, purchases…" />

      <!-- Attendance history -->
      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">
          Attendance<template v-if="histQ.trim()"> · {{ historyTotal }} matching</template><template v-else-if="historyTotal"> · {{ attended }} attended · {{ noShows }} no-show{{ noShows === 1 ? "" : "s" }} in the last {{ history.length }}</template>
        </h2>
        <p v-if="!history.length" class="px-1 text-sm text-faint">{{ histQ.trim() ? "No sessions match." : "No past sessions yet." }}</p>
        <ul class="space-y-2">
          <li v-for="s in history" :key="s.id">
            <RouterLink :to="withBack(`/schedule/${s.id}`, here)" class="flex items-center justify-between gap-2 rounded-xl border border-line bg-surface px-3 py-2.5 hover:border-bronze/30" :class="s.status === 'cancelled' ? 'opacity-50' : ''">
              <div class="min-w-0">
                <p class="truncate text-sm"><Highlight :text="s.title" :q="histQ" /></p>
                <p class="text-xs text-faint">{{ formatDay(dayOf(s.start), { weekday: "short", month: "short", day: "numeric" }) }}<template v-if="s.status === 'cancelled'"> · cancelled</template></p>
              </div>
              <Badge v-if="s.status !== 'cancelled'" :tone="attendanceTone(myStatus(s))">{{ attendanceLabel(myStatus(s)) }}</Badge>
            </RouterLink>
          </li>
        </ul>
        <div v-if="historyMore" class="flex justify-center">
          <button :class="btnClasses('ghost', 'sm')" @click="loadSessions(true)">Show more</button>
        </div>
      </section>

      <!-- Payments (all types), paged on the server -->
      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Payments<template v-if="paymentsTotal"> · {{ paymentsTotal }}</template></h2>
        <p v-if="!payments.length" class="px-1 text-sm text-faint">{{ histQ.trim() ? "No payments match." : "No payments recorded yet." }}</p>
        <ul class="space-y-2">
          <li v-for="p in payments" :key="p.id">
            <RouterLink :to="withBack(`/payments/${p.id}`, here)" class="flex items-center justify-between gap-2 rounded-xl border border-line bg-surface px-3 py-2.5 hover:border-bronze/30" :class="p.voidedAt ? 'opacity-60' : ''">
              <div class="min-w-0">
                <p class="text-sm font-medium">
                  <Highlight :text="payTypeLabel(p.type)" :q="histQ" /><template v-if="p.note"> · <span class="font-normal text-muted"><Highlight :text="p.note" :q="histQ" /></span></template>
                  <span v-if="p.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                </p>
                <p class="text-xs text-faint">{{ formatLongDate(p.date) }}{{ p.periodMonth ? ` · for ${monthLabel(p.periodMonth)}` : "" }} · {{ methodLabel(p.method) }}</p>
              </div>
              <span class="flex shrink-0 items-center gap-1 font-display text-sm tnum" :class="p.voidedAt ? 'line-through text-faint' : ''">{{ money(p.amount) }} <ChevronRight class="h-4 w-4 text-faint" /></span>
            </RouterLink>
          </li>
        </ul>
        <div v-if="paymentsMore" class="flex justify-center">
          <button :class="btnClasses('ghost', 'sm')" @click="loadPayments(true)">Show more ({{ paymentsTotal - payments.length }} left)</button>
        </div>
      </section>

      <!-- Purchases -->
      <section v-if="purchases.length || histQ.trim()" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Purchases · {{ purchasesTotal }}</h2>
        <p v-if="!purchases.length" class="px-1 text-sm text-faint">No purchases match.</p>
        <ul class="space-y-2">
          <li v-for="s in purchases" :key="s.id">
            <RouterLink :to="withBack(`/sales/${s.id}`, here)" class="flex items-center justify-between gap-2 rounded-xl border border-line bg-surface px-3 py-2.5 hover:border-bronze/30" :class="s.voidedAt ? 'opacity-60' : ''">
              <span class="min-w-0 truncate text-sm"><Highlight :text="s.itemName" :q="histQ" /> <span class="text-faint">×{{ s.qty }}</span></span>
              <span class="shrink-0 text-xs text-faint">{{ formatLongDate(s.date) }} · <span class="tnum text-fg" :class="s.voidedAt ? 'line-through' : ''">{{ money(s.total) }}</span></span>
            </RouterLink>
          </li>
        </ul>
      </section>
    </template>

    <Alert v-else>Trainee not found.</Alert>
  </div>
</template>
