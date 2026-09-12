#!/usr/bin/env bash
# Safe drill: dump the running Compose Postgres, restore that dump into a
# throwaway container, and compare player/node counts. Does not rewrite live data.
#
# Kyle on 2007.gliffy.tv: run this from the live Compose checkout whenever you
# want proof the latest dump is loadable. Use pg-restore.sh only when you
# actually intend to roll the hamlet back.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  else
    docker-compose "$@"
  fi
}

SERVICE="${POSTGRES_SERVICE:-postgres}"
NAME="hollowmere-drill-pg"
PORT="${DRILL_PG_PORT:-55432}"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! compose exec -T "$SERVICE" pg_isready -U hollowmere -d hollowmere >/dev/null 2>&1; then
  echo "Compose postgres is not ready. On the live host: docker compose up -d postgres" >&2
  exit 1
fi

DUMP="${1:-}"
if [ -z "$DUMP" ]; then
  DUMP="$("$ROOT/scripts/pg-backup.sh")"
fi
if [ ! -f "$DUMP" ]; then
  echo "dump not found: $DUMP" >&2
  exit 1
fi

live_players="$(compose exec -T "$SERVICE" psql -U hollowmere -d hollowmere -tAc 'SELECT count(*) FROM players')"
live_nodes="$(compose exec -T "$SERVICE" psql -U hollowmere -d hollowmere -tAc 'SELECT count(*) FROM nodes')"
echo "live players=$live_players nodes=$live_nodes"

echo "starting throwaway postgres on :$PORT"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" \
  -e POSTGRES_USER=hollowmere \
  -e POSTGRES_PASSWORD=hollowmere \
  -e POSTGRES_DB=hollowmere \
  -p "$PORT:5432" \
  postgres:16-alpine >/dev/null

for i in $(seq 1 40); do
  if docker exec "$NAME" pg_isready -U hollowmere -d hollowmere >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 40 ]; then
    echo "drill postgres did not become ready" >&2
    exit 1
  fi
  sleep 1
done

echo "restoring dump into drill db"
gzip -dc "$DUMP" | docker exec -i "$NAME" psql -U hollowmere -d hollowmere -v ON_ERROR_STOP=1 >/dev/null

drill_players="$(docker exec "$NAME" psql -U hollowmere -d hollowmere -tAc 'SELECT count(*) FROM players')"
drill_nodes="$(docker exec "$NAME" psql -U hollowmere -d hollowmere -tAc 'SELECT count(*) FROM nodes')"
echo "drill players=$drill_players nodes=$drill_nodes"

if [ "$live_players" != "$drill_players" ] || [ "$live_nodes" != "$drill_nodes" ]; then
  echo "DRILL FAIL: counts differ. dump may be incomplete." >&2
  exit 1
fi

echo "DRILL PASS — dump $DUMP restores with matching player/node counts."
echo "Live rollback (downtime) is: ./scripts/pg-restore.sh $DUMP --yes"
