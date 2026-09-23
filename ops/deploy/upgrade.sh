#!/usr/bin/env bash
# Upgrade this droplet to the checked-out commit — carefully, in order:
#
#   1. back up the live database to backups/pre-upgrade-<time>.archive.gz
#      (stops if the backup is empty);
#   2. build the new images (running containers are untouched);
#   3. REHEARSE on a throwaway copy: restore that backup into a separate,
#      temporary Mongo replica set, run the migrations twice and verify —
#      the live database is not touched. Stops on any failure;
#   4. upgrade for real: Mongo restarts as a replica set on the SAME data
#      volume (nothing is deleted), migrations run, the app starts, verify.
#
# Run from the repo on the droplet after `git pull`:
#   ./ops/deploy/upgrade.sh
# Add --yes to skip the pause before step 4.
#
# It never removes a volume. The temporary rehearsal container and network
# it creates are removed when it exits. Rollback: see DEPLOY.md.
set -euo pipefail
cd "$(dirname "$0")/../.."
set -a; . ./.env; set +a
say() { printf '\n==> %s\n' "$*"; }
project=$(docker compose config 2>/dev/null | sed -n 's/^name: //p' | head -1)
: "${project:?could not read the compose project name}"
stamp=$(date -u +%Y%m%d-%H%M%S)

say "1/4 back up $DB_NAME"
mkdir -p backups
pre="backups/pre-upgrade-$stamp.archive.gz"
docker compose exec -T mongo mongodump -u "$MONGO_USER" -p "$MONGO_PASSWORD" --authenticationDatabase admin \
  --db "$DB_NAME" --archive --gzip --quiet > "$pre"
[ -s "$pre" ] || { echo "the backup is empty — stopping before anything changes" >&2; exit 1; }
ls -la "$pre"
echo "copy it off the droplet too, e.g. from your machine:  scp root@<droplet>:$(pwd)/$pre ."

say "2/4 build the new images"
docker compose --profile ops build api migrate web
migrate_image=$(docker compose --profile ops config --images | grep -- '-migrate$' | head -1)
: "${migrate_image:=${project}-migrate}"

say "3/4 rehearse the migrations on a throwaway copy of that backup"
net="bronze-rehearsal-$stamp"
box="bronze-rehearsal-mongo-$stamp"
cleanup() {
  docker rm -f "$box" >/dev/null 2>&1 || true
  docker network rm "$net" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create "$net" >/dev/null
docker run -d --rm --name "$box" --network "$net" --network-alias rehearsal mongo:7 --replSet rs0 --bind_ip_all >/dev/null
for i in $(seq 1 60); do
  if docker exec "$box" mongosh --quiet --eval \
    "try { rs.status().ok } catch (e) { rs.initiate({_id: 'rs0', members: [{_id: 0, host: 'rehearsal:27017'}]}).ok }; quit(db.hello().isWritablePrimary ? 0 : 1)" \
    >/dev/null 2>&1; then break; fi
  [ "$i" = 60 ] && { echo "the rehearsal database didn't start" >&2; exit 1; }
  sleep 2
done
docker exec -i "$box" mongorestore --archive --gzip --quiet < "$pre"
rehearse() {
  docker run --rm --network "$net" -e MONGODB_URI="mongodb://rehearsal:27017/?replicaSet=rs0" \
    -e DB_NAME="$DB_NAME" -e STUDIO_TZ="${STUDIO_TZ:-Asia/Beirut}" "$migrate_image" "$@"
}
echo "--- dry run"; rehearse -dry-run
echo "--- apply";   rehearse
echo "--- apply again (must change nothing)"; rehearse
echo "--- pending after two runs:"; rehearse -list
echo "--- verify";  rehearse -verify
say "rehearsal passed on a copy; the live database is still untouched"

if [ "${1:-}" != "--yes" ]; then
  read -r -p "Upgrade the live system now? Mongo restarts as a replica set on the same volume. [y/N] " ok
  [ "$ok" = "y" ] || [ "$ok" = "Y" ] || { echo "stopped before touching the live system"; exit 0; }
fi

say "4/4 upgrade: Mongo as a replica set on the same data volume"
docker compose up -d mongo
cid=$(docker compose ps -q mongo)
for i in $(seq 1 100); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' "$cid")" = healthy ] && break
  [ "$i" = 100 ] && { echo "mongo didn't become healthy — check: docker compose logs mongo" >&2; exit 1; }
  sleep 3
done
docker compose run --rm migrate -dry-run
docker compose run --rm migrate
docker compose up -d
docker compose run --rm migrate -verify
for i in $(seq 1 40); do
  if docker compose exec -T web wget -qO- http://api:8080/api/health 2>/dev/null; then echo; break; fi
  [ "$i" = 40 ] && { echo "the API isn't answering — check: docker compose logs api" >&2; exit 1; }
  sleep 3
done
docker compose ps
say "upgraded. Backup kept at $pre"
