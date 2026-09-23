import { ref, type Ref } from "vue";
import { toast } from "./toast";

// Module-level cache shared across component mounts → instant revisits.
// Values are whatever a view stores under a string key (a list, an object, …).
//
// Keys are namespaced by domain — "trainees", "trainees:<id>", "dues:2026-09",
// "payments:2026-09", "sessions:week:2026-09-21", "inventory", … — so a
// mutation invalidates just the domains it touched instead of wiping every
// screen's saved data.
const store = new Map<string, unknown>();

/** Normalise legacy path-style keys ("/trainees/abc") to domain form. */
function norm(key: string): string {
  return key.replace(/^\//, "").replace(/\//g, ":");
}

export function readCache<T>(key: string): T | undefined {
  return store.get(norm(key)) as T | undefined;
}
export function writeCache<T>(key: string, value: T): void {
  store.set(norm(key), value);
}
/** Drop everything (sign-in/out only). */
export function clearCache(): void {
  store.clear();
}

/**
 * Drop cached entries for the given domains: "dues" removes "dues" and every
 * "dues:…" key. Call after a mutation with each domain it affected.
 */
export function invalidate(...domains: string[]): void {
  const ds = domains.map(norm);
  for (const k of [...store.keys()]) {
    if (ds.some((d) => k === d || k.startsWith(d + ":"))) store.delete(k);
  }
}

/**
 * Stale-while-revalidate data fetch for a fixed key.
 *
 * On mount: if a cached value exists, render it instantly (no loading flash)
 * and refresh in the background; otherwise show loading until the first fetch
 * resolves. Always revalidates so data converges to server truth. A response
 * that arrives after a newer request was started is ignored, so an old answer
 * never overwrites a fresher one.
 */
export function useCachedAsync<T>(key: string, fetcher: () => Promise<T>) {
  const cached = readCache<T>(key);
  const data = ref<T | undefined>(cached) as Ref<T | undefined>;
  const loading = ref(cached === undefined);
  const error = ref<string>();
  let token = 0;

  async function run() {
    const my = ++token;
    error.value = undefined;
    try {
      const res = await fetcher();
      if (my !== token) return;
      writeCache(key, res);
      data.value = res;
    } catch (e) {
      if (my !== token) return;
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
      if (my === token) loading.value = false;
    }
  }
  run();
  return { data, loading, error, reload: run };
}
