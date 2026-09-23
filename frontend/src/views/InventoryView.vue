<script setup lang="ts">
import { ref, reactive, computed } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { Plus, Package, ChevronRight, CircleCheck } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { useCachedAsync, invalidate } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { InventoryItem, Trainee, Sale } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import { useQueryState, withBack } from "@/lib/route-state";
import { shortage, byUrgency } from "@/lib/stock";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import SellSheet from "@/components/SellSheet.vue";
import Highlight from "@/components/ui/Highlight.vue";
import { fuzzyFilter } from "@/lib/fuzzy";
import { useServerList } from "@/lib/server-list";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { traineeOption } from "@/lib/options";

const route = useRoute();
const here = computed(() => route.fullPath);

const { data: itemData, loading, error, reload } = useCachedAsync("inventory", () => api.get<InventoryItem[]>("/inventory"));
const { data: trainees } = useCachedAsync("trainees", () => api.get<Trainee[]>("/trainees?archived=include"));
const items = computed(() => itemData.value ?? []);

type Show = "all" | "short" | "archived";
const show = useQueryState<Show>("show", () => "all", (v): v is Show => ["all", "short", "archived"].includes(v as string));
const activeItems = computed(() => items.value.filter((i) => i.active));
const shortCount = computed(() => activeItems.value.filter((i) => shortage(i)).length);
const archivedCount = computed(() => items.value.length - activeItems.value.length);

// One search over the shop: it narrows the stock list and the sales ledger
// together, so "tee" shows the item and every tee that went out the door.
const q = ref("");
const term = computed(() => q.value.trim());
const filteredItems = computed(() => {
  const base =
    show.value === "archived" ? items.value.filter((i) => !i.active)
    : show.value === "short" ? activeItems.value.filter((i) => shortage(i))
    : activeItems.value;
  // A search ranks by match; otherwise shortages come first.
  return term.value ? fuzzyFilter(base, term.value, (i) => [i.name, i.sku]) : [...base].sort(byUrgency);
});
const { page: itemPage, pageCount: itemPages, items: itemRows, total: itemTotal, from: itemFrom, to: itemTo } = usePaged(filteredItems, 10);
// Sales are searched and paged by the API, over every sale — not just a page.
const {
  items: saleRows, total: saleTotal, page: salePage, pageCount: salePages, from: saleFrom, to: saleTo,
  searching: salesSearching, reload: reloadSales,
} = useServerList<Sale>((s) => `/sales${s ? `?q=${encodeURIComponent(s)}` : ""}`, term, 10);

// Buyer picker: current trainees, searchable; "Walk-in" for no buyer.
const buyers = computed(() =>
  (trainees.value ?? []).filter((t) => t.status === "active" && !t.archivedAt).map(traineeOption),
);

// ── Add an item ────────────────────────────────────────────────────────────
const showAdd = ref(false);
const busy = ref(false);
const blank = () => ({ name: "", sku: "", price: 0, costPrice: 0, stock: 0, lowStockThreshold: 3 });
const form = reactive(blank());
const addErr = computed(() => (!form.name.trim() ? "Name the item." : !(form.price > 0) ? "Set a selling price." : form.stock < 0 ? "Stock can't be negative." : ""));
const addTried = ref(false);
async function addItem() {
  addTried.value = true;
  if (busy.value || addErr.value) return;
  busy.value = true;
  try {
    await api.post("/inventory", form);
    Object.assign(form, blank());
    addTried.value = false;
    showAdd.value = false;
    invalidate("inventory");
    await reload();
    toast("Item added.", "success");
  } catch (e) {
    toast(errMsg(e, "Couldn't add item."), "error");
  } finally {
    busy.value = false;
  }
}

// ── Sell ───────────────────────────────────────────────────────────────────
const sellFor = ref<string | null>(null);
const lastSale = ref<Sale>();
async function sold(sale: Sale) {
  sellFor.value = null;
  lastSale.value = sale;
  invalidate("inventory", "sales", "financials");
  await Promise.all([reload(), reloadSales()]);
  toast(`Sold ${sale.qty} × ${sale.itemName}.`, "success");
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Shop" title="Inventory">
      <template #action>
        <Button size="sm" @click="showAdd = !showAdd"><Plus class="h-4 w-4" /> Item</Button>
      </template>
    </PageHeader>

    <Card v-if="showAdd" class="space-y-3 p-4">
      <div class="grid grid-cols-3 gap-2">
        <label class="col-span-2 block"><span class="mb-1 block text-xs text-faint">Name *</span><input v-model="form.name" placeholder="e.g. 12oz gloves" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">SKU</span><input v-model="form.sku" placeholder="Optional" :class="inputCls" /></label>
      </div>
      <div class="grid grid-cols-4 gap-2">
        <label class="block"><span class="mb-1 block text-xs text-faint">Price *</span><input v-model.number="form.price" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Cost</span><input v-model.number="form.costPrice" type="number" inputmode="decimal" min="0" step="0.01" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Stock</span><input v-model.number="form.stock" type="number" inputmode="numeric" min="0" :class="inputCls" /></label>
        <label class="block"><span class="mb-1 block text-xs text-faint">Low at</span><input v-model.number="form.lowStockThreshold" type="number" inputmode="numeric" min="0" :class="inputCls" /></label>
      </div>
      <p v-if="addTried && addErr" class="text-xs text-overdue">{{ addErr }}</p>
      <div class="flex gap-2">
        <Button size="sm" :disabled="busy" @click="addItem">{{ busy ? "Adding…" : "Add item" }}</Button>
        <Button size="sm" variant="ghost" @click="showAdd = false">Cancel</Button>
      </div>
    </Card>

    <Skeleton v-if="loading" :rows="5" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="reload">Retry</button>
    </Alert>

    <template v-else>
      <SearchInput v-model="q" label="Search stock and sales" placeholder="Search stock and sales…" :searching="salesSearching"
        :matches="q && !salesSearching ? filteredItems.length + saleTotal : undefined" />
      <ChipGroup
        v-model="show"
        :options="[
          { v: 'all', l: 'In the shop', n: activeItems.length },
          { v: 'short', l: 'Needs restock', n: shortCount, tone: shortCount ? 'partial' : undefined },
          { v: 'archived', l: 'Archived', n: archivedCount || undefined },
        ]"
        label="Show"
        scroll
      />

      <div v-if="lastSale" class="flex items-center gap-2 rounded-xl border border-paid/40 bg-paid/10 px-3 py-2 text-sm" role="status">
        <CircleCheck class="h-4 w-4 shrink-0 text-paid" />
        <span class="min-w-0 flex-1 truncate">Sold {{ lastSale.qty }} × {{ lastSale.itemName }} · {{ money(lastSale.total) }}</span>
        <RouterLink :to="withBack(`/sales/${lastSale.id}/receipt`, here)" class="min-h-10 content-center font-medium text-bronze hover:underline">Receipt</RouterLink>
      </div>

      <EmptyState
        v-if="filteredItems.length === 0"
        :icon="Package"
        :title="q ? 'No matching items' : show === 'short' ? 'Nothing needs restocking' : show === 'archived' ? 'No archived items' : 'No items yet'"
        :description="q ? 'Try a different name or SKU.' : show === 'all' ? 'Add gloves, wraps, water, merch to track stock and sales.' : undefined"
      />

      <template v-else>
        <ul class="space-y-2">
          <li v-for="i in itemRows" :key="i.id" class="rounded-2xl border border-line bg-surface">
            <div class="flex items-center gap-2 pr-2">
              <RouterLink :to="withBack(`/inventory/${i.id}`, here)" class="flex min-w-0 flex-1 items-center gap-3 p-3">
                <div class="min-w-0 flex-1">
                  <p class="truncate font-medium"><Highlight :text="i.name" :q="q" /> <span v-if="i.sku" class="text-xs font-normal text-faint">· <Highlight :text="i.sku" :q="q" /></span></p>
                  <p class="text-xs text-faint tnum">{{ money(i.price) }} · <span :class="shortage(i) === 'out' ? 'text-overdue' : shortage(i) === 'low' ? 'text-partial' : ''">{{ i.stock }} in stock</span></p>
                </div>
                <Badge v-if="!i.active" tone="neutral">Archived</Badge>
                <Badge v-else-if="shortage(i) === 'out'" tone="overdue">Out</Badge>
                <Badge v-else-if="shortage(i) === 'low'" tone="partial">Low</Badge>
                <ChevronRight class="h-4 w-4 shrink-0 text-faint" />
              </RouterLink>
              <RouterLink v-if="i.active && shortage(i)" :to="withBack(`/inventory/${i.id}`, here, { act: 'restock' })" class="min-h-10 content-center px-2 text-sm font-medium text-bronze hover:underline">Restock</RouterLink>
              <Button v-if="i.active && i.stock > 0" size="sm" variant="ghost" :aria-expanded="sellFor === i.id" @click="sellFor = sellFor === i.id ? null : i.id">Sell</Button>
            </div>
            <div v-if="sellFor === i.id" class="px-3 pb-3">
              <SellSheet :item="i" :buyers="buyers" @sold="sold" @cancel="sellFor = null" />
            </div>
          </li>
        </ul>
        <Pagination v-model="itemPage" :page-count="itemPages" :total="itemTotal" :from="itemFrom" :to="itemTo" label="items" />
      </template>

      <section v-if="saleTotal > 0 || q" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Sales</h2>
        <p v-if="saleTotal === 0 && !salesSearching" class="px-1 text-sm text-faint">No sales match that.</p>
        <template v-else>
          <ul class="space-y-2">
            <li v-for="s in saleRows" :key="s.id">
              <RouterLink
                :to="withBack(`/sales/${s.id}`, here)"
                class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3 py-2.5 transition-colors hover:border-bronze/30"
              >
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">
                    <Highlight :text="s.itemName" :q="q" /> <span class="text-faint">×{{ s.qty }}</span>
                    <span v-if="s.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
                    <span v-else-if="s.returnedQty" class="ml-1 text-xs text-partial">{{ s.returnedQty }} returned</span>
                  </p>
                  <p class="truncate text-xs text-faint"><Highlight :text="s.traineeName || 'Walk-in'" :q="q" /> · {{ formatLongDate(s.date) }}</p>
                </div>
                <span class="font-display text-sm tnum" :class="s.voidedAt ? 'line-through text-faint' : ''">{{ money(s.total - (s.returnedTotal ?? 0)) }}</span>
              </RouterLink>
            </li>
          </ul>
          <Pagination v-model="salePage" :page-count="salePages" :total="saleTotal" :from="saleFrom" :to="saleTo" label="sales" />
        </template>
      </section>
    </template>
  </div>
</template>
