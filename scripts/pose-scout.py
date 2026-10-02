#!/usr/bin/env python3
"""Headless playthrough of the Hollowmere loop, for visual verification.

Registers an account, mints looks, walks the hamlet doing the whole loop
(forage, mill, hearth, chest, shop, wood), then heads south and picks a
fight with a hostile so a browser screenshot catches vitals, target HP,
and a beast on screen. Prints the account name at the end.

Frames are paced: every gather is walk + channel ticks at TickMs, so each
step waits on the world's own state frames (the ground truth) rather than
sleeps. Sell/buy/deposit/withdraw are acked with an evt frame.

Economy quirks the pacing has to respect:
- The mill and hearth CONSUME their input, so "wait until the pack holds
  pulp" is satisfied by the berry you still carry while the millstone
  turns. The pack must be watched across ticks, not sampled at the
  moment the item first appears.
- A bush holds 3 berries and then rests 12 ticks. The proven gather is
  three DIFFERENT bushes (bushes[:3], one each) — hammering one bush
  drains it into its rest and the loop stalls before the mill.
- The pedlar's board only opens once you have WALKED adjacent to her;
  sell/buy before arrival fail with "nobody to trade with".
- State frames carry only nodes NOT at rest, and node views expose
  `ready`/`left` (not `remaining`/`cooldown`), so a live re-read cannot
  tell a resting bush from a stocked one. Don't try; use fixed bushes.
- Interact sets a destination and walks you there; the channel only
  starts once you are adjacent. A gather also emits a flavour `evt` on
  SUCCESS, so an evt is not proof of refusal. The only reliable proof a
  channel started is a state frame where `you.action` names it
  (walk, then forage/mill/cook). Refusals never start a channel, so the
  step times out; re-send the interact and try again.
- A node at full stock is AT REST and omitted from state frames
  (atRest = ready && remaining == max). The mill and hearth have
  Remaining == Max == 0, so they NEVER appear in state frames. Every
  node lookup after the welcome must fall back to the welcome list.
"""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"
DEBUG = len(sys.argv) > 2 and sys.argv[2] == "--debug"


async def read_until(ws, typ, timeout=20):
    while True:
        msg = json.loads(await ws.recv())
        if msg.get("t") == typ:
            return msg


async def wait_state(ws, pred, timeout=60):
    while True:
        st = await read_until(ws, "state", timeout)
        if DEBUG:
            y = st.get("you", {})
            print("  state:", repr(y.get("action")), y["inv"])
        if pred(st):
            return st


async def wait_idle(ws, timeout=30):
    # A trade/deposit/withdraw is only legal once the walk to the
    # pedlar or chest has actually finished: the server checks
    # traderNear/chestNear at command time. The walk ends when a
    # state frame shows action idle (or walk cleared).
    return await wait_state(ws, lambda s: s["you"].get("action") in (None, "idle"), timeout)

async def trade_cmd(ws, verb, item):
    """Send a sell/buy/deposit/withdraw and wait for its evt ack."""
    await ws.send(json.dumps({"t": verb, "id": item}))
    try:
        ev = await read_until(ws, "evt", 15)
        print(verb, item, "->", ev["text"])
        return True
    except asyncio.TimeoutError:
        print(verb, item, ": no ack")
        return False


async def main():
    name = hollow_auth.fresh_name("Scout")
    cookie = hollow_auth.login(URL, name)
    hollow_auth.carve_looks(URL, cookie)
    ws = await hollow_auth.connect(URL, cookie)
    await ws.send(json.dumps({"t": "hello"}))
    welcome = await read_until(ws, "welcome")
    print("joined", welcome["username"], "at", welcome["you"]["x"], welcome["you"]["y"])

    def inv_of(st):
        return [(i["id"], i.get("n", 1)) for i in st["you"]["inv"]]

    def node_of(st, kind):
        # State frames omit at-rest nodes — the mill and hearth (stock
        # 0/0) are never in them. Fall back to the welcome list.
        return next((n for n in st["nodes"] if n["kind"] == kind),
                    next((n for n in welcome["nodes"] if n["kind"] == kind), None))

    def inv_count(st, item):
        return sum(i.get("n", 1) for i in st["you"]["inv"] if i["id"] == item)

    async def do_at(node, want, label):
        nonlocal st
        # Walk + channel ticks; the state frames are ground truth. The
        # mill and hearth CONSUME their input, so the first frame that
        # shows `want` can be a stale one (the berry still in the pack
        # while the millstone turns). Keep watching until the pack holds
        # MORE `want` than it did when the step started and no longer
        # holds what feeds it. (A bare "holds want" predicate is
        # instantly true once the loop has made one of the item, which
        # silently skips gathers and starves the purse.)
        #
        # Proof the channel actually started is a state frame with
        # `you.action` set (walk, then forage/mill/cook). A refusal
        # never sets it, so the step times out — re-send the interact
        # and try again rather than letting the loop drift.
        #
        # The "no longer holds feeds" clause only applies while the
        # channel is still running. Once the step is done the pack is
        # settled: with 3 berries and 1 pulp, pulp > base but berries
        # remain from the NEXT gather, and waiting for them to vanish
        # hangs forever.
        feeds = {"pulp": "berry", "tart": "pulp", "log": None, "berry": None}[want]
        for attempt in range(3):
            base = inv_count(st, want)
            await ws.send(json.dumps({"t": "interact", "id": node["id"]}))
            started = False
            try:
                while not started:
                    msg = json.loads(await asyncio.wait_for(ws.recv(), 15))
                    if DEBUG and msg.get("t") == "state":
                        print("  pre:", repr(msg["you"].get("action")), msg["you"]["inv"])
                    if msg.get("t") == "state" and msg.get("you", {}).get("action"):
                        started = True

                def done(s):
                    if inv_count(s, want) > base:
                        return True
                    act = s["you"].get("action")
                    if feeds is not None and act in ("idle", "walk") and inv_count(s, feeds) == 0:
                        # channel ended without producing: nothing left
                        # to feed it, retry with a fresh interact
                        return True
                    return False

                st = await wait_state(ws, done, 30)
                print(label, ":", inv_of(st))
                return st
            except asyncio.TimeoutError:
                print(label, f": attempt {attempt + 1} timed out")
        return None

    # Gather a purse's worth before the shop: 3 berries -> 3 pulp -> 3
    # tarts (7c each = 21c, axe is 12c). PROVEN variant: three distinct
    # bushes from the welcome list, one berry each, so no bush is driven
    # into its 12-tick rest mid-loop.
    bushes = [n for n in welcome["nodes"] if n["kind"] == "bush"][:3]
    st = welcome
    for bush in bushes:
        st = await do_at(bush, "berry", "foraged")
        if st is None:
            break
    for _ in range(3):
        mill = node_of(st, "mill")
        if mill is None:
            break
        st = await do_at(mill, "pulp", "milled")
        if st is None:
            break
    for _ in range(3):
        fire = node_of(st, "fire")
        if fire is None:
            break
        st = await do_at(fire, "tart", "baked")
        if st is None:
            break

    # The oak chest: opening it walks you there; the bank frame arrives
    # when you get there. Stash the tarts, then take two back — the
    # deposit/withdraw path exercised both ways.
    chest = node_of(st, "chest")
    if chest:
        await ws.send(json.dumps({"t": "interact", "id": chest["id"]}))
        try:
            await read_until(ws, "bank", 30)
            st = await wait_idle(ws, 10)
            print("chest open, pack:", inv_of(st))
            for _ in range(3):
                await trade_cmd(ws, "deposit", "tart")
            st = await wait_state(ws, lambda s: True, 5)
            print("after deposit:", inv_of(st), "bank:", st["you"].get("bank"))
            for _ in range(2):
                await trade_cmd(ws, "withdraw", "tart")
            st = await wait_state(ws, lambda s: True, 5)
            print("after withdraw:", inv_of(st), "bank:", st["you"].get("bank"))
        except asyncio.TimeoutError:
            print("chest: timed out")

    # Pedlar: interact walks you adjacent, the trade frame arrives when
    # you get there. Sell the loop goods, buy an axe, wield it. Sell
    # everything sellable: 3 tarts (21c) plus a pulp (3c) clears the
    # 12c axe comfortably.
    st = await wait_state(ws, lambda s: True, 5)
    trader = next((n for n in st["npcs"] if n.get("trader")), None)
    if trader:
        await ws.send(json.dumps({"t": "interact", "id": trader["id"]}))
        try:
            await read_until(ws, "trade", 30)
            st = await wait_idle(ws, 10)
            for item in ("tart", "tart", "tart", "pulp"):
                if inv_count(st, item) == 0:
                    continue
                await trade_cmd(ws, "sell", item)
                st = await wait_state(ws, lambda s: True, 5)
            print("purse:", st["you"]["coins"])
            await trade_cmd(ws, "buy", "axe")
            st = await wait_state(ws, lambda s: any(i["id"] == "axe" for i in s["you"]["inv"]), 30)
            print("pack:", inv_of(st), "coins:", st["you"]["coins"])
            await ws.send(json.dumps({"t": "equip", "id": "axe"}))
            st = await wait_state(ws, lambda s: (s["you"].get("equipped") or {}).get("hand") == "axe", 30)
            print("axe in hand")
        except asyncio.TimeoutError:
            print("shop: timed out")

    # Chop a tree (needs the axe equipped).
    st = await wait_state(ws, lambda s: True, 5)
    tree = node_of(st, "tree")
    if tree:
        await ws.send(json.dumps({"t": "interact", "id": tree["id"]}))
        try:
            st = await wait_state(ws, lambda s: any(i["id"] == "log" for i in s["you"]["inv"]), 30)
            await asyncio.sleep(1.5)
            st = await wait_state(ws, lambda s: True, 5)
            print("chopped:", inv_of(st))
        except asyncio.TimeoutError:
            print("chop: timed out")

    # South: fight a hostile until HP bars are interesting.
    st = await wait_state(ws, lambda s: True, 5)
    hostiles = [n for n in st["npcs"] if n.get("hostile")]
    if hostiles:
        target = min(hostiles, key=lambda n: abs(n["x"] - st["you"]["x"]) + abs(n["y"] - st["you"]["y"]))
        await ws.send(json.dumps({"t": "attack", "id": target["id"]}))
        for _ in range(30):
            st = await read_until(ws, "state", 10)
        you = st["you"]
        print("fighting:", you.get("target"), "your hp:", you["hp"], "/", you["maxHp"])
        for n in st["npcs"]:
            if n["id"] == you.get("target"):
                print("target hp:", n.get("hp"), "/", n.get("maxHp"))
        print("you at", you["x"], you["y"], "skills", you["skills"])
    print("PARKED name:", name, "password:", hollow_auth.DEFAULT_PASSWORD)


if __name__ == "__main__":
    asyncio.run(main())
