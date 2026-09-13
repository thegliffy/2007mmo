#!/usr/bin/env bash
# Restore a gzip SQL dump into the Compose Postgres used by Hollowmere.
# Stops the world process first so packs cannot write mid-restore.
#
# Live (2007.gliffy.tv), from the same checkout that serves the site:
#   ./scripts/pg-backup.sh
#   ./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz --yes
#
# The world is published on :8080 locally and :28080 on the live host.
# HEALTH_URL overrides the probe; otherwise both ports are tried.
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
  echo "Accounts, packs, coins, and node remaining counts will match the dump." >&2
  echo "Loot piles and in-flight mill/kiln/anvil channels are lost (memory-only)." >&2
  echo "Type 'restore' to continue." >&2
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

world_healthy() {
  if [ -n "${HEALTH_URL:-}" ]; then
    curl -sf "$HEALTH_URL" >/dev/null
    return $?
  fi
  # Local compose publishes :8080; the live host publishes :28080 behind Caddy.
  curl -sf http://127.0.0.1:8080/health >/dev/null && return 0
  curl -sf http://127.0.0.1:28080/health >/dev/null && return 0
  return 1
}

echo "waiting for /health (8080 local, 28080 live, or HEALTH_URL)"
for i in $(seq 1 40); do
  if world_healthy; then
    echo "restore complete — world accepted /health"
    exit 0
  fi
  sleep 1
done
echo "database restored but world did not become healthy; check docker compose logs world" >&2
exit 1
