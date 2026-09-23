<script setup lang="ts">
// Reads the append-only money trail for one payment / expense / sale and
// renders it as plain sentences: what changed, from what to what, when.
// Fetched lazily on first open — most rows are never inspected, and the
// endpoint is one query per record.
import { ref } from "vue";
import { History } from "lucide-vue-next";
import { api } from "@/lib/api";
import type { AuditEntry } from "@/lib/types";
import { money } from "@/lib/format";

const props = defineProps<{ entity: "payment" | "expense" | "sale"; id: string }>();

const open = ref(false);
const loading = ref(false);
const error = ref<string>();
const entries = ref<AuditEntry[]>();

async function toggle() {
  open.value = !open.value;
  if (!open.value || entries.value || loading.value) return;
  loading.value = true;
  error.value = undefined;
  try {
    entries.value = await api.get<AuditEntry[]>(`/audit/${props.entity}/${props.id}`);
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : "Couldn't load the history.";
  } finally {
    loading.value = false;
  }
}

// Fields worth showing, in the order a human would read them. Anything not
// listed (ids, timestamps, denormalised copies) is noise in a change log.
const LABELS: Record<string, string> = {
  amount: "Amount",
  type: "Type",
  category: "Category",
  periodMonth: "Period",
  note: "Note",
  traineeName: "Trainee",
  qty: "Quantity",
  unitPrice: "Unit price",
  total: "Total",
  itemName: "Item",
  voidReason: "Void reason",
};
const MONEY_FIELDS = new Set(["amount", "unitPrice", "total"]);

function show(field: string, v: unknown): string {
  if (v === undefined || v === null || v === "") return "—";
  if (MONEY_FIELDS.has(field) && typeof v === "number") return money(v);
  return String(v);
}

interface Change { label: string; before: string; after: string }

function changes(e: AuditEntry): Change[] {
  const before = (e.before ?? {}) as Record<string, unknown>;
  const after = (e.after ?? {}) as Record<string, unknown>;
  const out: Change[] = [];
  for (const [field, label] of Object.entries(LABELS)) {
    const b = before[field];
    const a = after[field];
    if (show(field, b) === show(field, a)) continue;
    out.push({ label, before: show(field, b), after: show(field, a) });
  }
  return out;
}

const when = (iso: string) =>
  new Date(iso).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
</script>

<template>
  <div>
    <button
      type="button"
      class="inline-flex items-center gap-1.5 text-xs font-medium text-faint transition-colors hover:text-bronze"
      @click="toggle"
    >
      <History class="h-3.5 w-3.5" /> {{ open ? "Hide history" : "History" }}
    </button>

    <div v-if="open" class="mt-2 space-y-2">
      <p v-if="loading" class="text-xs text-faint">Loading history…</p>
      <p v-else-if="error" class="text-xs text-overdue">{{ error }}</p>
      <p v-else-if="(entries ?? []).length === 0" class="text-xs text-faint">
        No changes recorded — this record is exactly as it was created.
      </p>
      <ul v-else class="space-y-2">
        <li v-for="e in entries" :key="e.id" class="rounded-lg border border-line bg-elevated/60 px-3 py-2">
          <p class="flex items-center justify-between gap-2 text-xs">
            <span class="font-medium" :class="e.action === 'void' ? 'text-overdue' : 'text-bronze'">
              {{ e.action === "void" ? "Voided" : "Edited" }}
            </span>
            <span class="text-faint">{{ when(e.at) }}</span>
          </p>
          <ul v-if="e.action !== 'void'" class="mt-1 space-y-0.5">
            <li v-for="c in changes(e)" :key="c.label" class="text-xs text-muted">
              {{ c.label }}: <span class="text-faint line-through">{{ c.before }}</span>
              <span class="mx-1 text-faint">→</span><span class="text-fg">{{ c.after }}</span>
            </li>
            <li v-if="changes(e).length === 0" class="text-xs text-faint">No visible field changed.</li>
          </ul>
        </li>
      </ul>
    </div>
  </div>
</template>
