#!/usr/bin/env bash
# Restore a gzip SQL dump into the Compose Postgres used by Hollowmere.
# Stops the world process first so packs cannot write mid-restore.
#
# Live (2007.gliffy.tv): run this from the same checkout that serves the site.
#   docker compose stop world
#   ./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz --yes
#   docker compose start world
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

YES=0
DUMP=""
for arg in "$@"; do
  case "$arg" in
    --yes|-y) YES=1 ;;
    --help|-h)
      echo "usage: $0 <dump.sql.gz> [--yes]"
      exit 0
      ;;
    *) DUMP="$arg" ;;
  esac
done

if [ -z "$DUMP" ]; then
  echo "usage: $0 <dump.sql.gz> [--yes]" >&2
  exit 1
fi
if [ ! -f "$DUMP" ]; then
  echo "dump not found: $DUMP" >&2
  exit 1
fi

SERVICE="${POSTGRES_SERVICE:-postgres}"
WORLD="${WORLD_SERVICE:-world}"

if [ "$YES" -ne 1 ]; then
  echo "This replaces the live hollowmere database from $DUMP" >&2
  echo "Packs and node remaining counts will match the dump. Type 'restore' to continue." >&2
  read -r ans
  if [ "$ans" != "restore" ]; then
    echo "aborted" >&2
    exit 1
  fi
fi

echo "stopping $WORLD (canonical writes must pause)"
compose stop "$WORLD" || true

echo "recreating database hollowmere"
compose exec -T "$SERVICE" psql -U hollowmere -d postgres -v ON_ERROR_STOP=1 <<'SQL'
SELECT pg_terminate_backend(pid)
  FROM pg_stat_activity
 WHERE datname = 'hollowmere' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS hollowmere;
CREATE DATABASE hollowmere OWNER hollowmere;
SQL

echo "loading $DUMP"
gzip -dc "$DUMP" | compose exec -T "$SERVICE" psql -U hollowmere -d hollowmere -v ON_ERROR_STOP=1

echo "starting $WORLD"
compose start "$WORLD"

echo "waiting for /health"
for i in $(seq 1 40); do
  if curl -sf http://127.0.0.1:8080/health >/dev/null; then
    echo "restore complete — world accepted /health"
    exit 0
  fi
  sleep 1
done
echo "database restored but world did not become healthy; check docker compose logs world" >&2
exit 1
