#!/usr/bin/env python3
"""Play the beginner metal loop over WS: forage for coin, buy a pick, mine, smelt, forge."""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"
PICK_COST = 14


async def read_until(ws, typ, timeout=16):
    last = None

    async def _wait():
        nonlocal last
        while True:
            last = json.loads(await ws.recv())
            if last.get("t") == typ:
                return last

    return await asyncio.wait_for(_wait(), timeout)


def count(inv, item):
    return sum(it["n"] for it in (inv or []) if it["id"] == item)


async def join(name):
    ws, _ = await hollow_auth.join(URL, name)
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    return ws, welcome, state


async def wait_inv(ws, item, n=1, ticks=90):
    inv = []
    for _ in range(ticks):
        st = await read_until(ws, "state")
        inv = st["you"]["inv"]
        if count(inv, item) >= n and st["you"].get("action") in ("", "idle", None, "walk"):
            return inv, st
    raise AssertionError("timeout waiting for %s x%d (inv=%s)" % (item, n, inv))


async def wait_idle_beside(ws, ticks=40):
    for _ in range(ticks):
        st = await read_until(ws, "state")
        act = st["you"].get("action")
        if act in ("", "idle", None):
            return st
    raise AssertionError("never went idle")


async def sell_all(ws, item):
    sold = 0
    while True:
        st = await read_until(ws, "state")
        have = count(st["you"]["inv"], item)
        if have < 1:
            return sold, st
        await ws.send(json.dumps({"t": "sell", "id": item}))
        # The sale is a request, not a channel; the next few frames should
        # show the count drop. Keep offering until the pack is empty of it.
        for _ in range(6):
            st = await read_until(ws, "state")
            if count(st["you"]["inv"], item) < have:
                sold += have - count(st["you"]["inv"], item)
                break


async def main():
    account = hollow_auth.fresh_name("KyleMetal")
    ws, welcome, state = await join(account)
    items = welcome.get("items") or {}
    assert items.get("pick", {}).get("tool") and items["pick"].get("verb") == "mine", items.get("pick")
    assert "copper" in items and "tin" in items and "bar" in items and "knife" in items, items.keys()
    skills = welcome.get("skillInfo") or {}
    assert skills.get("mine", {}).get("name") == "Mining", skills
    assert skills.get("smith", {}).get("name") == "Smithing", skills

    by_kind = {}
    for n in welcome["nodes"]:
        by_kind.setdefault(n["kind"], []).append(n)
    for kind in ("copper", "tin", "kiln", "anvil", "bush", "hazel"):
        assert by_kind.get(kind), "missing %s in welcome nodes: %s" % (kind, sorted(by_kind))
    tiles = welcome["map"]["tiles"]
    assert len(tiles) >= 32 and len(tiles[0]) >= 40, (len(tiles), len(tiles[0]) if tiles else 0)
    print("joined", welcome["username"], "map", welcome["map"]["w"], "x", welcome["map"]["h"])

    # A pick is 14 coins. Berries and nuts sell for 1. Four bushes and two
    # hazels more than cover it, which is the same way a new arrival gets
    # an axe — foraging pays for the first tool.
    berries = 0
    for n in by_kind["bush"]:
        for _ in range(3):
            await ws.send(json.dumps({"t": "interact", "id": n["id"]}))
            inv, _ = await wait_inv(ws, "berry", berries + 1)
            berries = count(inv, "berry")
    nuts = 0
    for n in by_kind["hazel"]:
        for _ in range(2):
            await ws.send(json.dumps({"t": "interact", "id": n["id"]}))
            inv, _ = await wait_inv(ws, "nut", nuts + 1)
            nuts = count(inv, "nut")
    print("foraged", berries, "berries", nuts, "nuts")

    await ws.send(json.dumps({"t": "interact", "id": "npc-wend"}))
    await wait_idle_beside(ws)
    sold_b, _ = await sell_all(ws, "berry")
    sold_n, st = await sell_all(ws, "nut")
    coins = st["you"].get("coins") or 0
    print("sold", sold_b, "berries", sold_n, "nuts; purse", coins)
    if coins < PICK_COST:
        raise AssertionError("needed %d coins for a pick, had %d" % (PICK_COST, coins))

    await ws.send(json.dumps({"t": "buy", "id": "pick"}))
    inv, st = await wait_inv(ws, "pick")
    print("bought pick", inv, "coins", st["you"]["coins"])

    # A pick in the pack breaks no rock. Put it in your hand.
    await ws.send(json.dumps({"t": "equip", "id": "pick"}))
    for _ in range(10):
        st = await read_until(ws, "state")
        if ((st["you"].get("equipped") or {}).get("hand")) == "pick":
            break
    else:
        raise AssertionError("the pick never reached the hand: %s" % st["you"].get("equipped"))
    print("pick in hand")

    await ws.send(json.dumps({"t": "interact", "id": by_kind["copper"][0]["id"]}))
    inv, _ = await wait_inv(ws, "copper")
    print("mine copper", inv)

    await ws.send(json.dumps({"t": "interact", "id": by_kind["tin"][0]["id"]}))
    inv, _ = await wait_inv(ws, "tin")
    print("mine tin", inv)

    await ws.send(json.dumps({"t": "interact", "id": by_kind["kiln"][0]["id"]}))
    inv, _ = await wait_inv(ws, "bar")
    assert count(inv, "copper") == 0 and count(inv, "tin") == 0, inv
    print("smelt bar", inv)

    await ws.send(json.dumps({"t": "interact", "id": by_kind["anvil"][0]["id"]}))
    inv, st = await wait_inv(ws, "knife")
    assert count(inv, "bar") == 0, inv
    mine_xp = (st["you"]["skills"].get("mine") or {}).get("xp") or 0
    smith_xp = (st["you"]["skills"].get("smith") or {}).get("xp") or 0
    assert mine_xp >= 16, st["you"]["skills"]
    assert smith_xp >= 22, st["you"]["skills"]
    print("forge knife", inv, "mine", mine_xp, "smith", smith_xp)

    await ws.close()
    ws2, _, state2 = await join(account)
    inv2 = state2["you"]["inv"]
    worn2 = (state2["you"].get("equipped") or {}).get("hand")
    assert count(inv2, "knife") == 1, inv2
    # The pick is worn, not packed, and worn gear survives a relog too.
    assert worn2 == "pick", (worn2, inv2)
    print("reconnect persist", inv2, "hand", worn2)
    await ws2.close()
    print("METAL LOOP PASS")


if __name__ == "__main__":
    asyncio.run(main())
