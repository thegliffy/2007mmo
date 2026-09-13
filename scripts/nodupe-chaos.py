#!/usr/bin/env python3
"""Chaos pass against a running world: double-click, drop the socket mid-channel,
double buy/sell, double pickup. The pack must never grow a free extra item.

Local only. Needs a world on :8080 and `pip install websockets`.
Do not point this at 2007.gliffy.tv.
"""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"


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
    ws, cookie = await hollow_auth.join(URL, name)
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    return ws, cookie, welcome, state


async def wait_inv(ws, item, n=1, ticks=50):
    inv = []
    for _ in range(ticks):
        st = await read_until(ws, "state")
        inv = st["you"]["inv"]
        if count(inv, item) >= n and st["you"].get("action") in ("", "idle", None, "walk"):
            return inv, st
    raise AssertionError("timeout waiting for %s x%d (inv=%s)" % (item, n, inv))


async def wait_idle(ws, ticks=40):
    for _ in range(ticks):
        st = await read_until(ws, "state")
        if st["you"].get("action") in ("", "idle", None):
            return st
    raise AssertionError("never went idle")


async def main():
    account = hollow_auth.fresh_name("KyleChaos")
    ws, cookie, welcome, state = await join(account)
    by_kind = {}
    for n in welcome["nodes"]:
        by_kind.setdefault(n["kind"], []).append(n)
    bush = by_kind["bush"][0]
    print("joined", welcome["username"])

    # Double-click a bramble. One berry, not two.
    await ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
    await ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
    inv, st = await wait_inv(ws, "berry", 1)
    berries = count(inv, "berry")
    if berries != 1:
        raise AssertionError("double-click forage granted %d berries: %s" % (berries, inv))
    print("double-click forage", berries)

    # Drop the socket mid-channel, then come back. The in-flight berry
    # must not appear twice.
    await ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
    await asyncio.sleep(0.4)
    await ws.close()
    print("dropped mid-channel")

    ws, _ = await hollow_auth.connect(URL, cookie)
    await ws.send(json.dumps({"t": "hello"}))
    await read_until(ws, "welcome")
    st = await read_until(ws, "state")
    berries2 = count(st["you"]["inv"], "berry")
    if berries2 > 2:
        raise AssertionError("reconnect mid-forage duplicated berries: %s" % st["you"]["inv"])
    # Finish whatever channel survived (same process) or start clean.
    if berries2 < 2:
        await ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
        inv, st = await wait_inv(ws, "berry", berries2 + 1)
        berries2 = count(inv, "berry")
    if berries2 > 2:
        raise AssertionError("too many berries after reconnect: %s" % st["you"]["inv"])
    print("reconnect mid-channel pack", berries2)

    # Pedlar: sell one berry, then sell it again. Purse grows once.
    await ws.send(json.dumps({"t": "interact", "id": "npc-wend"}))
    await wait_idle(ws)
    before = st["you"].get("coins") or 0
    await ws.send(json.dumps({"t": "sell", "id": "berry"}))
    await ws.send(json.dumps({"t": "sell", "id": "berry"}))
    for _ in range(8):
        st = await read_until(ws, "state")
        if count(st["you"]["inv"], "berry") == berries2 - 1:
            break
    coins = st["you"].get("coins") or 0
    if coins != before + 1:
        raise AssertionError("double-sell coins %d → %d" % (before, coins))
    if count(st["you"]["inv"], "berry") != berries2 - 1:
        raise AssertionError("double-sell pack: %s" % st["you"]["inv"])
    print("double-sell coins", coins)

    # Drop one berry and click the pile twice.
    await ws.send(json.dumps({"t": "drop", "id": "berry"}))
    pile = None
    for _ in range(8):
        st = await read_until(ws, "state")
        ground = st.get("ground") or []
        if ground:
            pile = ground[0]
            break
    if not pile:
        raise AssertionError("drop left no pile")
    packed = count(st["you"]["inv"], "berry")
    await ws.send(json.dumps({"t": "interact", "id": pile["id"]}))
    await ws.send(json.dumps({"t": "interact", "id": pile["id"]}))
    for _ in range(12):
        st = await read_until(ws, "state")
        if not st.get("ground"):
            break
    if count(st["you"]["inv"], "berry") != packed + 1:
        raise AssertionError("double-pickup pack: %s" % st["you"]["inv"])
    print("double-pickup berries", count(st["you"]["inv"], "berry"))

    await ws.close()
    print("NO-DUPE CHAOS PASS")


if __name__ == "__main__":
    asyncio.run(main())
