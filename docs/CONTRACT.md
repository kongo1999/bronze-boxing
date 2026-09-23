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
`RETURN_EXCEEDS_SALE`, `CONFLICT`, `NO_TRANSACTIONS`, `UNAUTHORIZED`, `RATE_LIMITED`,
`PLAN_FULL` (details: plan progress), `PLAN_MISMATCH` (wrong trainee, type or
dates for the plan), `NOT_STARTED` (an outcome recorded before the class began).
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
| GET | `/trainees?q=&archived=include\|only` | Array of [trainee](fixtures/trainee.json); archived trainees are left out unless asked for |
| POST | `/trainees` | Creates the trainee, its first membership term (billing from this month) and this month's charge |
| GET | `/trainees/:id` | |
| PUT | `/trainees/:id` | Details edit. A fee/status change appends a term: `termsApplyFrom: "next_month"` (default) or `"this_month"` (re-prices this month's issued charge; needs `termsReason`). A second change on the same day returns `DUPLICATE` with the existing term unless `termsConfirm: true`. |
| GET | `/trainees/:id/terms` | Effective-dated terms, newest first |
| POST | `/trainees/:id/terms` | `{effectiveDate, billingFromMonth, monthlyFee, status, reason?, confirm?}` |
| GET | `/trainees/:id/links` | Counts of linked records `{payments, sessions, sales, charges, plans}` |
| POST | `/trainees/:id/archive` | `{reason?}` — hides from the roster and appends an inactive term (billing stops from next month); history kept; audited. `/unarchive` restores. |
| DELETE | `/trainees/:id?confirmName=` | Hard delete. With linked records it's refused (`CONFLICT`, details = links) unless `confirmName` is the trainee's exact name; archiving is the safe path. |

`monthlyFee`/`status` on a trainee summarize the latest recorded terms (the
fee going forward, from `feeFromMonth`). Historical dues never come from them.

### Dues and payments
| Method | Path | Notes |
|---|---|---|
| GET | `/subscriptions?m=YYYY-MM` | The month's dues roster — [rows](fixtures/subscriptions.json) `{trainee, due, amountPaid, state, remaining, periodMonth, chargeId, source, projected, paymentCount, lastPaymentDate}`. Past/current months read stored charges (materialized idempotently from terms); future months add projected rows. Inactive/removed trainees keep their issued rows. |
| GET | `/subscription-charges?trainee=&state=` | Stored charges, newest month first ([sample](fixtures/subscription-charges.json)) |
| POST | `/subscription-charges/:id/adjust` | `{due, reason}` — audited; refused below what's paid (`DUE_BELOW_PAID`). Reconciles an imported charge. |
| GET | `/payments?m=` \| `?from=&to=` `&trainee=&periodMonth=&type=` | By cash `date`; `periodMonth` matches the dues period at any cash date. With `limit`(+`offset`) the response is a page `{items,total,hasMore,offset,limit}`, otherwise the original array. A bare `/payments` (no filter, no limit) is refused rather than downloading everything. |
| POST | `/payments` | `{trainee, amount, type, periodMonth, date? \| day?, method?, reference?, note}` — `method` ∈ cash, card, bank_transfer, other (absent on old rows = "unspecified"). Subscriptions need a trainee and `periodMonth`, must fit the month's remaining balance (`OVERPAYMENT`), and a month with no dues is `NO_CHARGE`. [Sample](fixtures/payment-partial.json) |
| GET | `/payments/:id` | |
| PUT | `/payments/:id` | Correction; an amount change needs `reason`. Moves money between charges in one transaction. A sale mirror can't be reclassified (`LINKED_SALE`). |
| POST | `/payments/:id/void` | `{reason}` (required). `DELETE /payments/:id?reason=` is the legacy spelling. |
| GET | `/payments/:id/receipt` | `{studio, studioInfo{name,address,phone,currency}, number, payment, issued, void, method, cashDay, timezone, charge?}` — `number` is the last 8 hex of the id; the SPA prints/shares it (`/payments/:id/receipt`) |
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
| GET | `/financials?m=` \| `?from=&to=` | `{income, outgoings, net, byType, byCategory, byMethod, counts, from, to, previous{income,outgoings,net,from,to}}` — net **cash** (money in − recorded money out by cash date; not profit). `previous` is the prior month, or the same-length window before a custom range. Income = payments + shop sales − returns. |
| GET | `/ledger?m= \| from&to &kind=payment\|sale\|return\|expense\|income &type= &method= &trainee= &q= &voided=0 &limit &offset` | Every money row, newest first, filtered on the server before paging: `{items, total, hasMore, totals, from, to}`. Voided rows are listed (flagged) and never counted. |
| GET | `/financials/export` | CSV of exactly the ledger rows for the same period and filters (oldest first) with Method, Reference, Period and Void-reason columns, then Total in / Total out / Net cash. |
| GET | `/cash-closings?m=` | Per studio day with income or a count: expected cash (cash + unspecified-method payments + shop sales − refunds) vs the counted closing amount, and the difference |
| PUT | `/cash-closings/:day` | `{counted, note}` — the till count for a past or current day |

### Inventory & sales
| Method | Path | Notes |
|---|---|---|
| GET | `/inventory?active=true\|false&stock=short\|low\|out` | By name. `out` = no stock; `low` = at or under `lowStockThreshold` (when set); `short` = either |
| POST | `/inventory` | Create opens the item's stock ledger ([item](fixtures/item.json)) |
| GET/PUT | `/inventory/:id` | PUT edits details; a PUT that changes `stock` is still accepted as a guarded correction (needs `reason`), but the adjustments endpoint is the normal path |
| DELETE | `/inventory/:id` | Only for an item with no sales and no movement beyond its opening line; otherwise `CONFLICT` (details: sales, movements) — archive instead |
| POST | `/inventory/:id/archive` \| `/unarchive` | `{reason?}` — archived items can't be sold (`ITEM_INACTIVE`); history kept; audited |
| POST | `/inventory/:id/adjustments` | `{kind: "restock", qty, unitCost?}` · `{kind: "damage", qty, reason}` · `{kind: "correction", count, expected?, reason}` — one transaction: stock, ledger line, audit. A write-off can't exceed stock (`INSUFFICIENT_STOCK`); a correction whose `expected` no longer matches is `CONFLICT`. Returns `{item, movement}` |
| POST | `/inventory/:id/sell` | `{qty ≥ 1, trainee?, method?, unitPrice?, priceReason?}` — one transaction: stock, sale (snapshots `unitCost`, `listPrice`), ledger line, audit. A price different from the list price needs `priceReason` ([sale](fixtures/sale.json)) |
| GET | `/inventory/:id/movements?limit=` | Stock ledger, newest first ([sample](fixtures/stock-movements.json)) |
| GET | `/sales?from=&to=&trainee=&item=&limit=&offset=` | Newest first; a page with `limit` |
| GET/PUT | `/sales/:id` | A quantity change needs `reason`; can't go below what was returned |
| POST | `/sales/:id/returns` | `{qty, amount?, reason, day?}` — partial return: restocks `qty`, records the refund (default qty × unit price) as money out on `day` (not in the future). Capped by units still out and money not yet refunded (`RETURN_EXCEEDS_SALE`); guarded against racing returns; refused on a voided sale. Returns `{return, sale, restocked}` |
| GET | `/sales/:id/returns` | The sale's returns, oldest first |
| GET | `/sales/:id/receipt` | `{studioInfo, number: "S-…", sale, returns, net, issued, void, method, cashDay, timezone}` |
| POST | `/sales/:id/void` | `{reason}` (required); restocks units still out; its returns stop counting too; `DELETE ?reason=` legacy |

### Sessions, reminders, dashboard, search, audit
| Method | Path | Notes |
|---|---|---|
| GET | `/sessions?from=&to=&trainee=&plan=&series=&order=desc&limit=&offset=` | `[from, to)` by `start`. With `limit` a page; without any filter or limit it's refused. |
| POST | `/sessions` | `durationMin` 1–1440; `capacity` (0 = no limit); attendees `{trainee, status, planId?}` must exist and be unique; a `planId` must fit the plan (`PLAN_MISMATCH`) and not overbook it (`PLAN_FULL`, `planOverride: true` to book anyway) ([sample](fixtures/session.json)) |
| POST | `/sessions/recurring/preview` | Same body as `/recurring`; returns `{count, occurrences[{start, conflict?}], conflicts, planProblem?{code, error, details}}` without writing |
| POST | `/sessions/recurring` | `{title, type, weekdays[0-6], time "HH:MM", durationMin, fromDay, toDay, attendees}` — studio days, inclusive; each occurrence is at the studio wall-clock `time` (stable across DST). Span ≤ 366 days, ≤ 200 occurrences. Every occurrence starts `scheduled`, even if dated in the past. Whole series rejected on any clash (`SCHEDULE_CONFLICT`, details list the dates). Legacy `from`/`to` instants still accepted. Creates a `session_series` document (`plannedCount`) and all occurrences in one transaction; attendees start `booked`. |
| GET | `/sessions/series?ids=` | Progress per series: `{planned, completed, scheduled, cancelled, attendanceNeeded, inferred, status}` |
| GET | `/sessions/series/:seriesId/progress` | Progress plus every occurrence `{id, start, status, booked, attendanceNeeded}` |
| PATCH | `/sessions/series/:seriesId` | `{fromOccurrence, scope: "future", changes{title?, time?, durationMin?, type?, location?, capacity?, status?}, reason}` — changes this and later occurrences, never completed ones; atomic and audited; re-checks clashes |
| POST | `/sessions/series/:seriesId/extend` | `{toDay, reason}` — adds occurrences on the same pattern; `plannedCount` grows; audited |
| POST | `/sessions/series/:seriesId/end` | `{fromOccurrence?, reason}` — cancels scheduled occurrences from that one (default: from now); `plannedCount` shrinks; completed classes untouched; audited |
| GET/PUT/DELETE | `/sessions/:id` | PUT is partial. DELETE is refused once attendance is recorded (cancel instead). |
| PATCH | `/sessions/:id/attendance` | `{trainee, status, planId?, override?}` — books (atomically within `capacity`, else `CAPACITY_EXCEEDED`) or updates; `planId: ""` unlinks |
| POST | `/sessions/:id/attendance/bulk` | `{status, only?: "booked"}` or `{status, trainees[]}` — e.g. mark everyone still booked as attended |
| GET | `/trainees/:id/session-plans` | The trainee's plans with computed `progress` |
| POST | `/trainees/:id/session-plans` | `{title, targetCount ≥ 1, startDate, endDate?, sessionType?, notes?}` |
| GET | `/session-plans?trainees=a,b&status=` | Plans for several trainees (booking forms) |
| GET/PATCH | `/session-plans/:id` | PATCH `{title?, targetCount?, startDate?, endDate?, sessionType?, status?, notes?, reason}` — a reason is required for target, dates or status changes; audited |
| GET/POST | `/reminders` | `dueDay` (`YYYY-MM-DD`) preferred; `dueDate` accepted. Filters: `status=open\|done`, `priority`, `relatedType`+`relatedId` |
| GET | `/reminders/counts` | `{overdue, today, open}` by effective (snoozed) studio day — the navigation badge |
| GET/PUT/DELETE | `/reminders/:id` | Completing a recurring reminder creates the next instance |
| POST | `/reminders/:id/snooze` | `{days}` or `{until}` |
| GET | `/dashboard` | Adds `partialCount`, `unpaidCount`, `outstanding` |
| GET | `/search?q=&kinds=&limit=5&offset=` | `{query, groups[{kind, total, hasMore, items[{kind, id, label, sub, date, amount, flag, score, typo, month, trainee}]}], didYouMean?}` — kinds: trainee, session, payment, sale, item, expense, reminder (in that order). `flag` marks void / archived / inactive / cancelled / completed / done records. `didYouMean` is set when every match needed a typo. |
| GET | `/audit/:entity/:id` | `entity` ∈ payment, expense, sale, charge, item, trainee, series, session, plan. Lines carry `actor` and `reason`. |

### Search

One matcher everywhere — `backend/internal/fuzzy` and its twin
`frontend/src/lib/fuzzy.ts`, both tested against
[`fixtures/fuzzy-cases.json`](fixtures/fuzzy-cases.json). Case, accents,
punctuation, spacing and Arabic letter variants are normalized; ranking is
exact > prefix > word prefix > substring > typo; one typo is allowed in a
word of 3–7 letters (a 3-letter word must keep its first letter), two in
longer ones; digits match exactly (phone fragments, references); words may be
in any order; `PT` = private, `no show` = no-show, `tee` = t-shirt. Every
query word must match, so unrelated records never appear.

`?q=` on `/trainees`, `/payments`, `/sessions`, `/sales` and `/ledger`
searches the whole filtered list on the server and returns it best match
first, then pages it (`limit`/`offset` as usual) — a match that would sit on
page five is still found. The list's other filters still apply.

The server keeps an in-memory index of every searchable record, built on
first use and kept current from a MongoDB change stream; before each search
it writes a marker to `search_sync` and reads the stream up to it, so a
write acknowledged before the search is always reflected (no stale results,
whatever code path or tool made the write). Without a replica set (dev-only)
it is rebuilt when older than two seconds instead.

`PUT /trainees/:id` now merges: fields left out of the body keep their
values (a partial update can no longer blank the phone or zero the fee).

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
| `cash_closings` | One counted closing amount per studio day (unique `day`) |
| `sale_returns` | Partial returns of sales (Phase 5 endpoint); counted in the cash month of the return while the sale is live |
| `session_series` | A recurring series: pattern, `plannedCount`, `inferred` for series found in old data |
| `trainee_session_plans` | A trainee's package of N sessions; progress is computed from linked attendance, never stored |

## Phase 0–1 migration impact

Run `migrate -dry-run` first; see DEPLOY.md.

1. `2026-09-001-core-indexes` — 19 indexes, including the unique `(trainee, periodMonth)` on charges.
2. `2026-09-002-stock-ledger-opening` — one `opening` movement per item equal to today's stock. Stock history before it is not reconstructed; historical end-of-month stock is only available from this point.
3. `2026-09-003-subscription-terms-charges` — one `migrated` term per trainee (today's fee/status, billing from the migration month, effective from the trainee's creation date — an inference, flagged by `source`). Charges: the migration month from the terms; every earlier (trainee, month) with live subscription payments becomes `imported_unverified` with due = paid; **no unpaid charge is invented for any earlier month**. Subscription payments without a trainee or period are listed for review and count toward no dues (as before).
4. `2026-09-004-reminder-due-day` — stores each reminder's studio-local `dueDay`.

5. `2026-09-005-closings-returns-indexes` — unique `day` on cash closings, indexes on sale returns.
6. `2026-09-006-series-and-plans` — indexes for series/plans/attendee plan links; each distinct `seriesId` already on sessions gets a `session_series` document with `inferred: true` and `plannedCount` = its non-cancelled occurrences (no sessions are changed).

Plan and series counting: a plan's `completed` counts linked bookings where the
class is completed **and** the trainee attended. A completed class where the
trainee is still `booked` is `attendanceNeeded`, not credited; no-shows and
cancellations are shown separately. A series' `completed` counts completed
occurrences. Recording an outcome (class done, attended, no-show) before the
class has started is refused (`NOT_STARTED`), with a 30-minute grace for
taking attendance at the door.

Behaviour changes a client will notice: voids and amount/quantity/stock
corrections require a reason; subscription payments require a trainee and a
valid period; unknown enum values, negative fees, zero durations and
references to deleted trainees are rejected; `from`/`to` are half-open and
parsed in studio time.
