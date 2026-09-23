import { computed, ref, watch, type Ref } from "vue";

/**
 * Client-side pagination over a list a view already has in memory.
 *
 * The studio's data is small and every page already fetches its slice by month
 * (or by week), so paging server-side would buy nothing and cost a round trip
 * per page. This keeps the same instant, cache-first feel.
 *
 * The page resets to 1 whenever the source list changes length — that's what a
 * search term being typed looks like from here, and staying on page 7 of a
 * now-2-page result reads as "no results".
 */
export function usePaged<T>(source: Ref<T[]>, pageSize = 10) {
  const page = ref(1);
  const total = computed(() => source.value.length);
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

  watch(total, () => {
    page.value = 1;
  });
  // Guard against a page going out of range any other way (e.g. deleting the
  // last row on the last page).
  watch(pageCount, (n) => {
    if (page.value > n) page.value = n;
  });

  const items = computed(() => source.value.slice((page.value - 1) * pageSize, page.value * pageSize));
  const from = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize + 1));
  const to = computed(() => Math.min(page.value * pageSize, total.value));

  return { page, pageCount, items, total, from, to, pageSize };
}
