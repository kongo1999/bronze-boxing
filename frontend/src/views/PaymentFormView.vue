<script setup lang="ts">
import { reactive, ref, computed, watch, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Trainee, SubStatus } from "@/lib/types";
import { money, monthLabel } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchSelect from "@/components/ui/SearchSelect.vue";
import { inputCls } from "@/lib/ui";

const route = useRoute();
const router = useRouter();
const trainees = ref<Trainee[]>([]);
const saving = ref(false);
const error = ref<string>();

const form = reactive({
  trainee: (route.query.trainee as string) || "",
  amount: Number(route.query.amount) || 0,
  type: (route.query.type as string) || "subscription",
  periodMonth: (route.query.periodMonth as string) || "",
  note: "",
});

onMounted(async () => {
  trainees.value = await api.get<Trainee[]>("/trainees");
});

// Searchable picker instead of a long native dropdown — the roster only grows.
const traineeOptions = computed(() =>
  trainees.value.map((t) => ({ id: t.id, label: t.name, sub: t.status === "inactive" ? "inactive" : undefined })),
);

// Dues context for subscription payments: what's owed, paid, and remaining
// for the chosen trainee + period. Shown inline and used to block overpaying
// (the server enforces the same rule; this is the friendly first line).
const dueInfo = ref<{ due: number; paid: number } | null>(null);
async function loadDue() {
  dueInfo.value = null;
  if (form.type !== "subscription" || !form.trainee || !/^\d{4}-\d{2}$/.test(form.periodMonth)) return;
  try {
    const subs = await api.get<SubStatus[]>(`/subscriptions?m=${form.periodMonth}`);
    const s = subs.find((x) => x.trainee.id === form.trainee);
    if (s) dueInfo.value = { due: s.due, paid: s.amountPaid };
  } catch {
    /* helper line only — the server still validates */
  }
}
watch([() => form.trainee, () => form.periodMonth, () => form.type], loadDue, { immediate: true });

const remaining = computed(() =>
  dueInfo.value ? Math.max(0, dueInfo.value.due - dueInfo.value.paid) : null,
);
const overpaying = computed(() => remaining.value !== null && form.amount > remaining.value);

async function submit() {
  if (form.amount <= 0) return (error.value = "Amount must be positive");
  if (overpaying.value) return;
  saving.value = true;
  error.value = undefined;
  try {
    await api.post("/payments", {
      trainee: form.trainee || undefined,
      amount: form.amount,
      type: form.type,
      periodMonth: form.type === "subscription" ? form.periodMonth : "",
      note: form.note,
    });
    router.push("/payments");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Failed to save";
    saving.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/payments" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Money
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">Log payment</h1>

    <Card class="space-y-3 p-4">
      <Alert v-if="error">{{ error }}</Alert>
      <div>
        <span class="mb-1 block text-xs text-faint">Trainee</span>
        <SearchSelect v-model="form.trainee" :options="traineeOptions" empty-label="— (none)" placeholder="— (none)" search-placeholder="Search trainees…" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Amount</span>
          <input v-model.number="form.amount" type="number" min="0" :class="inputCls" />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Type</span>
          <select v-model="form.type" :class="inputCls">
            <option value="subscription">Subscription</option>
            <option value="private">Private session</option>
            <option value="dropin">Drop-in</option>
            <option value="other">Other</option>
          </select>
        </label>
      </div>
      <label v-if="form.type === 'subscription'" class="block">
        <span class="mb-1 block text-xs text-faint">Period month</span>
        <input v-model="form.periodMonth" type="month" :class="inputCls" />
      </label>
      <p v-if="dueInfo" class="text-xs" :class="overpaying ? 'text-overdue' : 'text-faint'">
        {{ monthLabel(form.periodMonth) }}: {{ money(dueInfo.due) }} fee · {{ money(dueInfo.paid) }} paid ·
        <template v-if="remaining! > 0">{{ money(remaining!) }} remaining</template>
        <template v-else>fully paid</template>
        <template v-if="overpaying"> — that's more than what's owed</template>
      </p>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Note</span>
        <input v-model="form.note" :class="inputCls" />
      </label>
      <Button :disabled="saving || overpaying" @click="submit">{{ saving ? "Saving…" : "Record payment" }}</Button>
    </Card>
  </div>
</template>
