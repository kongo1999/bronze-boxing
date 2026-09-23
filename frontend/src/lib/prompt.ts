import { reactive } from "vue";

// A single app-wide prompt (rendered by PromptHost). Voids and material
// corrections of money or stock must say why; asking through one shared
// dialog keeps that a two-tap job everywhere instead of a bespoke form per
// screen. Resolves to the trimmed text, or null if cancelled.

export interface PromptOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  placeholder?: string;
  /** Quick picks shown as chips; tapping one fills the field. */
  suggestions?: string[];
  /** Destructive actions confirm in the danger tone. */
  tone?: "danger" | "primary";
  /** When false the text may be left empty (a plain confirm with a note). */
  required?: boolean;
  initial?: string;
}

interface PromptState extends PromptOptions {
  open: boolean;
  value: string;
  resolve?: (v: string | null) => void;
}

const state = reactive<PromptState>({ open: false, title: "", value: "" });

export function usePromptState() {
  return state;
}

export function askReason(opts: PromptOptions): Promise<string | null> {
  // A second prompt while one is open cancels the first.
  state.resolve?.(null);
  return new Promise((resolve) => {
    Object.assign(state, {
      message: undefined,
      confirmLabel: undefined,
      placeholder: undefined,
      suggestions: undefined,
      tone: "danger",
      required: true,
      initial: "",
      ...opts,
      open: true,
      value: opts.initial ?? "",
      resolve,
    });
  });
}

export function settlePrompt(value: string | null): void {
  const r = state.resolve;
  state.open = false;
  state.resolve = undefined;
  r?.(value === null ? null : value.trim());
}

// Common reasons, so the usual cases are one tap.
export const VOID_REASONS = ["Entered twice", "Wrong amount", "Wrong person", "Refunded"];
export const CORRECTION_REASONS = ["Typo", "Miscounted", "Price agreed differently"];
