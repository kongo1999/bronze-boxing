<script setup lang="ts">
import { reactive, ref, computed, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api, errMsg, isApiError } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { InventoryItem, Reminder, Trainee } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import DateWheel from "@/components/ui/DateWheel.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import { inputCls } from "@/lib/ui";
import { isDayKey, todayKey } from "@/lib/studio";
import { backTarget } from "@/lib/route-state";
import { toast } from "@/lib/toast";
import { refreshBadges } from "@/lib/badges";
import { traineeOption } from "@/lib/options";

const route = useRoute();
const router = useRouter();

// /reminders/new creates, /reminders/:id/edit updates. A new reminder can be
// started from a record (?relatedType=session&relatedId=…&relatedLabel=…).
const editId = (route.params.id as string | undefined) ?? "";
const isEdit = !!editId;
type Related = "" | "trainee" | "session" | "item";
const q = route.query;
const related0 = (["trainee", "session", "item"].includes(q.relatedType as string) ? q.relatedType : "") as Related;

const saving = ref(false);
const loading = ref(isEdit);
const error = ref<string>();
const fieldErr = ref<Record<string, string>>({});
const form = reactive({
  title: typeof q.title === "string" ? q.title : "",
  dueDay: isDayKey(q.day) ? q.day : todayKey(),
  priority: "normal",
  recurrence: "" as "" | "daily" | "weekly" | "monthly",
  relatedType: related0,
  relatedId: related0 && typeof q.relatedId === "string" ? q.relatedId : "",
  relatedLabel: typeof q.relatedLabel === "string" ? q.relatedLabel : "",
});

const trainees = ref<Trainee[]>([]);
const items = ref<InventoryItem[]>([]);

onMounted(async () => {
  // Options for the link picker (small, fully loaded sets).
  Promise.all([api.get<Trainee[]>("/trainees"), api.get<InventoryItem[]>("/inventory")])
    .then(([t, i]) => {
      trainees.value = t;
      items.value = i;
    })
    .catch(() => {
      /* the link is optional; the rest of the form still works */
    });
  if (!isEdit) return;
  try {
    const r = await api.get<Reminder>(`/reminders/${editId}`);
    Object.assign(form, {
      title: r.title,
      dueDay: r.dueDay,
      priority: r.priority,
      recurrence: r.recurrence ?? "",
      relatedType: (r.relatedType ?? "") as Related,
      relatedId: r.relatedId ?? "",
      relatedLabel: r.relatedLabel ?? "",
    });
    loading.value = false;
  } catch (e) {
    // Keep the form locked: saving blanks over a reminder we couldn't read
    // would overwrite it.
    error.value = errMsg(e, "Couldn't load this reminder — go back and retry.");
  }
});

const linkOptions = computed(() =>
  form.relatedType === "trainee"
    ? trainees.value.map(traineeOption)
    : form.relatedType === "item"
      ? items.value.map((i) => ({ id: i.id, label: i.name, sub: i.sku }))
      : [],
);
function setRelatedType(t: Related) {
  if (form.relatedType === t) return;
  form.relatedType = t;
  form.relatedId = "";
  form.relatedLabel = "";
}

const back = () => backTarget(route.query, "/reminders");

async function submit() {
  if (saving.value || loading.value) return;
  fieldErr.value = {};
  if (!form.title.trim()) {
    fieldErr.value = { title: "Title is required" };
    return;
  }
  saving.value = true;
  error.value = undefined;
  const body = {
    title: form.title,
    dueDay: form.dueDay,
    priority: form.priority,
    recurrence: form.recurrence,
    relatedType: form.relatedId ? form.relatedType : "",
    relatedId: form.relatedId,
  };
  try {
    if (isEdit) await api.put(`/reminders/${editId}`, body);
    else await api.post("/reminders", body);
    refreshBadges(true);
    invalidate("reminders", "dashboard");
    if (isEdit) toast("Reminder updated.", "success");
    router.push(back());
  } catch (e) {
    saving.value = false;
    if (isApiError(e) && e.field) fieldErr.value = { [e.field]: e.message };
    else error.value = errMsg(e, "Failed to save");
  }
}

const pill = (active: boolean) => [
  "min-h-10 rounded-lg border px-3 text-xs font-medium transition-colors",
  active ? "border-bronze bg-bronze/15 text-bronze" : "border-line text-faint hover:text-fg",
];
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="back()" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Reminders
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">{{ isEdit ? "Edit reminder" : "New reminder" }}</h1>

    <Alert v-if="error">{{ error }}</Alert>
    <Skeleton v-if="loading && !error" variant="detail" />

    <Card v-else-if="!loading" class="space-y-4 p-4">
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Title <span class="text-overdue">*</span></span>
        <input v-model="form.title" :class="inputCls" placeholder="Order new gloves" :aria-invalid="!!fieldErr.title" />
        <span v-if="fieldErr.title" class="mt-1 block text-xs text-overdue">{{ fieldErr.title }}</span>
      </label>

      <DateWheel v-model="form.dueDay" label="Due" />

      <div>
        <span class="mb-1 block text-xs text-faint">Priority</span>
        <div class="flex gap-1.5" role="radiogroup" aria-label="Priority">
          <button v-for="p in ['low', 'normal', 'high']" :key="p" type="button" role="radio" :aria-checked="form.priority === p" class="capitalize" :class="pill(form.priority === p)" @click="form.priority = p">{{ p }}</button>
        </div>
      </div>

      <div>
        <span class="mb-1 block text-xs text-faint">Repeat</span>
        <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Repeat">
          <button
            v-for="r in [{ v: '', l: 'Never' }, { v: 'daily', l: 'Daily' }, { v: 'weekly', l: 'Weekly' }, { v: 'monthly', l: 'Monthly' }]"
            :key="r.v"
            type="button"
            role="radio"
            :aria-checked="form.recurrence === r.v"
            :class="pill(form.recurrence === r.v)"
            @click="form.recurrence = r.v as typeof form.recurrence"
          >{{ r.l }}</button>
        </div>
        <p v-if="form.recurrence" class="mt-1 text-xs text-faint">Ticking it off schedules the next one; done ones stay in the history.</p>
      </div>

      <div>
        <span class="mb-1 block text-xs text-faint">About <span class="text-faint/70">(optional)</span></span>
        <div v-if="form.relatedType === 'session'" class="flex items-center justify-between rounded-xl border border-line bg-elevated px-3 py-2 text-sm">
          <span class="truncate">Session: {{ form.relatedLabel || "linked session" }}</span>
          <button type="button" class="min-h-10 px-2 text-xs text-faint hover:text-fg" @click="setRelatedType('')">Remove</button>
        </div>
        <template v-else>
          <div class="mb-1.5 flex gap-1.5" role="radiogroup" aria-label="Link to">
            <button type="button" role="radio" :aria-checked="form.relatedType === ''" :class="pill(form.relatedType === '')" @click="setRelatedType('')">Nothing</button>
            <button type="button" role="radio" :aria-checked="form.relatedType === 'trainee'" :class="pill(form.relatedType === 'trainee')" @click="setRelatedType('trainee')">A trainee</button>
            <button type="button" role="radio" :aria-checked="form.relatedType === 'item'" :class="pill(form.relatedType === 'item')" @click="setRelatedType('item')">An item</button>
          </div>
          <SearchSelect
            v-if="form.relatedType"
            v-model="form.relatedId"
            :options="linkOptions"
            :placeholder="form.relatedType === 'trainee' ? 'Pick a trainee' : 'Pick an item'"
            :search-placeholder="form.relatedType === 'trainee' ? 'Search trainees…' : 'Search items…'"
          />
        </template>
        <span v-if="fieldErr.relatedId" class="mt-1 block text-xs text-overdue">{{ fieldErr.relatedId }}</span>
      </div>

      <Button :disabled="saving" @click="submit">{{ saving ? "Saving…" : isEdit ? "Save changes" : "Save" }}</Button>
    </Card>
  </div>
</template>
