<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { RouterLink } from "vue-router";
import { MessageCircle, RefreshCw } from "lucide-vue-next";
import { api, errMsg } from "@/lib/api";
import { fuzzyFilter } from "@/lib/fuzzy";
import { formatDateTime } from "@/lib/format";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchInput from "@/components/ui/SearchInput.vue";

type Settings = { mode: "drafts" | "automatic"; groupHours: number; privateHours: number; groupTemplate: string; privateTemplate: string; templateLanguage: string };
type Alert = { sessionId: string; traineeId: string; name: string; phone: string; optedIn: boolean; title: string; type: "group" | "private"; start: string; dueAt: string; status: string; sentAt?: string; error?: string; text: string };
const settings = reactive<Settings>({ mode: "drafts", groupHours: 24, privateHours: 24, groupTemplate: "", privateTemplate: "", templateLanguage: "en" });
const providerReady = ref(false);
const alerts = ref<Alert[]>([]);
const q = ref("");
const busy = ref(false);
const error = ref("");
const saved = ref(false);
const pending = computed(() => alerts.value.filter((a) => a.status === "unsent").length);
const shown = computed(() => fuzzyFilter(alerts.value, q.value, (a) => [a.name, a.title, a.type, a.phone]));
const due = (a: Alert) => new Date(a.dueAt).getTime() <= Date.now();
const link = (a: Alert) => `https://wa.me/${a.phone}?text=${encodeURIComponent(a.text)}`;

async function load() {
  error.value = "";
  try {
    const [s, rows] = await Promise.all([
      api.get<{ settings: Settings; providerReady: boolean }>("/class-alert-settings"),
      api.get<Alert[]>("/class-alerts?days=14"),
    ]);
    Object.assign(settings, s.settings);
    providerReady.value = s.providerReady;
    alerts.value = rows;
    saved.value = true;
  } catch (e) { error.value = errMsg(e, "Couldn't load class reminders."); }
}
onMounted(load);
async function save() {
  if (busy.value) return;
  busy.value = true; error.value = "";
  try {
    const result = await api.put<{ settings: Settings; providerReady: boolean }>("/class-alert-settings", settings);
    Object.assign(settings, result.settings);
    providerReady.value = result.providerReady;
    saved.value = true;
    toast("Class reminder settings saved.", "success");
    await load();
  } catch (e) { error.value = errMsg(e, "Couldn't save class reminders."); }
  finally { busy.value = false; }
}
async function markSent(a: Alert) {
  try {
    await api.post(`/class-alerts/${a.sessionId}/${a.traineeId}/mark-sent`, {});
    a.status = "sent"; a.sentAt = new Date().toISOString();
    toast("Marked as sent.", "success");
  } catch (e) { toast(errMsg(e, "Couldn't mark the reminder sent."), "error"); }
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Schedule · communication" title="Class reminders">
      <template #action><Button size="sm" variant="ghost" @click="load"><RefreshCw class="h-4 w-4" /> Refresh</Button></template>
    </PageHeader>
    <Alert v-if="error">{{ error }}</Alert>
    <Card class="space-y-3 p-4">
      <div><h2 class="font-display text-lg font-semibold">How reminders go out</h2><p class="text-xs text-muted">{{ pending }} unsent for upcoming classes. Bookings with a missing phone are listed for follow-up. One reminder per member and class.</p></div>
      <div class="grid grid-cols-2 gap-2 rounded-xl border border-line bg-elevated p-1" role="radiogroup" aria-label="Reminder mode">
        <button type="button" role="radio" :aria-checked="settings.mode === 'drafts'" class="min-h-11 rounded-lg text-sm" :class="settings.mode === 'drafts' ? 'bg-bronze text-bronze-ink' : 'text-muted'" @click="settings.mode = 'drafts'">WhatsApp drafts</button>
        <button type="button" role="radio" :aria-checked="settings.mode === 'automatic'" class="min-h-11 rounded-lg text-sm" :class="settings.mode === 'automatic' ? 'bg-bronze text-bronze-ink' : 'text-muted'" @click="settings.mode = 'automatic'">Automatic</button>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <label class="text-xs">Group reminder · hours before<input v-model.number="settings.groupHours" :class="inputCls" type="number" min="1" max="168" /></label>
        <label class="text-xs">Private reminder · hours before<input v-model.number="settings.privateHours" :class="inputCls" type="number" min="1" max="168" /></label>
      </div>
      <template v-if="settings.mode === 'automatic'">
        <p class="rounded-lg bg-elevated p-3 text-xs text-muted">{{ providerReady ? 'Server connection is available.' : 'The server needs WhatsApp Cloud API credentials before automatic mode can be saved.' }} Members need recorded consent and an international phone number. The studio must have approved templates with four fields: member name, class title, date, time.</p>
        <div class="grid gap-2 sm:grid-cols-2"><label class="text-xs">Approved group template<input v-model="settings.groupTemplate" :class="inputCls" placeholder="group_class_reminder" /></label><label class="text-xs">Approved private template<input v-model="settings.privateTemplate" :class="inputCls" placeholder="private_class_reminder" /></label></div>
        <label class="block text-xs">Template language<input v-model="settings.templateLanguage" :class="inputCls" placeholder="en" /></label>
      </template>
      <Button size="sm" :disabled="busy" @click="save">{{ busy ? 'Saving…' : 'Save settings' }}</Button>
    </Card>
    <Card class="space-y-3 p-4">
      <div class="flex items-center justify-between gap-2"><div><h2 class="font-display text-lg font-semibold">Next 14 days</h2><p class="text-xs text-faint">{{ shown.length }} member reminders · sorted by class time</p></div></div>
      <SearchInput v-if="alerts.length >= 8 || q" v-model="q" label="Search class reminders" placeholder="Member or class…" :matches="q ? shown.length : undefined" />
      <p v-if="saved && !shown.length" class="text-sm text-muted">No matching upcoming class reminders.</p>
      <div v-for="a in shown" :key="`${a.sessionId}:${a.traineeId}`" class="space-y-2 border-t border-line pt-3 text-sm">
        <div class="flex items-start justify-between gap-2"><div><RouterLink :to="`/trainees/${a.traineeId}`" class="font-medium text-bronze hover:underline">{{ a.name }}</RouterLink><p class="text-xs text-muted">{{ a.title }} · {{ a.type }} · {{ formatDateTime(a.start) }}</p><p class="text-xs text-faint">Reminder due {{ formatDateTime(a.dueAt) }}</p></div><span class="rounded-full bg-elevated px-2 py-1 text-xs capitalize">{{ a.status }}</span></div>
        <p v-if="a.status === 'needs_phone'" class="text-xs text-overdue">No usable international phone number. Add one on the member's edit page.</p>
        <p v-if="!a.optedIn" class="text-xs text-overdue">No WhatsApp opt-in recorded. Add consent on the member's edit page before messaging.</p>
        <p v-if="a.status === 'failed'" class="text-xs text-overdue">Automatic delivery failed. Check the template and provider setup before sending a draft.</p>
        <p v-if="a.status === 'sending'" class="text-xs text-partial">Delivery started but has no confirmation. Check WhatsApp before sending a draft or marking it sent.</p>
        <div v-if="a.status !== 'sent'" class="flex flex-wrap items-center gap-2">
          <a v-if="a.optedIn" :href="link(a)" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-10 items-center gap-1 rounded-lg border border-line px-3 text-xs font-medium text-bronze"><MessageCircle class="h-4 w-4" /> Open draft</a>
          <Button v-if="a.optedIn && (a.status === 'unsent' || a.status === 'failed' || a.status === 'sending')" size="sm" variant="ghost" @click="markSent(a)">Mark sent</Button>
          <span v-if="settings.mode === 'automatic' && due(a) && a.status === 'unsent'" class="text-xs text-faint">Waiting for the next automatic check.</span>
        </div>
      </div>
    </Card>
  </div>
</template>
