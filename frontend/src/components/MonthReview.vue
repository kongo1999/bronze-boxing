<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { MonthReview } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { inputCls } from "@/lib/ui";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
const props = defineProps<{ month: string }>();
const state = ref<MonthReview>(); const error = ref(""); const loading = ref(false); const busy = ref(false); const note = ref("");
const items = computed(() => state.value ? [
  { label: "Classes not marked done", value: state.value.issues.unclosedClasses, href: `/schedule?m=${props.month}` },
  { label: "Attendance still unmarked", value: state.value.issues.unmarkedAttendance, href: `/schedule?m=${props.month}` },
  { label: "Partial or unpaid dues", value: state.value.issues.unresolvedDues, href: `/payments?m=${props.month}` },
  { label: "Cash count differences", value: state.value.issues.cashDifferences, href: `/financials/reconcile?m=${props.month}` },
  { label: "Items low or out of stock now", value: state.value.issues.lowStock, href: "/inventory?show=short" },
] : []);
async function load() { loading.value = true; try { state.value = await api.get<MonthReview>(`/month-review?m=${props.month}`); error.value = ""; note.value = state.value.review?.note ?? ""; } catch (e) { error.value = errMsg(e); } finally { loading.value = false; } }
watch(() => props.month, load, { immediate: true });
async function review() { if (busy.value) return; busy.value = true; try { await api.post("/month-review", { month: props.month, note: note.value }); await load(); } catch (e) { error.value = errMsg(e); } finally { busy.value = false; } }
</script>
<template>
  <Card class="space-y-3 p-4" data-print-hide>
    <div><h2 class="font-display text-lg font-semibold">Month-end check</h2><p class="text-xs text-faint">Review loose ends before relying on the report. Counts update as records change.</p></div>
    <p v-if="loading" class="text-xs text-faint">Checking…</p><p v-if="error" class="text-xs text-overdue">{{ error }} <button class="underline" @click="load">Retry</button></p>
    <ul v-if="state" class="space-y-1 text-sm"><li v-for="item in items" :key="item.label" class="flex justify-between gap-2"><RouterLink :to="item.href" class="text-muted hover:text-bronze">{{ item.label }}</RouterLink><span class="tnum" :class="item.value ? 'text-partial' : 'text-paid'">{{ item.value }}</span></li></ul>
    <p v-if="state?.review" class="text-xs text-paid">Reviewed {{ formatDateTime(state.review.reviewedAt) }} by {{ state.review.reviewedBy }}</p>
    <label class="block text-xs">Review note<input v-model="note" :class="inputCls" placeholder="Optional context for this month" /></label>
    <Button size="sm" :disabled="busy || !state" @click="review">{{ state?.review ? 'Update review' : 'Mark month reviewed' }}</Button>
  </Card>
</template>
