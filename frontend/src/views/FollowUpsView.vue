<script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { RouterLink } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { FollowUp } from "@/lib/types";
import { whatsappHref } from "@/lib/phone";
import { fuzzyFilter } from "@/lib/fuzzy";
import { money } from "@/lib/format";
import PageHeader from "@/components/ui/PageHeader.vue";
import Card from "@/components/ui/Card.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Alert from "@/components/ui/Alert.vue";

const rows = ref<FollowUp[]>([]);
const loading = ref(true);
const error = ref("");
const q = ref("");
const kind = ref("all");
const snoozed = ref(false);
const kinds = [
  { key: "all", label: "All" }, { key: "dues", label: "Dues" },
  { key: "promise", label: "Promises" }, { key: "trial", label: "Trials" },
  { key: "plan", label: "Plans" }, { key: "inactive", label: "Inactive" },
];
const shown = computed(() => fuzzyFilter(rows.value.filter((r) => kind.value === "all" || r.kind === kind.value), q.value, (r) => [r.title, r.detail, r.kind]));
async function load() {
  loading.value = true; error.value = "";
  try { rows.value = await api.get<FollowUp[]>(`/follow-ups${snoozed.value ? '?show=snoozed' : ''}`); }
  catch (e) { error.value = errMsg(e, "Couldn't load follow-ups."); }
  finally { loading.value = false; }
}
onMounted(load);
async function action(r: FollowUp, kind: "contacted" | "snooze" | "clear") {
  try { await api.post("/follow-ups/action", { key: r.key, action: kind, days: 7 }); await load(); }
  catch (e) { error.value = errMsg(e, "Couldn't update follow-up."); }
}
function draft(r: FollowUp) {
  const base = whatsappHref(r.phone);
  if (!base) return "";
  const text = r.kind === "dues" || r.kind === "promise"
    ? `Hi ${r.title}, just checking in about the ${money(r.amount ?? 0)} remaining on your Bronze Boxing payment. Let me know when you'd like to settle it. Thanks!`
    : r.kind === "trial"
      ? `Hi ${r.title}, thanks for your interest in Bronze Boxing. How did your trial go? Happy to help with the next step.`
      : `Hi ${r.title}, checking in from Bronze Boxing. Hope you're doing well! Let me know if you'd like to plan your next session.`;
  return `${base}?text=${encodeURIComponent(text)}`;
}
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="People" title="Follow-ups" />
    <p class="text-sm text-muted">People who may need a message. The list updates when payments, attendance, plans or trials change.</p>
    <SearchInput v-model="q" label="Search follow-ups" placeholder="Search name or reason…" :matches="q ? shown.length : undefined" />
    <div class="flex gap-2 overflow-x-auto pb-1" aria-label="Follow-up type">
      <button v-for="k in kinds" :key="k.key" type="button" class="shrink-0 rounded-full border px-3 py-1.5 text-xs" :class="kind === k.key ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-muted'" @click="kind = k.key">{{ k.label }}</button>
    </div>
    <button class="text-xs font-medium text-bronze" @click="snoozed = !snoozed; load()">{{ snoozed ? 'Show active follow-ups' : 'Show snoozed' }}</button>
    <p v-if="loading" class="text-sm text-faint">Loading follow-ups…</p>
    <Alert v-else-if="error">{{ error }} <button class="underline" @click="load">Retry</button></Alert>
    <p v-else-if="!shown.length" class="text-sm text-faint">No follow-ups match.</p>
    <ul v-else class="space-y-2">
      <li v-for="r in shown" :key="r.key">
        <Card class="p-3">
          <div class="flex items-start justify-between gap-3">
            <div><p class="font-medium">{{ r.title }}</p><p class="text-xs text-muted">{{ r.detail }}</p><p v-if="r.dueDay" class="mt-1 text-xs text-faint">Due {{ r.dueDay }}</p></div>
            <span class="rounded-full bg-elevated px-2 py-1 text-[0.625rem] uppercase text-faint">{{ r.kind }}</span>
          </div>
          <div class="mt-3 flex gap-2 border-t border-line pt-2 text-sm font-medium">
            <RouterLink :to="r.href" class="min-h-9 content-center text-bronze">Open record</RouterLink>
            <a v-if="draft(r)" :href="draft(r)" target="_blank" rel="noopener noreferrer" class="min-h-9 content-center text-paid">WhatsApp draft</a>
            <button v-if="snoozed" class="text-faint" @click="action(r, 'clear')">Bring back</button>
            <template v-else><button class="text-faint" @click="action(r, 'contacted')">Contacted</button><button class="text-faint" @click="action(r, 'snooze')">Snooze 7d</button></template>
          </div>
        </Card>
      </li>
    </ul>
  </div>
</template>
