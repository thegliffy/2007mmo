# Hollowmere ops (Phase 2 Week 1)

How Kyle runs the live Compose stack at **2007.gliffy.tv** without pretending we have Kubernetes.

The hamlet is **one world process + Postgres + Redis**. Postgres is canonical (packs and node remaining). Redis is session/presence only — you do not need a Redis dump to keep tarts.

All commands below assume you are in the git checkout that `docker compose` uses on that host (the same tree you would `git pull` into).

## A1 — Deploy

1. On the live host, in the Compose project directory:
   ```bash
   git fetch origin
   git checkout main
   git pull origin main
   docker compose up --build -d
   ```
2. Confirm the world is honest:
   ```bash
   curl -sf https://2007.gliffy.tv/health
   curl -sf https://2007.gliffy.tv/stats
   curl -sf https://2007.gliffy.tv/metrics | head
   ```
   Locally the same paths are `http://127.0.0.1:8080/...`.
3. Open the site, **hard-refresh** (Ctrl+Shift+R / Cmd+Shift+R) so the browser does not keep a cached `app.js` from the last map ship, enter a name you already used, and check the pack is still there. `index.html` pins `app.js?v=…` — bump that query if static hosting still serves an old script.

Compose rebuilds the `world` image from `Dockerfile`. Postgres data stays on the `pgdata` volume. A deploy does **not** wipe packs.

If you tag releases yourself:

```bash
git rev-parse --short HEAD > /tmp/hollowmere-prev-sha
```

Keep that SHA so rollback has a target.

## A2 — Rollback

Preferred: check out the last known-good commit and rebuild. The volume is unchanged.

```bash
git checkout <previous-sha>
docker compose up --build -d
curl -sf https://2007.gliffy.tv/health
```

Then `git checkout main` when you are ready to try the newer tree again.

If the bad deploy already wrote bad rows (rare; Week 1 has no migrations that rewrite packs), restore the last good dump with A3 instead of only rolling the binary.

Do **not** `docker compose down -v` on the live host. That deletes `pgdata`.

## A3 — Backup and restore (live Compose)

Scripts live in `scripts/` and talk to the Compose service named `postgres`.

### Nightly / before-deploy dump

```bash
./scripts/pg-backup.sh
```

Writes `backups/hollowmere-<UTC>.sql.gz` and points `backups/hollowmere-latest.sql.gz` at it. Copy that file off the box (rsync, S3, whatever you already trust). The dump is the whole `hollowmere` database: `players` (pack + woodland XP) and `nodes` (bramble / hazel remaining).

### Safe drill (does not touch live data)

```bash
./scripts/pg-backup-restore-drill.sh
```

This dumps the running volume (or you pass an existing `.sql.gz`), restores it into a throwaway `postgres:16-alpine` container, and checks that player/node counts match live. Use this after every deploy and whenever you want proof the latest dump is loadable.

### Actual live restore (downtime)

Only when you intend to rewind the hamlet:

```bash
./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz
# type: restore
```

Or non-interactive:

```bash
./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz --yes
```

The script stops `world`, drops and recreates `hollowmere`, loads the dump, starts `world`, and waits for `/health`. After restore, villagers see the packs from dump time. In-flight mill/hearth channels are lost (same T3 rule as a crash: committed rows win; nothing is duplicated).

## A4 — Metrics and rate limits

| URL | What |
|-----|------|
| `/health` | world + postgres + redis ping |
| `/stats` | JSON: tick p50/p99, online, WS, joins, chats, actions, rate-limit counters |
| `/metrics` | Prometheus text of the same numbers |

```bash
npm run stats
curl -s http://127.0.0.1:8080/metrics
```

Rate limits (defaults are generous enough for local bots; tighten on the live host):

| Knob | Default | Live suggestion |
|------|---------|-----------------|
| `HOLLOWMERE_LIMIT_HELLO_RATE` / `_BURST` | 12 / 40 per IP | 3 / 10 |
| `HOLLOWMERE_LIMIT_CONN_RATE` / `_BURST` | 20 / 80 per IP | 5 / 15 |
| `HOLLOWMERE_LIMIT_WS_RATE` / `_BURST` | 20 / 32 per client | 12 / 20 |
| `HOLLOWMERE_LIMIT_CHAT_RATE` / `_BURST` | 0.8 / 4 (~8 lines / 10s) | 0.5 / 3 |

Chat is also still one accepted line per 600ms tick. Auth (`hello`) and new WebSocket upgrades are limited per IP. Put the live values in the `world` service `environment:` on the 2007.gliffy.tv compose file (or an override file you do not commit if you prefer).

Refused frames increment `limitedHello` / `limitedWS` / `limitedChat` / `limitedConn` on `/stats` and `hollowmere_rate_limited_total{kind=...}` on `/metrics`.
