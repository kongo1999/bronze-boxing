<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { Trainee } from "@/lib/types";
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
const saving = ref(false);
const error = ref<string>();
// When editing, the form stays locked until the current values are actually
// loaded — otherwise a failed prefill would submit blanks over real data.
const loading = ref(editing);

onMounted(async () => {
  if (!editing) return;
  try {
    const t = await api.get<Trainee>(`/trainees/${id}`);
    Object.assign(form, {
      name: t.name ?? "",
      phone: t.phone ?? "",
      skillLevel: t.skillLevel ?? "",
      monthlyFee: t.monthlyFee ?? 0,
      status: t.status ?? "active",
      notes: t.notes ?? "",
    });
    loading.value = false;
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : "Couldn't load the trainee — go back and retry.";
  }
});

async function submit() {
  if (loading.value) return;
  if (!form.name.trim()) {
    error.value = "Name is required";
    return;
  }
  saving.value = true;
  error.value = undefined;
  try {
    const saved = editing
      ? await api.put<Trainee>(`/trainees/${id}`, form)
      : await api.post<Trainee>(`/trainees`, form);
    router.push(`/trainees/${saved.id}`);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Failed to save";
    saving.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/trainees" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Crew
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">{{ editing ? "Edit trainee" : "Add trainee" }}</h1>

    <Card class="space-y-3 p-4">
      <Alert v-if="error">{{ error }}</Alert>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Name</span>
        <input v-model="form.name" :class="inputCls" placeholder="Full name" />
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Phone</span>
        <input v-model="form.phone" :class="inputCls" placeholder="03 000 000" />
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
          <input v-model.number="form.monthlyFee" type="number" min="0" :class="inputCls" />
        </label>
      </div>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Status</span>
        <select v-model="form.status" :class="inputCls">
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
        </select>
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Notes</span>
        <textarea v-model="form.notes" rows="3" :class="inputCls" />
      </label>
      <Button :disabled="saving || loading" @click="submit">{{ saving ? "Saving…" : loading ? "Loading…" : "Save" }}</Button>
    </Card>
  </div>
</template>
