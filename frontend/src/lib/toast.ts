import { reactive } from "vue";

// Tiny global toast bus. Used to surface mutation results that have no inline
// form to host an error (deletes, sells, attendance changes, …).
export type ToastTone = "error" | "success" | "info";
export interface Toast {
  id: number;
  message: string;
  tone: ToastTone;
}

const state = reactive<{ items: Toast[] }>({ items: [] });
let seq = 0;

export function useToasts() {
  return state;
}

export function dismissToast(id: number): void {
  const i = state.items.findIndex((t) => t.id === id);
  if (i >= 0) state.items.splice(i, 1);
}

export function toast(message: string, tone: ToastTone = "info"): void {
  const id = ++seq;
  state.items.push({ id, message, tone });
  // Auto-dismiss; errors linger a little longer so they're readable.
  setTimeout(() => dismissToast(id), tone === "error" ? 5000 : 3000);
}

// Convenience: run a mutation, toast on failure, return success boolean.
// Keeps optimistic handlers terse: `if (!(await guard(() => api.del(...), "Couldn't delete"))) revert()`.
export async function guard(fn: () => Promise<unknown>, errMsg: string): Promise<boolean> {
  try {
    await fn();
    return true;
  } catch (e) {
    toast(e instanceof Error && e.message ? e.message : errMsg, "error");
    return false;
  }
}
