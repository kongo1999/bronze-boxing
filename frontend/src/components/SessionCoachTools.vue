<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { Session, AttendanceStatus, WaitlistEntry, Trainee, Payment, StudioPolicy, ProgressNote } from "@/lib/types";
import { withBack } from "@/lib/route-state";
import { whatsappHref } from "@/lib/phone";
import { money } from "@/lib/format";
import { traineeOption } from "@/lib/options";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import { fuzzyFilter } from "@/lib/fuzzy";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import SearchInput from "@/components/ui/SearchInput.vue";

const props = defineProps<{ session: Session }>();
const emit = defineEmits<{ saved: [session: Session] }>();
const route = useRoute();
const status = reactive<Record<string, AttendanceStatus | "">>({});
const note = ref(""); const busy = ref(false); const error = ref("");
const waitlist = ref<WaitlistEntry[]>([]); const roster = ref<Trainee[]>([]);
const payments = ref<Payment[]>([]); const addPick = ref("");
const policy = reactive<StudioPolicy>({ id: "default", cancelBeforeHours: 12, lateCancellationAction: "allow" });
const policyOpen = ref(false); const focus = reactive<Record<string, ProgressNote | null>>({});
const closeQ = ref(""); const waitQ = ref("");
const started = computed(() => new Date(props.session.start).getTime() - 30 * 60_000 <= Date.now());
const upcoming = computed(() => new Date(props.session.start).getTime() > Date.now());
const full = computed(() => !!props.session.capacity && props.session.attendees.length >= props.session.capacity);
const activeWait = computed(() => waitlist.value.filter((x) => ["waiting", "offered"].includes(x.status)));
const shownAttendees = computed(() => fuzzyFilter(props.session.attendees, closeQ.value, (a) => [a.traineeName, a.status]));
const shownWait = computed(() => fuzzyFilter(activeWait.value, waitQ.value, (e) => [e.traineeName, e.status]));
const options = computed(() => roster.value.filter((t) => t.status === "active" && !t.archivedAt && !props.session.attendees.some((a) => a.trainee === t.id) && !activeWait.value.some((a) => a.trainee === t.id)).map(traineeOption));
const ready = computed(() => props.session.attendees.every((a) => status[a.trainee] === "attended" || status[a.trainee] === "no_show"));
const paidFor = (tid: string) => payments.value.filter((p) => p.trainee === tid && !p.voidedAt).reduce((n, p) => n + p.amount, 0);
const phone = (tid: string) => roster.value.find((t) => t.id === tid)?.phone;
async function load() {
  Object.keys(status).forEach((k) => delete status[k]);
  props.session.attendees.forEach((a) => { status[a.trainee] = a.status === "booked" ? "" : a.status; });
  note.value = props.session.closeoutNote ?? "";
  try {
    const [w, t, p, pol] = await Promise.all([
      api.get<WaitlistEntry[]>(`/sessions/${props.session.id}/waitlist`),
      api.get<Trainee[]>("/trainees?archived=include"),
      api.get<Payment[]>(`/payments?sessionId=${props.session.id}`),
      api.get<StudioPolicy>("/studio-policy"),
    ]);
    waitlist.value = w; roster.value = t; payments.value = p; Object.assign(policy, pol);
  } catch (e) { error.value = errMsg(e, "Couldn't load class tools."); }
}
watch(() => [props.session.id, props.session.updatedAt], load, { immediate: true });
function allAttended() { for (const a of props.session.attendees) if (status[a.trainee] === "") status[a.trainee] = "attended"; }
async function closeout() {
  if (!ready.value || busy.value) return;
  busy.value = true; error.value = "";
  try {
    const s = await api.post<Session>(`/sessions/${props.session.id}/closeout`, { attendance: props.session.attendees.map((a) => ({ trainee: a.trainee, status: status[a.trainee] })), note: note.value });
    emit("saved", s); toast("Class closed. Attendance and note saved.", "success");
  } catch (e) { error.value = errMsg(e, "Couldn't close the class."); }
  finally { busy.value = false; }
}
async function join() {
  if (!addPick.value) return;
  try { await api.post(`/sessions/${props.session.id}/waitlist`, { trainee: addPick.value }); addPick.value = ""; await load(); }
  catch (e) { error.value = errMsg(e); }
}
async function cancelBooking(tid: string) {
  if (!confirm(upcoming.value ? "Cancel this booking? The next person in line will be offered the place. Late cancellations follow your studio rule." : "Cancel this booking? Your late cancellation rule may record a no-show.")) return;
  try { const out = await api.post<{ session: Session; late: boolean }>(`/sessions/${props.session.id}/cancel-booking`, { trainee: tid }); emit("saved", out.session); toast(out.late ? "Late cancellation recorded." : "Booking cancelled; waitlist updated.", "success"); }
  catch (e) { error.value = errMsg(e); }
}
async function offerAction(entry: WaitlistEntry, action: "accept" | "decline") {
  try { const s = await api.post<Session | { ok: boolean }>(`/sessions/${props.session.id}/waitlist/${entry.id}/${action}`); if (action === "accept") emit("saved", s as Session); else await load(); }
  catch (e) { error.value = errMsg(e); }
}
async function savePolicy() {
  try { await api.put("/studio-policy", policy); policyOpen.value = false; toast("Cancellation rule saved.", "success"); }
  catch (e) { error.value = errMsg(e); }
}
async function showFocus(tid: string) {
  if (tid in focus) { delete focus[tid]; return; }
  try { focus[tid] = (await api.get<ProgressNote[]>(`/trainees/${tid}/progress`))[0] ?? null; }
  catch (e) { error.value = errMsg(e); }
}
</script>

<template>
  <div class="space-y-3">
    <p v-if="error" class="rounded-xl border border-overdue/40 p-3 text-xs text-overdue">{{ error }}</p>
    <Card v-if="started && session.status !== 'cancelled'" class="space-y-3 p-4">
      <div><h2 class="font-display text-lg font-semibold">Class closeout</h2><p class="text-xs text-muted">Mark each person, add a coaching note, then close the class.</p></div>
      <Button v-if="session.attendees.some((a) => status[a.trainee] === '')" size="sm" variant="ghost" @click="allAttended">Mark unreviewed attended</Button>
      <SearchInput v-if="session.attendees.length >= 6" v-model="closeQ" label="Search closeout" placeholder="Find a trainee…" :matches="closeQ ? shownAttendees.length : undefined" />
      <ul class="space-y-2"><li v-for="a in shownAttendees" :key="a.trainee" class="rounded-xl border border-line p-2">
        <div class="flex items-center justify-between gap-2"><span class="text-sm font-medium">{{ a.traineeName }}</span><button class="text-xs text-bronze" @click="showFocus(a.trainee)">{{ a.trainee in focus ? 'Hide focus' : 'Latest focus' }}</button></div>
        <p v-if="a.trainee in focus" class="mt-1 text-xs text-muted">{{ focus[a.trainee]?.nextFocus || focus[a.trainee]?.note || 'No progress note yet.' }} <RouterLink :to="withBack(`/trainees/${a.trainee}`, route.fullPath, { session: session.id })" class="font-medium text-bronze">Add note</RouterLink></p>
        <div class="mt-2 flex gap-1" :aria-label="`${a.traineeName} attendance`"><button v-for="v in (['attended','no_show'] as const)" :key="v" type="button" class="min-h-9 flex-1 rounded-lg border text-xs" :class="status[a.trainee] === v ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted'" @click="status[a.trainee] = v">{{ v === 'no_show' ? 'No-show' : 'Attended' }}</button></div>
        <div v-if="session.fee && !a.planId && status[a.trainee] === 'attended'" class="mt-2 flex justify-between text-xs"><span>{{ paidFor(a.trainee) ? `Linked payment: ${money(paidFor(a.trainee))}` : `Class fee: ${money(session.fee)} · check payment` }}</span><RouterLink :to="withBack('/payments/new', route.fullPath, { trainee: a.trainee, type: session.type === 'private' ? 'private' : 'dropin', amount: String(session.fee), sessionId: session.id })" class="font-medium text-bronze">Record payment</RouterLink></div>
      </li></ul>
      <label class="block text-xs">Coaching note<textarea v-model="note" :class="inputCls" rows="2" placeholder="What went well? Focus for next class…" /></label>
      <Button :disabled="busy || !ready" @click="closeout">{{ session.status === 'completed' ? 'Update closeout' : 'Close class' }}</Button>
      <p v-if="!ready" class="text-xs text-partial">Review every trainee before closing.</p>
    </Card>
    <Card v-if="session.status === 'scheduled'" class="space-y-3 p-4">
      <div class="flex justify-between gap-2"><div><h2 class="font-display text-lg font-semibold">Waitlist & cancellations</h2><p class="text-xs text-muted">{{ activeWait.length }} waiting or offered</p></div><button class="text-xs text-bronze" @click="policyOpen = !policyOpen">Rules</button></div>
      <div v-if="policyOpen" class="space-y-2 rounded-xl bg-elevated p-3 text-xs"><label class="block">Late within how many hours?<input v-model.number="policy.cancelBeforeHours" :class="inputCls" type="number" min="0" max="168" /></label><label class="block">Late cancellation<select v-model="policy.lateCancellationAction" :class="inputCls"><option value="allow">Release place</option><option value="no_show">Record no-show after class starts</option></select></label><p class="text-faint">A no-show never uses a session plan credit. The class must have started before it can be recorded.</p><Button size="sm" @click="savePolicy">Save rule</Button></div>
      <div v-if="upcoming && full" class="flex gap-2"><div class="min-w-0 flex-1"><SearchSelect v-model="addPick" :options="options" placeholder="Add to waitlist…" search-placeholder="Search trainees…" /></div><Button size="sm" :disabled="!addPick" @click="join">Add</Button></div>
      <p v-else-if="upcoming" class="text-xs text-faint">This class has room. Book directly from Attendance above.</p>
      <p v-else class="text-xs text-faint">The class has started. Mark attendance in closeout; late cancellations follow the rule above.</p>
      <SearchInput v-if="activeWait.length >= 6" v-model="waitQ" label="Search waitlist" placeholder="Find someone waiting…" :matches="waitQ ? shownWait.length : undefined" />
      <ul v-if="activeWait.length" class="space-y-1"><li v-for="e in shownWait" :key="e.id" class="rounded-lg border border-line p-2 text-xs"><div class="flex justify-between"><span>{{ activeWait.indexOf(e) + 1 }}. {{ e.traineeName }}</span><span class="capitalize" :class="e.status === 'offered' ? 'text-bronze' : 'text-faint'">{{ e.status }}</span></div><div v-if="e.status === 'offered'" class="mt-2 flex gap-3 font-medium"><a v-if="whatsappHref(phone(e.trainee))" :href="`${whatsappHref(phone(e.trainee))}?text=${encodeURIComponent(`A place opened in ${session.title}. Would you like it?`)}`" target="_blank" rel="noopener noreferrer" class="text-paid">Offer on WhatsApp</a><button class="text-bronze" @click="offerAction(e, 'accept')">Book place</button><button class="text-faint" @click="offerAction(e, 'decline')">Decline</button></div><button v-else class="mt-1 text-faint underline" @click="offerAction(e, 'decline')">Remove from waitlist</button></li></ul>
      <details v-if="session.attendees.some((a) => a.status === 'booked')" class="text-xs"><summary class="cursor-pointer text-bronze">Cancel a booking</summary><div class="mt-2 space-y-1"><button v-for="a in session.attendees.filter((x) => x.status === 'booked')" :key="a.trainee" class="block min-h-8 text-overdue" @click="cancelBooking(a.trainee)">Cancel {{ a.traineeName }}</button></div></details>
    </Card>
  </div>
</template>
