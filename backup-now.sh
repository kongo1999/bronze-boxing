#!/usr/bin/env bash
# One-off Mongo backup before an update: a compressed archive of the app
# database in ./backups/, named by date and time — encrypted when
# BACKUP_PASSPHRASE is set in .env (restore with ./restore-backup.sh).
# Works whatever state Mongo is in (standalone or replica set), because it
# runs mongodump inside the running mongo container. Reads MONGO_USER,
# MONGO_PASSWORD, DB_NAME and BACKUP_PASSPHRASE from .env.
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a
mkdir -p backups
stamp=$(date -u +%Y%m%d-%H%M%S)
dump() {
  docker compose exec -T mongo mongodump \
    --username "$MONGO_USER" --password "$MONGO_PASSWORD" --authenticationDatabase admin \
    --db "$DB_NAME" --archive --gzip --quiet
}
if [ -n "${BACKUP_PASSPHRASE:-}" ]; then
  out="backups/bronze-$stamp.archive.gz.enc"
  dump | openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$out"
else
  out="backups/bronze-$stamp.archive.gz"
  dump > "$out"
  echo "note: BACKUP_PASSPHRASE is not set — this backup is not encrypted" >&2
fi
echo "backup written: $out ($(du -h "$out" | cut -f1))"
