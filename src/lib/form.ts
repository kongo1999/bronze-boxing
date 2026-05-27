import { ZodError } from "zod";

/** Shared shape returned by every form-backed server action (for useActionState). */
export type ActionState = {
  ok: boolean;
  error?: string;
  fieldErrors?: Record<string, string>;
};

export const initialActionState: ActionState = { ok: false };

/** Flatten a ZodError into one message per field for inline display. */
export function zodToFieldErrors(err: ZodError): Record<string, string> {
  const out: Record<string, string> = {};
  for (const issue of err.issues) {
    const key = issue.path.join(".") || "_form";
    if (!out[key]) out[key] = issue.message;
  }
  return out;
}

/** Wrap a server mutation, turning Zod failures into field errors. */
export async function runAction(fn: () => Promise<void>): Promise<ActionState> {
  try {
    await fn();
    return { ok: true };
  } catch (err) {
    if (err instanceof ZodError) {
      return { ok: false, fieldErrors: zodToFieldErrors(err) };
    }
    throw err; // redirect() and unexpected errors propagate to the framework.
  }
}
