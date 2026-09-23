<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft } from "lucide-vue-next";
import { api } from "@/lib/api";
import { clearCache } from "@/lib/cache";
import type { Reminder } from "@/lib/types";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Alert from "@/components/ui/Alert.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import { inputCls } from "@/lib/ui";
import { dateKey } from "@/lib/format";
import { toast } from "@/lib/toast";

const route = useRoute();
const router = useRouter();

// /reminders/new creates, /reminders/:id/edit updates.
const editId = (route.params.id as string | undefined) ?? "";
const isEdit = !!editId;

const saving = ref(false);
const loading = ref(isEdit);
const error = ref<string>();
const form = reactive({ title: "", dueDate: dateKey(new Date()), priority: "normal", done: false });

onMounted(async () => {
  if (!isEdit) return;
  try {
    // No GET /reminders/:id on the API — the list is small and already the
    // thing we came from, so pick this one out of it.
    const all = await api.get<Reminder[]>("/reminders");
    const r = all.find((x) => x.id === editId);
    if (!r) {
      error.value = "That reminder no longer exists.";
      return;
    }
    Object.assign(form, {
      title: r.title,
      dueDate: dateKey(new Date(r.dueDate)),
      priority: r.priority,
      done: r.done,
    });
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : "Couldn't load this reminder.";
  } finally {
    loading.value = false;
  }
});

async function submit() {
  if (!form.title.trim()) return (error.value = "Title is required");
  saving.value = true;
  error.value = undefined;
  const body = {
    title: form.title,
    dueDate: new Date(form.dueDate).toISOString(),
    priority: form.priority,
    done: form.done,
  };
  try {
    if (isEdit) await api.put(`/reminders/${editId}`, body);
    else await api.post("/reminders", body);
    clearCache();
    if (isEdit) toast("Reminder updated.", "success");
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
    <h1 class="font-display text-2xl font-semibold">{{ isEdit ? "Edit reminder" : "New reminder" }}</h1>

    <Skeleton v-if="loading" variant="detail" />

    <Card v-else class="space-y-3 p-4">
      <Alert v-if="error">{{ error }}</Alert>
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
      <Button :disabled="saving" @click="submit">{{ saving ? "Saving…" : isEdit ? "Save changes" : "Save" }}</Button>
    </Card>
  </div>
</template>
