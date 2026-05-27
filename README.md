# 🥊 Bronze Boxing

A mobile-first studio management app for a boxing coach to run the business side of
the gym from his phone: the trainee roster, the class schedule, the cash ledger, who
has paid this month, and the day's reminders.

Built with **Next.js 16 (App Router) · React 19 · TypeScript · Tailwind v4 · MongoDB (Mongoose)**.

---

## Features

- **Dashboard** — today's classes, cash collected this month, who still owes dues, and reminders for the day and week, all at a glance.
- **Trainees** — roster with contact info, skill level, monthly fee, notes, and a unified profile showing payment history and subscription status.
- **Schedule** — weekly agenda of group classes and private sessions, one-tap **recurring** class creation, live attendance marking (here / no-show), and cancel/complete.
- **Payments & subscriptions** — cash ledger by month, and a per-month view of who has **paid / partially paid / not paid** their subscription, with one-tap "Collect".
- **Reminders** — day and week to-dos with priority, checked off in place.

Cash-only for now (no payment gateway). v1 is single-admin (just the coach); the data
model leaves room to give trainees their own logins in v2.

## Design

Dark, warm "Ringside" theme with a bronze accent, condensed display type, and clear
status colors. Tokens and rules live in [`DESIGN.md`](./DESIGN.md); product context in
[`PRODUCT.md`](./PRODUCT.md).

## Architecture

A clean separation that keeps the backend testable and the UI thin:

```
src/
  lib/
    db.ts            Globally-cached Mongoose connection
    models/          Mongoose schemas (Trainee, Session, Payment, Reminder)
    dto.ts           JSON-safe shapes for the RSC boundary
    validation.ts    Zod input schemas
    services/        Business logic — the single source of truth
  app/
    <feature>/       Server-Component pages + server actions
    api/<feature>/   REST route handlers (same services, exposed over HTTP)
  components/         Design-system primitives, app shell, feature components
```

- **Services** are called directly by Server Components (no HTTP hop) for reads, and by
  both **server actions** (forms) and **REST route handlers** for writes, so there's
  one place where business rules live.
- Mongoose runs on the Node runtime (it can't run on Edge).

## Prerequisites

- **Node.js 22+**
- **MongoDB running locally on `mongodb://127.0.0.1:27017`** (a native install / Windows
  service). Any MongoDB URI works (e.g. Atlas) by editing `.env.local`.

## Getting started

```bash
# 1. Install dependencies
npm install

# 2. Configure environment (defaults to local Mongo on 27017)
cp .env.example .env.local

# 3. (Optional) Seed realistic sample data
npm run seed

# 4. Run the dev server
npm run dev
```

Open http://localhost:3000.

## Scripts

| Script | What it does |
|---|---|
| `npm run dev` | Start the dev server (Turbopack) |
| `npm run build` | Production build (typechecks + compiles every route) |
| `npm run start` | Serve the production build |
| `npm run lint` | ESLint |
| `npm run seed` | Reset and load sample trainees, sessions, payments, reminders |

## REST API

All endpoints run on the Node runtime and return JSON.

| Method | Endpoint | Description |
|---|---|---|
| `GET` / `POST` | `/api/trainees` | List (`?q=`, `?status=`) / create |
| `GET` / `PATCH` / `DELETE` | `/api/trainees/:id` | Read / update / delete (cascades) |
| `GET` / `POST` | `/api/sessions` | List (`?from=`, `?to=`, `?type=`) / create |
| `GET` / `PATCH` / `DELETE` | `/api/sessions/:id` | Read / update / delete (`?series=true`) |
| `GET` / `POST` | `/api/payments` | List (`?traineeId=`, `?type=`, `?from=`, `?to=`) / create |
| `DELETE` | `/api/payments/:id` | Delete |
| `GET` / `POST` | `/api/reminders` | List (`?includeDone=true`) / create |
| `PATCH` / `DELETE` | `/api/reminders/:id` | Update or toggle `{done}` / delete |

## Notes

- This project does **not** use Docker; it connects to a native local MongoDB.
- Times are stored in UTC and displayed in the browser's local timezone.
