<script setup lang="ts">
import { ref, reactive } from "vue";
import { Plus, Package } from "lucide-vue-next";
import { api } from "@/lib/api";
import { readCache, writeCache } from "@/lib/cache";
import type { InventoryItem, Trainee, Sale } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

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

async function addItem() {
  if (busy.value || !form.name.trim() || form.price <= 0) return;
  busy.value = true;
  try {
    await api.post("/inventory", form);
    Object.assign(form, { name: "", price: 0, stock: 0, lowStockThreshold: 3 });
    showAdd.value = false;
    await load();
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't add item.", "error");
  } finally {
    busy.value = false;
  }
}
function openSell(id: string) {
  sellFor.value = id;
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
      <Button size="sm" :disabled="busy || !form.name.trim() || form.price <= 0" @click="addItem">{{ busy ? "Adding…" : "Add item" }}</Button>
    </Card>

    <Skeleton v-if="loading" :rows="5" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="load">Retry</button>
    </Alert>

    <template v-else>
      <EmptyState v-if="items.length === 0" :icon="Package" title="No items yet" description="Add gloves, wraps, water, merch to track stock and sales." />

      <ul v-else class="space-y-2">
        <li v-for="i in items" :key="i.id" class="rounded-2xl border border-line bg-surface p-3">
          <div class="flex items-center gap-3">
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium">{{ i.name }}</p>
              <p class="text-xs text-faint">{{ money(i.price) }} · {{ i.stock }} in stock</p>
            </div>
            <Badge v-if="low(i)" tone="partial">Low</Badge>
            <Button size="sm" variant="ghost" :disabled="i.stock <= 0" @click="openSell(i.id)">Sell</Button>
          </div>
          <div v-if="sellFor === i.id" class="mt-3 space-y-2 rounded-xl bg-elevated p-3">
            <div class="grid grid-cols-2 gap-2">
              <label class="block"><span class="mb-1 block text-xs text-faint">Qty</span><input v-model.number="sell.qty" type="number" min="1" :max="i.stock" :class="inputCls" /></label>
              <label class="block">
                <span class="mb-1 block text-xs text-faint">Buyer</span>
                <select v-model="sell.trainee" :class="inputCls">
                  <option value="">Walk-in</option>
                  <option v-for="t in trainees" :key="t.id" :value="t.id">{{ t.name }}</option>
                </select>
              </label>
            </div>
            <p class="text-xs text-faint">Recorded as shop income (shows in Financials).</p>
            <div class="flex gap-2">
              <Button size="sm" :disabled="busy" @click="confirmSell(i)">{{ busy ? "Selling…" : `Confirm — ${money(i.price * sell.qty)}` }}</Button>
              <Button size="sm" variant="ghost" @click="sellFor = null">Cancel</Button>
            </div>
          </div>
        </li>
      </ul>

      <section v-if="sales.length > 0" class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Recent sales</h2>
        <ul class="space-y-2">
          <li v-for="s in sales.slice(0, 10)" :key="s.id" class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium">{{ s.itemName }} <span class="text-faint">×{{ s.qty }}</span></p>
              <p class="truncate text-xs text-faint">{{ s.traineeName || "Walk-in" }} · {{ formatLongDate(s.date) }}</p>
            </div>
            <span class="font-display text-sm tnum">{{ money(s.total) }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
