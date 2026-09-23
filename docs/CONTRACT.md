# API contract

All routes live under `/api`. Responses are JSON unless noted. Sample
responses in [`fixtures/`](fixtures/) are recorded from the real API by
`TestRecordContractFixtures` (`UPDATE_FIXTURES=1 go test ./internal/server/ -run Fixtures`);
ids, instants and the current month are replaced by `<id>`, `<timestamp>`, `<YYYY-MM>`.

Compatibility rule: existing fields and routes stay; new ones are appended.
Old clients keep working, except where a money or stock rule now requires
input they never sent (a reason for voids and corrections — see below).

## Dates and money

| Concept | Format | Meaning |
|---|---|---|
| Instant (`date`, `start`, `createdAt`, …) | RFC3339, stored UTC | When something happened. Rendered in the studio timezone. |
| Studio day (`day`, `dueDay`, `effectiveDate`) | `YYYY-MM-DD` | A calendar day in `STUDIO_TZ` (default `Asia/Beirut`), never the browser's zone. |
| Month (`m`, `periodMonth`, `billingFromMonth`) | `YYYY-MM` | A calendar month in `STUDIO_TZ`. |
| Range (`?from=&to=`) | RFC3339 or `YYYY-MM-DD` | Half-open `[from, to)`. A date-only bound is the start of that studio day, so `to=2026-09-28` excludes the 28th. Malformed bounds are rejected (400), not widened. |
| Money | number, 2 decimals | Rounded to the cent on every write. Dues balances are integer cents internally. |

A subscription payment has **two dates**: `date` is when the cash was
received (Financials and the statement filter by it); `periodMonth` is the
month the fee covers (dues filter by it).

## Errors

```json
{ "error": "human message", "code": "STABLE_CODE", "field": "amount", "details": { } }
```

`code` is stable; `field` names the input; `details` is structured. Codes:
`VALIDATION`, `INVALID_ID`, `NOT_FOUND`, `REASON_REQUIRED`, `INSUFFICIENT_STOCK`,
`OVERPAYMENT` (details: due/paid/remaining), `NO_CHARGE`, `UNVERIFIED_DUE`,
`DUE_BELOW_PAID`, `ALREADY_VOIDED`, `VOIDED_LOCKED`, `LINKED_SALE`,
`SCHEDULE_CONFLICT` (details: conflicts), `CAPACITY_EXCEEDED`, `DUPLICATE`
(details: existing), `TRAINEE_NOT_FOUND`, `ITEM_NOT_FOUND`, `ITEM_INACTIVE`,
`RETURN_EXCEEDS_SALE`, `CONFLICT`, `NO_TRANSACTIONS`, `UNAUTHORIZED`, `RATE_LIMITED`.
Example: [`fixtures/error-overpayment.json`](fixtures/error-overpayment.json).

## Routes

### Health & auth
| Method | Path | Notes |
|---|---|---|
| GET | `/health` | Public. `{status, db, authRequired, time, timezone}` — the SPA adopts `timezone` so it files days exactly as the API does |
| POST | `/auth/login` | `{username, password, remember}` → sets the `bb_session` HttpOnly cookie |
| POST | `/auth/logout` | Revokes the session and expires the cookie |
| GET | `/auth/me` | `{username}` |

### Trainees
| Method | Path | Notes |
|---|---|---|
| GET | `/trainees?q=` | Array of [trainee](fixtures/trainee.json) |
| POST | `/trainees` | Creates the trainee, its first membership term (billing from this month) and this month's charge |
| GET | `/trainees/:id` | |
| PUT | `/trainees/:id` | Details edit. A fee/status change appends a term: `termsApplyFrom: "next_month"` (default) or `"this_month"` (re-prices this month's issued charge; needs `termsReason`). A second change on the same day returns `DUPLICATE` with the existing term unless `termsConfirm: true`. |
| GET | `/trainees/:id/terms` | Effective-dated terms, newest first |
| POST | `/trainees/:id/terms` | `{effectiveDate, billingFromMonth, monthlyFee, status, reason?, confirm?}` |
| DELETE | `/trainees/:id` | Hard delete (history keeps name snapshots) |

`monthlyFee`/`status` on a trainee summarize the latest recorded terms (the
fee going forward, from `feeFromMonth`). Historical dues never come from them.

### Dues and payments
| Method | Path | Notes |
|---|---|---|
| GET | `/subscriptions?m=YYYY-MM` | The month's dues roster — [rows](fixtures/subscriptions.json) `{trainee, due, amountPaid, state, remaining, periodMonth, chargeId, source, projected, paymentCount, lastPaymentDate}`. Past/current months read stored charges (materialized idempotently from terms); future months add projected rows. Inactive/removed trainees keep their issued rows. |
| GET | `/subscription-charges?trainee=&state=` | Stored charges, newest month first ([sample](fixtures/subscription-charges.json)) |
| POST | `/subscription-charges/:id/adjust` | `{due, reason}` — audited; refused below what's paid (`DUE_BELOW_PAID`). Reconciles an imported charge. |
| GET | `/payments?m=` \| `?from=&to=` `&trainee=` | By cash `date` |
| POST | `/payments` | `{trainee, amount, type, periodMonth, date? \| day?, note}`. Subscriptions need a trainee and `periodMonth`, must fit the month's remaining balance (`OVERPAYMENT`), and a month with no dues is `NO_CHARGE`. [Sample](fixtures/payment-partial.json) |
| GET | `/payments/:id` | |
| PUT | `/payments/:id` | Correction; an amount change needs `reason`. Moves money between charges in one transaction. A sale mirror can't be reclassified (`LINKED_SALE`). |
| POST | `/payments/:id/void` | `{reason}` (required). `DELETE /payments/:id?reason=` is the legacy spelling. |
| GET | `/payments/:id/receipt` | `{studio, payment, issued, void, charge?}` |
| GET | `/payments/export?m=` | CSV |

Dues states: `unpaid` (nothing paid), `partial` (0 < paid < due), `paid`,
`waived` (due adjusted to 0), `unverified` (imported, due unknown — outside
paid/unpaid metrics until reconciled), `upcoming` (future month, nothing paid).

### Expenses & financials
| Method | Path | Notes |
|---|---|---|
| GET | `/expenses?m=` | |
| POST | `/expenses` | `{amount, category, note, date? \| day?}` ([sample](fixtures/expense.json)) |
| PUT | `/expenses/:id` | Amount change needs `reason` |
| POST | `/expenses/:id/void` | `{reason}` (required); `DELETE ?reason=` legacy |
| GET | `/financials?m=` \| `?from=&to=` | `{income, outgoings, net, byType, byCategory, from, to}` — net **cash** |
| GET | `/financials/export?m=` | CSV statement; voided rows listed, excluded from totals |

### Inventory & sales
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/inventory` | Create opens the item's stock ledger ([item](fixtures/item.json)) |
| GET/PUT/DELETE | `/inventory/:id` | A PUT that changes `stock` is a correction: needs `reason`, writes a ledger line |
| POST | `/inventory/:id/sell` | `{qty ≥ 1, trainee?, unitPrice?, priceReason?}` — one transaction: stock, sale (snapshots `unitCost`, `listPrice`), ledger line, audit ([sale](fixtures/sale.json)) |
| GET | `/inventory/:id/movements` | Stock ledger, newest first ([sample](fixtures/stock-movements.json)) |
| GET | `/sales?from=&to=&trainee=&item=` | |
| GET/PUT | `/sales/:id` | A quantity change needs `reason` |
| POST | `/sales/:id/void` | `{reason}` (required); restocks units still out; `DELETE ?reason=` legacy |

### Sessions, reminders, dashboard, search, audit
| Method | Path | Notes |
|---|---|---|
| GET | `/sessions?from=&to=` | `[from, to)` by `start` |
| POST | `/sessions` | `durationMin` 1–1440; attendees must exist and be unique ([sample](fixtures/session.json)) |
| POST | `/sessions/recurring` | `{title, type, weekdays[0-6], time "HH:MM", durationMin, fromDay, toDay, attendees}` — studio days, inclusive; each occurrence is at the studio wall-clock `time` (stable across DST). Span ≤ 366 days, ≤ 200 occurrences. Every occurrence starts `scheduled`, even if dated in the past. Whole series rejected on any clash (`SCHEDULE_CONFLICT`, details list the dates). Legacy `from`/`to` instants still accepted. |
| GET/PUT/DELETE | `/sessions/:id` | |
| PATCH | `/sessions/:id/attendance` | `{trainee, status}` |
| GET/POST | `/reminders` | `dueDay` (`YYYY-MM-DD`) preferred; `dueDate` accepted. Filters: `status=open\|done`, `priority`, `relatedType`+`relatedId` |
| GET/PUT/DELETE | `/reminders/:id` | Completing a recurring reminder creates the next instance |
| POST | `/reminders/:id/snooze` | `{days}` or `{until}` |
| GET | `/dashboard` | Adds `partialCount`, `unpaidCount`, `outstanding` |
| GET | `/search?q=` | |
| GET | `/audit/:entity/:id` | `entity` ∈ payment, expense, sale, charge, item, trainee, series, session, plan. Lines carry `actor` and `reason`. |

## Frontend date handling

The SPA never uses the browser's zone for a calendar decision
(`frontend/src/lib/studio.ts`): days and months of API instants are computed
in the studio zone, a session at "18:00" is sent as the instant the studio
clock reads 18:00 (`studioInstant`), and week/month navigation uses day and
month strings. The period on screen lives in the URL (`/schedule?week=`,
`/payments?m=`, `/financials?m=`, reminder filters), and forms return to the
page they were opened from (`?back=`).

## Collections added

| Collection | Purpose |
|---|---|
| `subscription_terms` | Append-only effective-dated fee/status per trainee |
| `subscription_charges` | One fixed row per (trainee, month); unique index; `paidCents` projection guarded transactionally |
| `stock_movements` | Append-only stock ledger; movements sum to `stock` |
| `schema_migrations` | Applied migration ids |

## Phase 0–1 migration impact

Run `migrate -dry-run` first; see DEPLOY.md.

1. `2026-09-001-core-indexes` — 19 indexes, including the unique `(trainee, periodMonth)` on charges.
2. `2026-09-002-stock-ledger-opening` — one `opening` movement per item equal to today's stock. Stock history before it is not reconstructed; historical end-of-month stock is only available from this point.
3. `2026-09-003-subscription-terms-charges` — one `migrated` term per trainee (today's fee/status, billing from the migration month, effective from the trainee's creation date — an inference, flagged by `source`). Charges: the migration month from the terms; every earlier (trainee, month) with live subscription payments becomes `imported_unverified` with due = paid; **no unpaid charge is invented for any earlier month**. Subscription payments without a trainee or period are listed for review and count toward no dues (as before).
4. `2026-09-004-reminder-due-day` — stores each reminder's studio-local `dueDay`.

Behaviour changes a client will notice: voids and amount/quantity/stock
corrections require a reason; subscription payments require a trainee and a
valid period; unknown enum values, negative fees, zero durations and
references to deleted trainees are rejected; `from`/`to` are half-open and
parsed in studio time.
