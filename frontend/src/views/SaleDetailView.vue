<script setup lang="ts">
import { ref, reactive, computed } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Undo2 } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync } from "@/lib/cache";
import type { Sale, Trainee } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;

const { data: sale, loading, error, reload } = useCachedAsync(`/sales/${id}`, () => api.get<Sale>(`/sales/${id}`));
const { data: trainees } = useCachedAsync("/trainees", () => api.get<Trainee[]>("/trainees"));

// ── Edit (correct qty / buyer) — PUT /sales/:id adjusts stock by the delta ──
const editing = ref(false);
const saving = ref(false);
const form = reactive({ qty: 1, trainee: "" as string | undefined });
function startEdit() {
  if (!sale.value) return;
  form.qty = sale.value.qty;
  form.trainee = sale.value.trainee ?? "";
  editing.value = true;
}
const newTotal = computed(() => (sale.value ? sale.value.unitPrice * (form.qty || 0) : 0));
async function save() {
  if (saving.value || !sale.value || form.qty <= 0) return;
  saving.value = true;
  try {
    await api.put(`/sales/${id}`, { qty: form.qty, trainee: form.trainee || "" });
    editing.value = false;
    await reload();
    toast("Sale updated.", "success");
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : "Couldn't update sale.", "error");
  } finally {
    saving.value = false;
  }
}

// ── Void — DELETE /sales/:id restocks then returns to the list ──────────────
const voiding = ref(false);
async function voidSale() {
  if (voiding.value || !sale.value) return;
  if (!confirm(`Void this sale? ${sale.value.qty} × ${sale.value.itemName} will be put back in stock.`)) return;
  voiding.value = true;
  try {
    await api.del(`/sales/${id}`);
    toast("Sale voided and restocked.", "success");
    router.push("/inventory");
  } catch (e) {
    voiding.value = false;
    toast(e instanceof Error && e.message ? e.message : "Couldn't void sale.", "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/inventory" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Inventory
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />
    <Alert v-else-if="error">{{ error }}</Alert>

    <template v-else-if="sale">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <h1 class="font-display text-xl font-semibold">{{ sale.itemName }}</h1>
            <p class="text-sm text-muted">{{ sale.traineeName || "Walk-in" }} · {{ formatLongDate(sale.date) }}</p>
          </div>
          <span class="font-display text-lg tnum">{{ money(sale.total) }}</span>
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

        <div v-if="!editing" class="mt-4 flex gap-2">
          <Button size="sm" @click="startEdit"><Pencil class="h-4 w-4" /> Edit sale</Button>
          <button
            :disabled="voiding"
            class="inline-flex h-9 items-center gap-1.5 rounded-xl border border-purple/40 px-3 text-sm font-medium text-purple transition-colors hover:bg-purple/10 disabled:opacity-50"
            @click="voidSale"
          >
            <Undo2 class="h-4 w-4" /> {{ voiding ? "Voiding…" : "Void" }}
          </button>
        </div>

        <div v-else class="mt-4 space-y-3 border-t border-line pt-4">
          <div class="grid grid-cols-2 gap-3">
            <label class="block"><span class="mb-1 block text-xs text-faint">Quantity</span><input v-model.number="form.qty" type="number" min="1" :class="inputCls" /></label>
            <label class="block">
              <span class="mb-1 block text-xs text-faint">Buyer</span>
              <select v-model="form.trainee" :class="inputCls">
                <option value="">Walk-in</option>
                <option v-for="t in (trainees ?? [])" :key="t.id" :value="t.id">{{ t.name }}</option>
              </select>
            </label>
          </div>
          <p class="text-xs text-faint">New total <span class="tnum text-fg">{{ money(newTotal) }}</span> · stock adjusts by the quantity change.</p>
          <div class="flex gap-2">
            <Button size="sm" :disabled="saving || form.qty <= 0" @click="save">{{ saving ? "Saving…" : "Save changes" }}</Button>
            <Button size="sm" variant="ghost" @click="editing = false">Cancel</Button>
          </div>
        </div>
      </Card>
    </template>

    <Alert v-else>Sale not found.</Alert>
  </div>
</template>
