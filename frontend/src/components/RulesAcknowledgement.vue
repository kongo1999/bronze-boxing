<script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { RouterLink } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { StudioRules, RulesAcknowledgement } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { inputCls } from "@/lib/ui";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";

const props = defineProps<{ trainee: string }>();
const rules = ref<StudioRules>(); const rows = ref<RulesAcknowledgement[]>([]); const error = ref("");
const method = ref<RulesAcknowledgement["method"]>("signed_paper"); const note = ref(""); const busy = ref(false);
const current = computed(() => rows.value.find((a) => a.version === rules.value?.version));
async function load() {
  try { [rules.value, rows.value] = await Promise.all([api.get<StudioRules>("/studio-rules"), api.get<RulesAcknowledgement[]>(`/trainees/${props.trainee}/rules-acknowledgements`)]); error.value = ""; }
  catch (e) { error.value = errMsg(e, "Couldn't load agreement status."); }
}
onMounted(load);
async function record() {
  if (!rules.value?.version || busy.value) return;
  busy.value = true;
  try { await api.post(`/trainees/${props.trainee}/rules-acknowledgements`, { version: rules.value.version, method: method.value, note: note.value }); await load(); }
  catch (e) { error.value = errMsg(e, "Couldn't record acknowledgement."); }
  finally { busy.value = false; }
}
</script>

<template>
  <Card class="space-y-2 p-4">
    <div class="flex items-center justify-between gap-2"><h2 class="font-display text-lg font-semibold">Rules acknowledgement</h2><RouterLink to="/rules" class="text-xs font-medium text-bronze">Read rules</RouterLink></div>
    <p v-if="error" class="text-xs text-overdue">{{ error }}</p>
    <p v-if="!rules?.version" class="text-xs text-faint">Publish the studio rules before recording an acknowledgement.</p>
    <p v-else-if="current" class="text-xs text-paid">Version {{ current.version }} recorded {{ formatDateTime(current.acknowledgedAt) }} · {{ current.method.replaceAll('_', ' ') }}</p>
    <template v-else-if="rules?.version"><p class="text-xs text-partial">Version {{ rules.version }} has not been acknowledged by this member.</p><div class="grid grid-cols-2 gap-2 text-xs"><label>How confirmed<select v-model="method" :class="inputCls"><option value="signed_paper">Signed paper</option><option value="confirmed_message">Confirmed message</option><option value="verbal">Verbal</option></select></label><label>Record note<input v-model="note" :class="inputCls" placeholder="Optional reference" /></label></div><Button size="sm" :disabled="busy" @click="record">Record acknowledgement</Button></template>
    <details v-if="rows.some((a) => a.version !== rules?.version)" class="text-xs text-faint"><summary class="cursor-pointer">Earlier versions · {{ rows.length - Number(!!current) }}</summary><p v-for="a in rows.filter((x) => x.version !== rules?.version)" :key="a.id" class="mt-1">Version {{ a.version }} · {{ formatDateTime(a.acknowledgedAt) }}</p></details>
  </Card>
</template>
