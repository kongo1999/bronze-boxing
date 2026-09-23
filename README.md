# Bronze Boxing

Studio-management app for a single boxing coach. **Go + Fiber API** + **Vue SPA**, MongoDB.
(Migrated from Next.js — see `Bronze Boxing/` in the Obsidian vault for the plan; old code is in git tag `nextjs-archive-2026-05-29`.)

## Stack
- **backend/** — Go 1.25, Fiber v2, MongoDB driver (mongo-driver)
- **frontend/** — Vue 3 + Vite + TypeScript + Tailwind v4 (Ringside design)
- **MongoDB 7 as a single-node replica set.** Every money and stock change is
  a multi-document transaction (a sale updates stock, the sale, the stock
  ledger and the audit trail together), and MongoDB only runs transactions on
  a replica set. The API refuses to start against a standalone `mongod`.

## Run locally

### 1. A MongoDB replica set
Pick one:

**Docker (disposable, easiest).** Data lives in memory and is gone after `down`:
```bash
docker compose -f docker-compose.test.yml up -d --wait
# MONGODB_URI=mongodb://localhost:27027/?replicaSet=rs0
```

**Native `mongod` (keeps data).** Turn the existing install into a one-node
replica set — add to `mongod.cfg`:
```yaml
replication:
  replSetName: rs0
```
restart the MongoDB service, then once: `mongosh --eval "rs.initiate()"`.
Use `MONGODB_URI=mongodb://127.0.0.1:27017/?replicaSet=rs0`.

### 2. Backend
```bash
cd backend
cp .env.example .env        # set MONGODB_URI / DB_NAME
go run ./cmd/migrate -dry-run   # what the migrations would change
go run ./cmd/migrate            # apply them (the API won't start until they are)
go run ./cmd/seed           # OPTIONAL demo data — wipes that database first
go run ./cmd/server         # http://localhost:8080
```
Health check: http://localhost:8080/api/health → `{"db":true,"status":"ok"}`

### 3. Frontend
```bash
cd frontend
npm install
npm run dev                 # http://localhost:5173 (proxies /api -> :8080)
```

## Tests
```bash
# backend unit tests (no database needed; integration tests skip)
cd backend && go test ./...

# backend integration tests: real handlers against the disposable replica set.
# Each test makes its own bbtest_* database and drops only that one.
docker compose -f docker-compose.test.yml up -d --wait
TEST_MONGODB_URI="mongodb://localhost:27027/?replicaSet=rs0" go test ./...

# frontend unit tests + typecheck/build
cd frontend && npm test && npm run build
```
Integration tests cover rollback after a mid-transaction failure, concurrent
sells and dues payments, void/edit retries, partial-payment states, fee
changes that must not rewrite past months, and migration idempotency.

## Migrations
Schema and data changes run explicitly with `cmd/migrate` — never inside a
request handler. `-dry-run` writes nothing; `-list` shows applied/pending;
`-verify` cross-checks every dues balance against its payments and every stock
count against its ledger. Each migration is idempotent (`-only ID -rerun`
proves it on a copy). **Back up before migrating a real database** (DEPLOY.md).

## API
See [`docs/CONTRACT.md`](docs/CONTRACT.md) for routes, date and money
semantics, error codes, and recorded response samples in `docs/fixtures/`.
