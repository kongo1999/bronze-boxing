<script setup lang="ts">
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Trash2, Phone } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync } from "@/lib/cache";
import type { Trainee, Payment } from "@/lib/types";
import { money, formatLongDate } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Badge from "@/components/ui/Badge.vue";
import { btnClasses } from "@/components/ui/button";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;

const { data: trainee, loading } = useCachedAsync(`/trainees/${id}`, () => api.get<Trainee>(`/trainees/${id}`));
const { data: payments } = useCachedAsync("payments:all", () => api.get<Payment[]>(`/payments?from=2000-01-01&to=2100-01-01`));

async function remove() {
  if (!confirm("Delete this trainee?")) return;
  await api.del(`/trainees/${id}`);
  router.push("/trainees");
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/trainees" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Crew
    </RouterLink>

    <p v-if="loading" class="text-sm text-faint">Loading…</p>

    <template v-else-if="trainee">
      <Card class="p-4">
        <div class="flex items-center gap-3">
          <Avatar :name="trainee.name" class="h-14 w-14 text-lg" />
          <div class="min-w-0 flex-1">
            <h1 class="font-display text-xl font-semibold">{{ trainee.name }}</h1>
            <p class="text-sm text-muted">
              <span v-if="trainee.skillLevel" class="capitalize">{{ trainee.skillLevel }}</span>
              <Badge v-if="trainee.status === 'inactive'" tone="neutral" class="ml-1">Inactive</Badge>
            </p>
          </div>
        </div>
        <div class="mt-4 grid grid-cols-2 gap-3">
          <div class="rounded-xl border border-line bg-elevated px-3 py-2">
            <p class="label-eyebrow text-[0.6rem] text-faint">Monthly fee</p>
            <p class="font-display text-lg tnum">{{ money(trainee.monthlyFee) }}</p>
          </div>
          <a
            v-if="trainee.phone"
            :href="`tel:${trainee.phone}`"
            class="flex items-center gap-2 rounded-xl border border-line bg-elevated px-3 py-2 text-sm"
          >
            <Phone class="h-4 w-4 text-bronze" /> {{ trainee.phone }}
          </a>
        </div>
        <p v-if="trainee.notes" class="mt-3 rounded-xl bg-elevated px-3 py-2 text-sm text-muted">{{ trainee.notes }}</p>
        <div class="mt-4 flex gap-2">
          <RouterLink :to="`/trainees/${id}/edit`" :class="btnClasses('ghost', 'sm')"><Pencil class="h-4 w-4" /> Edit</RouterLink>
          <button :class="btnClasses('danger', 'sm')" @click="remove"><Trash2 class="h-4 w-4" /> Delete</button>
        </div>
      </Card>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Payment history</h2>
        <ul class="space-y-2">
          <li
            v-for="p in (payments ?? []).filter((x) => x.trainee === id)"
            :key="p.id"
            class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5"
          >
            <div>
              <p class="text-sm font-medium capitalize">{{ p.type }}</p>
              <p class="text-xs text-faint">{{ formatLongDate(p.date) }}</p>
            </div>
            <span class="font-display text-sm tnum">{{ money(p.amount) }}</span>
          </li>
          <li v-if="(payments ?? []).filter((x) => x.trainee === id).length === 0" class="px-1 text-sm text-faint">
            No payments recorded yet.
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
