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
    const query = { ...route.query };
    if (v === fallback()) delete query[name];
    else query[name] = v;
    router.replace({ query });
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
