import { ref, watch, type Ref } from "vue";
import { useRoute, useRouter, type RouteLocationRaw } from "vue-router";

/**
 * A ref mirrored into the URL query (?m=2026-09, ?week=2026-09-21, ?tab=…).
 * The page's period survives a reload, a trip to a detail/form and back, and
 * browser back/forward. Changes replace the current history entry, so
 * stepping through months doesn't bury the previous page under twelve backs.
 */
export function useQueryState<T extends string = string>(
  name: string,
  fallback: () => T,
  valid: (v: unknown) => v is T = (v): v is T => typeof v === "string" && v !== "",
): Ref<T> {
  const route = useRoute();
  const router = useRouter();
  const read = () => {
    const v = route.query[name];
    return valid(v) ? v : fallback();
  };
  const state = ref(read()) as Ref<T>;
  watch(state, (v) => {
    if (route.query[name] === v) return;
    queueQuery(router, route, name, v === fallback() ? undefined : v);
  });
  // Browser back/forward (or a link) changed the URL: follow it.
  watch(
    () => route.query[name],
    () => {
      const v = read();
      if (v !== state.value) state.value = v;
    },
  );
  return state;
}

// Several params often change in the same tick (tapping a category sets both
// ?kind= and ?type=). Separate replace() calls would each start from the old
// query and the last would drop the others, so changes are merged and
// applied as one navigation per tick.
let pending: Record<string, string | undefined> | null = null;
function queueQuery(
  router: ReturnType<typeof useRouter>,
  route: ReturnType<typeof useRoute>,
  key: string,
  value: string | undefined,
): void {
  if (!pending) {
    pending = {};
    queueMicrotask(() => {
      const changes = pending ?? {};
      pending = null;
      const query = { ...route.query };
      for (const [k, v] of Object.entries(changes)) {
        if (v === undefined) delete query[k];
        else query[k] = v;
      }
      router.replace({ query });
    });
  }
  pending[key] = value;
}

/** Change query params (undefined removes one) merged with any pending changes. */
export function usePatchQuery() {
  const router = useRouter();
  const route = useRoute();
  return (changes: Record<string, string | undefined>) => {
    for (const [k, v] of Object.entries(changes)) queueQuery(router, route, k, v);
  };
}

/** Is this a safe in-app path to return to (not an external URL)? */
export function isInternalPath(v: unknown): v is string {
  return typeof v === "string" && v.startsWith("/") && !v.startsWith("//");
}

/** Where a form or detail page should return to: ?back=… or the fallback. */
export function backTarget(query: Record<string, unknown>, fallback: string): string {
  return isInternalPath(query.back) ? query.back : fallback;
}

/** Append ?back=<current page> (plus extra query) to a link. */
export function withBack(path: string, back: string, extra: Record<string, string | undefined> = {}): RouteLocationRaw {
  const [p, q = ""] = path.split("?");
  const query: Record<string, string> = Object.fromEntries(new URLSearchParams(q));
  for (const [k, v] of Object.entries(extra)) if (v !== undefined && v !== "") query[k] = v;
  query.back = back;
  return { path: p, query };
}

/** Set one query parameter on an in-app path, keeping the rest. */
export function withQuery(path: string, key: string, value: string): string {
  const [p, q = ""] = path.split("?");
  const params = new URLSearchParams(q);
  params.set(key, value);
  return `${p}?${params.toString()}`;
}
