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
- `API_TOKEN` — a long random string (`openssl rand -hex 32`). With it set, the app shows a login screen; enter this same token there to unlock. Leave empty only if the droplet is otherwise access-restricted.

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
- `API_TOKEN` set → every `/api` call requires `Authorization: Bearer <token>`
  (`/api/health` stays public and reports `authRequired: true`).
- The SPA detects this and shows a login screen; the entered token is verified
  against `/api/auth/check`, stored in the browser, and attached to every call.
  A 401 (token rotated/revoked) clears it and returns to the login screen.
- To rotate the token: change `API_TOKEN` in `.env`, `docker compose up -d api`,
  then everyone logs in again with the new token.
- `API_TOKEN` empty → no auth, no login screen. Only for access-restricted droplets.

## Troubleshooting
- **Build killed / OOM** → droplet too small; use 2 GB+ or `docker compose build` one service at a time.
- **API can't reach Mongo** → check `MONGO_USER`/`MONGO_PASSWORD` match in `.env`; `docker compose logs mongo`.
- **HTTPS not issued** → DNS A record must resolve to the droplet and ports 80+443 open before Caddy can get a cert; check `docker compose logs web`.
- **Validate compose before launch** → `docker compose config` prints the resolved config (run on the droplet).
