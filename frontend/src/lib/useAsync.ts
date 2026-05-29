import { ref, type Ref } from "vue";

// Minimal async-data helper: runs immediately, exposes loading/error/reload.
export function useAsync<T>(fn: () => Promise<T>) {
  const data = ref<T>() as Ref<T | undefined>;
  const loading = ref(true);
  const error = ref<string>();

  async function run() {
    loading.value = true;
    error.value = undefined;
    try {
      data.value = await fn();
    } catch (e) {
      error.value = e instanceof Error ? e.message : "Something went wrong";
    } finally {
      loading.value = false;
    }
  }
  run();
  return { data, loading, error, reload: run };
}
