<script setup lang="ts">
import { ref, reactive, computed } from "vue";
import { RouterLink } from "vue-router";
import { Plus, Package, Pencil, Trash2 } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache, invalidate } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { InventoryItem, Trainee, Sale } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { askReason } from "@/lib/prompt";
import { errMsg } from "@/lib/api";

const items = ref<InventoryItem[]>([]);
const trainees = ref<Trainee[]>([]);
const sales = ref<Sale[]>([]);
const showAdd = ref(false);
const busy = ref(false); // guards against double-submit (double-sell / duplicate item)
const loading = ref(false);
const error = ref<string>();
let loaded = false;
const form = reactive({ name: "", price: 0, stock: 0, lowStockThreshold: 3 });

// Per-item sell controls
const sellFor = ref<string | null>(null);
const sell = reactive({ qty: 1, trainee: "" });

const CACHE_KEY = "inventory:all";
type InvBundle = { items: InventoryItem[]; trainees: Trainee[]; sales: Sale[] };

// Cache-first: paint instantly from cache on revisit, then revalidate.
function showCached(): boolean {
  const hit = readCache<InvBundle>(CACHE_KEY);
  if (hit) {
    items.value = hit.items;
    trainees.value = hit.trainees;
    sales.value = hit.sales;
    loaded = true;
  }
  return !!hit;
}
async function load() {
  error.value = undefined;
  try {
    const [i, t, s] = await Promise.all([
      api.get<InventoryItem[]>("/inventory"),
      api.get<Trainee[]>("/trainees"),
      api.get<Sale[]>("/sales"),
    ]);
    items.value = i;
    trainees.value = t;
    sales.value = s;
    writeCache(CACHE_KEY, { items: i, trainees: t, sales: s });
    loaded = true;
  } catch (e) {
    if (loaded) toast("Couldn't refresh — showing saved data.", "error");
    else error.value = e instanceof Error && e.message ? e.message : "Couldn't load inventory.";
  } finally {
    loading.value = false;
  }
}
loading.value = !showCached();
load();

// One search over the shop: it narrows the stock list and the sales ledger
// together, so "tee" shows the item and every tee that went out the door.
const q = ref("");
const term = computed(() => q.value.trim().toLowerCase());
const filteredItems = computed(() =>
  term.value
    ? items.value.filter(
        (i) => i.name.toLowerCase().includes(term.value) || (i.sku ?? "").toLowerCase().includes(term.value),
      )
    : items.value,
);
const filteredSales = computed(() =>
  term.value
    ? sales.value.filter(
        (s) =>
          s.itemName.toLowerCase().includes(term.value) ||
          (s.traineeName ?? "walk-in").toLowerCase().includes(term.value),
      )
    : sales.value,
);
const { page: itemPage, pageCount: itemPages, items: itemRows, total: itemTotal, from: itemFrom, to: itemTo } =
  usePaged(filteredItems, 10);
const { page: salePage, pageCount: salePages, items: saleRows, total: saleTotal, from: saleFrom, to: saleTo } =
  usePaged(filteredSales, 10);

// Buyer picker options — active trainees, searchable, "Walk-in" for no buyer.
const traineeOptions = computed(() =>
  trainees.value.filter((t) => t.status === "active").map((t) => ({ id: t.id, label: t.name })),
);

function resetForm() {
  Object.assign(form, { name: "", price: 0, stock: 0, lowStockThreshold: 3 });
}
function cancelAdd() {
  showAdd.value = false;
  resetForm();
}
async function addItem() {
  if (busy.value || !form.name.trim() || form.price <= 0) return;
  busy.value = true;
  try {
    await api.post("/inventory", form);
    resetForm();
    showAdd.value = false;
    await load();
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't add item.", "error");
  } finally {
    busy.value = false;
  }
}

// Per-item inline edit.
const editFor = ref<string | null>(null);
const savingEdit = ref(false);
const editForm = reactive({ name: "", price: 0, stock: 0, lowStockThreshold: 3 });
function openEdit(i: InventoryItem) {
  editFor.value = i.id;
  sellFor.value = null; // don't stack two panels on one item
  Object.assign(editForm, {
    name: i.name,
    price: i.price,
    stock: i.stock,
    lowStockThreshold: i.lowStockThreshold ?? 0,
  });
}
async function saveEdit(i: InventoryItem) {
  if (savingEdit.value || !editForm.name.trim() || editForm.price <= 0) return;
  // Changing the count is a stock correction: it goes in the stock ledger
  // with a reason, so the ledger keeps adding up to what's on the shelf.
  let reason = "";
  if (editForm.stock !== i.stock) {
    const r = await askReason({
      title: "Correct the stock count?",
      message: `${i.name}: ${i.stock} → ${editForm.stock} in stock. Recorded as a correction in the item's stock history.`,
      confirmLabel: "Save correction",
      tone: "primary",
      suggestions: ["Counted the shelf", "Damaged", "Found extra"],
    });
    if (r === null) return;
    reason = r;
  }
  savingEdit.value = true;
  try {
    // Full-replace endpoint: carry over fields the form doesn't expose so they
    // aren't blanked.
    await api.put(`/inventory/${i.id}`, {
      ...editForm,
      sku: i.sku ?? "",
      costPrice: i.costPrice ?? 0,
      active: i.active,
      reason,
    });
    editFor.value = null;
    invalidate("inventory", "sales", "financials");
    await load();
    toast("Item updated.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't update item."), "error");
  } finally {
    savingEdit.value = false;
  }
}

// Delete removes the item from the shop list. Past sales keep their own copy of
// the name and price, so the books are unaffected — say so, since "delete" on a
// money-adjacent record reasonably makes people nervous.
const deletingId = ref<string | null>(null);
async function removeItem(i: InventoryItem) {
  if (deletingId.value) return;
  if (!confirm(`Delete "${i.name}" from the shop? Sales already recorded keep their history and totals.`)) return;
  deletingId.value = i.id;
  try {
    await api.del(`/inventory/${i.id}`);
    invalidate("inventory", "sales", "financials");
    await load();
    toast("Item deleted.", "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't delete item.", "error");
  } finally {
    deletingId.value = null;
  }
}

function openSell(id: string) {
  sellFor.value = id;
  editFor.value = null;
  Object.assign(sell, { qty: 1, trainee: "" });
}
async function confirmSell(item: InventoryItem) {
  if (busy.value) return; // prevent double-sell on rapid clicks
  busy.value = true;
  const qty = sell.qty;
  try {
    await api.post(`/inventory/${item.id}/sell`, {
      qty: sell.qty,
      trainee: sell.trainee || undefined,
    });
    sellFor.value = null;
    invalidate("inventory", "sales", "financials");
    await load();
    toast(`Sold ${qty} × ${item.name}.`, "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't record sale.", "error");
  } finally {
    busy.value = false;
  }
}
const low = (i: InventoryItem) => i.lowStockThreshold && i.stock <= i.lowStockThreshold;
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Shop" title="Inventory">
      <template #action>
        <Button size="sm" @click="showAdd = !showAdd"><Plus class="h-4 w-4" /> Item</Button>
      </template>
    </PageHeader>

    <Card v-if="showAdd" class="space-y-3 p-4">
      <input v-model="form.name" placeholder="Item name" :class="inputCls" />
      <div class="grid grid-cols-3 gap-2">
        <label class="block"><span class="mb-1 block text-xs text-faint">Price</span><input v-model.number="form.price" type="number" min="0" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Stock</span><input v-model.number="form.stock" type="number" min="0" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Low at</span><input v-model.number="form.lowStockThreshold" type="number" min="0" :class="inputCls" /></label>
      </div>
      <div class="flex gap-2">
        <Button size="sm" :disabled="busy || !form.name.trim() || form.price <= 0" @click="addItem">{{ busy ? "Adding…" : "Add item" }}</Button>
        <Button size="sm" variant="ghost" @click="cancelAdd">Cancel</Button>
      </div>
    </Card>

    <Skeleton v-if="loading" :rows="5" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else>
      <SearchInput v-model="q" placeholder="Search stock and sales…" />

      <EmptyState
        v-if="filteredItems.length === 0"
        :icon="Package"
        :title="q ? 'No matching items' : 'No items yet'"
        :description="q ? 'Try a different name or SKU.' : 'Add gloves, wraps, water, merch to track stock and sales.'"
      />

      <template v-else>
        <ul class="space-y-2">
          <li v-for="i in itemRows" :key="i.id" class="rounded-2xl border border-line bg-surface p-3">
            <div class="flex items-center gap-3">
              <div class="min-w-0 flex-1">
                <p class="truncate font-medium">{{ i.name }}</p>
                <p class="text-xs text-faint">{{ money(i.price) }} · {{ i.stock }} in stock</p>
              </div>
              <Badge v-if="low(i)" tone="partial">Low</Badge>
              <button class="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-purple transition-colors hover:bg-purple/10" aria-label="Edit item" @click="openEdit(i)"><Pencil class="h-4 w-4" /></button>
              <button
                class="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-faint transition-colors hover:bg-overdue/10 hover:text-overdue disabled:opacity-40"
                :disabled="deletingId === i.id"
                aria-label="Delete item"
                @click="removeItem(i)"
              ><Trash2 class="h-4 w-4" /></button>
              <Button size="sm" variant="ghost" :disabled="i.stock <= 0" @click="openSell(i.id)">Sell</Button>
            </div>

            <div v-if="editFor === i.id" class="mt-3 space-y-3 rounded-xl bg-elevated p-3">
              <input v-model="editForm.name" placeholder="Item name" :class="inputCls" />
              <div class="grid grid-cols-3 gap-2">
                <label class="block"><span class="mb-1 block text-xs text-faint">Price</span><input v-model.number="editForm.price" type="number" min="0" :class="inputCls" /></label>
                <label class="block"><span class="mb-1 block text-xs text-faint">Stock</span><input v-model.number="editForm.stock" type="number" min="0" :class="inputCls" /></label>
                <label class="block"><span class="mb-1 block text-xs text-faint">Low at</span><input v-model.number="editForm.lowStockThreshold" type="number" min="0" :class="inputCls" /></label>
              </div>
              <div class="flex gap-2">
                <Button size="sm" :disabled="savingEdit || !editForm.name.trim() || editForm.price <= 0" @click="saveEdit(i)">{{ savingEdit ? "Saving…" : "Save changes" }}</Button>
                <Button size="sm" variant="ghost" @click="editFor = null">Cancel</Button>
              </div>
            </div>
            <div v-if="sellFor === i.id" class="mt-3 space-y-2 rounded-xl bg-elevated p-3">
              <div class="grid grid-cols-2 gap-2">
                <label class="block"><span class="mb-1 block text-xs text-faint">Qty</span><input v-model.number="sell.qty" type="number" min="1" :max="i.stock" :class="inputCls" /></label>
                <div>
                  <span class="mb-1 block text-xs text-faint">Buyer</span>
                  <SearchSelect v-model="sell.trainee" :options="traineeOptions" empty-label="Walk-in" placeholder="Walk-in" search-placeholder="Search trainees…" />
                </div>
              </div>
              <p class="text-xs text-faint">Recorded as shop income (shows in Financials).</p>
              <div class="flex gap-2">
                <Button size="sm" :disabled="busy" @click="confirmSell(i)">{{ busy ? "Selling…" : `Confirm — ${money(i.price * sell.qty)}` }}</Button>
                <Button size="sm" variant="ghost" @click="sellFor = null">Cancel</Button>
              </div>
            </div>
          </li>
        </ul>
        <Pagination v-model="itemPage" :page-count="itemPages" :total="itemTotal" :from="itemFrom" :to="itemTo" label="items" />
      </template>

      <section v-if="sales.length > 0" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Sales</h2>
        <p v-if="filteredSales.length === 0" class="px-1 text-sm text-faint">No sales match that.</p>
        <template v-else>
          <ul class="space-y-2">
            <li v-for="s in saleRows" :key="s.id">
              <RouterLink
                :to="`/sales/${s.id}`"
                class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
              >
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">
                    {{ s.itemName }} <span class="text-faint">×{{ s.qty }}</span>
                    <span v-if="s.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                  </p>
                  <p class="truncate text-xs text-faint">{{ s.traineeName || "Walk-in" }} · {{ formatLongDate(s.date) }}</p>
                </div>
                <span class="font-display text-sm tnum" :class="s.voidedAt ? 'line-through text-faint' : ''">{{ money(s.total) }}</span>
              </RouterLink>
            </li>
          </ul>
          <Pagination v-model="salePage" :page-count="salePages" :total="saleTotal" :from="saleFrom" :to="saleTo" label="sales" />
        </template>
      </section>
    </template>
  </div>
</template>
