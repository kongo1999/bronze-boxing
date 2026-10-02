<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useRoute, RouterLink } from "vue-router";
import { api, errMsg } from "@/lib/api";
import type { ProgressNote } from "@/lib/types";
import { fuzzyFilter } from "@/lib/fuzzy";
import { formatLongDate } from "@/lib/format";
import { inputCls } from "@/lib/ui";
import { toast } from "@/lib/toast";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
const props = defineProps<{ trainee: string }>();
const route = useRoute();
const rows = ref<ProgressNote[]>([]); const q = ref(""); const open = ref(false); const busy = ref(false); const error = ref("");
const form = reactive({ goal: "", skills: "", nextFocus: "", note: "" });
const shown = computed(() => fuzzyFilter(rows.value, q.value, (n) => [n.goal, n.skills, n.nextFocus, n.note]));
async function load() { try { rows.value = await api.get<ProgressNote[]>(`/trainees/${props.trainee}/progress`); error.value = ""; } catch (e) { error.value = errMsg(e); } }
watch(() => props.trainee, load, { immediate: true });
async function save() {
  if (busy.value) return; busy.value = true;
  try { await api.post(`/trainees/${props.trainee}/progress`, { ...form, session: typeof route.query.session === "string" ? route.query.session : undefined }); Object.assign(form, { goal: "", skills: "", nextFocus: "", note: "" }); open.value = false; await load(); toast("Progress note saved.", "success"); }
  catch (e) { error.value = errMsg(e, "Couldn't save note."); }
  finally { busy.value = false; }
}
</script>
<template>
  <Card class="space-y-3 p-4">
    <div class="flex justify-between"><div><h2 class="font-display text-lg font-semibold">Boxing progress</h2><p class="text-xs text-faint">Goals, skills and the next class focus.</p></div><Button size="sm" variant="ghost" @click="open = !open">{{ open ? 'Close' : '+ Note' }}</Button></div>
    <p v-if="error" class="text-xs text-overdue">{{ error }}</p>
    <div v-if="open" class="space-y-2 rounded-xl bg-elevated p-3">
      <label class="block text-xs">Goal<input v-model="form.goal" :class="inputCls" placeholder="e.g. Improve defence" /></label>
      <label class="block text-xs">Skills worked on<input v-model="form.skills" :class="inputCls" placeholder="e.g. Jab and footwork" /></label>
      <label class="block text-xs">Next session focus<input v-model="form.nextFocus" :class="inputCls" placeholder="What to practise next" /></label>
      <label class="block text-xs">Coach note<textarea v-model="form.note" :class="inputCls" rows="2" /></label>
      <Button size="sm" :disabled="busy || !Object.values(form).some((v) => v.trim())" @click="save">Save note</Button>
    </div>
    <SearchInput v-if="rows.length >= 5" v-model="q" label="Search progress" placeholder="Search notes…" :matches="q ? shown.length : undefined" />
    <p v-if="!rows.length" class="text-xs text-faint">No progress notes yet.</p>
    <ul v-else class="space-y-2"><li v-for="n in shown" :key="n.id" class="rounded-xl border border-line p-3 text-xs"><p class="text-faint">{{ formatLongDate(n.createdAt) }}<RouterLink v-if="n.session" :to="`/schedule/${n.session}`" class="ml-2 text-bronze">Class</RouterLink></p><p v-if="n.goal"><strong>Goal:</strong> {{ n.goal }}</p><p v-if="n.skills"><strong>Skills:</strong> {{ n.skills }}</p><p v-if="n.nextFocus" class="text-bronze"><strong>Next:</strong> {{ n.nextFocus }}</p><p v-if="n.note" class="text-muted">{{ n.note }}</p></li></ul>
  </Card>
</template>
