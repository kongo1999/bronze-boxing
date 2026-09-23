<script setup lang="ts">
import { ref, reactive, computed, watch } from "vue";
import { useRoute, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Undo2, Receipt as ReceiptIcon, RotateCcw } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync, invalidate } from "@/lib/cache";
import type { Sale, SaleReturn, Trainee } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import { backTarget, withBack } from "@/lib/route-state";
import { methodLabel } from "@/lib/labels";
import { RETURN_REASONS } from "@/lib/stock";
import { todayKey } from "@/lib/studio";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { askReason, VOID_REASONS, CORRECTION_REASONS } from "@/lib/prompt";
import { errMsg } from "@/lib/api";
import { btnClasses } from "@/components/ui/button";
import { traineeOption } from "@/lib/options";

const route = useRoute();
const id = route.params.id as string;
const back = computed(() => backTarget(route.query, "/inventory"));
const here = computed(() => route.fullPath);

const { data: sale, loading, error, reload } = useCachedAsync(`sales:${id}`, () => api.get<Sale>(`/sales/${id}`));
const { data: trainees } = useCachedAsync("trainees", () => api.get<Trainee[]>("/trainees?archived=include"));
const returns = ref<SaleReturn[]>([]);
async function loadReturns() {
  try {
    returns.value = await api.get<SaleReturn[]>(`/sales/${id}/returns`);
  } catch {
    /* secondary to the sale itself */
  }
}
loadReturns();

// ── Edit (correct qty / buyer) — PUT /sales/:id adjusts stock by the delta ──
const editing = ref(false);
const saving = ref(false);
const form = reactive({ qty: 1, trainee: "" });

// Buyer options for the picker; a sale can also be a walk-in (no trainee).
const traineeOptions = computed(() =>
  (trainees.value ?? []).filter((t) => !t.archivedAt || t.id === sale.value?.trainee).map(traineeOption),
);
function startEdit() {
  if (!sale.value) return;
  form.qty = sale.value.qty;
  form.trainee = sale.value.trainee ?? "";
  editing.value = true;
}
const newTotal = computed(() => (sale.value ? sale.value.unitPrice * (form.qty || 0) : 0));
async function save() {
  if (saving.value || !sale.value || form.qty <= 0) return;
  let reason = "";
  if (form.qty !== sale.value.qty) {
    const r = await askReason({
      title: "Correct the quantity?",
      message: `${sale.value.qty} → ${form.qty} × ${sale.value.itemName}. Stock moves by the difference and the change is kept in the sale's history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: CORRECTION_REASONS,
    });
    if (r === null) return;
    reason = r;
  }
  saving.value = true;
  try {
    await api.put(`/sales/${id}`, { qty: form.qty, trainee: form.trainee || "", reason });
    editing.value = false;
    invalidate("inventory", "sales", "financials"); // stock and the sales list both moved
    await reload();
    toast("Sale updated.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't update sale."), "error");
  } finally {
    saving.value = false;
  }
}

// ── Partial return — units back on the shelf, refund on its own cash day ───
const returnable = computed(() => (sale.value ? sale.value.qty - (sale.value.returnedQty ?? 0) : 0));
const refundable = computed(() => (sale.value ? Math.round((sale.value.total - (sale.value.returnedTotal ?? 0)) * 100) / 100 : 0));
const returning = ref(false);
const rf = reactive({ qty: 1, amount: 0, amountEdited: false, reason: "", day: todayKey() });
const rErr = ref("");
const rBusy = ref(false);
function startReturn() {
  if (!sale.value) return;
  Object.assign(rf, { qty: 1, amount: Math.min(sale.value.unitPrice, refundable.value), amountEdited: false, reason: "", day: todayKey() });
  rErr.value = "";
  returning.value = true;
}
watch(
  () => rf.qty,
  (q) => {
    if (sale.value && !rf.amountEdited) rf.amount = Math.min(Math.round(sale.value.unitPrice * (q || 0) * 100) / 100, refundable.value);
  },
);
async function submitReturn() {
  if (rBusy.value || !sale.value) return;
  if (!Number.isInteger(rf.qty) || rf.qty < 1 || rf.qty > returnable.value) return (rErr.value = `Return between 1 and ${returnable.value}.`);
  if (!(rf.amount >= 0) || rf.amount > refundable.value) return (rErr.value = `The refund can be at most ${money(refundable.value)}.`);
  if (!rf.reason.trim()) return (rErr.value = "Say why it came back.");
  rBusy.value = true;
  rErr.value = "";
  try {
    await api.post(`/sales/${id}/returns`, { qty: rf.qty, amount: rf.amount, reason: rf.reason.trim(), day: rf.day });
    returning.value = false;
    invalidate("inventory", "sales", "financials");
    await Promise.all([reload(), loadReturns()]);
    toast(`Return recorded: ${rf.qty} back in stock, ${money(rf.amount)} refunded.`, "success");
  } catch (e) {
    rErr.value = errMsg(e, "Couldn't record the return.");
  } finally {
    rBusy.value = false;
  }
}

// ── Void — DELETE /sales/:id restocks; the record stays, marked VOID ────────
const voiding = ref(false);
async function voidSale() {
  if (voiding.value || !sale.value) return;
  const reason = await askReason({
    title: "Void this sale?",
    message: `${sale.value.qty - (sale.value.returnedQty ?? 0)} × ${sale.value.itemName} goes back in stock and the sale stays in the books marked VOID.`,
    confirmLabel: "Void sale",
    suggestions: VOID_REASONS,
  });
  if (reason === null) return;
  voiding.value = true;
  try {
    await api.post(`/sales/${id}/void`, { reason });
    toast("Sale voided and restocked.", "success");
    invalidate("inventory", "sales", "financials");
    await reload();
  } catch (e) {
    toast(errMsg(e, "Couldn't void sale."), "error");
  } finally {
    voiding.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back" class="inline-flex min-h-10 items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Back
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />
    <Alert v-else-if="error">{{ error }}</Alert>

    <template v-else-if="sale">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <h1 class="font-display text-xl font-semibold">
              <RouterLink :to="withBack(`/inventory/${sale.item}`, here)" class="hover:underline">{{ sale.itemName }}</RouterLink>
              <span v-if="sale.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-xs font-semibold uppercase tracking-wide text-overdue">Void</span>
            </h1>
            <p class="text-sm text-muted">
              <RouterLink v-if="sale.trainee" :to="withBack(`/trainees/${sale.trainee}`, here)" class="hover:underline">{{ sale.traineeName }}</RouterLink>
              <template v-else>Walk-in</template>
              · {{ formatLongDate(sale.date) }} · {{ methodLabel(sale.method || "cash") }}
            </p>
          </div>
          <span class="font-display text-lg tnum" :class="sale.voidedAt ? 'line-through text-faint' : ''">{{ money(sale.total) }}</span>
        </div>

        <div class="mt-4 grid grid-cols-2 gap-3">
          <div class="rounded-xl border border-line bg-elevated px-3 py-2">
            <p class="label-eyebrow text-[0.6rem] text-faint">Quantity</p>
            <p class="font-display text-lg tnum">{{ sale.qty }}</p>
          </div>
          <div class="rounded-xl border border-line bg-elevated px-3 py-2">
            <p class="label-eyebrow text-[0.6rem] text-faint">Unit price</p>
            <p class="font-display text-lg tnum">{{ money(sale.unitPrice) }}</p>
          </div>
        </div>
        <p v-if="sale.listPrice && Math.round(sale.listPrice * 100) !== Math.round(sale.unitPrice * 100)" class="mt-2 text-xs text-partial">
          List price was {{ money(sale.listPrice) }}<template v-if="sale.priceReason"> · {{ sale.priceReason }}</template>
        </p>
        <p v-if="sale.returnedQty" class="mt-2 text-sm text-muted tnum">
          {{ sale.returnedQty }} returned · {{ money(sale.returnedTotal ?? 0) }} refunded · net {{ money(sale.total - (sale.returnedTotal ?? 0)) }}
        </p>

        <p v-if="sale.voidedAt" class="mt-4 text-sm text-faint">
          Voided {{ formatLongDate(sale.voidedAt) }} — kept in the books, stock restored, no longer counts as income.
        </p>
        <div v-else-if="!editing && !returning" class="mt-4 flex flex-wrap gap-2">
          <Button size="sm" @click="startEdit"><Pencil class="h-4 w-4" /> Edit sale</Button>
          <button v-if="returnable > 0" :class="btnClasses('ghost', 'sm')" @click="startReturn"><RotateCcw class="h-4 w-4" /> Return items</button>
          <button
            :disabled="voiding"
            class="inline-flex h-10 items-center gap-1.5 rounded-xl border border-purple/40 px-3 text-sm font-medium text-purple transition-colors hover:bg-purple/10 disabled:opacity-50"
            @click="voidSale"
          >
            <Undo2 class="h-4 w-4" /> {{ voiding ? "Voiding…" : "Void" }}
          </button>
        </div>

        <form v-else-if="returning" class="mt-4 space-y-3 border-t border-line pt-4" @submit.prevent="submitReturn">
          <p class="text-sm font-medium">Return items</p>
          <div class="grid grid-cols-3 gap-2">
            <label class="block"><span class="mb-1 block text-xs text-faint">Units (of {{ returnable }})</span><input v-model.number="rf.qty" type="number" inputmode="numeric" min="1" :max="returnable" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Refund</span><input v-model.number="rf.amount" type="number" inputmode="decimal" min="0" step="0.01" :max="refundable" :class="inputCls" @input="rf.amountEdited = true" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Refunded on</span><input v-model="rf.day" type="date" :max="todayKey()" :class="inputCls" /></label>
          </div>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="r in RETURN_REASONS"
              :key="r"
              type="button"
              class="min-h-10 rounded-lg border px-3 text-xs"
              :class="rf.reason === r ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted hover:text-fg'"
              @click="rf.reason = r"
            >{{ r }}</button>
          </div>
          <input v-model="rf.reason" placeholder="Reason *" aria-label="Reason for the return" :class="inputCls" />
          <p class="text-xs text-faint">The units go back in stock; the refund is money out on the day you pick. To cancel a sale that shouldn't exist, use Void instead.</p>
          <p v-if="rErr" class="text-sm text-overdue" role="alert">{{ rErr }}</p>
          <div class="flex gap-2">
            <Button type="submit" size="sm" :disabled="rBusy">{{ rBusy ? "Saving…" : `Return ${rf.qty} · refund ${money(rf.amount || 0)}` }}</Button>
            <Button size="sm" variant="ghost" @click="returning = false">Cancel</Button>
          </div>
        </form>

        <div v-else class="mt-4 space-y-3 border-t border-line pt-4">
          <div class="grid grid-cols-2 gap-3">
            <label class="block"><span class="mb-1 block text-xs text-faint">Quantity</span><input v-model.number="form.qty" type="number" min="1" :class="inputCls" /></label>
            <div>
              <span class="mb-1 block text-xs text-faint">Buyer</span>
              <SearchSelect v-model="form.trainee" :options="traineeOptions" empty-label="Walk-in" placeholder="Walk-in" search-placeholder="Search trainees…" />
            </div>
          </div>
          <p class="text-xs text-faint">New total <span class="tnum text-fg">{{ money(newTotal) }}</span> · stock adjusts by the quantity change.</p>
          <div class="flex gap-2">
            <Button size="sm" :disabled="saving || form.qty <= 0" @click="save">{{ saving ? "Saving…" : "Save changes" }}</Button>
            <Button size="sm" variant="ghost" @click="editing = false">Cancel</Button>
          </div>
        </div>

        <div v-if="returns.length" class="mt-4 border-t border-line pt-3">
          <p class="label-eyebrow text-[0.6rem] text-faint">Returns</p>
          <ul class="mt-1 space-y-1 text-sm">
            <li v-for="r in returns" :key="r.id" class="flex justify-between gap-3">
              <span class="min-w-0 truncate">{{ r.qty }} returned · {{ r.reason }} <span class="text-faint">· {{ formatLongDate(r.date) }}</span></span>
              <span class="shrink-0 tnum text-overdue">−{{ money(r.amount) }}</span>
            </li>
          </ul>
        </div>

        <div class="mt-4 flex border-t border-line pt-3">
          <RouterLink :to="withBack(`/sales/${id}/receipt`, here)" :class="btnClasses('ghost', 'sm')"><ReceiptIcon class="h-4 w-4" /> Receipt</RouterLink>
        </div>

        <div class="mt-4 border-t border-line pt-3">
          <AuditTrail entity="sale" :id="id" />
        </div>
      </Card>
    </template>

    <Alert v-else>Sale not found.</Alert>
  </div>
</template>
