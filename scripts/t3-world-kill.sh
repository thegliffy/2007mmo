#!/usr/bin/env bash
# T3: kill the world container mid-session and confirm the pack is not duplicated.
# Requires: docker compose stack up, curl, python3.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

STATS=http://127.0.0.1:8080/stats
if ! curl -sf "$STATS" >/dev/null; then
  echo "world is not up. Run: docker compose up --build -d" >&2
  exit 1
fi

echo "== before kill =="
curl -s "$STATS"
echo
echo "Kill world container, then bring it back. The pack is Postgres-canonical:"
echo "  docker compose kill world && docker compose up -d world"
echo "Reconnect the same browser (same localStorage id). Berry/pulp/tart/nut counts must match"
echo "the last committed action — never increase from the crash itself."
docker compose kill world
sleep 1
docker compose up -d world
echo "waiting for health..."
for i in $(seq 1 40); do
  if curl -sf http://127.0.0.1:8080/health >/dev/null; then
    echo "world recovered"
    curl -s "$STATS"
    echo
    echo "T4-ish: compose brought world back; open http://127.0.0.1:8080 and reconnect."
    exit 0
  fi
  sleep 1
done
echo "world did not become healthy" >&2
exit 1
