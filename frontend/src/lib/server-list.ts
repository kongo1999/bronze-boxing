// A list the API searches and pages (sales, payment and session histories):
// the whole collection is searched on the server before paging, so a match
// that would sit on page five is still found. Typing is debounced; an answer
// to an older request never overwrites a newer one.
import { computed, ref, watch, type Ref } from "vue";
import { api, errMsg } from "./api";
import type { Page } from "./types";

export function useServerList<T>(url: (q: string) => string, term: Ref<string>, pageSize = 10) {
  const page = ref(1);
  const items = ref<T[]>([]) as Ref<T[]>;
  const total = ref(0);
  const loading = ref(true);
  const searching = ref(false);
  const error = ref("");
  let token = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function load() {
    const my = ++token;
    error.value = "";
    try {
      const base = url((term.value ?? "").trim());
      const sep = base.includes("?") ? "&" : "?";
      const pg = await api.get<Page<T>>(`${base}${sep}limit=${pageSize}&offset=${(page.value - 1) * pageSize}`);
      if (my !== token) return;
      items.value = pg.items;
      total.value = pg.total;
    } catch (e) {
      if (my === token) error.value = errMsg(e, "Couldn't load the list.");
    } finally {
      if (my === token) {
        loading.value = false;
        searching.value = false;
      }
    }
  }

  watch(term, () => {
    clearTimeout(timer);
    token++; // anything in flight is stale now
    searching.value = true;
    timer = setTimeout(() => {
      if (page.value !== 1) page.value = 1; // the page watcher loads
      else load();
    }, 200);
  });
  watch(page, () => load());
  load();

  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));
  const from = computed(() => (total.value ? (page.value - 1) * pageSize + 1 : 0));
  const to = computed(() => Math.min(page.value * pageSize, total.value));
  return { items, total, page, pageCount, from, to, loading, searching, error, reload: load };
}
