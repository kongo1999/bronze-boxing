<script setup lang="ts">
import { ref, computed } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { ChevronRight, UserPlus, Users, Phone, MessageCircle, SlidersHorizontal } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useCachedAsync } from "@/lib/cache";
import { usePaged } from "@/lib/paginate";
import type { SubStatus, Trainee } from "@/lib/types";
import { money, monthKey } from "@/lib/format";
import { useQueryState, withBack } from "@/lib/route-state";
import { STATE_LABEL, STATE_TONE } from "@/lib/dues";
import { telHref, whatsappHref } from "@/lib/phone";
import PageHeader from "@/components/ui/PageHeader.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Avatar from "@/components/ui/Avatar.vue";
import Badge from "@/components/ui/Badge.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import Alert from "@/components/ui/Alert.vue";
import SearchInput from "@/components/ui/SearchInput.vue";
import Pagination from "@/components/ui/Pagination.vue";
import ChipGroup from "@/components/ui/ChipGroup.vue";
import { btnClasses } from "@/components/ui/button";

const route = useRoute();
const here = computed(() => route.fullPath);

type Show = "active" | "inactive" | "all" | "archived";
const show = useQueryState<Show>("show", () => "active", (v): v is Show => ["active", "inactive", "all", "archived"].includes(v as string));
type Skill = "any" | "beginner" | "intermediate" | "advanced";
const skill = useQueryState<Skill>("skill", () => "any", (v): v is Skill => ["any", "beginner", "intermediate", "advanced"].includes(v as string));
type Dues = "any" | "owing" | "partial" | "unpaid" | "paid";
const dues = useQueryState<Dues>("dues", () => "any", (v): v is Dues => ["any", "owing", "partial", "unpaid", "paid"].includes(v as string));
type Sort = "name" | "recent" | "owed";
const sort = useQueryState<Sort>("sort", () => "name", (v): v is Sort => ["name", "recent", "owed"].includes(v as string));
const filtersOpen = ref(skill.value !== "any" || dues.value !== "any" || sort.value !== "name");

const month = monthKey();
const archived = computed(() => show.value === "archived");
const { data, loading, error, reload } = useCachedAsync("trainees", () => api.get<Trainee[]>("/trainees?archived=include"));
// This month's dues, for the owed amount and the dues filter.
const { data: subs } = useCachedAsync(`dues:${month}`, () => api.get<SubStatus[]>(`/subscriptions?m=${month}`));
const duesBy = computed(() => new Map((subs.value ?? []).map((s) => [s.trainee.id, s])));
const owed = (t: Trainee) => {
  const s = duesBy.value.get(t.id);
  return s && (s.state === "partial" || s.state === "unpaid") ? s.remaining : 0;
};

const q = ref("");
const filtered = computed(() => {
  const term = q.value.trim().toLowerCase();
  const digits = term.replace(/\D/g, "");
  const list = (data.value ?? []).filter((t) => {
    if (archived.value ? !t.archivedAt : !!t.archivedAt) return false;
    if (!archived.value && show.value !== "all" && t.status !== show.value) return false;
    if (skill.value !== "any" && t.skillLevel !== skill.value) return false;
    const s = duesBy.value.get(t.id);
    if (dues.value === "owing" && !owed(t)) return false;
    if ((dues.value === "partial" || dues.value === "unpaid" || dues.value === "paid") && s?.state !== dues.value) return false;
    if (!term) return true;
    return t.name.toLowerCase().includes(term) || (digits.length >= 3 && (t.phone ?? "").replace(/\D/g, "").includes(digits));
  });
  if (sort.value === "recent") list.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  else if (sort.value === "owed") list.sort((a, b) => owed(b) - owed(a) || a.name.localeCompare(b.name));
  else list.sort((a, b) => a.name.localeCompare(b.name));
  return list;
});
const activeCount = computed(() => (data.value ?? []).filter((t) => !t.archivedAt && t.status === "active").length);
const { page, pageCount, items, total, from, to } = usePaged(filtered, 12);
</script>

<template>
  <div class="space-y-4">
    <PageHeader eyebrow="Roster" title="Crew">
      <template #action>
        <RouterLink :to="withBack('/trainees/new', here)" :class="btnClasses('primary', 'sm')">
          <UserPlus class="h-4 w-4" /> Add
        </RouterLink>
      </template>
    </PageHeader>

    <SearchInput v-model="q" label="Search the crew" placeholder="Search by name or phone…" :matches="q ? filtered.length : undefined" />

    <div class="flex items-start gap-2">
      <ChipGroup
        v-model="show"
        class="flex-1"
        :options="[
          { v: 'active', l: 'Active', n: activeCount },
          { v: 'inactive', l: 'Inactive' },
          { v: 'all', l: 'All' },
          { v: 'archived', l: 'Archived' },
        ]"
        label="Show"
        scroll
      />
      <button
        type="button"
        class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border transition-colors"
        :class="filtersOpen || skill !== 'any' || dues !== 'any' || sort !== 'name' ? 'border-bronze text-bronze' : 'border-line text-faint hover:text-fg'"
        :aria-expanded="filtersOpen"
        aria-label="More filters and sorting"
        @click="filtersOpen = !filtersOpen"
      ><SlidersHorizontal class="h-4 w-4" /></button>
    </div>
    <div v-if="filtersOpen" class="space-y-2 rounded-2xl border border-line bg-surface p-3">
      <div>
        <span class="mb-1 block text-xs text-faint">This month's dues</span>
        <ChipGroup v-model="dues" :options="[{ v: 'any', l: 'Any' }, { v: 'owing', l: 'Owing' }, { v: 'partial', l: 'Partial' }, { v: 'unpaid', l: 'Unpaid' }, { v: 'paid', l: 'Paid' }]" label="Dues" scroll />
      </div>
      <div>
        <span class="mb-1 block text-xs text-faint">Level</span>
        <ChipGroup v-model="skill" :options="[{ v: 'any', l: 'Any' }, { v: 'beginner', l: 'Beginner' }, { v: 'intermediate', l: 'Intermediate' }, { v: 'advanced', l: 'Advanced' }]" label="Skill level" scroll />
      </div>
      <div>
        <span class="mb-1 block text-xs text-faint">Sort</span>
        <ChipGroup v-model="sort" :options="[{ v: 'name', l: 'Name' }, { v: 'recent', l: 'Newest' }, { v: 'owed', l: 'Most owed' }]" label="Sort" />
      </div>
    </div>

    <Skeleton v-if="loading" :rows="5" />

    <Alert v-else-if="error">
      {{ error }}
      <button class="ml-1 font-medium underline" @click="reload">Retry</button>
    </Alert>

    <EmptyState
      v-else-if="filtered.length === 0"
      :icon="Users"
      :title="q || dues !== 'any' || skill !== 'any' ? 'No matches' : archived ? 'No archived trainees' : 'No trainees here'"
      :description="q ? 'Try a different name or phone digits.' : archived ? 'Archived trainees keep their full history.' : 'Add your first trainee to start tracking dues and attendance.'"
    >
      <RouterLink v-if="!q && show === 'active'" :to="withBack('/trainees/new', here)" :class="btnClasses('primary', 'sm')"><UserPlus class="h-4 w-4" /> Add trainee</RouterLink>
    </EmptyState>

    <template v-else>
      <ul class="space-y-2">
        <li v-for="t in items" :key="t.id" class="flex items-center gap-2 rounded-2xl border border-line bg-surface pr-2 transition-colors hover:border-bronze/30">
          <RouterLink :to="withBack(`/trainees/${t.id}`, here)" class="flex min-w-0 flex-1 items-center gap-3 p-3">
            <Avatar :name="t.name" :class="t.status === 'inactive' || t.archivedAt ? 'opacity-50' : ''" />
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium">{{ t.name }}</p>
              <p class="truncate text-sm text-muted">
                {{ t.phone || (t.skillLevel ? t.skillLevel : "No phone") }}
              </p>
            </div>
            <div class="flex flex-col items-end gap-1">
              <span v-if="owed(t)" class="font-display text-sm tnum text-partial">{{ money(owed(t)) }} <span class="text-xs text-faint">owed</span></span>
              <span v-else-if="t.monthlyFee > 0" class="font-display text-sm tnum">{{ money(t.monthlyFee) }}<span class="text-xs text-faint">/mo</span></span>
              <Badge v-if="duesBy.get(t.id) && duesBy.get(t.id)!.state !== 'paid'" :tone="STATE_TONE[duesBy.get(t.id)!.state]">{{ STATE_LABEL[duesBy.get(t.id)!.state] }}</Badge>
              <Badge v-else-if="t.status === 'inactive'" tone="neutral">Inactive</Badge>
            </div>
          </RouterLink>
          <a v-if="telHref(t.phone)" :href="telHref(t.phone)" class="grid h-10 w-10 shrink-0 place-items-center rounded-lg text-bronze hover:bg-elevated" :aria-label="`Call ${t.name}`"><Phone class="h-4 w-4" /></a>
          <a v-if="whatsappHref(t.phone)" :href="whatsappHref(t.phone)" target="_blank" rel="noopener" class="grid h-10 w-10 shrink-0 place-items-center rounded-lg text-paid hover:bg-elevated" :aria-label="`WhatsApp ${t.name}`"><MessageCircle class="h-4 w-4" /></a>
          <ChevronRight v-if="!t.phone" class="h-4 w-4 shrink-0 text-faint" />
        </li>
      </ul>
      <Pagination v-model="page" :page-count="pageCount" :total="total" :from="from" :to="to" label="trainees" />
    </template>
  </div>
</template>
