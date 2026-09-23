// A search box backed by the API: debounced (~200 ms, no Enter needed),
// with out-of-order answers ignored, and "searching" kept distinct from
// "no matches" so an empty list never flashes while a request is in flight.
import { ref, watch, type Ref } from "vue";
import { errMsg } from "./api";

export function useRemoteSearch<T>(
  term: Ref<string>,
  fetcher: (q: string) => Promise<T>,
  opts: { delay?: number; minLength?: number } = {},
) {
  const result = ref<T>() as Ref<T | undefined>;
  const searching = ref(false);
  const error = ref("");
  let timer: ReturnType<typeof setTimeout> | undefined;
  let token = 0;

  async function run(q: string) {
    const my = ++token;
    searching.value = true;
    error.value = "";
    try {
      const res = await fetcher(q);
      if (my === token) result.value = res;
    } catch (e) {
      if (my === token) error.value = errMsg(e, "Search failed.");
    } finally {
      if (my === token) searching.value = false;
    }
  }

  watch(
    term,
    (val) => {
      clearTimeout(timer);
      token++; // anything in flight is now stale
      const q = (val ?? "").trim();
      if (q.length < (opts.minLength ?? 1)) {
        result.value = undefined;
        searching.value = false;
        error.value = "";
        return;
      }
      searching.value = true;
      timer = setTimeout(() => run(q), opts.delay ?? 200);
    },
    { immediate: true },
  );

  return { result, searching, error, retry: () => run((term.value ?? "").trim()) };
}
