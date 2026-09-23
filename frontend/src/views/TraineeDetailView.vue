<script setup lang="ts">
import { ref, computed } from "vue";
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Pencil, Trash2, Phone, Wallet } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync, invalidate } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { Trainee, Payment, SubStatus } from "@/lib/types";
import { money, formatLongDate, monthKey, monthLabel } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Badge from "@/components/ui/Badge.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import Pagination from "@/components/ui/Pagination.vue";
import { btnClasses } from "@/components/ui/button";
import { toast } from "@/lib/toast";
import { STATE_LABEL, STATE_TONE, collectable, collectLink } from "@/lib/dues";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;
const month = monthKey();

const { data: trainee, loading, error } = useCachedAsync(`/trainees/${id}`, () => api.get<Trainee>(`/trainees/${id}`));
const { data: payments } = useCachedAsync("payments:all", () => api.get<Payment[]>(`/payments?from=2000-01-01&to=2100-01-01`));
const { data: subs } = useCachedAsync(`subs:${month}`, () => api.get<SubStatus[]>(`/subscriptions?m=${month}`));

// This month's dues, straight from the same endpoint the Money page uses — so
// the two screens can never disagree about who owes what.
const dues = computed(() => (subs.value ?? []).find((s) => s.trainee.id === id));
const remaining = computed(() => dues.value?.remaining ?? 0);

// Voided payments stay visible (struck through) — the ledger never hides a
// record, it marks it.
const history = computed(() =>
  (payments.value ?? []).filter((p) => p.trainee === id),
);
const { page, pageCount, items, total, from, to } = usePaged(history, 8);

const collectHref = computed(() =>
  dues.value ? collectLink(dues.value) : `/payments/new?trainee=${id}&type=subscription&periodMonth=${month}`,
);

const deleting = ref(false);
async function remove() {
  if (deleting.value || !confirm("Delete this trainee? Their payment records stay in the books.")) return;
  deleting.value = true;
  try {
    await api.del(`/trainees/${id}`);
    invalidate("trainees", "dues", "subs");
    router.push("/trainees");
  } catch (e) {
    deleting.value = false;
    toast(e instanceof Error && e.message ? e.message : "Couldn't delete trainee.", "error");
  }
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/trainees" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Crew
    </RouterLink>

    <Skeleton v-if="loading" variant="detail" />

    <Alert v-else-if="error">{{ error }}</Alert>

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
          <button :class="btnClasses('danger', 'sm')" :disabled="deleting" @click="remove"><Trash2 class="h-4 w-4" /> {{ deleting ? "Deleting…" : "Delete" }}</button>
        </div>
      </Card>

      <!-- Dues: what this month looks like, and the one tap that settles it. -->
      <Card class="flex items-center justify-between gap-3 p-4">
        <div class="min-w-0">
          <p class="label-eyebrow text-[0.6rem] text-faint">Dues · {{ monthLabel(month) }}</p>
          <p v-if="dues" class="mt-0.5 font-display text-lg tnum">
            {{ money(dues.amountPaid) }} <span class="text-faint">/ {{ money(dues.due) }}</span>
          </p>
          <p v-else class="mt-0.5 text-sm text-muted">
            {{ trainee.monthlyFee > 0 ? "Not on this month's subscription list." : "No monthly fee set." }}
          </p>
          <p v-if="dues && remaining > 0 && dues.state !== 'unverified'" class="text-xs text-faint">{{ money(remaining) }} still owed</p>
          <p v-if="dues?.state === 'unverified'" class="text-xs text-faint">Imported from older records — confirm the amount due on the Money page.</p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <Badge v-if="dues" :tone="STATE_TONE[dues.state]">{{ STATE_LABEL[dues.state] }}</Badge>
          <RouterLink
            v-if="dues ? collectable(dues) : trainee.monthlyFee > 0"
            :to="collectHref"
            :class="btnClasses('primary', 'sm')"
          ><Wallet class="h-4 w-4" /> Collect</RouterLink>
        </div>
      </Card>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Payment history</h2>
        <ul class="space-y-2">
          <li
            v-for="p in items"
            :key="p.id"
            class="flex items-center justify-between rounded-xl border border-line bg-surface px-3 py-2.5"
            :class="p.voidedAt ? 'opacity-60' : ''"
          >
            <div class="min-w-0">
              <p class="text-sm font-medium capitalize">
                {{ p.type }}
                <span v-if="p.voidedAt" class="ml-1 rounded bg-overdue/15 px-1.5 py-0.5 align-middle text-[0.625rem] font-semibold uppercase tracking-wide text-overdue">Void</span>
              </p>
              <p class="text-xs text-faint">
                {{ formatLongDate(p.date) }}{{ p.periodMonth ? ` · ${monthLabel(p.periodMonth)}` : "" }}
              </p>
            </div>
            <span class="font-display text-sm tnum" :class="p.voidedAt ? 'line-through text-faint' : ''">{{ money(p.amount) }}</span>
          </li>
          <li v-if="total === 0" class="px-1 text-sm text-faint">No payments recorded yet.</li>
        </ul>
        <Pagination v-model="page" :page-count="pageCount" :total="total" :from="from" :to="to" label="payments" />
      </section>
    </template>

    <Alert v-else>Trainee not found.</Alert>
  </div>
</template>
