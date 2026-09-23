#!/usr/bin/env bash
# One-off Mongo backup before an update: a compressed archive of the app
# database in ./backups/, named by date and time. Reads MONGO_USER,
# MONGO_PASSWORD and DB_NAME from .env. Copy the file off the droplet.
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a
mkdir -p backups
out="backups/bronze-$(date +%Y%m%d-%H%M%S).archive.gz"
docker compose exec -T mongo mongodump \
  --username "$MONGO_USER" --password "$MONGO_PASSWORD" --authenticationDatabase admin \
  --db "$DB_NAME" --archive --gzip > "$out"
echo "backup written: $out ($(du -h "$out" | cut -f1))"
