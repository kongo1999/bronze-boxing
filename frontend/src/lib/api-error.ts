/**
 * An error the API returned. `code` is stable and machine-readable
 * (e.g. OVERPAYMENT, INSUFFICIENT_STOCK, REASON_REQUIRED, SCHEDULE_CONFLICT);
 * `field` names the input it concerns, so forms can show the message beside
 * the right field; `details` carries structured extras (remaining balance,
 * conflicting dates, the existing record being duplicated…).
 *
 * Its own module so the demo data layer can raise the same errors as the
 * API without importing the client.
 */
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string,
    public field?: string,
    public details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function isApiError(e: unknown, code?: string): e is ApiError {
  return e instanceof ApiError && (code === undefined || e.code === code);
}
