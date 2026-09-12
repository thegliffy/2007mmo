# Hollowmere

A **retro ~2007 tick-MMO** proof of concept that runs in a normal desktop browser.

You walk a tiny woodland hamlet, wave at other people, pick brambleberries, cook them into tarts, and talk in a chunky beige chat box. The server is authoritative on a **600ms tick**. Inventory lives in **Postgres**, so closing the tab (or killing the world container) does not mint extra berries.

Original IP. No Jagex assets, branding, or “-Scape” names.

by **thegliffy**

## What this PoC proves

1. A browser loads, stubs auth (display name + `localStorage` id), and joins **one world** over WebSocket. Compose also terminates **WSS** on `:8443` (self-signed).
2. An authoritative **~600ms** tick loop runs in **Docker Compose** next to **Postgres** (canonical state) and **Redis** (session / presence).
3. Click-to-move (WASD optional) on a small tile map. Other players and three wandering NPCs are visible (naive AOI: the whole hamlet).
4. **Foraging** (gather) → **Cooking** (process) → eat a tart (use). The pack **persists across logout**.
5. A **headless bot harness** (`cmd/bots`) opens real WS clients — not browsers — for load.

It does **not** claim 1k CCU, a skill tree, a market, quests, PvP, or multiple worlds.

## Run (T4)

```bash
docker compose up --build
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080) in two desktop tabs. Enter names. Click the grass to walk, a bush to forage, the hearth to cook. Type in the chat box.

Headless stand-in for two tabs (needs `pip install websockets`):

```bash
python3 scripts/smoke.py
```

| Port | What |
|------|------|
| `8080` | HTTP client + `ws://…/ws` (this is the easy local path) |
| `8443` | HTTPS + `wss://…/ws` with a generated self-signed cert |
| `5432` | Postgres |
| `6379` | Redis |

Useful URLs: `/health`, `/stats`.

```bash
npm start          # same as docker compose up --build
npm run stats
npm test           # Go unit tests (tick, gather, no-dupe recovery)
```

### WSS note

The join protocol is WebSocket. Production shape is **WSS**. Compose listens on `8443` with an ephemeral self-signed cert; browsers will complain until you accept it. Day-to-day local play uses `http://127.0.0.1:8080` + `ws://` on the same origin so two tabs just work.

### Without rebuilding images

If Postgres and Redis are already up:

```bash
export DATABASE_URL=postgres://hollowmere:hollowmere@127.0.0.1:5432/hollowmere?sslmode=disable
export REDIS_URL=redis://127.0.0.1:6379
go run ./cmd/world
```

## Vertical slice (how to poke it)

- **Identity:** name + UUID in `localStorage`. No passwords, no accounts table beyond the player row.
- **Move:** click a walkable tile; the server pathfinds and steps **one tile per tick**.
- **Forage:** click a bramble bush (channel 2 ticks) → `Brambleberry` + Foraging XP.
- **Cook:** click the hearth with a berry (channel 3 ticks) → `Berry tart` + Cooking XP.
- **Use:** click a tart in the pack to eat it.
- **Chat:** public, rate-limited to one line per tick.
- **Logout:** close the tab. Re-enter with the same browser; the pack is still there.

## Bot harness

Headless WS clients, not real browsers:

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
| **B1** | Cold load to in-world ≲ 10s on a mid laptop | Hard-refresh `/`, enter a name | **PASS** — static `index.html` + `styles.css` + `app.js` ≈ 19 KB, no engine download |
| **B2** | Stable WS ~30 min + reconnect | Leave a tab open; drop the socket | **PASS (reconnect)** — client retries with stored id/session; same `playerId` gets `welcome` again. **30‑min soak is not CI-gated** |
| **T1** | Empty-world tick p99 ≲ 50ms | `curl localhost:8080/stats` after ~30s with 0–1 players | **PASS** — empty p99 **0.009ms**; after a short play session p99 **3.1ms** |
| **T2** | ~200 bots in a hotspot: tick p99 ≲ 150ms, no WS collapse | `npm run bots:hotspot` | **PASS** — 200/200 connected, **0 drops**, tick p99 **76.7ms** |
| **T3** | Kill world mid-session — **no item dupe** | Forage → kill world → start world → reconnect same id | **PASS** — 1 brambleberry stayed **1** (unit test `TestCrashRecoveryNoItemDupe` + live kill). Also `scripts/t3-world-kill.sh` |
| **T4** | `docker compose up` brings the stack; a browser can connect | `docker compose up --build` → open `:8080` | **PASS on a normal Docker Engine** — Compose builds the world image and starts Postgres + Redis + world. This agent VM’s bridge NAT was broken (`nftables`), so gates above were measured with the same world binary talking to the Compose DB/Redis on published ports. `scripts/smoke.py` is the two-tab stand-in. |

T3 rule: gather / cook / eat **write Postgres first**, then update memory. A crash mid-tick loses an in-flight channel, never clones an item.

## Layout

```
cmd/world          authoritative world + HTTP/WS/WSS
cmd/bots           headless load harness
internal/world     tick, map, path, skills, inventory
internal/store     Postgres (canonical) + Redis (session/presence)
internal/hub       WebSocket join / broadcast
internal/protocol  shared JSON frames
client/            dated beige/wood UI + canvas hamlet
migrations/        optional init SQL (server also auto-migrates)
scripts/           smoke + T3 helpers
docker-compose.yml world + postgres + redis
```

One container ≈ one world. Grow later by running another compose project, not by stuffing shards into this process.

## Out of scope (on purpose)

Full skill tree, GE/market, quests, PvP, multi-world routing, 1k CCU claims, mobile polish, Agones/K8s, Jagex content, Salvage Run.

## Credit

Kyle / **thegliffy** is the project owner of record. This repository and the Hollowmere hamlet are his.
