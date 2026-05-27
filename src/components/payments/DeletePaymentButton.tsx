"use client";

import { Trash2 } from "lucide-react";
import { deletePaymentAction } from "@/app/payments/actions";

export function DeletePaymentButton({ id, returnTo }: { id: string; returnTo: string }) {
  return (
    <form
      action={deletePaymentAction}
      onSubmit={(e) => {
        if (!confirm("Delete this payment?")) e.preventDefault();
      }}
    >
      <input type="hidden" name="id" value={id} />
      <input type="hidden" name="returnTo" value={returnTo} />
      <button
        type="submit"
        aria-label="Delete payment"
        className="text-faint transition-colors hover:text-overdue"
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </form>
  );
}
