# Deploying Bronze Boxing to a droplet (Docker Compose)

The whole app runs as three containers on one droplet:

| Service | What | Exposed? |
|---------|------|----------|
| `web`   | Caddy — serves the Vue SPA, reverse-proxies `/api`, auto-HTTPS | **yes** (80/443) |
| `api`   | Go (Fiber) API | no (private network) |
| `mongo` | MongoDB 7 as a single-node replica set, data in a named volume | no (private network) |
| `migrate` | One-shot data migrations (`docker compose run --rm migrate`) | no — never started by `up` |

Only `web` is reachable from the internet. The API and database are only reachable
on the private compose network.

> **Build note:** these images are **not** built on the dev laptop (Docker isn't run
> there). They build on the droplet, where Docker runs natively.

---

## 1. Create the droplet
- Ubuntu 22.04 or 24.04, **at least 2 GB RAM** (Mongo + builds need headroom; 1 GB will OOM during the Go/Node build — use a 2 GB droplet, or build elsewhere).
- Add your SSH key. Note the public IP.

## 2. (Optional, for HTTPS) Point a domain at it
- Create a DNS **A record** for e.g. `studio.example.com` → droplet IP.
- HTTPS is automatic once `SITE_ADDRESS` is set to that domain (step 5). Skip this to start on plain HTTP via the IP.

## 3. Install Docker + Compose plugin
```bash
ssh root@DROPLET_IP
curl -fsSL https://get.docker.com | sh        # installs Docker Engine + compose plugin
docker compose version                         # verify
```

## 4. Get the code
```bash
git clone https://github.com/kongo1999/bronze-boxing.git
cd bronze-boxing
```

## 5. Configure secrets
```bash
cp .env.example .env
nano .env
```
Set at minimum:
- `MONGO_PASSWORD` — a long random string (`openssl rand -hex 24`). **Set before first boot** — it's baked into the Mongo volume on creation.
- `SITE_ADDRESS` — your domain (HTTPS) or leave `:80` (HTTP on the IP).
- `ADMIN_USERNAME` / `ADMIN_PASSWORD` — the login for the app. The admin
  account is created from these when the API boots; sign in to the app with
  exactly these credentials. Leave the password empty only if the droplet is
  otherwise access-restricted (disables login entirely).

## 6. Launch
```bash
docker compose build
docker compose up -d mongo              # first boot: creates the root user, then the replica set
docker compose run --rm migrate         # indexes + dues/stock ledgers (safe on an empty database)
docker compose up -d
docker compose ps
docker compose logs -f api              # "connected to MongoDB (… transactions true)"
```
Mongo runs as a **one-node replica set** because every money and stock change
is a multi-document transaction. The compose file handles it: a keyfile is
generated once into the `mongo_keyfile` volume, and the health check runs
`rs.initiate()` the first time. The API refuses to start if Mongo is not a
replica set, or while migrations are pending — it logs exactly what to run.
Visit `http://DROPLET_IP` (or `https://your-domain`). The SPA loads and talks to the live API.

## 7. Firewall
```bash
ufw allow OpenSSH
ufw allow 80
ufw allow 443
ufw enable
```
Mongo (27017) is **not** published to the host, so it's not internet-reachable — don't add a rule for it.

---

## Updating after a code change
Back up, update, migrate, start — in that order:
```bash
cd bronze-boxing
./backup-now.sh                          # or the mongodump command under "Backups"
git pull
docker compose build
docker compose run --rm migrate -dry-run # read what will change
docker compose run --rm migrate          # apply (a no-op when nothing is pending)
docker compose up -d
docker compose run --rm migrate -verify  # dues balances and stock ledgers add up
```
If `migrate -dry-run` lists items under **needs review**, they are real-data
questions (e.g. an old subscription payment with no period) — the migration
never guesses; fix them in the app afterwards.

### One-time upgrade: standalone Mongo → replica set (September 2026 release)
The first deploy of this release restarts the existing `mongo_data` volume as
a replica set. Data is kept; nothing is deleted. Rehearsed locally on a copy
of the old standalone layout (legacy data → replica set → migrate twice →
verify → API boots).
1. **Back up** (see "Backups") and copy the archive off the droplet.
2. `git pull && docker compose build`
3. `docker compose up -d mongo` — recreated with `--replSet rs0 --keyFile …`;
   wait for `docker compose ps` to show it **healthy** (the health check
   initiates the replica set).
4. `docker compose run --rm migrate -dry-run`, read it, then
   `docker compose run --rm migrate`.
5. `docker compose up -d`, then `docker compose run --rm migrate -verify`.

**Rollback:** `git checkout <previous commit> && docker compose up -d --build`.
The previous version starts on the same volume (Mongo runs standalone again
and ignores the replica-set config); the new collections (`subscription_terms`,
`subscription_charges`, `stock_movements`, `schema_migrations`) are simply
unused by it. Restore the backup only if you must also undo data entered
after the upgrade.

## Backups (Mongo)
Take one before every update. `backup-now.sh` in the repo root wraps this.
```bash
# dump to a file on the host
docker compose exec -T mongo mongodump --username "$MONGO_USER" --password "$MONGO_PASSWORD" \
  --authenticationDatabase admin --db "$DB_NAME" --archive > backup-$(date +%F).archive

# restore
docker compose exec -T mongo mongorestore --username "$MONGO_USER" --password "$MONGO_PASSWORD" \
  --authenticationDatabase admin --archive < backup-YYYY-MM-DD.archive
```
The data also persists in the `mongo_data` Docker volume across container restarts.

## Seeding demo data (optional, first run)
The compose file ships a one-off `seed` service (behind a profile, so `up` never
runs it by accident). **It wipes all collections** and loads demo data:
```bash
docker compose --profile seed run --rm --build seed
```
For a real studio, skip this and enter data through the UI.

---

## How auth works
- Accounts live in the `users` collection (bcrypt password hashes). On boot the
  API creates/syncs the **admin account** from `ADMIN_USERNAME` /
  `ADMIN_PASSWORD` in `.env` — the env file is the source of truth for that
  one account.
- The SPA shows a sign-in screen (username + password). `POST /api/auth/login`
  (rate-limited per real client IP — the rightmost `X-Forwarded-For` set by
  Caddy, so it can't be spoofed) exchanges credentials for a session. The token
  is delivered to the browser as an **`HttpOnly` cookie** (`SameSite=Lax`,
  `Secure` auto-enabled under HTTPS) — JavaScript never sees it, so an XSS bug
  can't steal it. **"Remember me"** picks a persistent 30-day cookie vs a
  session cookie that dies with the browser. Sessions are server-side (hashed
  tokens in `auth_sessions`, auto-expired by Mongo TTL) and **slide** — an
  actively used session renews so admins aren't logged out mid-use — so sign-out
  and password rotation still revoke them for real. API/CLI clients may instead
  send the token as an `Authorization: Bearer` header.
- **Rotate the password:** edit `ADMIN_PASSWORD` in `.env`, then
  `docker compose up -d api`. All existing sessions for the account are revoked
  and everyone signs in again with the new password.
- `ADMIN_PASSWORD` empty → auth disabled, no login screen (the API logs a
  warning). Only for access-restricted droplets — never on the open internet.
- `/api/health` stays public for uptime checks and reports `authRequired`.

## Troubleshooting
- **API exits: "MongoDB is not a replica set"** → the `mongo` service is still the old definition, or unhealthy. `docker compose up -d mongo`, wait for healthy, then `docker compose up -d api`. Emergency-only escape: `MONGO_ALLOW_STANDALONE=true` (money/stock changes stop being atomic).
- **API exits: "pending migrations"** → back up, then `docker compose run --rm migrate`.
- **Build killed / OOM** → droplet too small; use 2 GB+ or `docker compose build` one service at a time.
- **API can't reach Mongo** → check `MONGO_USER`/`MONGO_PASSWORD` match in `.env`; `docker compose logs mongo`.
- **HTTPS not issued** → DNS A record must resolve to the droplet and ports 80+443 open before Caddy can get a cert; check `docker compose logs web`.
- **Validate compose before launch** → `docker compose config` prints the resolved config (run on the droplet).
