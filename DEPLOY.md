# Deploying Bronze Boxing to a droplet (Docker Compose)

The whole app runs as three containers on one droplet:

| Service | What | Exposed? |
|---------|------|----------|
| `web`   | Caddy — serves the Vue SPA, reverse-proxies `/api`, auto-HTTPS | **yes** (80/443) |
| `api`   | Go (Fiber) API | no (private network) |
| `mongo` | MongoDB 7, data in a named volume | no (private network) |

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
docker compose up -d --build
docker compose ps
docker compose logs -f api        # watch it connect to Mongo
```
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
```bash
cd bronze-boxing
git pull
docker compose up -d --build      # rebuilds changed images, recreates containers
```

## Backups (Mongo)
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
  (rate-limited per IP) exchanges credentials for a 30-day session token, which
  the browser stores and sends as a Bearer header on every call. Sessions are
  server-side (hashed tokens in `auth_sessions`, auto-expired by Mongo TTL), so
  sign-out and password rotation revoke them for real.
- **Rotate the password:** edit `ADMIN_PASSWORD` in `.env`, then
  `docker compose up -d api`. All existing sessions for the account are revoked
  and everyone signs in again with the new password.
- `ADMIN_PASSWORD` empty → auth disabled, no login screen (the API logs a
  warning). Only for access-restricted droplets — never on the open internet.
- `/api/health` stays public for uptime checks and reports `authRequired`.

## Troubleshooting
- **Build killed / OOM** → droplet too small; use 2 GB+ or `docker compose build` one service at a time.
- **API can't reach Mongo** → check `MONGO_USER`/`MONGO_PASSWORD` match in `.env`; `docker compose logs mongo`.
- **HTTPS not issued** → DNS A record must resolve to the droplet and ports 80+443 open before Caddy can get a cert; check `docker compose logs web`.
- **Validate compose before launch** → `docker compose config` prints the resolved config (run on the droplet).
