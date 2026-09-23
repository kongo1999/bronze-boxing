<script setup lang="ts">
import { reactive, ref, computed, watch, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { Trainee, SubStatus } from "@/lib/types";
import { money, monthLabel } from "@/lib/format";
import { currentMonth, formatDay, isDayKey, isMonthKey, todayKey } from "@/lib/studio";
import { backTarget } from "@/lib/route-state";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import { inputCls } from "@/lib/ui";

const route = useRoute();
const router = useRouter();
const q = route.query;
const trainees = ref<Trainee[]>([]);
const saving = ref(false);
const error = ref<string>();
const fieldErr = ref<Record<string, string>>({});

// Two different dates, both explicit: the fee period is the month the dues
// cover (defaulting to the Money month you came from); the cash date is when
// the money is received (defaulting to today). Collecting July's dues in
// September files the dues under July and the cash under September.
const viewingMonth = isMonthKey(q.m) ? q.m : currentMonth();
const form = reactive({
  trainee: typeof q.trainee === "string" ? q.trainee : "",
  amount: Number(q.amount) || 0,
  type: typeof q.type === "string" ? q.type : "subscription",
  periodMonth: isMonthKey(q.periodMonth) ? q.periodMonth : viewingMonth,
  day: isDayKey(q.day) ? q.day : todayKey(),
  note: "",
});

onMounted(async () => {
  try {
    trainees.value = await api.get<Trainee[]>("/trainees");
  } catch (e) {
    error.value = errMsg(e, "Couldn't load trainees.");
  }
});

// Searchable picker instead of a long native dropdown — the roster only grows.
const traineeOptions = computed(() =>
  trainees.value.map((t) => ({ id: t.id, label: t.name, sub: t.status === "inactive" ? "inactive" : undefined })),
);

// Dues context for subscription payments: what's owed, paid, and remaining
// for the chosen trainee + period. Shown inline and used to block overpaying
// (the server enforces the same rule; this is the friendly first line).
const dueInfo = ref<SubStatus | null>(null);
const dueLoaded = ref(false);
let dueToken = 0;
async function loadDue() {
  const my = ++dueToken;
  dueInfo.value = null;
  dueLoaded.value = false;
  if (form.type !== "subscription" || !form.trainee || !isMonthKey(form.periodMonth)) return;
  try {
    const subs = await api.get<SubStatus[]>(`/subscriptions?m=${form.periodMonth}`);
    if (my !== dueToken) return;
    dueInfo.value = subs.find((x) => x.trainee.id === form.trainee) ?? null;
    dueLoaded.value = true;
  } catch {
    /* helper line only — the server still validates */
  }
}
watch([() => form.trainee, () => form.periodMonth, () => form.type], loadDue, { immediate: true });

const remaining = computed(() => (dueInfo.value ? dueInfo.value.remaining : null));
const overpaying = computed(() => remaining.value !== null && Math.round(form.amount * 100) > Math.round(remaining.value * 100));
const cashMonth = computed(() => form.day.slice(0, 7));

async function submit() {
  fieldErr.value = {};
  if (!(form.amount > 0)) return (fieldErr.value = { amount: "Amount must be positive" });
  if (form.type === "subscription" && !form.trainee) return (fieldErr.value = { trainee: "Pick whose dues this pays" });
  if (overpaying.value) return;
  saving.value = true;
  error.value = undefined;
  try {
    await api.post("/payments", {
      trainee: form.trainee || undefined,
      amount: Math.round(form.amount * 100) / 100,
      type: form.type,
      periodMonth: form.type === "subscription" ? form.periodMonth : "",
      day: form.day,
      note: form.note,
    });
    invalidate("payments", "dues", "subs", "financials", "dashboard", "trainees");
    router.push(backTarget(route.query, `/payments?m=${form.type === "subscription" ? form.periodMonth : cashMonth.value}`));
  } catch (e) {
    saving.value = false;
    if (isApiError(e) && e.field) fieldErr.value = { [e.field]: e.message };
    else error.value = errMsg(e, "Failed to save");
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="backTarget(route.query, `/payments?m=${viewingMonth}`)" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Money
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">Log payment</h1>

    <Card class="space-y-3 p-4">
      <Alert v-if="error">{{ error }}</Alert>
      <div>
        <span class="mb-1 block text-xs text-faint">Trainee<span v-if="form.type === 'subscription'" class="text-overdue"> *</span></span>
        <SearchSelect v-model="form.trainee" :options="traineeOptions" empty-label="— (none)" placeholder="— (none)" search-placeholder="Search trainees…" />
        <span v-if="fieldErr.trainee" class="mt-1 block text-xs text-overdue">{{ fieldErr.trainee }}</span>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Amount <span class="text-overdue">*</span></span>
          <input v-model.number="form.amount" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" :aria-invalid="!!fieldErr.amount || overpaying" />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Type</span>
          <select v-model="form.type" :class="inputCls">
            <option value="subscription">Subscription</option>
            <option value="private">Private session</option>
            <option value="dropin">Drop-in</option>
            <option value="other">Other</option>
          </select>
        </label>
      </div>
      <span v-if="fieldErr.amount" class="block text-xs text-overdue">{{ fieldErr.amount }}</span>

      <div class="grid grid-cols-2 gap-3">
        <label v-if="form.type === 'subscription'" class="block">
          <span class="mb-1 block text-xs text-faint">Dues for (period)</span>
          <input v-model="form.periodMonth" type="month" :class="inputCls" :aria-invalid="!!fieldErr.periodMonth" />
        </label>
        <label class="block" :class="form.type === 'subscription' ? '' : 'col-span-2'">
          <span class="mb-1 block text-xs text-faint">Cash received on</span>
          <input v-model="form.day" type="date" :class="inputCls" :aria-invalid="!!fieldErr.day" />
        </label>
      </div>
      <span v-if="fieldErr.periodMonth || fieldErr.day" class="block text-xs text-overdue">{{ fieldErr.periodMonth || fieldErr.day }}</span>
      <p class="text-xs text-faint">
        <template v-if="form.type === 'subscription'">
          Pays {{ monthLabel(form.periodMonth) }}'s dues · counts as cash in {{ monthLabel(cashMonth) }}
          <template v-if="form.day !== todayKey()"> ({{ formatDay(form.day, { month: "short", day: "numeric" }) }})</template>.
        </template>
        <template v-else>Counts as cash in {{ monthLabel(cashMonth) }}.</template>
      </p>

      <div v-if="form.type === 'subscription' && form.trainee" class="rounded-xl border border-line bg-elevated px-3 py-2 text-xs" :class="overpaying ? 'text-overdue' : 'text-muted'">
        <template v-if="dueInfo">
          {{ monthLabel(form.periodMonth) }}: {{ money(dueInfo.due) }} due · {{ money(dueInfo.amountPaid) }} paid
          <template v-if="dueInfo.paymentCount"> in {{ dueInfo.paymentCount }} payment{{ dueInfo.paymentCount === 1 ? "" : "s" }}</template> ·
          <template v-if="dueInfo.state === 'unverified'">imported from older records — confirm the amount due before adding to it.</template>
          <template v-else-if="remaining! > 0"><span class="font-medium text-fg">{{ money(remaining!) }} remaining</span></template>
          <template v-else>fully paid</template>
          <template v-if="overpaying"> — that's more than what's owed.</template>
        </template>
        <template v-else-if="dueLoaded">No dues are recorded for {{ monthLabel(form.periodMonth) }} — log it as another type, or set a monthly fee first.</template>
        <template v-else>Checking {{ monthLabel(form.periodMonth) }}'s dues…</template>
      </div>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Note</span>
        <input v-model="form.note" :class="inputCls" />
      </label>
      <Button :disabled="saving || overpaying" @click="submit">{{ saving ? "Saving…" : "Record payment" }}</Button>
    </Card>
  </div>
</template>
