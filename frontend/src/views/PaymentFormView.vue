<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Trainee } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";

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

const inputCls = "w-full rounded-xl border border-line bg-elevated px-3 py-2.5 text-sm outline-none focus:border-bronze/40";

async function submit() {
  if (form.amount <= 0) return (error.value = "Amount must be positive");
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
      <p v-if="error" class="rounded-lg bg-overdue/10 px-3 py-2 text-sm text-overdue">{{ error }}</p>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Trainee</span>
        <select v-model="form.trainee" :class="inputCls">
          <option value="">— (none)</option>
          <option v-for="t in trainees" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </label>
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
        <span class="mb-1 block text-xs text-faint">Period month (YYYY-MM)</span>
        <input v-model="form.periodMonth" :class="inputCls" placeholder="2026-05" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Note</span>
        <input v-model="form.note" :class="inputCls" />
      </label>
      <Button :disabled="saving" @click="submit">{{ saving ? "Saving…" : "Record payment" }}</Button>
    </Card>
  </div>
</template>
