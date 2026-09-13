# Hollowmere ops

How Kyle runs the live Compose stack at **2007.gliffy.tv** without pretending we have Kubernetes.

The hamlet is **one world process + Postgres + Redis**. Postgres is canonical (accounts, packs, node remaining). Redis holds **login sessions** and presence — you do not need a Redis dump to keep tarts, but flushing Redis **logs everybody out**.

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
3. Open the site, **hard-refresh** (Ctrl+Shift+R / Cmd+Shift+R) so the browser does not keep a cached `app.js` from the last map ship, enter a name you already used, and check the pack is still there. `index.html` pins `app.js?v=click-near-2` — bump that query if static hosting still serves an old script.

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

## A4 — Accounts and sessions

Players have **real accounts**: a name, a password, and an HttpOnly session cookie.

| Endpoint | What |
|----------|------|
| `POST /auth/register` | `{"username","password"}` → 201, sets the session cookie |
| `POST /auth/login` | `{"username","password"}` → 200, sets the session cookie |
| `POST /auth/logout` | drops that session and closes its socket |
| `GET /auth/me` | `{"username"}` or 401 |
| `POST /auth/password` | `{"current","next"}` → rotates the password and signs out every other session |

Facts worth knowing before an incident:

- **Passwords** are stored only as scrypt hashes (`scrypt$N$r$p$salt$key`, N=16384, ~16 MiB per hash). The format carries its own parameters, so raising the cost later re-hashes each password on the owner's next login. There is no recovery path — no email on file means **a forgotten password cannot be reset**, only the row deleted.
- **Sessions live in Redis** (`session:<token>`, plus `acct-sessions:<accountID>` for revocation), 7 days sliding. `redis-cli FLUSHALL` signs out the whole hamlet; it loses no packs.
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

| URL | What |
|-----|------|
| `/health` | world + postgres + redis ping — the only one served publicly |
| `/stats` | JSON: tick/loop/lag p50/p99, online, WS, joins, chats, actions, dropped frames, rate-limit counters, auth counters |
| `/metrics` | Prometheus text of the same numbers |

```bash
npm run stats
curl -s http://127.0.0.1:8080/metrics
```

On the live host `/stats` and `/metrics` are **404 at the edge** — Caddy refuses
them so the `loginFails` and `limitedLogin` counters cannot tell someone
guessing passwords whether the throttle is biting. Read them on the box,
straight past Caddy:

```bash
curl -s http://127.0.0.1:28080/stats
```

The matcher lives in `deploy/caddy/Caddyfile.snippet`; `/health` stays public
because it is only a boolean.

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

Two auth counters are worth an alert:

| Counter | Means |
|---------|-------|
| `loginFails` / `hollowmere_login_failures_total` | Rejected credentials. Climbing on its own usually means people forgot passwords. |
| `limitedLogin` / `hollowmere_rate_limited_total{kind="login"}` | Guesses refused by the throttle. Climbing means someone is working through a list. |
| `unauthWS` / `hollowmere_unauthenticated_ws_total` | WebSocket upgrades refused for a missing or dead session. A steady trickle is normal (expired cookies); a spike is someone poking `/ws` directly. |
