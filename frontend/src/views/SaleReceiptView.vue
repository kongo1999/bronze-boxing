<script setup lang="ts">
// A receipt for one shop sale (GET /sales/:id/receipt), with any returns
// and the net paid. Printable (app chrome hides in print) and shareable as
// text; a voided sale's receipt is marked VOID.
import { computed, onMounted, ref } from "vue";
import { useRoute, RouterLink } from "vue-router";
import { ChevronLeft, Printer, Share2 } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import type { SaleReceipt } from "@/lib/types";
import { money, formatDateTime } from "@/lib/format";
import { formatDay, dayOf } from "@/lib/studio";
import { backTarget } from "@/lib/route-state";
import { methodLabel } from "@/lib/labels";
import logoUrl from "@/assets/logo.png";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import { btnClasses } from "@/components/ui/button";
import { toast } from "@/lib/toast";

const route = useRoute();
const id = route.params.id as string;
const rc = ref<SaleReceipt>();
const error = ref<string>();
const loading = ref(true);

onMounted(async () => {
  try {
    rc.value = await api.get<SaleReceipt>(`/sales/${id}/receipt`);
  } catch (e) {
    error.value = errMsg(e, "Couldn't load the receipt.");
  } finally {
    loading.value = false;
  }
});

const back = computed(() => backTarget(route.query, `/sales/${id}`));
const s = computed(() => rc.value?.sale);
const longDay = (day: string) => formatDay(day, { month: "long", day: "numeric", year: "numeric" });

function asText(): string {
  const r = rc.value;
  if (!r || !s.value) return "";
  const lines = [
    `${r.studioInfo.name} — Receipt #${r.number}${r.void ? " (VOID)" : ""}`,
    `${s.value.qty} × ${s.value.itemName} @ ${money(s.value.unitPrice)} = ${money(s.value.total)}`,
    `Sold to: ${s.value.traineeName || "Walk-in"}`,
    `Paid on: ${longDay(r.cashDay)} · ${methodLabel(r.method)}`,
  ];
  for (const x of r.returns) lines.push(`Returned ${x.qty} on ${longDay(dayOf(x.date))}: −${money(x.amount)}`);
  if (r.returns.length) lines.push(`Net paid: ${money(r.net)}`);
  if (r.void) lines.push(`VOID: ${s.value.voidReason || "voided"} — this receipt is not valid.`);
  return lines.join("\n");
}
function printIt() {
  window.print();
}
async function share() {
  const text = asText();
  try {
    if (navigator.share) {
      await navigator.share({ title: `Receipt #${rc.value?.number}`, text });
      return;
    }
    await navigator.clipboard.writeText(text);
    toast("Receipt copied — paste it into a message.", "success");
  } catch (e) {
    if ((e as Error)?.name !== "AbortError") toast("Couldn't share the receipt.", "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-2" data-print-hide>
      <RouterLink :to="back" class="inline-flex min-h-10 items-center gap-1 text-sm text-muted hover:text-fg">
        <ChevronLeft class="h-4 w-4" /> Back
      </RouterLink>
      <div v-if="rc" class="flex gap-2">
        <button :class="btnClasses('ghost', 'sm')" @click="share"><Share2 class="h-4 w-4" /> Share</button>
        <button :class="btnClasses('primary', 'sm')" @click="printIt"><Printer class="h-4 w-4" /> Print</button>
      </div>
    </div>

    <Skeleton v-if="loading" variant="card" />
    <Alert v-else-if="error">{{ error }}</Alert>

    <article
      v-else-if="rc && s"
      class="receipt relative overflow-hidden rounded-2xl bg-[oklch(0.985_0.004_85)] p-6 text-[oklch(0.2_0.01_285)] shadow-[var(--shadow-lift)]"
      :aria-label="`Receipt ${rc.number}${rc.void ? ', void' : ''}`"
    >
      <div v-if="rc.void" class="pointer-events-none absolute inset-0 grid place-items-center" aria-hidden="true">
        <span class="-rotate-[24deg] rounded-xl border-[6px] border-[oklch(0.55_0.2_26)] px-6 py-1 font-display text-7xl font-bold tracking-widest text-[oklch(0.55_0.2_26)] opacity-35">VOID</span>
      </div>

      <header class="flex items-center gap-3 border-b border-[oklch(0.85_0.01_85)] pb-4">
        <img :src="logoUrl" alt="" class="h-12 w-auto" />
        <div class="min-w-0">
          <p class="font-display text-lg font-semibold leading-tight">{{ rc.studioInfo.name }}</p>
          <p v-if="rc.studioInfo.address" class="text-xs opacity-70">{{ rc.studioInfo.address }}</p>
          <p v-if="rc.studioInfo.phone" class="text-xs opacity-70">{{ rc.studioInfo.phone }}</p>
        </div>
        <div class="ml-auto text-right">
          <p class="label-eyebrow text-[0.625rem] opacity-60">Receipt</p>
          <p class="whitespace-nowrap font-display font-semibold tnum">#{{ rc.number }}</p>
        </div>
      </header>

      <p v-if="rc.void" class="mt-4 rounded-lg bg-[oklch(0.55_0.2_26/0.12)] px-3 py-2 text-sm font-medium text-[oklch(0.45_0.18_26)]">
        This receipt is void<template v-if="s.voidReason"> — {{ s.voidReason }}</template>. The sale no longer counts.
      </p>

      <table class="mt-4 w-full text-sm">
        <thead>
          <tr class="text-left text-xs opacity-60"><th class="font-normal">Item</th><th class="text-right font-normal">Qty</th><th class="text-right font-normal">Price</th><th class="text-right font-normal">Total</th></tr>
        </thead>
        <tbody>
          <tr>
            <td class="py-1 font-medium">{{ s.itemName }}</td>
            <td class="py-1 text-right tnum">{{ s.qty }}</td>
            <td class="py-1 text-right tnum">{{ money(s.unitPrice) }}</td>
            <td class="py-1 text-right tnum">{{ money(s.total) }}</td>
          </tr>
          <tr v-for="r in rc.returns" :key="r.id" class="opacity-80">
            <td class="py-1">Returned · {{ formatDay(dayOf(r.date), { month: "short", day: "numeric" }) }}</td>
            <td class="py-1 text-right tnum">−{{ r.qty }}</td>
            <td></td>
            <td class="py-1 text-right tnum">−{{ money(r.amount) }}</td>
          </tr>
        </tbody>
      </table>

      <dl class="mt-4 space-y-2 text-sm">
        <div class="flex justify-between gap-4"><dt class="opacity-60">Sold to</dt><dd class="text-right font-medium">{{ s.traineeName || "Walk-in" }}</dd></div>
        <div class="flex justify-between gap-4"><dt class="opacity-60">Paid on</dt><dd class="text-right">{{ longDay(rc.cashDay) }}</dd></div>
        <div class="flex justify-between gap-4"><dt class="opacity-60">Method</dt><dd class="text-right">{{ methodLabel(rc.method) }}</dd></div>
      </dl>

      <div class="mt-5 flex items-baseline justify-between border-t border-[oklch(0.85_0.01_85)] pt-4">
        <span class="label-eyebrow text-xs opacity-60">{{ rc.returns.length ? "Net paid" : "Total" }}</span>
        <span class="font-display text-3xl font-semibold tnum" :class="rc.void ? 'line-through opacity-50' : ''">{{ money(rc.net) }}</span>
      </div>

      <footer class="mt-6 text-[0.6875rem] opacity-50">
        Sale {{ s.id }} · issued {{ formatDateTime(rc.issued) }} ({{ rc.timezone }})
      </footer>
    </article>
  </div>
</template>
