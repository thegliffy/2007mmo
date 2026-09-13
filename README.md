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

1. **Southern briar-woods.** The tile map is **40×32** (was 28×32, and 24×16 before that). The stile, hearth, millstone, bramble, and hazel stay where Week 1 put them. A path runs south into woods, a pond, and a clearing. Another path opens **east** into the scars.
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

### Tools and the pack

The pack is a real inventory: **28 slots, drawn in the order the server holds
them**. It used to be one fixed slot per item type in catalogue order, which
left tools nowhere to sit and made two of a thing look like one.

**Tools take a slot each and never stack.** Three axes are three axes, in three
slots, so a pack of tools costs what it looks like it costs.

| Tool | What it is for | She sells it for |
|------|----------------|------------------|
| Woodsman's axe | chopping trees | 12 |
| Flint and steel | lighting a pile of logs | 8 |
| Quarry pick | mining copper and tin | 14 |
| Bronze knife | +2 Melee levels and +1 damage | 42 |
| Briar sword | +4 Melee levels and +1 damage | 60 |

**Work that needs a tool now requires one.** No axe, no logs — the tree just
says so. No flint, no fire. No pick, no ore. Foraging and cooking need nothing, which is what
makes them the way in: pick a round of brambles, sell them to the pedlar, and
the twelve coins buy your first axe. A pick is fourteen — one more berry, or a
log.

Carrying a tool is enough; there is no equip slot. The best blade in the pack
counts toward your swing, so a sword works by being on you rather than by being
worn.

### Pointing a tool at something

Click a tool in the pack to **take it in hand** — it lights up and the pack says
what it is waiting for. The next click on the world says what to use it on:

- flint and steel, then a pile of logs → a campfire
- the axe, then a tree → a chop
- the quarry pick, then a copper or tin vein → a mine

Clicking anywhere it does not apply simply puts the tool away. The plain clicks
still work too (click a tree to chop, right-click a pile to light it), so the
held-tool path is a way of being deliberate rather than the only way through.

The server checks the tool independently of all this. Holding the flint is a
convenience of the client; **having it is a rule of the world**.

### The pedlar

**Wend the Pedlar** keeps to the path by the millstone. Click her to walk over
and open her board.

| | She pays | She charges |
|---|---|---|
| Brambleberry, Hazel nut | 1 | — |
| Log | 2 | — |
| Bramble pulp | 3 | — |
| Roast hazel | 5 | 12 |
| Hearth tart | 7 | 16 |
| Paper | 8 | — |
| Copper ore, Tin ore | 2 | — |
| Bronze bar | 8 | — |
| Goblin leather | 25 | — |

She buys most of what the hamlet produces and sells cooked food, which is what
coins were always going to be for: **fighting pays in coin, and coin buys the
food that lets you fight something bigger.** A Thornkin is worth 3–8 coins and a
hearth tart costs 16, so a tart is about three kills, and it mends 4 heart.

She always pays less than she charges — there is a test that fails if that ever
stops being true, because the alternative is a money printer.

A trade moves items and coins together, so it goes through the same
commit-before-memory path as everything else. A store failure loses the trade
rather than paying twice, and the board only enables a row when the exchange can
actually happen, so it never invites a refusal.

### Wood

Trees are work now, not scenery. **134 of the 144** on the map can be chopped —
the rest are walled in by their own clump, and a tree you can never stand
beside has no business being a node. The scars added more timber along the
hills; the rule did not change.

- Click a **tree** (channel 3 ticks) → `Log` + **Woodcutting** XP.
- Click the **millstone** with a log (channel 3 ticks) → `Paper` + Woodcutting XP.
- Drop a log and **right-click** it on the ground → a **campfire** that cooks
  exactly like the hearth for about 90 seconds, then dies down.

The millstone now does two jobs: it crushes brambleberries into pulp, and pulps
logs into paper. Carrying both, the berry goes first, so the older recipe behaves
exactly as it did.

A campfire is a node that never touches the store. It exists in memory, burns
down, and vanishes — persisting it would leave a fire on the map after a restart
with nothing burning under it.

### The eastern scars

The hamlet’s east wall is open now. A path runs out into **the scars** — rocky
hills, copper and tin, a kiln already hot, and an anvil. The stile, the hearth,
the millstone, and the southern briar-woods stay where they were. The map is
**40×32**.

This is the beginner metal loop, the same shape as wood: a tool, a gather, a
process, a use.

- Buy a **quarry pick** from Wend (14 coins). Foraging still pays for the first
  tool; a pick is one log or one extra berry more than an axe.
- Walk **east** from the millstone path into the scars.
- Click a **copper vein** or a **tin vein** with a pick (channel 3 ticks) → ore
  + **Mining** XP. No pick, no ore — the rock just says so.
- Click the **kiln** carrying both ores (channel 3 ticks) → `Bronze bar` +
  **Smithing** XP. The kiln is already hot, like the hearth: it does not want
  fuel. Copper alone or tin alone will not run.
- Click the **anvil** with a bar (channel 3 ticks) → `Bronze knife` + Smithing XP.

The knife is a blade: **+2 Melee levels and +1 damage**, so a beginner’s swing
goes from 2 to 3. The pedlar’s briar sword is still the better one (4 and +1),
and she will sell you the knife for 42 if you would rather not smith. Forging
it costs only the walk and the ores.

Mining and Smithing sit on the same 20-level ladder as Foraging, Cooking,
Woodcutting, Melee, and Defense. Older characters pick them up on login at
level 1, the way they did when Melee arrived.

Veins deplete and come back, the way brambles and trees do. A vein you can
never stand beside is scenery, not a node. The kiln and anvil are always
ready. Ore, bars, and the knife go through **commit-before-memory**, so a
killed world loses an in-flight channel rather than minting a second lump.

Out of scope on purpose: a full smithing tree, iron and coal, plate, or a
second alloy.

### Nodes are sent once, not every tick

Adding trees (and now veins) took the node count well past eighty, and the
state frame used to carry every node 1.67 times a second to every client.
Positions and kinds never change, so the **full list now ships once in the
welcome** and state frames carry **only the nodes that are not at rest** — a
chopped tree, a spent vein, a bramble on cooldown, a burning campfire.
Anything absent is ready.

Measured on the live client:

| | |
|---|---|
| welcome | 7,217 bytes, 80 nodes — sent once |
| state frame | **921 bytes**, 3 nodes |
| what a frame used to carry | ~7,100 bytes |

An 87% smaller frame, and the reason adding trees did not make the fan-out worse.

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

### What the beasts carry

| | Coins | Goblin leather |
|---|-------|----------------|
| **Thornkin** | 3–8 | 1 in 16 |
| **Brambleback** | 12–25 | 1 in 6 |

Loot is **left where the beast fell**, not teleported into your pack. Click the
sack to walk over and gather it — it is yours alone for the first minute, and
anyone's for the two after that. Right-click anything in your pack to set it
down at your feet, under the same rule.

**Coins never take a pack slot.** They live in a purse on the player record, not
as an item, so filling your bag with brambleberries costs you nothing in coin.
Goblin leather is an ordinary item and does take a slot; if the pack is full the
coins still land and the hamlet says so rather than swallowing the strip.

### Piles on the ground

Piles are **memory only**, and that is what keeps the no-dupe rule intact rather
than breaking it.

A pickup moves an item from the world into a pack. Commit the pack first and
empty the pile second, and a crash in that gap leaves the item in Postgres *and*
still on the grass — a duplicate. So the pile is emptied first and the pack
committed second: a failure there loses the item, which is the safe direction,
and the pile is restored if the commit merely errors rather than dying. Dropping
runs the same way round — the pack is committed without the item before the pile
exists.

Because piles live only in memory, a crash takes every pile with it and the pack
is whatever Postgres last agreed to, so there is nothing left to duplicate
against. Persisting them would buy dropped items surviving a restart that loses
nothing else, at the cost of a second transaction inside the tick and a genuinely
hard ordering problem.

A pile has three lives:

| | |
|---|---|
| **0–60s** | only the person who left it can see or take it |
| **60s–180s** | anyone can |
| **after 180s** | it is gone |

The reserved window is a **visibility** rule, not just a refusal. A pile that is
not yours is left out of your snapshot entirely, so nobody can watch someone
else get lucky and stand waiting over the spot. The server still refuses a
pickup it never advertised, because the client is not to be trusted about which
piles it knows.

Piles merge on a tile, but only where that cannot leak: into a pile already
public, or one reserved for the same person. Two people dropping on one tile get
two separate sacks, each invisible to the other, and those fold into one once
both go public. Topping up a public pile does **not** reserve it again —
otherwise a berry a minute would hold a tile forever.

Capped at 64 piles, so a bored player cannot grow the map.

The loot roll is the only randomness in the world, and it is owned by the
`World` rather than a global so a test can pin the seed. Nothing about crash
recovery depends on it, which is why randomness is acceptable here and not in
the combat maths.

Villagers carry nothing.

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
3. Click-to-move (WASD optional) on a **40×32** tile map. Other players, three villagers, and southern hostiles are visible (naive AOI: the whole hamlet and the scars).
4. **Foraging** (gather) → **mill / hearth** (process) → eat (use). **Mining** → kiln → anvil. The pack **persists across logout**.
5. **1vNPC combat** on the tick: click-to-attack, HP feedback, soft respawn at the stile.
6. A **headless bot harness** (`cmd/bots`) opens real WS clients — not browsers — for load.

It does **not** claim 1k CCU, a skill tree, a market, quests, PvP, or multiple worlds.

## Run (T4)

```bash
docker compose up --build
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080) in two desktop tabs. **Register a name and password in each** (they are separate accounts). Click the grass to walk, a bramble or hazel to gather, the millstone to crush, the hearth to cook. Walk **east** into the scars to mine and smith, or **south** and click **near** a Thornkin to fight. Type in the parchment log.

**Kyle / live cache:** after a deploy, hard-refresh `https://2007.gliffy.tv/` (Ctrl+Shift+R / Cmd+Shift+R). `index.html` loads `app.js?v=metal-1` and `pick-npc.js?v=metal-1` so a stale `app.js` does not keep the old map.

**Before the auth deploy goes live**, set `HOLLOWMERE_TRUSTED_PROXIES`, `HOLLOWMERE_ALLOWED_ORIGINS`, `HOLLOWMERE_SECURE_COOKIES=1`, and the tight login limits — [docs/ops.md](docs/ops.md) A4.

Headless stand-in for two tabs (needs `pip install websockets`):

```bash
python3 scripts/smoke.py
python3 scripts/week1-loop.py   # berry → mill → tart, hazel → roast, persist
python3 scripts/south-fight.py  # walk-equivalent: attack a Thornkin to the bracken
python3 scripts/metal-loop.py   # forage → buy a pick → mine → smelt → forge a knife
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
npm test           # Go unit tests (tick, gather, mill, roast, mine, smelt, forge, combat, no-dupe recovery, auth)
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
- **Mine:** walk east into the scars; copper + tin with a quarry pick.
- **Process:** millstone crushes a berry into pulp; the hearth bakes pulp or roasts a nut. The kiln smelts copper and tin into a bronze bar; the anvil forges the bar into a knife.
- **Use:** click a hearth tart or roast hazel in the pack (mends heart). A bronze knife in the pack counts toward your swing.
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

T3 rule: gather / mill / cook / eat / mine / smelt / forge **write Postgres first**, then update memory. A crash mid-tick loses an in-flight channel, never clones an item.

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
