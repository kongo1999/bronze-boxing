<script setup lang="ts">
// Selling one item: quantity (checked against stock before submitting),
// buyer, how they paid, and an optional price change with a reason — then a
// confirmation of exactly what will be recorded before anything is saved.
import { computed, reactive, ref } from "vue";
import { Minus, Plus } from "lucide-vue-next";
import type { InventoryItem, Sale } from "@/lib/types";
import { api, errMsg } from "@/lib/api";
import { money } from "@/lib/format";
import { METHODS, methodLabel } from "@/lib/labels";
import { PRICE_REASONS } from "@/lib/stock";
import { inputCls } from "@/lib/ui";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import Button from "@/components/ui/Button.vue";

const props = defineProps<{ item: InventoryItem; buyers: { id: string; label: string }[] }>();
const emit = defineEmits<{ sold: [sale: Sale]; cancel: [] }>();

const f = reactive({ qty: 1, trainee: "", method: "cash", priceOn: false, unitPrice: props.item.price, reason: "" });
const step = ref<"edit" | "confirm">("edit");
const busy = ref(false);
const err = ref("");
const tried = ref(false);

const cents = (v: number) => Math.round(v * 100);
const qtyErr = computed(() =>
  !Number.isInteger(f.qty) || f.qty < 1 ? "Enter at least 1." : f.qty > props.item.stock ? `Only ${props.item.stock} in stock.` : "",
);
const unit = computed(() => (f.priceOn ? Number(f.unitPrice) : props.item.price));
const priceChanged = computed(() => cents(unit.value) !== cents(props.item.price));
const priceErr = computed(() => {
  if (!f.priceOn) return "";
  if (!Number.isFinite(unit.value) || unit.value < 0) return "Enter a price of 0 or more.";
  if (priceChanged.value && !f.reason.trim()) return "Say why the price differs.";
  return "";
});
const total = computed(() => (cents(unit.value) * (f.qty || 0)) / 100);
const buyerName = computed(() => props.buyers.find((b) => b.id === f.trainee)?.label ?? "Walk-in");

function step1(d: number) {
  f.qty = Math.min(Math.max(1, (Number(f.qty) || 0) + d), Math.max(1, props.item.stock));
}
function review() {
  tried.value = true;
  if (qtyErr.value || priceErr.value) return;
  err.value = "";
  step.value = "confirm";
}
async function confirm() {
  if (busy.value) return;
  busy.value = true;
  err.value = "";
  try {
    const sale = await api.post<Sale>(`/inventory/${props.item.id}/sell`, {
      qty: f.qty,
      trainee: f.trainee || undefined,
      method: f.method,
      ...(priceChanged.value ? { unitPrice: unit.value, priceReason: f.reason.trim() } : {}),
    });
    emit("sold", sale);
  } catch (e) {
    err.value = errMsg(e, "Couldn't record the sale.");
    step.value = "edit";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="space-y-3 rounded-xl bg-elevated p-3">
    <template v-if="step === 'edit'">
      <div class="grid grid-cols-2 gap-2">
        <div>
          <label :for="`qty-${item.id}`" class="mb-1 block text-xs text-faint">Quantity</label>
          <div class="flex items-center gap-1">
            <button type="button" class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line text-muted hover:text-fg" aria-label="One less" @click="step1(-1)"><Minus class="h-4 w-4" /></button>
            <input
              :id="`qty-${item.id}`"
              v-model.number="f.qty"
              type="number"
              inputmode="numeric"
              min="1"
              :max="item.stock"
              :class="[inputCls, 'text-center tnum']"
              :aria-invalid="tried && !!qtyErr"
            />
            <button type="button" class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line text-muted hover:text-fg" aria-label="One more" @click="step1(1)"><Plus class="h-4 w-4" /></button>
          </div>
          <p v-if="qtyErr && (tried || f.qty > item.stock)" class="mt-1 text-xs text-overdue">{{ qtyErr }}</p>
        </div>
        <div>
          <span class="mb-1 block text-xs text-faint">Buyer</span>
          <SearchSelect v-model="f.trainee" :options="buyers" empty-label="Walk-in" placeholder="Walk-in" search-placeholder="Search trainees…" />
        </div>
      </div>
      <div>
        <span class="mb-1 block text-xs text-faint">Paid by</span>
        <ChipGroup v-model="f.method" :options="METHODS.map((m) => ({ v: m.v, l: m.l }))" label="Payment method" scroll />
      </div>
      <div>
        <button v-if="!f.priceOn" type="button" class="min-h-10 text-sm font-medium text-bronze hover:underline" @click="f.priceOn = true">
          Change price ({{ money(item.price) }} each)
        </button>
        <div v-else class="space-y-2">
          <label class="block">
            <span class="mb-1 block text-xs text-faint">Unit price (list {{ money(item.price) }})</span>
            <input v-model.number="f.unitPrice" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" />
          </label>
          <template v-if="priceChanged">
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="r in PRICE_REASONS"
                :key="r"
                type="button"
                class="min-h-10 rounded-lg border px-3 text-xs"
                :class="f.reason === r ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted hover:text-fg'"
                @click="f.reason = r"
              >{{ r }}</button>
            </div>
            <input v-model="f.reason" placeholder="Why the price differs" aria-label="Reason for the price change" :class="inputCls" />
          </template>
          <p v-if="tried && priceErr" class="text-xs text-overdue">{{ priceErr }}</p>
          <button type="button" class="min-h-10 text-xs text-faint hover:text-fg" @click="Object.assign(f, { priceOn: false, unitPrice: item.price, reason: '' })">Use the list price</button>
        </div>
      </div>
      <p v-if="err" class="text-sm text-overdue" role="alert">{{ err }}</p>
      <div class="flex gap-2">
        <Button size="sm" :disabled="item.stock <= 0" @click="review">Review sale — {{ money(total) }}</Button>
        <Button size="sm" variant="ghost" @click="emit('cancel')">Cancel</Button>
      </div>
    </template>

    <template v-else>
      <p class="text-sm font-medium">Record this sale?</p>
      <dl class="space-y-1.5 text-sm">
        <div class="flex justify-between gap-3"><dt class="text-faint">Item</dt><dd class="text-right">{{ item.name }}</dd></div>
        <div class="flex justify-between gap-3"><dt class="text-faint">Quantity</dt><dd class="tnum">{{ f.qty }}</dd></div>
        <div class="flex justify-between gap-3">
          <dt class="text-faint">Unit price</dt>
          <dd class="text-right tnum">{{ money(unit) }}<span v-if="priceChanged" class="block text-xs text-partial">list {{ money(item.price) }} · {{ f.reason }}</span></dd>
        </div>
        <div class="flex justify-between gap-3"><dt class="text-faint">Buyer</dt><dd class="text-right">{{ buyerName }}</dd></div>
        <div class="flex justify-between gap-3"><dt class="text-faint">Paid by</dt><dd>{{ methodLabel(f.method) }}</dd></div>
        <div class="flex justify-between gap-3"><dt class="text-faint">Left in stock</dt><dd class="tnum">{{ item.stock - f.qty }}</dd></div>
        <div class="flex items-baseline justify-between gap-3 border-t border-line pt-2">
          <dt class="font-medium">Total</dt><dd class="font-display text-lg font-semibold tnum">{{ money(total) }}</dd>
        </div>
      </dl>
      <p v-if="err" class="text-sm text-overdue" role="alert">{{ err }}</p>
      <div class="flex gap-2">
        <Button size="sm" :disabled="busy" @click="confirm">{{ busy ? "Recording…" : `Record sale — ${money(total)}` }}</Button>
        <Button size="sm" variant="ghost" :disabled="busy" @click="step = 'edit'">Back</Button>
      </div>
    </template>
  </div>
</template>
