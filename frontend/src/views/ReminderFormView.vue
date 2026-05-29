<script setup lang="ts">
import { reactive, ref } from "vue";
import { useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";

const router = useRouter();
const saving = ref(false);
const error = ref<string>();
const form = reactive({ title: "", dueDate: new Date().toISOString().slice(0, 10), priority: "normal" });
const inputCls = "w-full rounded-xl border border-line bg-elevated px-3 py-2.5 text-sm outline-none focus:border-bronze/40";

async function submit() {
  if (!form.title.trim()) return (error.value = "Title is required");
  saving.value = true;
  error.value = undefined;
  try {
    await api.post("/reminders", {
      title: form.title,
      dueDate: new Date(form.dueDate).toISOString(),
      priority: form.priority,
    });
    router.push("/reminders");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "Failed to save";
    saving.value = false;
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/reminders" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Reminders
    </RouterLink>
    <h1 class="font-display text-2xl font-semibold">New reminder</h1>
    <Card class="space-y-3 p-4">
      <p v-if="error" class="rounded-lg bg-overdue/10 px-3 py-2 text-sm text-overdue">{{ error }}</p>
      <label class="block">
        <span class="mb-1 block text-xs text-faint">Title</span>
        <input v-model="form.title" :class="inputCls" placeholder="Order new gloves" />
      </label>
      <div class="grid grid-cols-2 gap-3">
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Due date</span>
          <input v-model="form.dueDate" type="date" :class="inputCls" />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs text-faint">Priority</span>
          <select v-model="form.priority" :class="inputCls">
            <option value="low">Low</option>
            <option value="normal">Normal</option>
            <option value="high">High</option>
          </select>
        </label>
      </div>
      <Button :disabled="saving" @click="submit">{{ saving ? "Saving…" : "Save" }}</Button>
    </Card>
  </div>
</template>
