<script setup lang="ts">
import { computed, ref, reactive, onMounted } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { TrialLead, Trainee } from "@/lib/types";
import { fuzzyFilter } from "@/lib/fuzzy";
import { todayKey } from "@/lib/studio";
import { whatsappHref } from "@/lib/phone";
import { traineeOption } from "@/lib/options";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";

const route = useRoute();
const rows = ref<TrialLead[]>([]);
const trainees = ref<Trainee[]>([]);
const error = ref(""); const busy = ref(false); const adding = ref(false);
const q = ref(""); const show = ref("open");
const form = reactive({ name: "", phone: "", source: "", trialDay: "", followUpDay: "", notes: "" });
const shown = computed(() => fuzzyFilter(rows.value.filter((l) => show.value === "all" || (show.value === "open" ? !["converted", "lost"].includes(l.status) : l.status === show.value)), q.value, (l) => [l.name, l.phone, l.source, l.status]));
const counts = computed(() => ({ open: rows.value.filter((l) => !["converted", "lost"].includes(l.status)).length, attended: rows.value.filter((l) => !!l.trialAttendedAt).length, converted: rows.value.filter((l) => l.status === "converted").length, decided: rows.value.filter((l) => ["converted", "lost"].includes(l.status)).length }));
const linkPick = reactive<Record<string, string>>({});
const editing = ref("");
const editForm = reactive({ name: "", phone: "", source: "", trialDay: "", followUpDay: "", notes: "" });
function beginEdit(l: TrialLead) { editing.value = l.id; Object.assign(editForm, { name: l.name, phone: l.phone ?? "", source: l.source ?? "", trialDay: l.trialDay ?? "", followUpDay: l.followUpDay ?? "", notes: l.notes ?? "" }); }
async function saveEdit(l: TrialLead) { if (await update(l, editForm)) editing.value = ""; }
const options = computed(() => trainees.value.map(traineeOption));
async function load() {
  error.value = "";
  try { [rows.value, trainees.value] = await Promise.all([api.get<TrialLead[]>("/trial-leads"), api.get<Trainee[]>("/trainees?archived=include")]); }
  catch (e) { error.value = errMsg(e, "Couldn't load trials."); }
}
onMounted(load);
async function add() {
  if (busy.value) return; busy.value = true;
  try { await api.post("/trial-leads", form); Object.assign(form, { name: "", phone: "", source: "", trialDay: "", followUpDay: "", notes: "" }); adding.value = false; await load(); toast("Enquiry added.", "success"); }
  catch (e) { toast(errMsg(e, "Couldn't add enquiry."), "error"); }
  finally { busy.value = false; }
}
async function update(l: TrialLead, change: Partial<TrialLead>) {
  try { await api.patch(`/trial-leads/${l.id}`, change); await load(); return true; }
  catch (e) { toast(errMsg(e, "Couldn't update trial."), "error"); return false; }
}
function createLink(l: TrialLead) { return `/trainees/new?name=${encodeURIComponent(l.name)}&phone=${encodeURIComponent(l.phone ?? "")}&lead=${l.id}`; }
const statuses: TrialLead["status"][] = ["enquiry", "booked", "attended", "missed", "converted", "lost"];
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="People" title="Trials & enquiries"><template #action><Button size="sm" @click="adding = !adding">+ Enquiry</Button></template></PageHeader>
    <p class="text-sm text-muted">Keep trial visitors in view until they join or decide not to continue.</p>
    <div class="grid grid-cols-3 gap-2 text-center"><Card class="p-2"><p class="text-xl font-semibold">{{ counts.open }}</p><p class="text-[0.6875rem] text-faint">Open</p></Card><Card class="p-2"><p class="text-xl font-semibold">{{ counts.attended }}</p><p class="text-[0.6875rem] text-faint">Attended trial</p></Card><Card class="p-2"><p class="text-xl font-semibold">{{ counts.decided ? Math.round(counts.converted / counts.decided * 100) : 0 }}%</p><p class="text-[0.6875rem] text-faint">Converted of decided</p></Card></div>
    <Card v-if="adding" class="space-y-3 p-4">
      <label class="block text-xs">Name *<input v-model="form.name" :class="inputCls" placeholder="Full name" /></label>
      <label class="block text-xs">Phone<input v-model="form.phone" :class="inputCls" type="tel" /></label>
      <label class="block text-xs">Where they heard about us<input v-model="form.source" :class="inputCls" placeholder="Referral, Instagram…" /></label>
      <div class="grid grid-cols-2 gap-2"><label class="text-xs">Trial day<input v-model="form.trialDay" :class="inputCls" type="date" /></label><label class="text-xs">Follow up on<input v-model="form.followUpDay" :class="inputCls" type="date" :min="todayKey()" /></label></div>
      <label class="block text-xs">Notes<textarea v-model="form.notes" :class="inputCls" rows="2" /></label>
      <Button size="sm" :disabled="busy || !form.name.trim()" @click="add">Save enquiry</Button>
    </Card>
    <Alert v-if="error">{{ error }} <button class="underline" @click="load">Retry</button></Alert>
    <SearchInput v-model="q" label="Search trials" placeholder="Name, phone, source…" :matches="q ? shown.length : undefined" />
    <div class="flex gap-2 overflow-x-auto pb-1"><button v-for="v in ['open','all','converted','lost']" :key="v" type="button" class="shrink-0 rounded-full border px-3 py-1.5 text-xs capitalize" :class="show === v ? 'border-bronze text-bronze' : 'border-line text-muted'" @click="show = v">{{ v }}</button></div>
    <p v-if="!shown.length" class="text-sm text-faint">No matching trials.</p>
    <ul class="space-y-2"><li v-for="l in shown" :key="l.id"><Card class="space-y-2 p-3" :class="route.query.focus === l.id ? 'border-bronze' : ''">
      <div class="flex items-start justify-between"><div><p class="font-medium">{{ l.name }}</p><p class="text-xs text-faint">{{ l.phone || 'No phone' }}<span v-if="l.source"> · {{ l.source }}</span></p></div><span class="text-xs capitalize text-bronze">{{ l.status }}</span></div>
      <p v-if="l.trialDay || l.followUpDay || l.trialAttendedAt" class="text-xs text-muted"><span v-if="l.trialDay">Trial {{ l.trialDay }}</span><span v-if="l.trialAttendedAt"> · Attended</span><span v-if="l.followUpDay"> · Follow up {{ l.followUpDay }}</span></p>
      <p v-if="l.notes" class="text-xs text-muted">{{ l.notes }}</p>
      <div class="flex flex-wrap items-center gap-2 border-t border-line pt-2">
        <select :value="l.status" :class="inputCls" aria-label="Trial status" @change="update(l, { status: ($event.target as HTMLSelectElement).value as TrialLead['status'] })"><option v-for="s in statuses.filter((s) => s !== 'converted' || !!l.trainee)" :key="s" :value="s">{{ s }}</option></select>
        <a v-if="whatsappHref(l.phone)" :href="`${whatsappHref(l.phone)}?text=${encodeURIComponent(`Hi ${l.name}, thanks for visiting Bronze Boxing! How did your trial go?`)}`" target="_blank" rel="noopener noreferrer" class="text-xs font-medium text-paid">WhatsApp</a>
        <button class="text-xs font-medium text-bronze" @click="editing === l.id ? editing = '' : beginEdit(l)">{{ editing === l.id ? 'Close' : 'Edit details' }}</button>
      </div>
      <div v-if="editing === l.id" class="space-y-2 rounded-lg bg-elevated p-2 text-xs">
        <label class="block">Name<input v-model="editForm.name" :class="inputCls" /></label><label class="block">Phone<input v-model="editForm.phone" :class="inputCls" type="tel" /></label><label class="block">Source<input v-model="editForm.source" :class="inputCls" /></label>
        <div class="grid grid-cols-2 gap-2"><label>Trial day<input v-model="editForm.trialDay" :class="inputCls" type="date" /></label><label>Follow up<input v-model="editForm.followUpDay" :class="inputCls" type="date" /></label></div>
        <label class="block">Notes<textarea v-model="editForm.notes" :class="inputCls" rows="2" /></label><Button size="sm" :disabled="!editForm.name.trim()" @click="saveEdit(l)">Save details</Button>
      </div>
      <div v-if="!l.trainee && l.status !== 'lost'" class="space-y-2 rounded-lg bg-elevated p-2">
        <RouterLink :to="createLink(l)" class="block text-xs font-medium text-bronze">Create member from this trial →</RouterLink>
        <p class="text-xs text-faint">Or link someone already in Crew:</p>
        <SearchSelect v-model="linkPick[l.id]" :options="options" placeholder="Find an existing member…" search-placeholder="Search members…" />
        <Button v-if="linkPick[l.id]" size="sm" @click="update(l, { status: 'converted', trainee: linkPick[l.id] })">Link & convert</Button>
      </div>
      <RouterLink v-if="l.trainee" :to="`/trainees/${l.trainee}`" class="text-xs font-medium text-bronze">Open member →</RouterLink>
    </Card></li></ul>
  </div>
</template>
