<script setup lang="ts">
// One shop item: what's on the shelf, the actions that change it (restock,
// write-off, a counted correction — each with its reason in the stock
// history), its sales, and archive instead of delete once it has history.
import { computed, reactive, ref, watch } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Archive, ArchiveRestore, Trash2, PackagePlus, PackageMinus, ClipboardCheck } from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { useCachedAsync, invalidate } from "@/lib/cache";
import type { InventoryItem, Page, Sale, StockMovement, Trainee } from "@/lib/types";
import { money, formatDateTime, formatLongDate } from "@/lib/format";
import { backTarget, withBack } from "@/lib/route-state";
import { shortage, MOVE_LABEL, DAMAGE_REASONS, COUNT_REASONS } from "@/lib/stock";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { askReason } from "@/lib/prompt";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import AuditTrail from "@/components/ui/AuditTrail.vue";
import SellSheet from "@/components/SellSheet.vue";
import { btnClasses } from "@/components/ui/button";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;
const back = computed(() => backTarget(route.query, "/inventory"));
const here = computed(() => route.fullPath);

const { data: item, loading, error, reload } = useCachedAsync(`inventory:${id}`, () => api.get<InventoryItem>(`/inventory/${id}`));
const { data: trainees } = useCachedAsync("trainees", () => api.get<Trainee[]>("/trainees?archived=include"));
const buyers = computed(() =>
  (trainees.value ?? []).filter((t) => t.status === "active" && !t.archivedAt).map((t) => ({ id: t.id, label: t.name })),
);

const moves = ref<StockMovement[]>([]);
async function loadMoves() {
  try {
    moves.value = await api.get<StockMovement[]>(`/inventory/${id}/movements?limit=100`);
  } catch {
    /* the history is secondary to the item itself */
  }
}
loadMoves();

const sales = ref<Sale[]>([]);
const salesTotal = ref(0);
async function loadSales(append = false) {
  try {
    const pg = await api.get<Page<Sale>>(`/sales?item=${id}&limit=10&offset=${append ? sales.value.length : 0}`);
    sales.value = append ? [...sales.value, ...pg.items] : pg.items;
    salesTotal.value = pg.total;
  } catch {
    /* secondary */
  }
}
loadSales();

async function refresh() {
  invalidate("inventory", "sales", "financials");
  await Promise.all([reload(), loadMoves(), loadSales()]);
}

// ── Stock actions ───────────────────────────────────────────────────────────
type Act = "restock" | "damage" | "correction" | "sell" | "edit" | "";
const act = ref<Act>((route.query.act as Act) || "");
const a = reactive({ qty: 1, count: 0, unitCost: undefined as number | undefined, reason: "" });
const acting = ref(false);
const actErr = ref("");
function open(k: Act) {
  act.value = act.value === k ? "" : k;
  actErr.value = "";
  Object.assign(a, { qty: 1, count: item.value?.stock ?? 0, unitCost: item.value?.costPrice || undefined, reason: "" });
  if (k === "edit" && item.value) Object.assign(ef, pick(item.value));
}
watch(item, (it) => {
  if (it && act.value === "restock" && a.unitCost === undefined) a.unitCost = it.costPrice || undefined;
});
const after = computed(() => {
  const s = item.value?.stock ?? 0;
  return act.value === "restock" ? s + (a.qty || 0) : act.value === "damage" ? s - (a.qty || 0) : a.count;
});
async function submitAct() {
  if (!item.value || acting.value) return;
  const body: Record<string, unknown> = { kind: act.value, reason: a.reason.trim() };
  if (act.value === "correction") {
    if (!Number.isInteger(a.count) || a.count < 0) return (actErr.value = "Enter the number on the shelf.");
    if (a.count === item.value.stock) return (actErr.value = "That's already the stock on record.");
    Object.assign(body, { count: a.count, expected: item.value.stock });
  } else {
    if (!Number.isInteger(a.qty) || a.qty < 1) return (actErr.value = "Enter at least 1.");
    if (act.value === "damage" && a.qty > item.value.stock) return (actErr.value = `Only ${item.value.stock} in stock.`);
    body.qty = a.qty;
    if (act.value === "restock" && a.unitCost !== undefined && a.unitCost !== null && String(a.unitCost) !== "") body.unitCost = Number(a.unitCost);
  }
  if (act.value !== "restock" && !a.reason.trim()) return (actErr.value = "Say why — it goes in the stock history.");
  acting.value = true;
  actErr.value = "";
  try {
    await api.post(`/inventory/${id}/adjustments`, body);
    const label = act.value === "restock" ? "Restocked" : act.value === "damage" ? "Written off" : "Stock corrected";
    act.value = "";
    await refresh();
    toast(`${label}. ${item.value?.stock ?? ""} in stock.`, "success");
  } catch (e) {
    if (isApiError(e, "CONFLICT")) await reload();
    actErr.value = errMsg(e, "Couldn't save that.");
  } finally {
    acting.value = false;
  }
}

// ── Edit details (never the stock — that's what the actions are for) ───────
const pick = (i: InventoryItem) => ({ name: i.name, sku: i.sku ?? "", price: i.price, costPrice: i.costPrice ?? 0, lowStockThreshold: i.lowStockThreshold ?? 0 });
const ef = reactive({ name: "", sku: "", price: 0, costPrice: 0, lowStockThreshold: 0 });
const saving = ref(false);
async function saveEdit() {
  if (!item.value || saving.value) return;
  if (!ef.name.trim()) return (actErr.value = "Name the item.");
  if (!(ef.price > 0)) return (actErr.value = "Set a selling price.");
  saving.value = true;
  actErr.value = "";
  try {
    await api.put(`/inventory/${id}`, { ...ef, active: item.value.active });
    act.value = "";
    await refresh();
    toast("Item updated.", "success");
  } catch (e) {
    actErr.value = errMsg(e, "Couldn't update the item.");
  } finally {
    saving.value = false;
  }
}

async function sold(sale: Sale) {
  act.value = "";
  await refresh();
  toast(`Sold ${sale.qty} × ${sale.itemName}.`, "success");
  router.push(withBack(`/sales/${sale.id}/receipt`, here.value));
}

// ── Archive / delete ────────────────────────────────────────────────────────
const hasHistory = computed(() => sales.value.length > 0 || moves.value.some((m) => m.kind !== "opening"));
async function setArchived(archive: boolean) {
  if (!item.value) return;
  let reason = "";
  if (archive) {
    const r = await askReason({
      title: `Archive ${item.value.name}?`,
      message: "It leaves the shop list and can't be sold. Its sales and stock history stay. You can bring it back any time.",
      confirmLabel: "Archive",
      tone: "primary",
      suggestions: ["Discontinued", "Seasonal", "Replaced by a new item"],
      required: false,
    });
    if (r === null) return;
    reason = r;
  }
  try {
    await api.post(`/inventory/${id}/${archive ? "archive" : "unarchive"}`, { reason });
    await refresh();
    toast(archive ? "Item archived." : "Item is back in the shop.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't change the item."), "error");
  }
}
async function remove() {
  if (!item.value || !confirm(`Delete "${item.value.name}"? It has no sales or stock history, so nothing else changes.`)) return;
  try {
    await api.del(`/inventory/${id}`);
    invalidate("inventory");
    toast("Item deleted.", "success");
    router.replace(back.value);
  } catch (e) {
    toast(errMsg(e, "Couldn't delete the item."), "error");
  }
}

const actBtn = (k: Act) => btnClasses("ghost", "sm") + (act.value === k ? " !border-bronze !text-bronze" : "");
const margin = computed(() => (item.value?.costPrice ? item.value.price - item.value.costPrice : undefined));
const moveLink = (m: StockMovement) => (m.refType === "sale" && m.ref ? `/sales/${m.ref}` : "");
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back" class="inline-flex min-h-10 items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Back
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />
    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="reload">Retry</button>
    </Alert>

    <template v-else-if="item">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="font-display text-xl font-semibold">{{ item.name }}</h1>
            <p class="text-sm text-muted">
              {{ money(item.price) }} each<template v-if="item.sku"> · SKU {{ item.sku }}</template>
            </p>
            <div class="mt-1.5 flex flex-wrap gap-1.5">
              <Badge v-if="!item.active" tone="neutral">Archived</Badge>
              <Badge v-else-if="shortage(item) === 'out'" tone="overdue">Out of stock</Badge>
              <Badge v-else-if="shortage(item) === 'low'" tone="partial">Low stock</Badge>
            </div>
          </div>
          <div class="shrink-0 text-right">
            <p class="font-display text-3xl font-semibold tnum">{{ item.stock }}</p>
            <p class="text-xs text-faint">in stock<template v-if="item.lowStockThreshold"> · low at {{ item.lowStockThreshold }}</template></p>
          </div>
        </div>
        <p v-if="item.costPrice" class="mt-2 text-xs text-faint tnum">Cost {{ money(item.costPrice) }} · margin {{ money(margin ?? 0) }} per unit</p>

        <div class="mt-4 flex flex-wrap gap-2">
          <Button v-if="item.active && item.stock > 0" size="sm" :aria-expanded="act === 'sell'" @click="open('sell')">Sell</Button>
          <button :class="actBtn('restock')" :aria-expanded="act === 'restock'" @click="open('restock')"><PackagePlus class="h-4 w-4" /> Restock</button>
          <button :class="actBtn('damage')" :disabled="item.stock <= 0" :aria-expanded="act === 'damage'" @click="open('damage')"><PackageMinus class="h-4 w-4" /> Write off</button>
          <button :class="actBtn('correction')" :aria-expanded="act === 'correction'" @click="open('correction')"><ClipboardCheck class="h-4 w-4" /> Count</button>
          <button :class="actBtn('edit')" :aria-expanded="act === 'edit'" @click="open('edit')"><Pencil class="h-4 w-4" /> Edit</button>
        </div>

        <div v-if="act === 'sell'" class="mt-3">
          <SellSheet :item="item" :buyers="buyers" @sold="sold" @cancel="act = ''" />
        </div>

        <form v-else-if="act === 'restock' || act === 'damage' || act === 'correction'" class="mt-3 space-y-3 rounded-xl bg-elevated p-3" @submit.prevent="submitAct">
          <p class="text-sm font-medium">
            {{ act === "restock" ? "Stock delivered" : act === "damage" ? "Write off damaged or lost stock" : "Correct to what's on the shelf" }}
          </p>
          <div class="grid grid-cols-2 gap-2">
            <label v-if="act !== 'correction'" class="block">
              <span class="mb-1 block text-xs text-faint">{{ act === "restock" ? "Units received" : "Units written off" }}</span>
              <input v-model.number="a.qty" type="number" inputmode="numeric" min="1" :class="inputCls" />
            </label>
            <label v-else class="block">
              <span class="mb-1 block text-xs text-faint">Counted on the shelf</span>
              <input v-model.number="a.count" type="number" inputmode="numeric" min="0" :class="inputCls" />
            </label>
            <label v-if="act === 'restock'" class="block">
              <span class="mb-1 block text-xs text-faint">Cost per unit (optional)</span>
              <input v-model.number="a.unitCost" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" />
            </label>
          </div>
          <div v-if="act !== 'restock'" class="space-y-2">
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="r in act === 'damage' ? DAMAGE_REASONS : COUNT_REASONS"
                :key="r"
                type="button"
                class="min-h-10 rounded-lg border px-3 text-xs"
                :class="a.reason === r ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted hover:text-fg'"
                @click="a.reason = r"
              >{{ r }}</button>
            </div>
            <input v-model="a.reason" placeholder="Reason *" aria-label="Reason" :class="inputCls" />
          </div>
          <input v-else v-model="a.reason" placeholder="Note (optional), e.g. supplier or invoice" aria-label="Note" :class="inputCls" />
          <p class="text-xs text-faint tnum">{{ item.stock }} → <span class="font-medium text-fg">{{ after }}</span> in stock</p>
          <p v-if="actErr" class="text-sm text-overdue" role="alert">{{ actErr }}</p>
          <div class="flex gap-2">
            <Button type="submit" size="sm" :disabled="acting">{{ acting ? "Saving…" : act === "restock" ? "Add to stock" : act === "damage" ? "Write off" : "Save count" }}</Button>
            <Button size="sm" variant="ghost" @click="act = ''">Cancel</Button>
          </div>
        </form>

        <form v-else-if="act === 'edit'" class="mt-3 space-y-3 rounded-xl bg-elevated p-3" @submit.prevent="saveEdit">
          <div class="grid grid-cols-3 gap-2">
            <label class="col-span-2 block"><span class="mb-1 block text-xs text-faint">Name *</span><input v-model="ef.name" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">SKU</span><input v-model="ef.sku" :class="inputCls" /></label>
          </div>
          <div class="grid grid-cols-3 gap-2">
            <label class="block"><span class="mb-1 block text-xs text-faint">Price *</span><input v-model.number="ef.price" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Cost</span><input v-model.number="ef.costPrice" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" /></label>
            <label class="block"><span class="mb-1 block text-xs text-faint">Low at</span><input v-model.number="ef.lowStockThreshold" type="number" inputmode="numeric" min="0" :class="inputCls" /></label>
          </div>
          <p class="text-xs text-faint">A price change applies to new sales; past sales keep the price they were sold at. To change the stock, use Restock, Write off or Count.</p>
          <p v-if="actErr" class="text-sm text-overdue" role="alert">{{ actErr }}</p>
          <div class="flex gap-2">
            <Button type="submit" size="sm" :disabled="saving">{{ saving ? "Saving…" : "Save changes" }}</Button>
            <Button size="sm" variant="ghost" @click="act = ''">Cancel</Button>
          </div>
        </form>
      </Card>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Stock history</h2>
        <ul class="divide-y divide-line/60 rounded-2xl border border-line bg-surface">
          <li v-for="m in moves" :key="m.id" class="flex items-center gap-3 px-3 py-2.5">
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm">
                <RouterLink v-if="moveLink(m)" :to="withBack(moveLink(m), here)" class="font-medium hover:underline">{{ MOVE_LABEL[m.kind] }}</RouterLink>
                <span v-else class="font-medium">{{ MOVE_LABEL[m.kind] }}</span>
                <template v-if="m.reason"> · <span class="text-muted">{{ m.reason }}</span></template>
              </p>
              <p class="text-xs text-faint">{{ formatDateTime(m.at) }}<template v-if="m.actor"> · {{ m.actor }}</template></p>
            </div>
            <div class="text-right tnum">
              <p class="text-sm font-medium" :class="m.delta > 0 ? 'text-paid' : m.delta < 0 ? 'text-overdue' : ''">{{ m.delta > 0 ? "+" : "" }}{{ m.delta }}</p>
              <p class="text-xs text-faint">→ {{ m.stockAfter }}</p>
            </div>
          </li>
          <li v-if="!moves.length" class="px-3 py-3 text-sm text-faint">No stock history yet.</li>
        </ul>
      </section>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Sales · {{ salesTotal }}</h2>
        <ul v-if="sales.length" class="space-y-2">
          <li v-for="s in sales" :key="s.id">
            <RouterLink :to="withBack(`/sales/${s.id}`, here)" class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 hover:border-bronze/30">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">
                  ×{{ s.qty }} · {{ s.traineeName || "Walk-in" }}
                  <span v-if="s.voidedAt" class="ml-1 text-xs font-semibold uppercase text-overdue">Void</span>
                  <span v-else-if="s.returnedQty" class="ml-1 text-xs text-partial">{{ s.returnedQty }} returned</span>
                </p>
                <p class="text-xs text-faint">{{ formatLongDate(s.date) }}</p>
              </div>
              <span class="font-display text-sm tnum" :class="s.voidedAt ? 'line-through text-faint' : ''">{{ money(s.total - (s.returnedTotal ?? 0)) }}</span>
            </RouterLink>
          </li>
        </ul>
        <p v-else class="px-1 text-sm text-faint">No sales yet.</p>
        <button v-if="sales.length < salesTotal" class="min-h-10 px-1 text-sm font-medium text-bronze hover:underline" @click="loadSales(true)">Show more</button>
      </section>

      <Card class="space-y-3 p-4">
        <div class="flex flex-wrap gap-2">
          <button v-if="item.active" :class="btnClasses('ghost', 'sm')" @click="setArchived(true)"><Archive class="h-4 w-4" /> Archive</button>
          <button v-else :class="btnClasses('ghost', 'sm')" @click="setArchived(false)"><ArchiveRestore class="h-4 w-4" /> Back in the shop</button>
          <button v-if="!hasHistory" :class="btnClasses('danger', 'sm')" @click="remove"><Trash2 class="h-4 w-4" /> Delete</button>
        </div>
        <p class="text-xs text-faint">
          {{ hasHistory ? "Items with sales or stock history are archived, not deleted, so that history stays linked." : "Delete is only for an item added by mistake." }}
        </p>
        <AuditTrail entity="item" :id="id" />
      </Card>
    </template>
  </div>
</template>
