#!/usr/bin/env bash
# Scheduled Mongo backups, run by the `backup` compose service (mongo:7
# image, which ships mongodump and openssl).
#
# Every BACKUP_INTERVAL_HOURS (default 24, first one at start-up) it writes
# a compressed archive of the app database to /backups (./backups on the
# host):
#   - encrypted (AES-256, PBKDF2) when BACKUP_PASSPHRASE is set:
#       bronze-YYYYmmdd-HHMMSS.archive.gz.enc
#   - otherwise plain, with a warning each time — such files stay on this
#     server; the off-site copier only ever uploads encrypted ones.
# Files of ours older than BACKUP_RETENTION_DAYS (default 14) are pruned;
# nothing else in the directory is touched.
set -uo pipefail

interval_h=${BACKUP_INTERVAL_HOURS:-24}
keep_days=${BACKUP_RETENTION_DAYS:-14}
uri=${BACKUP_MONGO_URI:-"mongodb://${MONGO_USER}:${MONGO_PASSWORD}@mongo:27017/?authSource=admin&replicaSet=rs0"}
dir=${BACKUP_DIR:-/backups}
mkdir -p "$dir"

log() { echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) backup: $*"; }

backup_once() {
  local stamp out
  stamp=$(date -u +%Y%m%d-%H%M%S)
  if [ -n "${BACKUP_PASSPHRASE:-}" ]; then
    out="$dir/bronze-$stamp.archive.gz.enc"
    if mongodump --uri "$uri" --db "$DB_NAME" --archive --gzip --quiet |
      openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$out.part"; then
      mv "$out.part" "$out"
      log "wrote $out ($(du -h "$out" | cut -f1), encrypted)"
    else
      rm -f "$out.part"
      log "FAILED — no backup written this round"
    fi
  else
    out="$dir/bronze-$stamp.archive.gz"
    if mongodump --uri "$uri" --db "$DB_NAME" --archive="$out.part" --gzip --quiet; then
      mv "$out.part" "$out"
      log "wrote $out ($(du -h "$out" | cut -f1)) — UNENCRYPTED: set BACKUP_PASSPHRASE; this file stays on this server"
    else
      rm -f "$out.part"
      log "FAILED — no backup written this round"
    fi
  fi
  # Retention: only our own files, only past the configured age.
  find "$dir" -maxdepth 1 -type f -name 'bronze-*.archive.gz*' ! -name '*.part' -mtime +"$keep_days" -print |
    while read -r old; do rm -f -- "$old" && log "pruned $old (older than $keep_days days)"; done
}

# BACKUP_ONCE=1: take one backup now and exit (e.g. right before an update:
#   docker compose run --rm -e BACKUP_ONCE=1 backup).
if [ "${BACKUP_ONCE:-}" = "1" ]; then
  backup_once
  exit 0
fi
log "every ${interval_h}h, keeping ${keep_days} days, into $dir"
while true; do
  backup_once
  sleep $((interval_h * 3600))
done
