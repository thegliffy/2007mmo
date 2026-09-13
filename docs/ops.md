# Hollowmere ops

How Kyle runs the live Compose stack at **2007.gliffy.tv** without pretending we have Kubernetes.

The hamlet is **one world process + Postgres + Redis**. Postgres is canonical (accounts, packs, coins, personal chest, node remaining). Redis holds **login sessions** and presence — you do not need a Redis dump to keep tarts, bronze, or what you left in the oak chest, but flushing Redis **logs everybody out**.

All commands below assume you are in the git checkout that `docker compose` uses on that host (the same tree you would `git pull` into). On the live box the world is published at **127.0.0.1:28080** and Caddy terminates TLS in front of it. The snippet the host should be running is [`deploy/caddy/Caddyfile.snippet`](../deploy/caddy/Caddyfile.snippet).

## P0 checklist

Do these from the live checkout after every merge to `main`, and whenever you want proof the hamlet can be rewound.

| # | What | Exact command |
|---|------|----------------|
| 1 | Record the SHA you are about to leave | `git rev-parse --short HEAD \| tee /tmp/hollowmere-prev-sha` |
| 2 | Dump Postgres | `./scripts/pg-backup.sh` then copy `backups/hollowmere-latest.sql.gz` off the box |
| 3 | Prove the dump loads | `./scripts/pg-backup-restore-drill.sh` — must print `DRILL PASS` |
| 4 | Pull and rebuild | see **A1** |
| 5 | Health (public) | `curl -sf https://2007.gliffy.tv/health` |
| 6 | Stats (on the box only) | `curl -s http://127.0.0.1:28080/stats` — watch `lagP99Ms`, `online`, `ws`, `reconnects` |
| 7 | Hard-refresh the site | Ctrl+Shift+R / Cmd+Shift+R. `index.html` pins `app.js?v=iso-grid` |
| 8 | Rollback if the world is wrong | **A2** |

`/stats` and `/metrics` are **404 at the edge on purpose**. Do not curl them via `https://2007.gliffy.tv`.

P1 appearance (skin / hair / tunic) lives on `players.looks`. P2 bank lives on `players.bank` / `players.bank_coins` — personal, no shared stash. Out of scope still: player trade, GE, new skills or regions.

## A1 — Deploy

On the live host, in the Compose project directory:

```bash
# 1. Remember where you were (rollback target).
git rev-parse --short HEAD | tee /tmp/hollowmere-prev-sha

# 2. Dump first. A rebuild does not wipe pgdata, but a bad migration
#    is cheaper to undo from a file than from memory.
./scripts/pg-backup.sh
# copy backups/hollowmere-latest.sql.gz off the box

# 3. Pull the tree compose builds from.
git fetch origin
git checkout main
git pull origin main

# 4. Rebuild the world image and recreate the world container.
#    Postgres and Redis stay up; the pgdata volume is untouched.
docker compose up --build -d
```

Confirm the world is honest. **Health is public. Stats are not.**

```bash
curl -sf https://2007.gliffy.tv/health
# on the box, past Caddy (this is the live scrape):
curl -s http://127.0.0.1:28080/health
curl -s http://127.0.0.1:28080/stats
curl -s http://127.0.0.1:28080/metrics | head
```

Locally the same paths are `http://127.0.0.1:8080/...`.

Open the site and **hard-refresh** (Ctrl+Shift+R / Cmd+Shift+R) so the browser does not keep a cached `app.js` from the last map ship. Log in with a name you already used and check the pack is still there.

`client/index.html` pins the scripts:

```
script src="iso.js?v=iso-grid"
script src="sprites.js?v=iso-grid"
script src="art.js?v=iso-grid"
script src="app.js?v=iso-grid"
script src="pick-npc.js?v=iso-grid"
```

Bump that `?v=` whenever a client fix must land through a cache. The world also sends `Cache-Control: no-cache` on HTML/JS/CSS; the query string is the belt as well as the braces.

Compose rebuilds the `world` image from `Dockerfile`. There is no separate image registry — rollback is a git checkout of the previous SHA, then the same `docker compose up --build -d`.

## A2 — Rollback

Preferred: check out the SHA you wrote down in A1 and rebuild. The volume is unchanged.

```bash
PREV=$(cat /tmp/hollowmere-prev-sha)   # or any known-good short SHA
git fetch origin
git checkout "$PREV"
docker compose up --build -d
curl -sf https://2007.gliffy.tv/health
curl -s http://127.0.0.1:28080/stats
```

Hard-refresh the site again (the `?v=` on `app.js` changes with the tree). Then `git checkout main` when you are ready to try the newer tree.

If the bad deploy already wrote bad rows (a migration that rewrote packs, a botched admin delete), restore the last good dump with A3 instead of only rolling the binary.

Do **not** `docker compose down -v` on the live host. That deletes `pgdata`.

Do **not** `docker compose down` without `-v` either unless you mean a full stop; `compose up --build -d` after `git checkout` is enough.

## A3 — Backup and restore (live Compose)

Scripts live in `scripts/` and talk to the Compose service named `postgres`.

The dump is the whole `hollowmere` database, which today is:

| Table | What you get back |
|-------|-------------------|
| `accounts` | names, scrypt hashes, roles, bans |
| `players` | pack JSON, skills (including Mining / Smithing), hp, coins, personal chest (`bank` JSON + `bank_coins`), `account_id`, `looks` (appearance JSON, null until the creator runs) |
| `nodes` | bramble, hazel, trees, copper, tin, kiln, anvil remaining / cooldown |
| `admin_actions` | host-tool audit log |
| `schema_migrations` | which steps have run |

Loot piles and in-flight mill / kiln / anvil channels are **memory-only**. A restore (or a crash) drops them. That is the safe direction: committed packs win, nothing is duplicated.

### Nightly / before-deploy dump

```bash
cd /path/to/the/2007mmo/checkout    # the tree docker compose uses
./scripts/pg-backup.sh
```

Writes `backups/hollowmere-<UTC>.sql.gz` and points `backups/hollowmere-latest.sql.gz` at it. Copy that file off the box (rsync, S3, whatever you already trust).

### Safe drill (does not touch live data)

```bash
./scripts/pg-backup-restore-drill.sh
# or a specific dump:
./scripts/pg-backup-restore-drill.sh backups/hollowmere-latest.sql.gz
```

Dumps the running volume (unless you pass a file), restores it into a throwaway `postgres:16-alpine` container, and checks that **accounts, players, nodes, coin totals, schema version, and node-kind counts** match live. Use this after every deploy and whenever you want proof the latest dump is loadable. A pass prints `DRILL PASS`.

### Actual live restore (downtime)

Only when you intend to rewind the hamlet:

```bash
./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz
# type: restore
```

Or non-interactive:

```bash
# live host publishes the world on :28080; the script tries 8080 then 28080.
./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz --yes
# if you want to be explicit:
HEALTH_URL=http://127.0.0.1:28080/health \
  ./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz --yes
```

The script stops `world`, drops and recreates `hollowmere`, loads the dump, starts `world`, and waits for `/health`. After restore, villagers see the packs and purses from dump time.

## A4 — Accounts and sessions

Players have **real accounts**: a name, a password, and an HttpOnly session cookie.

| Endpoint | What |
|----------|------|
| `POST /auth/register` | `{"username","password"}` → 201, sets the session cookie |
| `POST /auth/login` | `{"username","password"}` → 200, sets the session cookie |
| `POST /auth/logout` | drops that session and closes its socket |
| `GET /auth/me` | `{"username"}` plus `looks` / `needsLooks` |
| `GET /auth/looks` | catalog + current face (session required) |
| `POST /auth/looks` | first-write appearance; later posts return the face that already stuck |
| `POST /auth/password` | `{"current","next"}` → rotates the password and signs out every other session |

Facts worth knowing before an incident:

- **Passwords** are stored only as scrypt hashes (`scrypt$N$r$p$salt$key`, N=16384, ~16 MiB per hash). The format carries its own parameters, so raising the cost later re-hashes each password on the owner's next login. There is no recovery path — no email on file means **a forgotten password cannot be reset**, only the row deleted.
- **Sessions live in Redis** (`session:<token>`, plus `acct-sessions:<accountID>` for revocation), 7 days sliding. `redis-cli FLUSHALL` signs out the whole hamlet; it loses no packs.
- **One live session per account.** A successful login (or an authenticated `/ws` join) revokes every other Redis token for that account and closes any other open socket with `code=replaced` / “Signed in somewhere else.” The old tab stops retrying; its cookie is already dead. The pack is whatever Postgres last agreed to — the kick does not mint items.
- **Concurrent hashing is capped** at 4, so a login flood costs ~64 MiB rather than one 16 MiB allocation per request.
- **One account owns exactly one player row**, enforced by a unique index on `players.account_id`.
- Player rows created before accounts existed keep `account_id IS NULL` and are simply unreachable. To see them: `SELECT id, name FROM players WHERE account_id IS NULL;`

### Live settings that matter

Put these in the `world` service `environment:` on the 2007.gliffy.tv compose file. **The defaults are wrong for a proxied host.**

| Variable | Live value | Why |
|----------|-----------|-----|
| `HOLLOWMERE_TRUSTED_PROXIES` | Caddy's address or CIDR | Without it every player shares one rate-limit bucket and one failed-login budget, because every request arrives from Caddy. Only then is `X-Forwarded-For` believed. |
| `HOLLOWMERE_ALLOWED_ORIGINS` | `2007.gliffy.tv` | Cross-site WebSocket upgrades and auth posts are refused unless the Origin matches this or the request Host. |
| `HOLLOWMERE_SECURE_COOKIES` | `1` | Forces `Secure` on the session cookie. Otherwise it is set only when the world itself terminates TLS or Caddy sends `X-Forwarded-Proto: https`. |
| `HOLLOWMERE_LIMIT_LOGIN_RATE` / `_BURST` | `0.2` / `8` | Per source address. Compose ships `60` / `400` so the 200-bot harness can sign in — **that is a load-test value, not a live one.** |
| `HOLLOWMERE_LIMIT_LOGIN_USER_RATE` / `_BURST` | `0.1` / `6` | Per account name, so a spray from many addresses at one account is still capped. |

Live TLS is Caddy on the host → `127.0.0.1:28080`. Find the bridge gateway the
proxy reaches the world through, and re-check it after any `docker compose up`:

```bash
docker inspect 2007mmo-world-1 -f '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}'
```

If it drifts from the compose value, update `HOLLOWMERE_TRUSTED_PROXIES` and
recreate `world`. A stale value is not loud: logins keep working, but every
player silently shares one throttle bucket and the cookie loses `Secure`.

`X-Forwarded-For` is read **right to left**, skipping hops that are themselves
trusted. Caddy appends the address it saw rather than replacing the header, so
the leftmost entry is whatever the client sent — trusting it would let anyone
pick the address their password guesses are counted against.

### A6 — Accounts: recovery and moderation

No email is collected, so there is no self-service reset. Recovery is an
operator running the admin tool on the host, which ships inside the world image:

```bash
docker compose exec world /app/admin list            # accounts, with role and ban state
docker compose exec world /app/admin reset Kyle      # new random password, printed once
docker compose exec world /app/admin revoke Kyle     # sign out everywhere, password untouched
docker compose exec world /app/admin grant Kyle admin    # player | moderator | admin
docker compose exec world /app/admin ban Kyle 24h        # bar and evict
docker compose exec world /app/admin unban Kyle
docker compose exec world /app/admin mute Kyle 15m       # quiet in chat
docker compose exec world /app/admin audit               # who did what
```

Every command writes to an append-only `admin_actions` table; `admin audit`
reads it back. Role grants are host-only by design — see
[docs/adr/0001-admin-role.md](adr/0001-admin-role.md).

A **ban** fails `Auth.Resolve`, so the heartbeat loop evicts any open socket
within about ten seconds (measured: 6.6s) and a fresh login gets 403. A **mute**
lives in Redis under a TTL, so it expires on its own and survives a restart;
the hub caches it and picks up a change within about ten seconds.

`reset` prints a strong random password **once** — it is not stored anywhere in
readable form and cannot be shown again. Relay it out of band and have the
player change it from the Account panel. Use `revoke` instead when a cookie
leaked but the password is still good.

Both sign the account out everywhere, and an already-connected player is
**evicted within about ten seconds** — the heartbeat loop re-checks every open
socket's session on the same cadence it writes presence. Verified: reset to
eviction measured at 5.8s, and the player is told why rather than the socket
simply vanishing.

Anyone who can run this can already read the database, so it adds no exposure
that `docker compose exec` did not already have.

### Deleting or renaming an account

```bash
docker compose exec postgres psql -U hollowmere -d hollowmere \
  -c "DELETE FROM accounts WHERE username_key='someone';"
```

`players.account_id` is `ON DELETE CASCADE`, so that removes the character too. Take a dump first (A3).

## A5 — Metrics and rate limits

| URL | What | Public? |
|-----|------|---------|
| `/health` | world + postgres + redis ping | yes |
| `/stats` | JSON: tick/loop/lag p50/p99/max, online, WS, joins, reconnects, chats, actions, dropped frames, rate-limit + auth counters | **no** — Caddy 404 |
| `/metrics` | Prometheus text of the same numbers | **no** — Caddy 404 |

### How to scrape on the live Caddy setup

Caddy on 2007.gliffy.tv proxies to `127.0.0.1:28080` and **refuses** `/stats` and `/metrics` at the edge (`deploy/caddy/Caddyfile.snippet`). Prometheus, a watch script, or a human with a shell must hit the world process on the loopback, never the public hostname:

```bash
# on the 2007.gliffy.tv box
curl -s http://127.0.0.1:28080/stats
curl -s http://127.0.0.1:28080/metrics

# local compose
curl -s http://127.0.0.1:8080/stats
npm run stats
npm run metrics
```

A public scrape that works is a bug. `curl -sf https://2007.gliffy.tv/metrics` must 404.

Numbers ops actually needs:

| `/stats` | `/metrics` | Meaning |
|----------|------------|---------|
| `tickP50Ms` / `tickP99Ms` | `hollowmere_tick_p50_ms` / `_p99_ms` | simulation step alone |
| `loopP50Ms` / `loopP99Ms` | `hollowmere_loop_p50_ms` / `_p99_ms` | tick plus state fan-out |
| `lagP50Ms` / `lagP99Ms` | `hollowmere_tick_lag_p50_ms` / `_p99_ms` | how late the tick fired — **gate on this** |
| `online` / `ws` | `hollowmere_online` / `hollowmere_ws` | CCU vs open sockets |
| `joins` / `reconnects` | `hollowmere_joins_total` / `hollowmere_reconnects_total` | first hellos vs same-process returns |
| `actions` | `hollowmere_actions_total` | interact / use / trade / drop |
| `limitedHello` etc. | `hollowmere_rate_limited_total{kind=...}` | hello, ws, chat, conn, login, auth |
| `unauthWS` | `hollowmere_unauthenticated_ws_total` | upgrades refused (dead cookie) |
| `loginFails` | `hollowmere_login_failures_total` | bad password / register |

`samples` / `hollowmere_tick_samples` is how many ticks the percentiles cover. At 0 they are meaningless (just booted).

Rate limits (defaults are generous enough for local bots; tighten on the live host):

| Knob | Default | Live suggestion |
|------|---------|-----------------|
| `HOLLOWMERE_LIMIT_HELLO_RATE` / `_BURST` | 12 / 40 per IP | 3 / 10 |
| `HOLLOWMERE_LIMIT_CONN_RATE` / `_BURST` | 20 / 80 per IP | 5 / 15 |
| `HOLLOWMERE_LIMIT_WS_RATE` / `_BURST` | 20 / 32 per client | 12 / 20 |
| `HOLLOWMERE_LIMIT_CHAT_RATE` / `_BURST` | 0.8 / 4 (~8 lines / 10s) | 0.5 / 3 |
| `HOLLOWMERE_LIMIT_LOGIN_RATE` / `_BURST` | 0.2 / 8 per IP (compose: 60 / 400) | 0.2 / 8 |
| `HOLLOWMERE_LIMIT_LOGIN_USER_RATE` / `_BURST` | 0.1 / 6 per account name | 0.1 / 6 |

Chat is also still one accepted line per 600ms tick. Auth (`hello`) and new WebSocket upgrades are limited per IP. Put the live values in the `world` service `environment:` on the 2007.gliffy.tv compose file (or an override file you do not commit if you prefer).

Refused frames increment `limitedHello` / `limitedWS` / `limitedChat` / `limitedConn` / `limitedLogin` on `/stats` and `hollowmere_rate_limited_total{kind=...}` on `/metrics`.

### Which latency number to watch

Three are reported, and they fail differently:

| Metric | Covers | Use it for |
|--------|--------|-----------|
| `tickP50Ms` / `tickP99Ms` | the authoritative step alone | is the simulation itself slow |
| `loopP50Ms` / `loopP99Ms` | tick **plus** building and queueing a state frame for every client | the real per-cycle cost; this is the one that grows with player count |
| `lagP50Ms` / `lagP99Ms` | how late each tick fired against its 600ms schedule | **is the world actually keeping up** |

Gate on **lag**. Sustained lag near or above the tick interval means the world
is falling behind regardless of what the other two say. `tick*` on its own
excludes the fan-out, which is most of the cost at player counts that matter.

`framesDropped` / `hollowmere_frames_dropped_total` counts state frames
discarded because a client was not draining its socket. A slow client losing a
frame is survivable — the next one is a full snapshot — but a climbing counter
means clients are not keeping up, which no other metric shows.

Auth and reconnect counters worth an alert:

| Counter | Means |
|---------|-------|
| `loginFails` / `hollowmere_login_failures_total` | Rejected credentials. Climbing on its own usually means people forgot passwords. |
| `limitedLogin` / `hollowmere_rate_limited_total{kind="login"}` | Guesses refused by the throttle. Climbing means someone is working through a list. |
| `unauthWS` / `hollowmere_unauthenticated_ws_total` | WebSocket upgrades refused for a missing or dead session. A steady trickle is normal (expired cookies); a spike is someone poking `/ws` directly. |
| `reconnects` / `hollowmere_reconnects_total` | Hellos that found the player already in this process. A climb without a deploy usually means tabs flapping or a proxy dropping idle sockets. |

## Known issue — WS reconnect flaps (P1)

Live tabs have been seen to drop and come back on their own. This P0 cut takes the cheap server-side bites:

- A second window no longer fights the first: the old socket is told `code=replaced` and the client stops retrying.
- The 10s heartbeat no longer evicts everyone because Redis was slow. Only a missing or banned session closes the socket.
- A failed `/auth/me` probe (world restarting) no longer dumps the player at the stile.
- Caddy's snippet now flushes WebSocket frames immediately (`flush_interval -1`).

What is still open, and belongs with P1 feel work rather than this gate:

- No 30-minute soak is CI-gated. A proxy idle timeout or a NAT in front of Caddy can still drop a quiet tab; the client will reconnect if the cookie is good.
- `reconnects` climbing while `online` is flat is the signal that the remaining flap is still happening. Capture `curl -s http://127.0.0.1:28080/stats` when it does.

## No-dupe chaos (local)

Against a local stack, not the live host:

```bash
python3 scripts/nodupe-chaos.py          # double-click, drop mid-channel, pedlar, pile, chest
go test ./internal/world/ -count=1 -run 'Dupe|Crash|Double|Disconnect|StoreFails|Aborted|Bank|Chest|Deposit|Withdraw'
```
