<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { PaymentPromise } from "@/lib/types";
import { money } from "@/lib/format";
import { fuzzyFilter } from "@/lib/fuzzy";
import Card from "@/components/ui/Card.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
const props = defineProps<{ trainee: string }>();
const rows = ref<PaymentPromise[]>([]); const error = ref("");
const q = ref("");
const shown = computed(() => fuzzyFilter(rows.value, q.value, (p) => [p.periodMonth, p.dueDay, p.state, p.note]));
async function load() { try { rows.value = await api.get<PaymentPromise[]>(`/payment-promises?trainee=${props.trainee}`); error.value = ""; } catch (e) { error.value = errMsg(e); } }
watch(() => props.trainee, load, { immediate: true });
</script>
<template>
  <Card class="space-y-2 p-4">
    <div class="flex justify-between gap-2"><div><h2 class="font-display text-lg font-semibold">Payment dates agreed</h2><p class="text-xs text-faint">Promises are separate from payments received.</p></div><RouterLink to="/payments" class="text-xs font-medium text-bronze">Money</RouterLink></div>
    <p v-if="error" class="text-xs text-overdue">{{ error }}</p>
    <SearchInput v-if="rows.length >= 5" v-model="q" label="Search payment dates" placeholder="Find an agreement…" :matches="q ? shown.length : undefined" />
    <p v-if="!rows.length" class="text-xs text-faint">No agreed payment dates recorded.</p>
    <ul v-else class="space-y-1 text-xs"><li v-for="p in shown" :key="p.id" class="flex justify-between gap-2 border-t border-line pt-2"><div><p class="font-medium">{{ p.periodMonth }} · {{ p.dueDay }}</p><p class="text-muted">{{ money(p.amount) }} promised · {{ money(p.outstanding) }} left</p></div><span class="capitalize" :class="p.state === 'overdue' ? 'text-overdue' : p.state === 'fulfilled' ? 'text-paid' : 'text-partial'">{{ p.state.replace('_', ' ') }}</span></li></ul>
  </Card>
</template>
