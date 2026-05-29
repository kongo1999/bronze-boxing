<script setup lang="ts">
import { useRoute, useRouter, RouterLink } from "vue-router";
import { ChevronLeft, Trash2, MapPin, Clock } from "lucide-vue-next";
import { api } from "@/lib/api";
import { useAsync } from "@/lib/useAsync";
import type { Session, Attendee } from "@/lib/types";
import { formatTime } from "@/lib/format";
import Card from "@/components/ui/Card.vue";
import Badge from "@/components/ui/Badge.vue";
import Avatar from "@/components/ui/Avatar.vue";
import { btnClasses } from "@/components/ui/button";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;
const { data: session, loading } = useAsync(() => api.get<Session>(`/sessions/${id}`));

const dateLabel = (iso: string) =>
  new Date(iso).toLocaleDateString("en-US", { weekday: "long", month: "long", day: "numeric" });

// Optimistic: update the attendee chip immediately, fire one request, revert on failure.
async function setAttendance(a: Attendee, status: string) {
  const prev = a.status;
  a.status = status as Attendee["status"];
  try {
    await api.patch(`/sessions/${id}/attendance`, { trainee: a.trainee, status });
  } catch {
    a.status = prev;
  }
}
async function remove() {
  if (!confirm("Delete this session?")) return;
  await api.del(`/sessions/${id}`);
  router.push("/schedule");
}
</script>

<template>
  <div class="space-y-4">
    <RouterLink to="/schedule" class="inline-flex items-center gap-1 text-sm text-muted hover:text-fg">
      <ChevronLeft class="h-4 w-4" /> Schedule
    </RouterLink>
    <p v-if="loading" class="text-sm text-faint">Loading…</p>

    <template v-else-if="session">
      <Card class="p-4">
        <div class="flex items-start justify-between gap-2">
          <div>
            <h1 class="font-display text-xl font-semibold">{{ session.title }}</h1>
            <p class="text-sm text-muted">{{ dateLabel(session.start) }}</p>
          </div>
          <Badge :tone="session.type === 'group' ? 'bronze' : 'info'">{{ session.type === "group" ? "Group" : "Private" }}</Badge>
        </div>
        <div class="mt-3 flex flex-wrap gap-3 text-sm text-muted">
          <span class="inline-flex items-center gap-1"><Clock class="h-4 w-4" /> {{ formatTime(session.start) }} · {{ session.durationMin }}min</span>
          <span v-if="session.location" class="inline-flex items-center gap-1"><MapPin class="h-4 w-4" /> {{ session.location }}</span>
        </div>
        <button :class="[btnClasses('danger', 'sm'), 'mt-4']" @click="remove"><Trash2 class="h-4 w-4" /> Delete</button>
      </Card>

      <section class="space-y-2">
        <h2 class="px-1 label-eyebrow text-[0.625rem] text-faint">Attendance</h2>
        <ul class="space-y-2">
          <li
            v-for="a in session.attendees"
            :key="a.trainee"
            class="flex items-center gap-3 rounded-xl border border-line bg-surface px-3 py-2.5"
          >
            <Avatar :name="a.traineeName ?? '?'" class="h-8 w-8 text-xs" />
            <span class="flex-1 truncate text-sm font-medium">{{ a.traineeName }}</span>
            <div class="flex gap-1">
              <button
                v-for="st in ['attended', 'no_show', 'booked']"
                :key="st"
                class="rounded-lg border px-2 py-1 text-xs capitalize transition-colors"
                :class="a.status === st ? 'border-bronze bg-bronze/15 text-bronze' : 'border-line text-faint hover:text-fg'"
                @click="setAttendance(a, st)"
              >
                {{ st === "no_show" ? "no-show" : st }}
              </button>
            </div>
          </li>
          <li v-if="session.attendees.length === 0" class="px-1 text-sm text-faint">No one booked.</li>
        </ul>
      </section>
    </template>
  </div>
</template>
