<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { api, errMsg } from "@/lib/api";
import type { StudioRules, StudioPolicy } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";

const rules = ref<StudioRules>();
const policy = reactive<StudioPolicy>({ id: "default", cancelBeforeHours: 12, lateCancellationAction: "allow" });
const draft = reactive({ groupPolicy: "", privatePolicy: "", clientAgreement: "" });
const editing = ref(false); const loading = ref(true); const saving = ref(false); const error = ref("");
async function load() {
  loading.value = true; error.value = "";
  try {
    const [r, p] = await Promise.all([api.get<StudioRules>("/studio-rules"), api.get<StudioPolicy>("/studio-policy")]);
    rules.value = r; Object.assign(policy, p);
    Object.assign(draft, { groupPolicy: r.groupPolicy ?? "", privatePolicy: r.privatePolicy ?? "", clientAgreement: r.clientAgreement ?? "" });
  } catch (e) { error.value = errMsg(e, "Couldn't load the studio rules."); }
  finally { loading.value = false; }
}
onMounted(load);
async function publish() {
  if (saving.value) return;
  saving.value = true; error.value = "";
  try { rules.value = await api.put<StudioRules>("/studio-rules", draft); editing.value = false; toast(`Rules version ${rules.value.version} published.`, "success"); }
  catch (e) { error.value = errMsg(e, "Couldn't publish the rules."); }
  finally { saving.value = false; }
}
async function saveCancellation() {
  try { await api.put("/studio-policy", policy); toast("Cancellation rule saved.", "success"); }
  catch (e) { error.value = errMsg(e, "Couldn't save the cancellation rule."); }
}
function printRules() { window.print(); }
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Studio" title="Rules & regulations" />
    <Alert v-if="error">{{ error }} <button class="underline" @click="load">Retry</button></Alert>
    <p v-if="loading" class="text-sm text-muted">Loading rules…</p>
    <template v-else>
      <div class="flex flex-wrap items-center justify-between gap-2" data-print-hide><p class="text-xs text-muted">Published wording is versioned. A member's acknowledgement stays tied to the version they saw.</p><div class="flex gap-2"><Button size="sm" variant="ghost" @click="printRules">Print / save PDF</Button><Button size="sm" @click="editing = !editing">{{ editing ? 'Close editor' : 'Edit wording' }}</Button></div></div>
      <Card v-if="editing" class="space-y-3 p-4" data-print-hide>
        <p class="text-xs text-muted">Use the club's approved wording. Saving a change publishes a new version; existing member acknowledgements remain on their earlier version.</p>
        <label class="block text-xs">Group class policy<textarea v-model="draft.groupPolicy" :class="inputCls" rows="6" placeholder="Group attendance, payment, cancellation and conduct…" /></label>
        <label class="block text-xs">Private class policy<textarea v-model="draft.privatePolicy" :class="inputCls" rows="6" placeholder="Private booking, package, rescheduling and cancellation…" /></label>
        <label class="block text-xs">Client agreement<textarea v-model="draft.clientAgreement" :class="inputCls" rows="8" placeholder="The agreement clients review and accept…" /></label>
        <Button size="sm" :disabled="saving" @click="publish">Publish new version</Button>
      </Card>
      <Card class="space-y-3 p-4" data-print-hide>
        <h2 class="font-display text-lg font-semibold">Cancellation setting</h2>
        <div class="grid grid-cols-2 gap-2 text-xs"><label>Late within hours<input v-model.number="policy.cancelBeforeHours" :class="inputCls" type="number" min="0" max="168" /></label><label>Late cancellation<select v-model="policy.lateCancellationAction" :class="inputCls"><option value="allow">Release place</option><option value="no_show">No-show after class starts</option></select></label></div>
        <Button size="sm" variant="ghost" @click="saveCancellation">Save cancellation rule</Button>
      </Card>
      <article class="space-y-5 rounded-2xl border border-line bg-surface p-5">
        <div class="border-b border-line pb-3"><h2 class="font-display text-2xl font-semibold">Bronze Boxing Club</h2><p class="text-xs text-faint">Rules & regulations · Version {{ rules?.version || 'not published' }}<template v-if="rules?.updatedAt"> · {{ formatDateTime(rules.updatedAt) }}</template></p></div>
        <section><h3 class="font-display text-lg font-semibold">Group classes</h3><p class="mt-1 whitespace-pre-wrap text-sm text-muted">{{ rules?.groupPolicy || 'No group policy published yet.' }}</p></section>
        <section><h3 class="font-display text-lg font-semibold">Private classes</h3><p class="mt-1 whitespace-pre-wrap text-sm text-muted">{{ rules?.privatePolicy || 'No private policy published yet.' }}</p></section>
        <section><h3 class="font-display text-lg font-semibold">Client agreement</h3><p class="mt-1 whitespace-pre-wrap text-sm text-muted">{{ rules?.clientAgreement || 'No client agreement published yet.' }}</p></section>
      </article>
    </template>
  </div>
</template>
