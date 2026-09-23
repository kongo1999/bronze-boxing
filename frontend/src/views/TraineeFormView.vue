<script setup lang="ts">
import { reactive, ref, computed, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api, isApiError, errMsg } from "@/lib/api";
import { invalidate } from "@/lib/cache";
import type { Trainee, SubscriptionTerm } from "@/lib/types";
import { money, monthLabel, monthKey, shiftMonth } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import { inputCls } from "@/lib/ui";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string | undefined;
const editing = !!id;

const form = reactive({
  name: "",
  phone: "",
  skillLevel: "",
  monthlyFee: 0,
  status: "active",
  notes: "",
});
// What the trainee record said when the form loaded, to tell a fee/status
// change (which becomes an effective-dated term) from a plain edit.
const original = reactive({ monthlyFee: 0, status: "active", feeFromMonth: "" });
const applyFrom = ref<"next_month" | "this_month">("next_month");
const termsReason = ref("");
const sameDayTerm = ref<SubscriptionTerm>();

const saving = ref(false);
const error = ref<string>();
const fieldErr = ref<Record<string, string>>({});
// When editing, the form stays locked until the current values are actually
// loaded — otherwise a failed prefill would submit blanks over real data.
const loading = ref(editing);

const thisMonth = monthKey();
const nextMonth = shiftMonth(thisMonth, 1);

onMounted(async () => {
  if (!editing) return;
  try {
    const t = await api.get<Trainee>(`/trainees/${id}`);
    Object.assign(form, {
      name: t.name ?? "",
      phone: t.phone ?? "",
      skillLevel: t.skillLevel ?? "",
      monthlyFee: t.monthlyFee ?? 0,
      status: t.status === "inactive" ? "inactive" : "active",
      notes: t.notes ?? "",
    });
    Object.assign(original, { monthlyFee: t.monthlyFee ?? 0, status: form.status, feeFromMonth: t.feeFromMonth ?? "" });
    loading.value = false;
  } catch (e) {
    error.value = errMsg(e, "Couldn't load the trainee — go back and retry.");
  }
});

const feeChanged = computed(() => editing && Math.round(form.monthlyFee * 100) !== Math.round(original.monthlyFee * 100));
const statusChanged = computed(() => editing && form.status !== original.status);
const termsChanged = computed(() => feeChanged.value || statusChanged.value);
const needsReason = computed(() => termsChanged.value && applyFrom.value === "this_month" && feeChanged.value && form.status === "active");

async function submit(confirmSameDay = false) {
  if (loading.value || saving.value) return;
  fieldErr.value = {};
  if (!form.name.trim()) {
    fieldErr.value = { name: "Name is required" };
    return;
  }
  if (form.monthlyFee < 0) {
    fieldErr.value = { monthlyFee: "The fee can't be negative" };
    return;
  }
  if (needsReason.value && !termsReason.value.trim()) {
    fieldErr.value = { termsReason: "Say why this month's dues change" };
    return;
  }
  saving.value = true;
  error.value = undefined;
  try {
    const body = {
      ...form,
      termsApplyFrom: applyFrom.value,
      termsReason: termsReason.value.trim(),
      termsConfirm: confirmSameDay,
    };
    const saved = editing
      ? await api.put<Trainee>(`/trainees/${id}`, body)
      : await api.post<Trainee>(`/trainees`, form);
    invalidate("trainees", "dues", "dashboard");
    router.push(`/trainees/${saved.id}`);
  } catch (e) {
    saving.value = false;
    if (isApiError(e, "DUPLICATE") && e.details?.existing) {
      // A fee/status change is already recorded today: show it for review.
      sameDayTerm.value = e.details.existing as SubscriptionTerm;
      return;
    }
    if (isApiError(e) && e.field) {
      const f = e.field === "reason" ? "termsReason" : e.field;
      fieldErr.value = { [f]: e.message };
      if (e.code === "DUE_BELOW_PAID" || e.code === "REASON_REQUIRED") fieldErr.value.termsReason = e.message;
      return;
    }
    error.value = errMsg(e, "Failed to save");
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink :to="editing ? `/trainees/${id}` : '/trainees'" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> {{ editing ? "Trainee" : "Crew" }}
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">{{ editing ? "Edit trainee" : "Add trainee" }}</h1>

    <Card class="space-y-3 p-4">
      <Alert v-if="error">{{ error }}</Alert>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Name <span class="text-overdue">*</span></span>
        <input v-model="form.name" :class="inputCls" placeholder="Full name" :aria-invalid="!!fieldErr.name" />
        <span v-if="fieldErr.name" class="mt-1 block text-xs text-overdue">{{ fieldErr.name }}</span>
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Phone</span>
        <input v-model="form.phone" type="tel" inputmode="tel" :class="inputCls" placeholder="03 000 000" />
      </label>
      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Skill level</span>
          <select v-model="form.skillLevel" :class="inputCls">
            <option value="">—</option>
            <option value="beginner">Beginner</option>
            <option value="intermediate">Intermediate</option>
            <option value="advanced">Advanced</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Monthly fee</span>
          <input v-model.number="form.monthlyFee" type="number" min="0" step="0.01" inputmode="decimal" :class="inputCls" :aria-invalid="!!fieldErr.monthlyFee" />
          <span v-if="fieldErr.monthlyFee" class="mt-1 block text-xs text-overdue">{{ fieldErr.monthlyFee }}</span>
        </label>
      </div>
      <p v-if="!editing && form.monthlyFee > 0" class="text-xs text-faint">
        Joining now owes the full {{ money(form.monthlyFee) }} for {{ monthLabel(thisMonth) }} — adjust that month from Money if you agree otherwise.
      </p>
      <p v-else-if="editing && original.feeFromMonth > thisMonth" class="text-xs text-faint">
        {{ money(original.monthlyFee) }}/mo is already scheduled from {{ monthLabel(original.feeFromMonth) }}.
      </p>

      <div>
        <span class="mb-1 block text-xs text-faint">Status</span>
        <div class="grid grid-cols-2 gap-1 rounded-xl border border-line bg-elevated p-1" role="radiogroup" aria-label="Status">
          <button
            v-for="st in ['active', 'inactive']"
            :key="st"
            type="button"
            role="radio"
            :aria-checked="form.status === st"
            class="min-h-10 rounded-lg text-sm font-medium capitalize transition-colors"
            :class="form.status === st ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
            @click="form.status = st"
          >{{ st }}</button>
        </div>
      </div>

      <!-- A fee or status change becomes an effective-dated term: which month
           does it start billing? Past months are never rewritten. -->
      <div v-if="termsChanged" class="space-y-2 rounded-xl border border-bronze/30 bg-bronze/5 p-3">
        <p class="text-sm font-medium">
          <template v-if="feeChanged">Fee {{ money(original.monthlyFee) }} → {{ money(form.monthlyFee) }}</template>
          <template v-if="feeChanged && statusChanged"> · </template>
          <template v-if="statusChanged">Now {{ form.status }}</template>
        </p>
        <div class="grid grid-cols-2 gap-1 rounded-xl border border-line bg-elevated p-1" role="radiogroup" aria-label="Applies from">
          <button
            type="button"
            role="radio"
            :aria-checked="applyFrom === 'next_month'"
            class="min-h-10 rounded-lg px-2 text-xs font-medium transition-colors"
            :class="applyFrom === 'next_month' ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
            @click="applyFrom = 'next_month'"
          >From {{ monthLabel(nextMonth) }}</button>
          <button
            type="button"
            role="radio"
            :aria-checked="applyFrom === 'this_month'"
            class="min-h-10 rounded-lg px-2 text-xs font-medium transition-colors"
            :class="applyFrom === 'this_month' ? 'bg-bronze text-bronze-ink' : 'text-muted hover:text-fg'"
            @click="applyFrom = 'this_month'"
          >From {{ monthLabel(thisMonth) }}</button>
        </div>
        <p class="text-xs text-faint">
          <template v-if="applyFrom === 'next_month'">{{ monthLabel(thisMonth) }}'s dues stay as issued; the change starts next month.</template>
          <template v-else-if="form.status === 'inactive'">{{ monthLabel(thisMonth) }}'s dues already issued stay owed — waive them from Money if you agree to.</template>
          <template v-else>{{ monthLabel(thisMonth) }}'s dues are re-priced now, recorded as an adjustment.</template>
        </p>
        <label v-if="needsReason" class="block">
          <span class="mb-1 block text-xs text-faint">Why does this month change? <span class="text-overdue">*</span></span>
          <input v-model="termsReason" :class="inputCls" placeholder="e.g. agreed discount" :aria-invalid="!!fieldErr.termsReason" />
        </label>
        <span v-if="fieldErr.termsReason" class="block text-xs text-overdue">{{ fieldErr.termsReason }}</span>
      </div>

      <Alert v-if="sameDayTerm" tone="info">
        A change was already recorded today: {{ money(sameDayTerm.monthlyFee) }}/mo, {{ sameDayTerm.status }}, from
        {{ monthLabel(sameDayTerm.billingFromMonth) }}.
        <button class="ml-1 font-medium underline" @click="sameDayTerm = undefined; submit(true)">Record this one too</button>
      </Alert>

      <label class="block">
        <span class="mb-1 block text-xs text-faint">Notes</span>
        <textarea v-model="form.notes" rows="3" :class="inputCls" />
      </label>
      <Button :disabled="saving || loading" @click="submit()">{{ saving ? "Saving…" : loading ? "Loading…" : "Save" }}</Button>
    </Card>
  </div>
</template>
