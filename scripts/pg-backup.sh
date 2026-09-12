#!/usr/bin/env bash
# Dump the Compose Postgres volume (canonical packs + nodes) to backups/.
# Works against the local stack and Kyle's live 2007.gliffy.tv Compose project.
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
OUT_DIR="${BACKUP_DIR:-$ROOT/backups}"
mkdir -p "$OUT_DIR"
STAMP="${BACKUP_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT="$OUT_DIR/hollowmere-${STAMP}.sql.gz"

if ! compose exec -T "$SERVICE" pg_isready -U hollowmere -d hollowmere >/dev/null 2>&1; then
  echo "postgres service '$SERVICE' is not ready in this Compose project." >&2
  echo "From the 2007.gliffy.tv host checkout: docker compose up -d postgres" >&2
  exit 1
fi

echo "dumping $SERVICE → $OUT" >&2
compose exec -T "$SERVICE" pg_dump -U hollowmere -d hollowmere --no-owner --no-acl --format=plain \
  | gzip -c > "$OUT"
ln -sfn "$(basename "$OUT")" "$OUT_DIR/hollowmere-latest.sql.gz"
echo "ok $OUT ($(wc -c < "$OUT") bytes)" >&2
# path only on stdout so other scripts can capture it
echo "$OUT"
