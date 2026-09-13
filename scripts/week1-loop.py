#!/usr/bin/env python3
"""Play the Week 1 woodland loop over WS: berry → mill → tart, hazel → roast, persist."""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"


async def read_until(ws, typ, timeout=12):
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
    """Log in as `name` (the account is created on first run)."""
    ws, _ = await hollow_auth.join(URL, name)
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    return ws, welcome, state


async def wait_inv(ws, item, n=1, ticks=45):
    inv = []
    for _ in range(ticks):
        st = await read_until(ws, "state")
        inv = st["you"]["inv"]
        if count(inv, item) >= n and st["you"].get("action") in ("", "idle", None, "walk"):
            return inv, st
    raise AssertionError("timeout waiting for %s x%d (inv=%s)" % (item, n, inv))


async def main():
    # A fresh account each run: the loop below asserts exact pack counts,
    # and a reused account would still be holding last run's tart.
    account = hollow_auth.fresh_name("KyleW1")
    ws, welcome, state = await join(account)
    items = welcome.get("items") or {}
    assert "pulp" in items and items["tart"]["name"] == "Hearth tart", items
    # The welcome carries every node; state frames carry only the ones that
    # are not at rest, so a resting millstone would simply not be there.
    nodes = {n["kind"]: n for n in welcome["nodes"]}
    assert "bush" in nodes and "hazel" in nodes and "mill" in nodes and "fire" in nodes, nodes.keys()
    print("joined", welcome["username"], "nodes", sorted(nodes))

    await ws.send(json.dumps({"t": "interact", "id": nodes["bush"]["id"]}))
    inv, _ = await wait_inv(ws, "berry")
    print("forage berry", inv)

    await ws.send(json.dumps({"t": "interact", "id": nodes["mill"]["id"]}))
    inv, _ = await wait_inv(ws, "pulp")
    assert count(inv, "berry") == 0, inv
    print("mill pulp", inv)

    await ws.send(json.dumps({"t": "interact", "id": nodes["fire"]["id"]}))
    inv, _ = await wait_inv(ws, "tart")
    assert count(inv, "pulp") == 0, inv
    print("bake tart", inv)

    await ws.send(json.dumps({"t": "interact", "id": nodes["hazel"]["id"]}))
    inv, _ = await wait_inv(ws, "nut")
    print("forage nut", inv)

    await ws.send(json.dumps({"t": "interact", "id": nodes["fire"]["id"]}))
    inv, _ = await wait_inv(ws, "roast")
    assert count(inv, "nut") == 0, inv
    print("roast hazel", inv)

    await ws.close()
    # Same account, new socket: the pack must still be there.
    ws2, _, state2 = await join(account)
    inv2 = state2["you"]["inv"]
    assert count(inv2, "tart") == 1 and count(inv2, "roast") == 1, inv2
    print("reconnect persist", inv2)
    await ws2.close()
    print("WEEK1 LOOP PASS")


if __name__ == "__main__":
    asyncio.run(main())
