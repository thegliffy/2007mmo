#!/usr/bin/env bash
# Safe drill: dump the running Compose Postgres, restore that dump into a
# throwaway container, and compare accounts / players / nodes / coins / metal
# kinds. Does not rewrite live data.
#
# Kyle on 2007.gliffy.tv — exact commands, no guesswork:
#
#   cd /path/to/the/2007mmo/checkout    # the tree docker compose uses
#   ./scripts/pg-backup.sh
#   ./scripts/pg-backup-restore-drill.sh
#   # or drill an existing dump:
#   ./scripts/pg-backup-restore-drill.sh backups/hollowmere-latest.sql.gz
#
# A pass means the latest dump is loadable and the hamlet's canonical
# rows (accounts, packs, coins, woodland + scar nodes) survived the trip.
# Use pg-restore.sh only when you actually intend to rewind the live volume.
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
if [ ! -s "$DUMP" ]; then
  echo "dump is empty: $DUMP" >&2
  exit 1
fi

live_sql() {
  compose exec -T "$SERVICE" psql -U hollowmere -d hollowmere -tAc "$1"
}

echo "live schema=$(live_sql 'SELECT COALESCE(max(version),0) FROM schema_migrations')"
echo "live accounts=$(live_sql 'SELECT count(*) FROM accounts') players=$(live_sql 'SELECT count(*) FROM players') nodes=$(live_sql 'SELECT count(*) FROM nodes')"
echo "live coins=$(live_sql 'SELECT COALESCE(sum(coins),0) FROM players')"
echo "live node kinds:"
live_sql "SELECT kind || '=' || count(*) FROM nodes GROUP BY kind ORDER BY 1"

LIVE_ACCOUNTS="$(live_sql 'SELECT count(*) FROM accounts')"
LIVE_PLAYERS="$(live_sql 'SELECT count(*) FROM players')"
LIVE_NODES="$(live_sql 'SELECT count(*) FROM nodes')"
LIVE_COINS="$(live_sql 'SELECT COALESCE(sum(coins),0) FROM players')"
LIVE_SCHEMA="$(live_sql 'SELECT COALESCE(max(version),0) FROM schema_migrations')"
LIVE_KINDS="$(live_sql "SELECT kind || '=' || count(*) FROM nodes GROUP BY kind ORDER BY 1")"
LIVE_METAL="$(live_sql "SELECT count(*) FROM nodes WHERE kind IN ('copper','tin','kiln','anvil')")"

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

drill_sql() {
  docker exec "$NAME" psql -U hollowmere -d hollowmere -tAc "$1"
}

# A dump from before accounts existed would restore, then fail these
# lookups — that is the point: Kyle should not trust an old woodland-only
# dump against today's hamlet.
if ! drill_sql "SELECT to_regclass('public.accounts')" | grep -q accounts; then
  echo "DRILL FAIL: dump has no accounts table. This is not a current Hollowmere dump." >&2
  exit 1
fi

DRILL_ACCOUNTS="$(drill_sql 'SELECT count(*) FROM accounts')"
DRILL_PLAYERS="$(drill_sql 'SELECT count(*) FROM players')"
DRILL_NODES="$(drill_sql 'SELECT count(*) FROM nodes')"
DRILL_COINS="$(drill_sql 'SELECT COALESCE(sum(coins),0) FROM players')"
DRILL_SCHEMA="$(drill_sql 'SELECT COALESCE(max(version),0) FROM schema_migrations')"
DRILL_KINDS="$(drill_sql "SELECT kind || '=' || count(*) FROM nodes GROUP BY kind ORDER BY 1")"
DRILL_METAL="$(drill_sql "SELECT count(*) FROM nodes WHERE kind IN ('copper','tin','kiln','anvil')")"

echo "drill schema=$DRILL_SCHEMA accounts=$DRILL_ACCOUNTS players=$DRILL_PLAYERS nodes=$DRILL_NODES coins=$DRILL_COINS metal_nodes=$DRILL_METAL"
echo "drill node kinds:"
echo "$DRILL_KINDS"

fail=0
check() {
  local name="$1" live="$2" drill="$3"
  if [ "$live" != "$drill" ]; then
    echo "DRILL FAIL: $name live=$live drill=$drill" >&2
    fail=1
  fi
}
check accounts "$LIVE_ACCOUNTS" "$DRILL_ACCOUNTS"
check players "$LIVE_PLAYERS" "$DRILL_PLAYERS"
check nodes "$LIVE_NODES" "$DRILL_NODES"
check coins "$LIVE_COINS" "$DRILL_COINS"
check schema "$LIVE_SCHEMA" "$DRILL_SCHEMA"
check kinds "$LIVE_KINDS" "$DRILL_KINDS"
check metal_nodes "$LIVE_METAL" "$DRILL_METAL"

if [ "$fail" -ne 0 ]; then
  echo "DRILL FAIL: dump $DUMP is incomplete or from a different schema." >&2
  exit 1
fi

if [ "${LIVE_METAL:-0}" -lt 1 ]; then
  echo "DRILL WARN: live volume has no copper/tin/kiln/anvil rows yet." >&2
  echo "That is fine on a fresh volume (world seeds them on boot). After the" >&2
  echo "eastern-scars deploy has been up once, this should be several veins." >&2
fi

echo "DRILL PASS — dump $DUMP restores accounts, packs, coins, and node kinds."
echo "Live rollback (downtime) is: ./scripts/pg-restore.sh $DUMP --yes"
echo "On the live host, if /health is not on :8080:"
echo "  HEALTH_URL=http://127.0.0.1:28080/health ./scripts/pg-restore.sh $DUMP --yes"
