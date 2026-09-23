<script setup lang="ts">
import { computed, reactive, ref, onMounted } from "vue";
import { useRoute, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Undo2, FileText, Link2 } from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { Payment, Trainee } from "@/lib/types";
import { money, monthLabel, formatDateTime } from "@/lib/format";
import { dayOf, monthOf } from "@/lib/studio";
import { backTarget, withBack } from "@/lib/route-state";
import { METHODS, methodLabel, payTypeLabel } from "@/lib/labels";
import { askReason, VOID_REASONS, CORRECTION_REASONS } from "@/lib/prompt";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import { btnClasses } from "@/components/ui/button";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

const route = useRoute();
const id = route.params.id as string;
const payment = ref<Payment>();
const loading = ref(true);
const error = ref<string>();
const historyKey = ref(0); // bump to reload the audit trail after a change

async function load() {
  error.value = undefined;
  try {
    payment.value = await api.get<Payment>(`/payments/${id}`);
  } catch (e) {
    error.value = errMsg(e, "Couldn't load this payment.");
  } finally {
    loading.value = false;
  }
}
onMounted(load);

const back = computed(() =>
  backTarget(route.query, payment.value ? `/payments?m=${monthOf(payment.value.date)}&tab=payments` : "/payments"),
);
const linkedSale = computed(() => !!payment.value?.saleId || payment.value?.type === "sale");

// ── Edit ────────────────────────────────────────────────────────────────
const editing = ref(false);
const saving = ref(false);
const fieldErr = ref<Record<string, string>>({});
const trainees = ref<Trainee[]>([]);
const form = reactive({ trainee: "", amount: 0, type: "subscription", periodMonth: "", day: "", method: "cash", reference: "", note: "" });
async function startEdit() {
  const p = payment.value;
  if (!p) return;
  Object.assign(form, {
    trainee: p.trainee ?? "", amount: p.amount, type: p.type, periodMonth: p.periodMonth ?? "",
    day: dayOf(p.date), method: p.method ?? "", reference: p.reference ?? "", note: p.note ?? "",
  });
  fieldErr.value = {};
  editing.value = true;
  if (!trainees.value.length) {
    try {
      trainees.value = await api.get<Trainee[]>("/trainees");
    } catch {
      /* picker falls back to the current name */
    }
  }
}
const traineeOptions = computed(() => {
  const opts = trainees.value.map((t) => ({ id: t.id, label: t.name, sub: t.status === "inactive" ? "inactive" : undefined }));
  const p = payment.value;
  if (p?.trainee && !opts.some((o) => o.id === p.trainee)) opts.unshift({ id: p.trainee, label: p.traineeName ?? "—", sub: undefined });
  return opts;
});
async function save() {
  const p = payment.value;
  if (!p || saving.value) return;
  fieldErr.value = {};
  if (!(form.amount > 0)) return (fieldErr.value = { amount: "Amount must be positive" });
  let reason = "";
  if (Math.round(p.amount * 100) !== Math.round(form.amount * 100)) {
    const r = await askReason({
      title: "Correct the amount?",
      message: `${money(p.amount)} → ${money(form.amount)}. The change is kept in this payment's history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: CORRECTION_REASONS,
    });
    if (r === null) return;
    reason = r;
  }
  saving.value = true;
  try {
    payment.value = await api.put<Payment>(`/payments/${id}`, {
      trainee: form.trainee,
      amount: form.amount,
      type: form.type,
      periodMonth: form.type === "subscription" ? form.periodMonth : "",
      day: form.day !== dayOf(p.date) ? form.day : undefined,
      method: form.method,
      reference: form.method === "cash" ? "" : form.reference,
      note: form.note,
      reason,
    });
    editing.value = false;
    historyKey.value++;
    invalidate("payments", "dues", "subs", "financials", "dashboard", "trainees");
    toast("Payment updated.", "success");
  } catch (e) {
    if (isApiError(e) && e.field) fieldErr.value = { [e.field]: e.message };
    else toast(errMsg(e, "Couldn't update payment."), "error");
  } finally {
    saving.value = false;
  }
}

// ── Void ────────────────────────────────────────────────────────────────
const voiding = ref(false);
async function voidIt() {
  const p = payment.value;
  if (!p || voiding.value) return;
  const reason = await askReason({
    title: "Void this payment?",
    message: `${money(p.amount)} from ${p.traineeName || "—"} stays in the books marked VOID and stops counting toward revenue and dues. Its receipt will show VOID.`,
    confirmLabel: "Void payment",
    suggestions: VOID_REASONS,
  });
  if (reason === null) return;
  voiding.value = true;
  try {
    await api.post(`/payments/${id}/void`, { reason });
    await load();
    historyKey.value++;
    invalidate("payments", "dues", "subs", "financials", "dashboard", "trainees");
    toast("Payment voided.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't void payment."), "error");
  } finally {
    voiding.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Money
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />
    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else-if="payment">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="label-eyebrow text-[0.625rem] text-faint">{{ payTypeLabel(payment.type) }}</p>
            <h1 class="font-display text-3xl font-semibold tnum" :class="payment.voidedAt ? 'line-through text-faint' : ''">{{ money(payment.amount) }}</h1>
            <RouterLink v-if="payment.trainee" :to="withBack(`/trainees/${payment.trainee}`, route.fullPath)" class="text-sm text-bronze hover:underline">{{ payment.traineeName }}</RouterLink>
            <p v-else class="text-sm text-muted">{{ payment.traineeName || "No trainee" }}</p>
          </div>
          <span v-if="payment.voidedAt" class="rounded bg-overdue/15 px-2 py-1 text-xs font-semibold uppercase tracking-wide text-overdue">Void</span>
        </div>

        <dl class="mt-4 grid grid-cols-2 gap-3 text-sm">
          <div v-if="payment.periodMonth" class="rounded-xl border border-line bg-elevated px-3 py-2">
            <dt class="label-eyebrow text-[0.6rem] text-faint">Dues for</dt>
            <dd>{{ monthLabel(payment.periodMonth) }}</dd>
          </div>
          <div class="rounded-xl border border-line bg-elevated px-3 py-2">
            <dt class="label-eyebrow text-[0.6rem] text-faint">Cash received</dt>
            <dd>{{ formatDateTime(payment.date) }}</dd>
          </div>
          <div class="rounded-xl border border-line bg-elevated px-3 py-2">
            <dt class="label-eyebrow text-[0.6rem] text-faint">Method</dt>
            <dd>{{ methodLabel(payment.method) }}<span v-if="payment.reference" class="text-faint"> · {{ payment.reference }}</span></dd>
          </div>
          <div v-if="payment.createdBy" class="rounded-xl border border-line bg-elevated px-3 py-2">
            <dt class="label-eyebrow text-[0.6rem] text-faint">Recorded by</dt>
            <dd>{{ payment.createdBy }}</dd>
          </div>
        </dl>
        <p v-if="payment.note" class="mt-3 rounded-xl bg-elevated px-3 py-2 text-sm text-muted">{{ payment.note }}</p>
        <p v-if="linkedSale" class="mt-3 flex items-center gap-1 text-xs text-faint">
          <Link2 class="h-3.5 w-3.5" /> Mirrors a shop sale — its type can't change; correct the sale in Inventory.
        </p>
        <Alert v-if="payment.voidedAt" class="mt-3">
          Voided {{ formatDateTime(payment.voidedAt) }}<template v-if="payment.voidedBy"> by {{ payment.voidedBy }}</template>
          <template v-if="payment.voidReason"> — “{{ payment.voidReason }}”</template>. It no longer counts.
        </Alert>

        <div class="mt-4 flex flex-wrap gap-2">
          <RouterLink :to="withBack(`/payments/${id}/receipt`, route.fullPath)" :class="btnClasses('ghost', 'sm')"><FileText class="h-4 w-4" /> Receipt</RouterLink>
          <template v-if="!payment.voidedAt && !editing">
            <Button size="sm" variant="ghost" @click="startEdit"><Pencil class="h-4 w-4" /> Edit</Button>
            <button :class="btnClasses('danger', 'sm')" :disabled="voiding" @click="voidIt"><Undo2 class="h-4 w-4" /> {{ voiding ? "Voiding…" : "Void" }}</button>
          </template>
        </div>

        <div v-if="editing" class="mt-4 space-y-3 border-t border-line pt-4">
          <div>
            <span class="mb-1 block text-xs text-faint">Trainee</span>
            <SearchSelect v-model="form.trainee" :options="traineeOptions" empty-label="— (none)" placeholder="— (none)" search-placeholder="Search trainees…" />
            <span v-if="fieldErr.trainee" class="mt-1 block text-xs text-overdue">{{ fieldErr.trainee }}</span>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <label class="block"><span class="mb-1 block text-xs text-faint">Amount</span>
              <input v-model.number="form.amount" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" :aria-invalid="!!fieldErr.amount" />
            </label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Type</span>
              <select v-model="form.type" :class="inputCls" :disabled="linkedSale">
                <option value="subscription">Subscription</option>
                <option value="private">Private session</option>
                <option value="dropin">Drop-in</option>
                <option value="other">Other</option>
                <option v-if="linkedSale" value="sale">Shop sale</option>
              </select>
            </label>
          </div>
          <span v-if="fieldErr.amount || fieldErr.type" class="block text-xs text-overdue">{{ fieldErr.amount || fieldErr.type }}</span>
          <div class="grid grid-cols-2 gap-2">
            <label v-if="form.type === 'subscription'" class="block"><span class="mb-1 block text-xs text-faint">Dues for (period)</span>
              <input v-model="form.periodMonth" type="month" :class="inputCls" :aria-invalid="!!fieldErr.periodMonth" />
            </label>
            <label class="block" :class="form.type === 'subscription' ? '' : 'col-span-2'"><span class="mb-1 block text-xs text-faint">Cash received on</span>
              <input v-model="form.day" type="date" :class="inputCls" />
            </label>
          </div>
          <span v-if="fieldErr.periodMonth" class="block text-xs text-overdue">{{ fieldErr.periodMonth }}</span>
          <div>
            <span class="mb-1 block text-xs text-faint">Method</span>
            <ChipGroup v-model="form.method" :options="METHODS.map((m) => ({ v: m.v, l: m.l }))" label="Payment method" />
          </div>
          <label v-if="form.method && form.method !== 'cash'" class="block"><span class="mb-1 block text-xs text-faint">Reference <span class="text-faint/70">(optional)</span></span>
            <input v-model="form.reference" :class="inputCls" placeholder="Slip or transfer number" />
          </label>
          <label class="block"><span class="mb-1 block text-xs text-faint">Note</span><input v-model="form.note" :class="inputCls" /></label>
          <div class="flex gap-2">
            <Button size="sm" :disabled="saving" @click="save">{{ saving ? "Saving…" : "Save changes" }}</Button>
            <Button size="sm" variant="ghost" @click="editing = false">Cancel</Button>
          </div>
        </div>
      </Card>

      <Card class="p-4">
        <AuditTrail :key="historyKey" entity="payment" :id="id" />
      </Card>
    </template>
  </div>
</template>
