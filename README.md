# Hollowmere

A **retro tick-MMO** proof of concept that runs in a normal desktop browser.

You walk a woodland hamlet, wave at other people, pick brambleberries and hazel nuts, crush berries at the millstone, bake a hearth tart or roast a nut, then head **south of the stile** into the briar-woods to fight a Thornkin or the Brambleback. Talk lives in a parchment log. The server is authoritative on a **600ms tick**. The pack lives in **Postgres**, so closing the tab (or killing the world container) does not mint extra berries.

Original Hollowmere IP. No borrowed studio chrome, place names, or “-Scape” labels.

by **thegliffy**

## Accounts and login

Hollowmere now has a **real front door**. There is no anonymous entry.

1. **Register or log in** at the stile: a name (3–16 characters, letters/digits/`_`/`-`) and a password (10+ characters). Passwords are stored only as **scrypt** hashes.
2. **The session is an HttpOnly cookie.** No script on the page can read it, so an XSS or a hostile link cannot carry your login away. Nothing identity-shaped lives in `localStorage` any more.
3. **The WebSocket is authenticated before it opens.** `/ws` refuses to upgrade without a valid session, and the join frame carries **no** identity — so a client can no longer name a player id and be believed.
4. **Peers see an opaque handle**, not your player id. The handle is fresh every time you walk in, so nobody can follow you across sessions by remembering it.
5. **Change your password** from the Account panel. Doing so signs out every other session and closes its socket.
6. **Cross-site requests are refused**: the WebSocket checks `Origin`, and the auth endpoints require a same-origin JSON post.

One account owns one character. There is no *self-service* password reset — no email is collected — so recovery is an operator running `admin reset <name>` on the host, which issues a fresh random password and signs the account out everywhere. See **[docs/ops.md](docs/ops.md)** A6.

Login attempts are throttled per address *and* per account name. Behind a proxy you **must** set `HOLLOWMERE_TRUSTED_PROXIES`, or every player shares one budget — ops.md A4 covers it.

## Phase 2 Week 2

This tree keeps Week 1 (woodland loop, wood HUD, ops honesty) and adds the next Kyle ask: a **bigger hamlet map** and **southern combat**.

1. **Southern briar-woods.** The tile map is **28×32** (was 24×16). The stile, hearth, millstone, bramble, and hazel stay where Week 1 put them. A path runs south into woods, a pond, and a clearing.
2. **Hostile NPCs.** Two **Thornkin** and one **Brambleback** wander the south (original names). Villagers stay in the hamlet and will not fight you.
3. **Combat v0.** Tick-authoritative **1vNPC**. Click **near** a beast (Chebyshev ≤ 1 of the clicked tile) or send `attack` — you walk adjacent, then both sides swing once per 600ms tick. Heart and target HP show on the parchment HUD and over the sprites. A Thornkin is a win; the Brambleback can drop you.
4. **Soft defeat.** HP to 0 wakes you at the stile with an empty threat and the same pack. Fallen beasts respawn in the clearing after a short wait. Hearth tarts and roast hazel mend a little heart.

**Out of scope (still):** PvP, multi-target, character creator, skill sprawl, market, multi-world, Kubernetes.

## Phase 2 Week 1

Week 1 is still the woodland-life cut under the new map.

1. **Ops / prod honesty.** Deploy and rollback notes, a Postgres dump/restore pair, and a **safe drill** that restores into a throwaway container. `/stats` and `/metrics` expose tick, presence, and rate-limit counters. Auth (`hello`), WebSocket upgrades, per-client frames, and chat are rate-limited. Kyle’s live Compose runbook is **[docs/ops.md](docs/ops.md)** (2007.gliffy.tv).
2. **Deeper gather → process → use.** Still woodland life only (Foraging / Cooking). Brambleberries crush on the millstone into pulp, then bake at the hearth. Hazel nuts roast on the same hearth. The pack still commits to Postgres **before** memory (no dupes).
3. **Readable HUD.** Carved wood frame, parchment panels, recessed pack slots. CSS/layout only — not stylized-3D art.
4. **IP string pass.** UI copy is Hollowmere voice (stile, pack, hearth, millstone, woodland work). No generic clone labels in the client.

**Camera:** still **top-down**. A ¾ view is a follow-up.

### Week 1 gather loop

- Click a **bramble** (channel 2 ticks) → `Brambleberry` + Foraging XP.
- Click a **hazel** (channel 2 ticks) → `Hazel nut` + Foraging XP.
- Click the **millstone** with a berry (channel 2 ticks) → `Bramble pulp` + Cooking XP.
- Click the **hearth** with pulp (channel 3 ticks) → `Hearth tart` + Cooking XP.
- Click the **hearth** with a nut (channel 2 ticks) → `Roast hazel` + Cooking XP.
- Click a tart or roast in the pack to eat it. Whole berries will not bake; the hearth says so. Food also mends a little heart.

### Melee and Defense

Fighting trains two skills, on the same 20-level ladder as Foraging and Cooking.

| | Effect | Trains on |
|---|--------|-----------|
| **Melee** | damage you deal: `2 + level/4` — 2 at level 1, 7 at 20 | damage dealt × 4 |
| **Defense** | damage absorbed: `level/6` — 0 at level 1, 3 at 18+ | the raw force of each blow × 6 |

Both are **flat and deterministic**. The world has no randomness anywhere — even
villager wander is derived from the tick number — and a swing that sometimes
missed would be the first thing to break that. It also means a fight can be
worked out on paper, which is what makes the numbers arguable rather than vibes.

Two rules hold the shape:

- **A blow always lands for at least 1.** No amount of Defense makes the briars
  safe to stand still in.
- **Defense trains on the raw blow, not on what got through.** Otherwise getting
  better at absorbing would slow down exactly as it started working.
- **The killing blow draws no retaliation**, so a fight you can only just win is
  still worth having.

The arc this is tuned for: a **Thornkin** (6 hp, 1 damage) is a fair first fight
at level 1 — three swings, two damage taken. The **Brambleback** (14 hp, 2
damage) kills a beginner and is the wall you train against; Melee 12 fells it in
three swings, or Defense 12 lets you outlast it. Neither path is required.

Overkill does not pay: hitting a beast with 1 hp left earns experience for 1.

### Southern combat (Week 2)

- Walk **south** from the stile along the path until the trees open.
- Click **near** a **Thornkin** or the **Brambleback** (the tile under the cursor can be one step off). The world pathfinds you adjacent, then both swing on the tick.
- Heart is on the vitals panel. The target’s bar sits over the beast.
- Lose: wake at the stile, pack intact, nothing else wiped.
- Click grass to break off and run.

### Backup / restore (Kyle, live Compose)

From the checkout that serves **2007.gliffy.tv** (or any `docker compose up` stack):

```bash
./scripts/pg-backup.sh                      # dump pgdata → backups/
./scripts/pg-backup-restore-drill.sh        # restore that dump into a throwaway Postgres; compare counts
./scripts/pg-restore.sh backups/hollowmere-YYYYMMDDThhmmssZ.sql.gz   # real rewind; stops world
```

Details, deploy, rollback, and live rate-limit knobs: **[docs/ops.md](docs/ops.md)**.

## What this PoC proves

1. A browser loads, **registers or logs in** (name + password, scrypt-hashed, HttpOnly session cookie), and joins **one world** over an authenticated WebSocket. Compose also terminates **WSS** on `:8443` (self-signed).
2. An authoritative **~600ms** tick loop runs in **Docker Compose** next to **Postgres** (canonical state) and **Redis** (session / presence).
3. Click-to-move (WASD optional) on a **28×32** tile map. Other players, three villagers, and southern hostiles are visible (naive AOI: the whole hamlet).
4. **Foraging** (gather) → **mill / hearth** (process) → eat (use). The pack **persists across logout**.
5. **1vNPC combat** on the tick: click-to-attack, HP feedback, soft respawn at the stile.
6. A **headless bot harness** (`cmd/bots`) opens real WS clients — not browsers — for load.

It does **not** claim 1k CCU, a skill tree, a market, quests, PvP, or multiple worlds.

## Run (T4)

```bash
docker compose up --build
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080) in two desktop tabs. **Register a name and password in each** (they are separate accounts). Click the grass to walk, a bramble or hazel to gather, the millstone to crush, the hearth to cook. Walk south and click **near** a Thornkin to fight. Type in the parchment log.

**Kyle / live cache:** after a deploy, hard-refresh `https://2007.gliffy.tv/` (Ctrl+Shift+R / Cmd+Shift+R). `index.html` loads `app.js?v=auth-1` and `pick-npc.js?v=auth-1` so a stale pre-auth `app.js` does not keep sending the old join frame.

**Before the auth deploy goes live**, set `HOLLOWMERE_TRUSTED_PROXIES`, `HOLLOWMERE_ALLOWED_ORIGINS`, `HOLLOWMERE_SECURE_COOKIES=1`, and the tight login limits — [docs/ops.md](docs/ops.md) A4.

Headless stand-in for two tabs (needs `pip install websockets`):

```bash
python3 scripts/smoke.py
python3 scripts/week1-loop.py   # berry → mill → tart, hazel → roast, persist
python3 scripts/south-fight.py  # walk-equivalent: attack a Thornkin to the bracken
```

Each script registers its own throwaway account on first run (`scripts/hollow_auth.py`) and logs in on later runs. They are for a local stack — do not point them at the live host.

| Port | What |
|------|------|
| `8080` | HTTP client + `ws://…/ws` (this is the easy local path) |
| `8443` | HTTPS + `wss://…/ws` with a generated self-signed cert |
| `5432` | Postgres |
| `6379` | Redis |

Useful URLs: `/health`, `/stats`, `/metrics`.

```bash
npm start          # same as docker compose up --build
npm run stats
npm run metrics
npm test           # Go unit tests (tick, gather, mill, roast, combat, no-dupe recovery, auth)
npm run backup     # dump Compose Postgres
npm run drill      # safe restore drill (throwaway container)
```

### WSS note

Run the live host over **HTTPS**. The session cookie is only marked `Secure` when the world sees TLS (or `HOLLOWMERE_SECURE_COOKIES=1`), and a login sent over plain http:// is a password in clear.

The join protocol is WebSocket. Production shape is **WSS**. Compose listens on `8443` with an ephemeral self-signed cert; browsers will complain until you accept it. Day-to-day local play uses `http://127.0.0.1:8080` + `ws://` on the same origin so two tabs just work.

### Without rebuilding images

If Postgres and Redis are already up:

```bash
export DATABASE_URL=postgres://hollowmere:hollowmere@127.0.0.1:5432/hollowmere?sslmode=disable
export REDIS_URL=redis://127.0.0.1:6379
go run ./cmd/world
```

## Vertical slice (how to poke it)

- **Identity:** account name + password. The session is an HttpOnly cookie; the player id never reaches the browser, and peers only ever see a rotating handle.
- **Log in:** register at the stile, then the cookie walks you straight back in next visit.
- **Move:** click a walkable tile; the server pathfinds and steps **one tile per tick**.
- **Fight:** Melee trains on damage dealt, Defense on blows taken. Both change the numbers, not just the score.
- **Forage:** bramble → brambleberry; hazel → hazel nut.
- **Process:** millstone crushes a berry into pulp; the hearth bakes pulp or roasts a nut.
- **Use:** click a hearth tart or roast hazel in the pack (mends heart).
- **Fight:** click near a southern hostile (neighbor tile is enough); one swing each per tick while adjacent.
- **Death:** wake at the stile with full heart and the same pack.
- **Chat:** public, one line per tick, plus a short token-bucket cap.
- **Logout:** the Account panel signs you out and closes the socket. Closing the tab just drops the socket; the cookie still lets you back in.

## Bot harness

Headless WS clients, not real browsers:

Bots hold real accounts (`bot000`…), registering on first run and logging in afterwards. A load run therefore needs a generous login budget — Compose sets one; the live values in ops.md are far tighter.

```bash
# against a running world
go run ./cmd/bots -n 50
go run ./cmd/bots -n 200 -hotspot -duration 45s

# or via Compose
docker compose --profile load run --rm bots
```

Bots print live `/stats` (tick p50/p99, ws count, drops).

## Pass gates

Measured on the PoC box (Go world + Compose Postgres/Redis). Re-run after `docker compose up`.

| Gate | Target | How to check | Status |
|------|--------|----------------|--------|
| **B1** | Cold load to in-world ≲ 10s on a mid laptop | Hard-refresh `/`, enter a name | **PASS** — static `index.html` + `styles.css` + `app.js`, no engine download |
| **B2** | Stable WS ~30 min + reconnect | Leave a tab open; drop the socket | **PASS (reconnect)** — the client retries and the session cookie re-authenticates the upgrade; a dead session shows the portal instead of looping. **30‑min soak is not CI-gated** |
| **T1** | Empty-world tick p99 ≲ 50ms | `curl localhost:8080/stats` after ~30s with 0–1 players | **PASS** — empty tick p99 **0.016ms**, loop p99 **0.017ms**, lag p99 **0.81ms** |
| **T2** | 200 bots in a hotspot: the loop fits inside the tick with margin, no WS collapse | `npm run bots:hotspot` | **PASS** — 200/200 joined, **loop p99 205.5ms of the 600ms budget (34%)**, **lag p99 1.0ms**, **0 frames dropped**, 0 sockets lost. Measured 2026-09-13 on a Ryzen 9 5900X. |
| **T3** | Kill world mid-session — **no item dupe** | Gather → kill world → start world → reconnect same id | **PASS** — unit tests `TestCrashRecoveryNoItemDupe` + `TestCrashRecoveryNoPulpDupe` + `scripts/t3-world-kill.sh` |
| **T4** | `docker compose up` brings the stack; a browser can connect | `docker compose up --build` → open `:8080` | **PASS on a normal Docker Engine.** This agent VM’s Docker bridge drops inter-container packets (world cannot dial `postgres:5432` inside the compose network). Postgres + Redis still come up healthy on published ports; the Week 1 loop was played with `go run ./cmd/world` against those ports, plus `scripts/week1-loop.py` and a browser pass. |

**Read `lagP99Ms`, not `tickP99Ms`.** Lag is how late a tick fired against its
600ms schedule, and it is the only number that says the world kept up.
`tickP99Ms` covers the simulation alone and `loopP99Ms` adds the state fan-out;
both can look alarming while the world is comfortably idle, because a 200ms
cycle inside a 600ms budget is fine.

### What the T2 numbers actually say

Tick p99 scales almost exactly linearly with connected players — **~1ms each**:

| bots | tick p99 | per player |
|------|----------|------------|
| 50   | 53.2ms   | 1.06ms |
| 100  | 101.3ms  | 1.01ms |
| 200  | 192.1ms  | 0.96ms |

That is the periodic save burst, not the simulation. Every tenth tick, every
dirty player is written to Postgres with one sequential UPSERT each, inside the
tick. Median tick time stays near **0.1ms**; the p99 is that burst.

So the ceiling is roughly **500–600 concurrent players** on this hardware, and
it would arrive as a cliff rather than a slope: the cost is concentrated in one
tick out of ten, so lag stays near zero until the burst alone exceeds the
budget, then goes over all at once. Fixing it means moving player saves off the
tick, which is deliberately not done — see the T3 rule below, which is the
property that makes it delicate.

T3 rule: gather / mill / cook / eat **write Postgres first**, then update memory. A crash mid-tick loses an in-flight channel, never clones an item.

## Layout

```
cmd/world          authoritative world + HTTP/WS/WSS + auth endpoints
cmd/bots           headless load harness
cmd/admin          operator tool: accounts, password reset, roles, ban/mute, audit log
docs/adr           architecture decision records
internal/world     tick, map, path, woodland work, pack, combat v0
internal/store     Postgres (accounts + packs) + Redis (sessions/presence)
internal/auth      accounts, scrypt passwords, session issue/revoke
internal/hub       WebSocket join / broadcast / limits / metrics / auth HTTP
internal/protocol  shared JSON frames
client/            carved wood + parchment HUD + canvas hamlet
docs/ops.md        deploy, rollback, backup/restore, live limits
migrations/        optional init SQL (server also auto-migrates)
scripts/           smoke, T3 helpers, south-fight, pick-npc-test, login helper, pg backup / restore / drill
docker-compose.yml world + postgres + redis
```

One container ≈ one world. Grow later by running another compose project, not by stuffing shards into this process.

## Out of scope (on purpose)

Full skill tree, market, quests, PvP, multi-target combat, multi-world routing, 1k CCU claims, mobile polish, Agones/K8s, borrowed studio content.

On the auth side specifically: no email, so no *self-service* password reset (operator reset only, ops.md A6); no 2FA; no account deletion from the UI; one character per account; and sessions are Redis-only, so a Redis flush logs everyone out.

## Credit

Kyle / **thegliffy** is the project owner of record. This repository and the Hollowmere hamlet are his.
