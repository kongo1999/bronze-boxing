import { ref, type Ref } from "vue";
import { toast } from "./toast";

// Module-level cache shared across component mounts → instant revisits.
// Values are whatever a view stores under a string key (a list, an object, …).
const store = new Map<string, unknown>();

export function readCache<T>(key: string): T | undefined {
  return store.get(key) as T | undefined;
}
export function writeCache<T>(key: string, value: T): void {
  store.set(key, value);
}
export function clearCache(): void {
  store.clear();
}

/**
 * Stale-while-revalidate data fetch for a fixed key.
 *
 * On mount: if a cached value exists, render it instantly (no loading flash)
 * and refresh in the background; otherwise show loading until the first fetch
 * resolves. Always revalidates so data converges to server truth. Same shape
 * as the old useAsync ({ data, loading, error, reload }), so it's a drop-in.
 */
export function useCachedAsync<T>(key: string, fetcher: () => Promise<T>) {
  const cached = readCache<T>(key);
  const data = ref<T | undefined>(cached) as Ref<T | undefined>;
  const loading = ref(cached === undefined);
  const error = ref<string>();

  async function run() {
    error.value = undefined;
    try {
      const res = await fetcher();
      writeCache(key, res);
      data.value = res;
    } catch (e) {
      const msg = e instanceof Error && e.message ? e.message : "Something went wrong";
      // If we already have data on screen (cache hit / earlier success), keep it
      // and flag the failed refresh with a toast. Only block the page with an
      // inline error when there's nothing to show.
      if (data.value !== undefined) {
        toast("Couldn't refresh — showing saved data.", "error");
      } else {
        error.value = msg;
      }
    } finally {
      loading.value = false;
    }
  }
  run();
  return { data, loading, error, reload: run };
}
