# Bronze Boxing

Studio-management app for a single boxing coach. **Go + Fiber API** + **Vue SPA**, MongoDB.
(Migrated from Next.js — see `Bronze Boxing/` in the Obsidian vault for the plan; old code is in git tag `nextjs-archive-2026-05-29`.)

## Stack
- **backend/** — Go 1.25, Fiber v2, MongoDB driver (mongo-driver), native Mongo on `27017`
- **frontend/** — Vue 3 + Vite + TypeScript + Tailwind v4 (Ringside design) *(in progress)*

## Run

**MongoDB** must be running natively on `127.0.0.1:27017` (no Docker).

### Backend (Go — light, single process)
```bash
cd backend
cp .env.example .env        # optional; defaults work for local
go run ./cmd/seed           # one-off: load demo data
go run ./cmd/server         # http://localhost:8080
```
Health check: http://localhost:8080/api/health → `{"db":true,"status":"ok"}`

### Frontend (Vue — you run this)
```bash
cd frontend
npm install
npm run dev                 # http://localhost:5173 (proxies /api -> :8080)
```

## API (prefix `/api`)
`health` · `trainees` · `sessions` (+ `/recurring`, `/:id/attendance`) · `payments` (+ `/subscriptions`, `/:id/receipt`, `/export`) · `reminders` · `expenses` (+ `/financials`) · `inventory` (+ `/:id/sell`, `/sales`) · `dashboard` · `search`

See `Bronze Boxing/Data Model & API Contract.md` in the vault for full details.
