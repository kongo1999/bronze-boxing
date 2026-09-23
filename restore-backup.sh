#!/usr/bin/env bash
# Restore a backup into a NEW, separate database — never over the live one —
# and verify it. Use it for the restore drill, or to look at old data.
#
#   ./restore-backup.sh backups/bronze-20260923-020000.archive.gz.enc
#   ./restore-backup.sh backups/bronze-20260923-020000.archive.gz.enc bronze_restore_check
#
# Encrypted files (.enc) need BACKUP_PASSPHRASE in .env (or the environment).
# The target database must not exist yet; the script refuses the live
# DB_NAME and any database that already has collections. Switching the app
# over to a restored database is a separate, deliberate step (DEPLOY.md).
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a

file=${1:?usage: $0 <backup file> [target database]}
target=${2:-bronze_restore_$(date +%Y%m%d_%H%M%S)}
[ -f "$file" ] || { echo "no such file: $file" >&2; exit 1; }
if [ "$target" = "$DB_NAME" ]; then
  echo "refusing: $target is the live database. Restore into a new name, check it, then switch deliberately." >&2
  exit 1
fi

mongosh_eval() {
  docker compose exec -T mongo mongosh --quiet -u "$MONGO_USER" -p "$MONGO_PASSWORD" --authenticationDatabase admin --eval "$1"
}
existing=$(mongosh_eval "db.getSiblingDB('$target').getCollectionNames().length")
if [ "$existing" != "0" ]; then
  echo "refusing: database $target already has $existing collections. Pick a new name." >&2
  exit 1
fi

decrypt() {
  case "$file" in
    *.enc)
      : "${BACKUP_PASSPHRASE:?BACKUP_PASSPHRASE is needed to decrypt $file}"
      openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -pass env:BACKUP_PASSPHRASE -in "$file" ;;
    *) cat "$file" ;;
  esac
}

echo "restoring $file into $target ..."
decrypt | docker compose exec -T mongo mongorestore -u "$MONGO_USER" -p "$MONGO_PASSWORD" --authenticationDatabase admin \
  --archive --gzip --nsFrom "${DB_NAME}.*" --nsTo "${target}.*" --quiet

echo "document counts in $target:"
mongosh_eval "const d = db.getSiblingDB('$target'); d.getCollectionNames().sort().forEach(c => print('  ' + c + ': ' + d.getCollection(c).countDocuments()))"

echo "integrity check:"
docker compose --profile ops run --rm -e DB_NAME="$target" migrate -verify
echo "restored and verified: $target (the live database $DB_NAME was not touched)"
