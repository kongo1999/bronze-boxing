<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { PaymentPromise, SubStatus } from "@/lib/types";
import { money } from "@/lib/format";
import { todayKey } from "@/lib/studio";
import { withBack } from "@/lib/route-state";
import { traineeOption } from "@/lib/options";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import { fuzzyFilter } from "@/lib/fuzzy";

const props = defineProps<{ month: string; dues: SubStatus[] }>();
const route = useRoute();
const rows = ref<PaymentPromise[]>([]); const error = ref(""); const busy = ref(false);
const open = ref(false); const q = ref("");
const form = reactive({ trainee: "", amount: 0, dueDay: todayKey(), note: "" });
const available = computed(() => props.dues.filter((d) => d.remaining > 0 && d.state !== "unverified").map((d) => traineeOption(d.trainee)));
const due = computed(() => props.dues.find((d) => d.trainee.id === form.trainee));
const shown = computed(() => fuzzyFilter(rows.value, q.value, (p) => [p.traineeName, p.periodMonth, p.state, p.note]));
async function load() { try { rows.value = await api.get<PaymentPromise[]>(`/payment-promises?m=${props.month}`); error.value = ""; } catch (e) { error.value = errMsg(e); } }
watch(() => props.month, load, { immediate: true });
watch(due, (d) => { if (d) form.amount = d.remaining; });
async function save() {
  if (busy.value) return; busy.value = true;
  try { await api.post("/payment-promises", { ...form, periodMonth: props.month }); open.value = false; form.trainee = ""; form.amount = 0; form.note = ""; await load(); toast("Payment date saved.", "success"); }
  catch (e) { error.value = errMsg(e, "Couldn't save payment date."); }
  finally { busy.value = false; }
}
async function cancel(p: PaymentPromise) {
  if (!confirm(`Cancel ${p.traineeName}'s promise? The balance will still be owed.`)) return;
  try { await api.post(`/payment-promises/${p.id}/cancel`); await load(); }
  catch (e) { error.value = errMsg(e); }
}
</script>

<template>
  <Card class="space-y-3 p-4">
    <div class="flex items-center justify-between gap-2"><div><h2 class="font-display text-lg font-semibold">Payment promises</h2><p class="text-xs text-faint">Agreed next payment date; separate from money received.</p></div><Button size="sm" variant="ghost" @click="open = !open">{{ open ? 'Close' : '+ Promise' }}</Button></div>
    <p v-if="error" class="text-xs text-overdue">{{ error }}</p>
    <div v-if="open" class="space-y-2 rounded-xl bg-elevated p-3">
      <SearchSelect v-model="form.trainee" :options="available" placeholder="Choose someone who owes" search-placeholder="Search trainees…" />
      <p v-if="due" class="text-xs text-muted">{{ money(due.remaining) }} remains for {{ props.month }}</p>
      <div class="grid grid-cols-2 gap-2"><label class="text-xs">Agreed amount<input v-model.number="form.amount" :class="inputCls" type="number" min="0.01" :max="due?.remaining" step="0.01" /></label><label class="text-xs">Due on<input v-model="form.dueDay" :class="inputCls" type="date" :min="todayKey()" /></label></div>
      <label class="block text-xs">Note<input v-model="form.note" :class="inputCls" placeholder="Optional agreement detail" /></label>
      <Button size="sm" :disabled="busy || !form.trainee || form.amount <= 0 || form.amount > (due?.remaining ?? 0)" @click="save">Save promise</Button>
    </div>
    <SearchInput v-if="rows.length >= 5" v-model="q" label="Search promises" placeholder="Find a promise…" :matches="q ? shown.length : undefined" />
    <p v-if="!rows.length" class="text-xs text-faint">No payment dates agreed for this month.</p>
    <ul v-else class="space-y-2"><li v-for="p in shown" :key="p.id" class="rounded-xl border border-line p-3 text-sm">
      <div class="flex justify-between gap-2"><span class="font-medium">{{ p.traineeName }}</span><span class="text-xs capitalize" :class="p.state === 'overdue' ? 'text-overdue' : p.state === 'fulfilled' ? 'text-paid' : 'text-partial'">{{ p.state }}</span></div>
      <p class="text-xs text-muted">{{ money(p.amount) }} by {{ p.dueDay }} · {{ money(p.outstanding) }} of this promise left</p><p v-if="p.note" class="text-xs text-faint">{{ p.note }}</p>
      <div v-if="p.outstanding > 0" class="mt-2 flex gap-3 text-xs font-medium"><RouterLink :to="withBack('/payments/new', route.fullPath, { trainee: p.trainee, type: 'subscription', periodMonth: p.periodMonth, amount: String(p.outstanding) })" class="text-bronze">Collect</RouterLink><button class="text-faint underline" @click="cancel(p)">Cancel promise</button></div>
    </li></ul>
  </Card>
</template>
