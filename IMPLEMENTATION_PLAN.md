# Bronze Boxing implementation plan

## Purpose and scope

Implement the usability and feature improvements from the September 2026 project review, including typo-tolerant search in lists and long pickers, a separate partial-payment workflow, `completed/target` session counters for series and trainees, and a complete monthly financial/statistical report. Preserve the app's single-coach, phone-first character. The owner must be able to trust historical dues, stock, and financial totals, then complete common tasks in a few taps. This is an implementation handoff for Claude Code, not a request to replace the current design.

The current stack is Vue 3/Vite/TypeScript (`frontend/`), Go/Fiber (`backend/`), MongoDB, and Caddy/Docker Compose. Read `PRODUCT.md`, `DESIGN.md`, `README.md`, and `DEPLOY.md` before changing behavior. The current working tree has uncommitted user changes, including new search, pagination, audit, and schedule work. Preserve them. Inspect `git status` and diffs first; never reset or overwrite them. Work in small reviewable commits or a separate branch/worktree if available.

The existing baseline passes `go test ./...` in `backend/` and `npm run build` in `frontend/`. No live MongoDB end-to-end run was performed for the review, so verify production behavior against a test database before release.

## Product decisions to apply consistently

1. **Two dates for subscriptions:** `periodMonth` means the month the fee covers; `date` means when money was actually received. Cash reporting filters by `date`; dues filters by `periodMonth`. Show both labels wherever ambiguity is possible.
2. **Historical dues are fixed records:** changing today's fee or status must never silently rewrite an earlier month's charge. Keep former trainees in historical views.
3. **Financial records are corrected, not erased:** payments, expenses, sales, monthly charges, and stock adjustments retain history. Require a reason for voids and material corrections.
4. **Financials initially means cash in minus recorded cash out:** label the existing result **Net cash**. Do not call it profit until inventory purchase cost and cost of goods sold are modeled without double counting.
5. **One studio timezone:** use `STUDIO_TZ` for calendar boundaries and `YYYY-MM-DD` for date-only values. Store actual timestamps in UTC. Do not let browser locale change which studio day/month a record belongs to.
6. **Keep the UI compact:** put actionable exceptions on Home, detailed data in sections, and primary actions within thumb reach. Do not add a dense dashboard to Home.
7. **Preserve compatibility:** retain existing routes and API fields where possible. Add fields/endpoints incrementally, migrate existing data, and make demo mode follow the same behavior.
8. **Payment states are distinct:** `unpaid` means zero paid against a positive charge, `partial` means paid amount is greater than zero but less than due, and `paid` means paid amount equals due. Show partial accounts and exact remaining balance separately everywhere. Future charges may be `upcoming`; imported unknown charges remain `unverified`, never presented as paid.
9. **Two useful session counters:** a recurring series shows completed occurrences over created occurrences (for example `9/12`, with scheduled and cancelled counts alongside it). A trainee's assigned session plan shows sessions that trainee actually attended over the target allowance. A completed class with that trainee still marked `booked` or `no_show` must not increase their personal completed count.
10. **Search precedes pagination:** all scrollable data lists and long/dynamic pickers must search the full eligible result set, rank likely matches despite minor typos, then paginate or cap suggestions. Tiny fixed choice controls may remain visible buttons or native selects because they require no scrolling.
11. **Monthly metrics have named denominators:** distinguish transactions from unique people, created sessions from completed sessions, and attendance marked from attendance still pending. Every report card must define the date/period and records it counts.
12. **No implicit monthly proration:** a trainee joining mid-month owes the configured charge for that month unless the owner explicitly adjusts or waives it; making a trainee inactive mid-month does not erase an already issued charge. Future months follow the recorded status/fee terms.

## Proposed data and API contracts

These names make the implementation concrete. Claude Code may refine them while preserving the stated invariants and documenting any change before migration. Keep existing response fields and append new fields so the current frontend can be upgraded incrementally.

| Data | New/changed fields and constraints | Main purpose |
|---|---|---|
| `subscription_terms` | `trainee`, studio-local `effectiveDate`, `billingFromMonth`, `monthlyFee`, `status`, `createdAt`, `actor`; index by trainee/date, append-only | Record when status changes for historical active counts and which month a fee starts billing. Do not rewrite older terms. |
| `subscription_charges` | `trainee`, `periodMonth`, `due`, `paidAmount`, `state`, `source` (`normal`, `adjusted`, `imported_unverified`), `createdAt`, `updatedAt`; unique `(trainee,periodMonth)` | Fixed monthly amount and atomic balance guard. Store adjustments in the audit trail. |
| `payments` | Existing `date` remains cash receipt timestamp; require `periodMonth` and `trainee` for subscriptions; add `method`, `reference` | Separate cash timing, dues period, and payment evidence. |
| `stock_movements` | `item`, signed `delta`, `kind` (`opening`, `sale`, `return`, `restock`, `damage`, `correction`), `reason`, `ref`, `at`, `actor` | Explain every stock change and reconcile it to sales/returns. |
| `sale_returns` | `sale`, `qty`, `amount`, `date`, `reason`, `createdAt`, `actor`; total returned quantity cannot exceed sold quantity | Support partial returns while preserving the original sale. |
| `sales` | Snapshot `unitCost` at sale time; existing price/total remain snapshots | Future margin reporting without using today's item cost for older sales. |
| `reminders` | Optional `relatedType`, `relatedId`, `recurrence`, `snoozedUntil` | Link and repeat work without breaking existing reminders. |
| `trainee_session_plans` | `trainee`, `title`, `targetCount`, `startDate`, `endDate?`, `sessionType?`, `status`, `createdAt`, `updatedAt`; target must be positive | Track an individual trainee's planned allowance, such as twelve sessions. |
| `session_series` | `seriesId`, `plannedCount`, date range, title, created time, status; unique `seriesId` | Keep the denominator for `9/12 completed` when occurrences are cancelled. An explicit series extension/reduction adjusts `plannedCount` with an audit reason; migrate legacy series from occurrences still present and mark that denominator inferred. |
| Session attendee assignment | Optional `planId` on each attendee; validate that it belongs to that trainee; unique trainee per session | Credit an attended occurrence to exactly one trainee plan without changing legacy attendance records. |
| Search documents | Normalized searchable tokens/aliases, entity type and ID, display text, destination, update time; indexed candidate terms | Support typo-tolerant search across paged lists and global Search on self-hosted MongoDB. |

| API contract | Request/result and compatibility behavior |
|---|---|
| `POST /api/trainees/:id/terms` | `{effectiveDate,billingFromMonth,monthlyFee,status,reason?}`; append an effective-dated change, preserve older charges, and update the current trainee summary. Show any same-date existing event for review before another is added. |
| `GET /api/subscriptions?m=YYYY-MM` | Keep the existing array of trainee status rows; add `remaining`, `source`, and an explicit `unverified` state for unreconciled historical data. Never silently assume an old unpaid charge. |
| `POST /api/subscription-charges/:id/adjust` | `{due,reason}`; transactionally update the current charge and audit it. Reject `due < paidAmount` unless a documented credit/refund flow is used. |
| `GET /api/payments/:id` and existing `GET /api/payments/:id/receipt` | Return one payment and receipt data including method, reference, period, cash date, and void status. The frontend supplies print/share layout. |
| `POST /api/inventory/:id/adjustments`, `GET /api/inventory/:id/movements` | `{delta,kind,reason}` for authorized non-sale stock changes; return movement list newest first. No silent stock overwrite in normal UI. |
| `POST /api/sales/:id/returns` | `{qty,reason,date?}`; one transaction inserts the return, restores stock, updates sale return totals, and writes audit/movement entries. Financial summaries subtract live returns in the cash month of the return. |
| `POST /api/sessions/recurring/preview` | Same request as recurring create; return generated local dates and all conflicts without writing. Revalidate on actual create. |
| `PATCH /api/sessions/series/:seriesId` | Explicit `{scope:"future",fromOccurrence,changes,reason}`; preserve past occurrences and attendance. Add the static `series` route before `/:id`. |
| `GET /api/sessions/series/:seriesId/progress` | Return `created`, `completed`, `scheduled`, `cancelled`, and occurrence IDs. Counts are based on stored sessions, not the original recurring request's requested week count. |
| `POST /api/trainees/:id/session-plans`, `GET /api/trainees/:id/session-plans` | Create/list individual plans and return `target`, `completed`, `remaining`, `upcomingBooked`, `noShows`, and credited session IDs. |
| `PATCH /api/sessions/:id/attendance` | Keep existing status behavior; accept optional `planId` when assigning or updating an attendee. Recompute plan progress from authoritative session/attendance records or update a transactional projection; do not count twice on repeated requests. |
| `GET /api/reports/monthly?m=YYYY-MM` | Return `{month,from,to,generatedAt,financials,subscriptions,sessions,attendance,sessionPlans,inventory,comparison}` with definitions and drill-down query links. Use studio-time month bounds and a consistent database read snapshot. |
| List endpoints with `q` | Apply typo-tolerant search to the full filtered collection before pagination; return result rows plus `total`/`hasMore`. Keep the old array response only until its frontend caller is migrated. |

For voided money and sale records, keep the current response fields and add reasons/actor metadata. For endpoint errors, use stable machine-readable codes alongside readable messages (for example `INSUFFICIENT_STOCK`, `OVERPAYMENT`, `SCHEDULE_CONFLICT`) so forms can show field-specific guidance.

## Delivery rules for Claude Code

- Complete phases in order. Each phase must leave `go test ./...` and `npm run build` green. Add meaningful tests for rules that protect money, stock, dates, and recurrence.
- Before each phase, write down the existing contract and migration impact. After it, update `README.md`/`DEPLOY.md` and give a short manual verification script. Avoid UI-only changes for rules that the API must enforce.
- Use clear loading, empty, error, and retry states. Disable duplicate submits. After a mutation, invalidate only relevant cached data or refresh the affected view; do not depend on a full page reload.
- Never seed or wipe a real database. Test migrations on a copied database and provide a backup/rollback procedure.
- If a product decision here proves incompatible with existing real data, stop that change and report the concrete data conflict and proposed adjustment. Continue independent tasks.

## Phase 0 — baseline and test environment

**Files:** `backend/cmd/server/main.go`, `backend/internal/db/mongo.go`, `docker-compose.yml`, `backend/internal/server/*`, `frontend/src/lib/demo.ts`, docs.

1. Inventory all current routes, list response shapes, and date semantics. Record sample JSON fixtures for a trainee, subscription, partial payment, expense, session, series, trainee session plan, item, sale, and monthly report.
2. Add a disposable MongoDB replica-set environment for local development and CI. MongoDB transactions used below require a replica set; the current standalone local Mongo setup is insufficient. Update Docker Compose, health checks, `.env.example`, and local setup instructions. Verify auth and connection strings in both local and droplet paths.
3. Establish backend integration tests against that disposable database. Keep pure unit tests for date, money, and overlap helpers. Add a small frontend test harness (Vitest/Vue Test Utils) for derived UI rules; add a few Playwright flows only for the critical owner journeys listed in Phase 8.
4. Add migration tooling with dry-run, progress counts, idempotency, and backup-first instructions. Do not hide schema changes in a request handler.

**Exit:** A clean test database can start, run migrations twice safely, and run backend integration tests. Existing data and routes still work.

## Phase 1 — accounting and inventory integrity

### 1A. Atomic writes and audit

**Files:** `backend/internal/server/{payments,expenses,inventory,audit}.go`, `backend/internal/models/models.go`, `backend/internal/db/mongo.go`.

- Wrap each multi-document financial action in a MongoDB transaction: stock decrement plus sale insert; sale correction plus stock delta; sale void plus restock; payment/expense/sale edit or void plus audit entry. A failed action must leave all records unchanged. Use conditional filters to prevent double voids and negative stock.
- Record audit lines for creation as well as edit/void. Add actor, reason, and request time. Require a reason for voids and quantity/amount corrections; display it in `AuditTrail.vue`. Existing audit records remain readable.
- Reject invalid enum values, negative monthly fees, zero/negative session durations, nonpositive sale quantity, negative sale unit price, invalid/missing linked trainee IDs, duplicate attendees, and malformed dates. Validate on the server regardless of form validation.
- Make subscription overpayment checks safe under concurrent requests. A read-then-insert check alone can race. Use a per-month charge/balance document updated conditionally within the payment transaction, and reverse/reapply it for payment corrections and voids.
- Add integration tests for failure after the first write, concurrent sells, concurrent subscription payments, edit/void retries, deleted item/trainee references, and audit completeness.

**Exit:** Stock, sales, dues, and audit cannot diverge after a failed request or concurrent operation.

### 1B. Historical subscription charges

**Files:** `backend/internal/models/models.go`, `backend/internal/server/{trainees,payments,dashboard}.go`, a migration command, `frontend/src/{lib/types.ts,views/TraineeFormView.vue,views/PaymentsView.vue,views/TraineeDetailView.vue}`.

- Add effective-dated membership/fee terms (trainee ID, studio-local effective date, first billed month, fee, active state, recorded time) and a monthly charge collection with a unique `(trainee, periodMonth)` index. A charge stores the due amount and provenance at creation. A fee/status change appends a new effective term; it cannot mutate earlier charges. Use terms to answer “active at month end” historically.
- On trainee edit, ask whether a fee/status change applies this month or next month. If changing a current charge, require an explicit adjustment reason and audit that adjustment. Keep `Trainee.monthlyFee/status` as the current summary only, not historical truth.
- Materialize charges idempotently for the current period and requested periods based on the effective terms. Do not create charges for months before a trainee's active term. `/subscriptions?m=` reads stored charges plus live subscription payments, including trainees now inactive. Return `due`, `amountPaid`, `remaining`, `state`, and provenance while preserving existing fields.
- Migration: create current terms/charges from known current values. Earlier fee and active history cannot be reconstructed from the present model. For old months with payments, mark imported charge amounts **unverified** and provide an admin reconciliation action. Do not invent unpaid charges for other old months. Show an explicit “historical dues need review” state instead of presenting inferred values as fact.
- Require a valid `periodMonth` and trainee for a subscription payment. Default the form's period to the selected Money month or the current studio month. Keep the cash receipt date separate. Decide and test what happens when a charge is waived or adjusted after partial payment.
- Calculate subscription state only from the charge and live payments for that exact trainee/period. Return `remaining = max(0, due - amountPaid)`; never collapse `partial` into `paid` or count a partial account as fully paid. A fully paid count excludes partial accounts. Explicitly keep imported `unverified` rows out of paid/unpaid metrics until reconciled.
- Add tests: fee increase after a paid month; inactive trainee in an old month; a $50 charge with a $20 payment becomes `partial` with $30 remaining, then becomes `paid` after $30 more; overpayment rejection; late payment for an older period; void/edit returns the state to `partial` or `unpaid`; migration idempotency.

**Exit:** A past month's due and paid state remain stable after later trainee changes, and every subscription payment belongs to exactly one charge period.

## Phase 2 — dates, money display, and navigation state

**Files:** `backend/internal/server/helpers.go`, `backend/internal/models/helpers.go`, `frontend/src/lib/format.ts`, `frontend/src/views/{Schedule,SessionForm,ReminderForm,PaymentForm,Payments,Financials}View.vue`, `frontend/src/components/ui/{DateWheel,MonthPicker}.vue`, `frontend/src/router/index.ts`.

1. Parse `YYYY-MM-DD` in the configured studio location, with `[from,to)` query semantics. Use an explicit date-only helper for reminders. Do not create a reminder due date via `new Date(dateString).toISOString()`. Define how a browser outside Beirut renders studio dates.
2. Display money with up to two decimal places when needed; preserve whole-number appearance for whole amounts if desired. Align server rounding and frontend input step/validation. Add tests for `12`, `12.5`, and `12.55`.
3. Add recording-date fields to payment and expense forms. Default to today in the studio timezone. When viewing an old month, clearly show whether the action will be recorded in that old month or today; do not silently place it elsewhere.
4. Encode current period/context in URLs: schedule `?week=YYYY-MM-DD`, Money/Financials `?m=YYYY-MM`, relevant record ID as a focus/highlight query when useful. Preserve context through create/edit/detail/back navigation, search results, and browser back/forward.
5. Make `DateWheel.vue` support older records and more than its current five-year range. Date/time pickers need keyboard and screen-reader alternatives; a native date/time input is acceptable where the wheel is difficult to use.
6. Replace blanket cache clearing with keyed invalidation for affected lists/details. Guard against old fetch responses replacing newer state. Keep a visible retry when schedule refresh fails instead of showing an empty week as if no sessions exist.

**Exit:** Midnight/month boundaries test correctly in Beirut and another browser timezone; after saving, the owner returns to the period and record they were working on.

## Phase 3 — Money and Financials workflows

### Money (`PaymentsView.vue`, `PaymentFormView.vue`, `TraineeDetailView.vue`)

- Separate the dues roster from the transaction ledger with compact tabs/sections. Give **Partial payments** its own visible tab/list, alongside Unpaid, Paid, and All; show counts and outstanding totals for each. Sort partial and unpaid accounts above paid ones by default. The current `paidCount` summary incorrectly describes every non-paid account as “unpaid”; replace it with independent fully paid, partial, and unpaid counts.
- Every dues row and the trainee profile must show `paid / due`, the exact remaining amount, the period, the state badge, and the last payment date. The Partial tab is visible even if only one person is in it. Link each row to the trainee's payment history and a one-tap **Collect remaining** action.
- Search by trainee, note, type, period, and reference. Search the entire selected month before pagination, including partial accounts that are not on the current visible page.
- A Collect action pre-fills trainee, period, and exact remaining amount. Show existing payments and the total due before saving. After save, return to the same month with the updated trainee visible.
- Add payment method (`cash`, `card`, `bank transfer`, `other`) and optional external reference. Store both on the payment and in CSV/receipt. Preserve old rows as “unspecified”.
- Build a printable/shareable receipt view from `GET /payments/:id/receipt`; include studio identity, payment ID, cash date, trainee, type, period, amount, method, and void status. Ensure a voided receipt is visibly invalid. Provide receipt access from the transaction row and trainee history.
- Add payment detail or a reliable transaction deep link for Search. Require a void reason and show edit/void history. Do not let a user reclassify a shop sale payment without reconciling its linked sale.

### Financials (`FinancialsView.vue`, `backend/internal/server/expenses.go`)

- Change “Profit/Loss” to “Net cash”. Show income by type and expenses by category using the API's existing `byType` and `byCategory`. Tap a category to filter the ledger.
- Add previous-month comparison, custom date range, and explicit expense date. Keep CSV export matching the visible range/filter and include status, method/reference where applicable.
- Provide a combined transaction ledger with payment, sale, and expense rows; distinguish cash date from subscription period. Keep voided rows visible but excluded from totals. Offer a reconciliation view for days where entered cash does not match a manually supplied closing amount.
- Only introduce gross margin/profit after stock purchase costs and cost of goods sold have a complete, nonduplicating model. Document how a purchase expense relates to inventory value before adding that calculation.

**Exit:** A coach can immediately see everyone partially paid, collect the remaining balance, retrieve an old due's receipt, and see which cash month changed; all visible totals reconcile with the exported statement.

## Phase 4 — Schedule and attendance

**Files:** `frontend/src/views/{Schedule,SessionForm,SessionDetail}View.vue`, `backend/internal/server/sessions.go`, session models.

1. Add date jump and status filters to the week view. Show `booked/capacity` when capacity exists; highlight full sessions and sessions still needing attendance. Prefill a new session from the currently viewed day/time.
2. Enforce capacity in create, update, recurring create, and attendance add paths. Prevent duplicate attendee IDs and invalid/deleted trainee IDs. Define `capacity=0` as unlimited, or migrate to an explicit nullable capacity; use the same meaning throughout UI/API.
3. Keep “session completed” separate from attendee statuses. On completion, offer “mark booked trainees attended” with a review step; do not automatically mark a no-show attended. Add “mark all attended” and easy exceptions on the detail page. A completed class with `booked` attendees displays an **Attendance needed** warning.
4. Add a series progress counter to Schedule and session detail: **completed / planned**, plus scheduled, cancelled, and attendance-needed counts. Store `plannedCount` when creating the series; count a session as complete only when its status is `completed`. Do not auto-complete past occurrences merely because they are created with a past date. For a one-off session show `0/1` or `1/1` where useful.
5. Add a trainee session-plan flow on the trainee profile: create an allowance such as 12 sessions, optional date window/type, then assign sessions to it while booking trainees. Show **9/12 attended**, **3 remaining**, future bookings, no-shows, and unassigned plan slots. A credited completion requires both the session status `completed` and that trainee's attendance `attended`. Cancelled sessions and no-shows are separate and do not reduce remaining by default. Make any future no-show credit policy an explicit plan setting, not an implicit calculation.
6. A trainee can have multiple plans over time; one attendance record credits at most one plan. Repeated attendance updates, moving a booking between sessions/plans, cancellation, and reopening a session must recalculate progress without double counting. Warn before assigning more future bookings than a plan's remaining target, and require explicit override if allowed.
7. Add duplicate/reschedule actions. For recurring sessions, show the series ID and all future occurrences. Provide explicit “this occurrence” versus “this and future” edit/cancel choices. Future-series edits must be atomic and preserve already-recorded attendance on past occurrences.
8. Provide a pre-submit preview of recurring dates, count, plan assignments, and conflicts. Keep whole-series rejection when a conflict exists, but identify conflicting dates so the owner can adjust them. Limit excessively long series and validate time/weekday values on the server.
9. Test week rollover, DST transitions, group/private overlap rules, capacity races, cancelled sessions, recurring edits, and attendance preservation. Add plan/series counter tests for 9 attended of 12, completed class with an unmarked attendee, no-show, cancellation, duplicate update, and reassignment.

**Exit:** The coach sees both `9/12` series completion and each trainee's `9/12` attended plan progress, can identify the remaining sessions, and can change a recurring schedule without silently altering past sessions.

## Phase 5 — Crew, Inventory, and Reminders

### Crew (`TraineesView.vue`, `TraineeFormView.vue`, `TraineeDetailView.vue`)

- Add status, skill, and dues filters plus sort by name, recently added, or amount owed. Keep one-tap call and add a WhatsApp link only when a valid phone number exists; normalize phone input and display it clearly.
- Replace routine hard delete with archive/inactivate. If hard delete remains, show linked sessions/payments/sales and require a stronger confirmation. Preserve historical name snapshots and links.
- On the profile, show upcoming sessions, attendance/no-show history, purchase history, month-selectable dues, and receipts. Put active session-plan counters and a **Assign sessions** action near the top. Add “Book session” and “Collect” shortcuts. Fetch history by trainee with server paging; remove the broad 2000–2100 payment download.

### Inventory and sales (`InventoryView.vue`, `SaleDetailView.vue`, `backend/internal/server/inventory.go`)

- Add low-stock/out-of-stock filters and put actionable shortages first. Expose SKU, cost price, and active/archive controls already present in the model. Do not sell inactive items.
- Replace direct stock overwrite as the normal operation with restock, damage/write-off, and correction actions. Store quantity delta, reason, timestamp, and actor in an inventory movement ledger. Show stock history on item detail. Preserve a guarded admin correction for migration mistakes.
- Add a sale confirmation showing quantity, unit price, total, buyer, and remaining stock. Validate quantity before submit. Add optional unit-price override/discount with a reason; snapshot price and cost on the sale. Show sale receipt and support partial return with corresponding stock/income reversal. Keep a full void for erroneous sales.
- Link inventory purchases to expenses only after deciding whether purchase cost is cash outflow, inventory asset, or both in reports. Avoid double counting.

### Reminders (`RemindersView.vue`, `ReminderFormView.vue`)

- Group open reminders as overdue, today, and upcoming; hide completed by default behind a filter. Add priority filter and one-tap snooze to tomorrow/next week.
- Add optional recurring cadence and optional link to a trainee, session, or inventory item. Tapping the linked record should open it. Keep completion history clear for recurring instances.
- Add an in-app overdue count first. Browser notifications may follow as an opt-in feature only after permissions, scheduling, and offline behavior are defined.

**Exit:** Common owner tasks—contact a trainee, fill a class, restock an item, and clear a reminder—are reachable in a few taps without losing historical context.

## Phase 6 — Search, shared usability, and scale

**Files:** `backend/internal/server/search.go`, `frontend/src/views/SearchView.vue`, `frontend/src/components/ui/*`, `frontend/src/lib/{api,cache,paginate}.ts`.

- Put the shared `SearchInput` above **every scrollable data list**, including Crew, trainee payment/session history, Schedule, session attendance, Money dues and transactions, Financials expenses/ledger, Inventory stock and sales, Reminders, session-series occurrences, trainee session plans, and report drill-down lists. When a page has two lists, either one clearly labeled search filters both (as on Money/Inventory) or give each list its own labeled field. Searching happens before paging and retains the current status/month/week filter. Show match count, clear button, and a helpful no-results state.
- Replace every **long or dynamic dropdown/picker** with a searchable `SearchSelect`: trainee/buyer pickers in session creation/edit, payment creation/edit, inventory sale and sale correction; any future item/session/plan selector; and month/date navigation via a direct jump field. Tiny fixed choice controls (for example Group/Private or three priorities) should remain fully visible, not become scrolling dropdowns. Do not make a user scroll through a long roster or 60 minute values to select one known entry.
- Implement one shared fuzzy matcher for local options and server search ranking: normalize case, accents, punctuation, extra spaces, and phone digits; rank exact > prefix > word prefix > substring > typo match; tolerate a one-character typo for short names and up to two for longer terms; allow reordered words and common aliases such as `PT`/`private` and `no show`/`no-show`. Avoid unrelated suggestions by applying a score threshold. Highlight the matching text, show the top 10–20 suggestions, and provide “show more” when needed.
- Expand global Search to trainee name/phone, session title/attendees, payment name/note/reference, item name/SKU, sales, expenses, and reminders. Return typed groups with `hasMore`/count and a “Did you mean…” suggestion when the exact search is empty. Make every result open the precise record or a filtered/highlighted page; avoid the current generic Inventory link and unclickable payment result.
- Do **not** fuzzy-filter only the current client page. For paged collections, query server-side across the full filtered dataset, get indexed candidates using normalized terms/aliases, then rank the bounded candidates in Go before pagination. Maintain/backfill search documents on create/edit/archive/void and verify no stale result survives a mutation. Keep local fuzzy matching only for an already fully loaded small option set. Add indexes for frequent date, trainee, period, status, and sale/item queries. Benchmark with realistic data and avoid unbounded full-collection loads in the SPA.
- Debounce remote typing (roughly 200 ms), cancel or ignore out-of-order requests, preserve the query in the URL where useful, and distinguish “searching” from “no matches.” Do not require an Enter press for suggestions. Keep search available offline only for data already cached; never imply that cached results are complete.
- Test names with transposed/missing letters, partial phone numbers, SKU fragments, reordered words, aliases, one and two typos, diacritics, no-match thresholds, 1,000+ rows, and a match that exists only beyond page one. Verify the same expected result in a list, a dropdown, and global Search.
- Standardize form labels, required markers, inline field errors, save states, success messages, and retry states. Protect edit forms from saving when initial data failed to load.
- Meet `PRODUCT.md`'s 40px mobile tap target goal for checkboxes, edit, void, and pagination controls. Make `SearchSelect.vue` a keyboard-operable combobox with appropriate labels/ARIA, arrow-key selection, Escape, and focus return. Test reduced motion and small screens.
- Persist only non-sensitive view preferences (last month/week/filter) if helpful. Keep financial data in the existing memory cache unless an explicit offline data/security design is approved.

**Exit:** A typo still finds the intended trainee/item/session, a match after page one is found, and every long picker can be searched without scrolling through the roster. Lists remain responsive as data grows; forms and custom controls work with keyboard and touch.

## Phase 7 — complete monthly financial and studio report

**Files:** new `backend/internal/server/reports.go`, shared financial/dues/attendance aggregation helpers, new `frontend/src/views/MonthlyReportView.vue`, router/nav, types, and CSV/print view.

1. Add `/reports?m=YYYY-MM` under Financials and link it from Home/Financials without turning Home into a chart dashboard. Default to the current studio month; allow any past month and direct month/year jump. Show when the report was generated and refresh after relevant edits.
2. **Financial section:** gross cash received from trainee payments; shop sales less returns; total income; expenses by category; net cash; payment method split; prior-month absolute and percentage change. Exclude voids and avoid legacy sale/payment double counting. Show receipt/transaction drill-downs and reconcile exactly to the statement CSV.
3. **Subscription section:** distinct trainees with a verified charge for the month; total billed; amount paid **toward that period** even if cash was received in a later month; outstanding amount; fully paid, partially paid, unpaid, and unverified counts. Show a separate list of partial payers with paid/due/remaining and a Collect remaining action. Do not treat a partial payer as fully subscribed/paid. State clearly that this section is about the fee period while Financials is about cash date.
4. **Trainee section:** active at month end, newly joined, archived/inactive during the month, and unique trainees who attended at least once. Use effective-dated membership records for historical active counts; do not infer old status from today's profile.
5. **Session section:** occurrences starting in the month, created series, scheduled, completed, cancelled, group/private split, completion percentage, and a link to each series' `completed/planned` progress. Show counts for active trainee plans, plan credits earned in the month, and outstanding plan sessions at month end. Do not claim all past scheduled sessions were completed automatically.
6. **Attendance section:** total attendance entries marked attended, no-show, and still booked/unmarked; distinct people attended and distinct people with a no-show; attendance rate `attended / (attended + no_show)` only among decided attendance, with the number still unmarked shown alongside. Exclude cancelled sessions. If capacity is set, show occupancy with the exact denominator; never mix unlimited-capacity classes into that percentage.
7. **Inventory section:** units sold, sales revenue, units returned, top items by quantity/revenue, and low/out-of-stock counts at report time. If a historical end-of-month stock count is needed, derive it from the stock movement ledger, not today's `stock` field; label any metric that is only current-state.
8. Use a single backend aggregation path for the screen and CSV/print export, with a consistent read snapshot. Add compact cards and expandable breakdowns with definitions/tooltips and links to filtered source rows. Make a print-friendly monthly report; PDF generation can wait if browser print meets the need.
9. Test a month crossing timezone/DST boundaries; late payment for an older period; one partial/one paid/one unpaid subscriber; payment and sale voids; return in a later month; recurring sessions spanning months; cancelled session; attended/no-show/unmarked entries; a trainee who attended multiple classes counting once in distinct-person metrics. Assert report totals equal ledger/dues source totals.

**Exit:** The coach can open any month and see a complete, reconcilable breakdown of cash income and expenses, subscription states, sessions, attendance, trainee activity, and shop activity, with partial payers and remaining balances visible.

## Phase 8 — Home, demo mode, security, and operations

- Use the existing `/dashboard` data to add a restrained Home “Needs attention” strip: next session, overdue reminder count, **partial** and unpaid dues counts separately, trainee plans nearing their target/end date, and low stock count. Each item opens the relevant filtered section. Add a Monthly report link. Keep the existing six launch tiles and avoid a chart grid.
- Make `frontend/src/lib/demo.ts` implement recurring creation, session-plan progress, partial-payment states, fuzzy search, monthly report data, and date/month filters with the same semantics as the live API. Add demo contract tests and an obvious demo indicator so preview actions cannot be mistaken for saved real data.
- Remove the raw token from `POST /auth/login` JSON; return success and user metadata, and update `frontend/src/lib/auth.ts` to rely on HTTP success plus the HttpOnly cookie. Check cookie `Secure` behavior at Caddy, logout expiration, and same-origin request protection.
- Add scheduled, encrypted Mongo backups with retention and an off-host destination configurable at deployment. Document and exercise a restore into a disposable database. Include restore verification in release checks.
- Update `README.md`, `PRODUCT.md`, `DEPLOY.md`, `.env.example`, and any stale schedule notes to match the implemented behavior. Document data migration, replica-set setup, auth, backup, and rollback.

**Exit:** Home shows actionable exceptions, preview behavior is honest, no login token is exposed to frontend JavaScript, and a backup can be restored successfully.

## Phase 9 — release verification and acceptance flows

Automate the highest-risk paths where practical and run them manually on a staging copy of real data. Record outcomes before deployment.

1. Create a trainee with a $50 monthly fee; register $20. Verify the Partial list, Home partial count, trainee profile, and monthly report all show **$20 paid, $30 remaining**, while Paid does not include that trainee. Collect the remaining $30; verify the trainee moves to Paid and disappears from Partial. Change the fee next month; verify the older month stays paid at its original amount.
2. Inactivate a trainee; verify old dues, payments, attendance, and sales remain visible. Void a payment with a reason; verify dues and cash totals reverse and audit history records it.
3. On an older Money month, collect its due today; verify the due belongs to the old period while cash appears in today's month. Print/share a receipt.
4. Add an expense dated in a selected past month; verify that month and its CSV update. Compare financial totals with the statement rows, excluding voids.
5. Sell the last unit of an item; attempt a concurrent second sale and a failed insert; verify stock never goes negative and no orphan sale/stock movement exists. Correct quantity, partially return, and fully void according to the implemented rules.
6. Create a 12-occurrence recurring series and assign a 12-session plan to a trainee. Complete nine sessions and mark that trainee attended in those nine; verify both series and trainee counters show `9/12`, with three remaining. Mark another class completed but leave the attendee booked; verify their personal counter stays `9/12` and attendance is flagged. Record a no-show and a cancellation; verify both appear separately and do not count as attended. Also verify month/DST boundaries, capacity, conflict preview, series edits, and group/private overlap.
7. Search each list and long picker for a known name beyond page one, a misspelled name, a phone fragment, a misspelled item, and an alias. Verify relevant suggestions rank first, unrelated matches stay hidden, keyboard selection works, and every result opens the exact record.
8. Open a monthly report containing a partial payer, late payment, void, sale return, multiple attendances by one person, and no-show. Verify cash/expense totals against the statement, dues totals against charges, and distinct-person counts against attendance records. Export and print it.
9. Use every main page on a narrow phone viewport and keyboard-only desktop session. Verify tap targets, picker behavior, errors, offline/retry messaging, browser back, and preserved month/week context.
10. Run `go test ./...`, `go vet ./...`, `npm run build`, frontend unit tests, integration tests, and core end-to-end flows. Test migration twice on a staging copy, then test backup restore and rollback.

## Suggested implementation sequence and handoff checkpoints

| Checkpoint | Deliverable | Depends on |
|---|---|---|
| A | Test replica set, migration framework, atomic stock/sale and audit writes | Phase 0 |
| B | Fixed historical dues, validated payments, and explicit Partial/Unpaid/Paid lists | A |
| C | Timezone/date fixes, cents display, context-preserving navigation | B |
| D | Money receipts and Financials clarity | C |
| E | Schedule series counter, trainee 9/12 plans, capacity, and attendance controls | C |
| F | Crew, Inventory, Reminders workflows | D/E as relevant |
| G | Fuzzy search in all data lists and long pickers; accessibility and paging | Earlier list contracts stable |
| H | Monthly financial and statistical report with reconciled exports | B, E, F |
| I | Home, demo, security, backups, and documentation | G, H |
| J | Staging verification and deployment | All required checkpoints |

At each checkpoint, provide: changed files, migration notes, API changes, screenshots of affected phone/desktop screens, test results, known limitations, and a short list of remaining tasks. Do not declare the project finished on build success alone; the acceptance flows above must pass against a database.
