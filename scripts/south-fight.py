#!/usr/bin/env python3
"""Attack a southern Thornkin over WS until it falls. Combat v0 smoke."""
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


async def main():
    ws, _ = await hollow_auth.join(URL, "KyleFight")
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    tiles = welcome.get("map", {}).get("tiles") or []
    assert len(tiles) >= 32 and len(tiles[0]) >= 28, (len(tiles), len(tiles[0]) if tiles else 0)
    hostiles = [n for n in state.get("npcs") or [] if n.get("hostile")]
    assert len(hostiles) >= 2, hostiles
    south = [n for n in hostiles if n.get("y", 0) >= 18]
    assert south, hostiles
    thorn = next((n for n in south if n.get("name") == "Thornkin"), south[0])
    print("map", welcome["map"]["w"], "x", welcome["map"]["h"], "thornkin", thorn["id"], "at", thorn["x"], thorn["y"])

    await ws.send(json.dumps({"t": "attack", "id": thorn["id"]}))
    fell = False
    last_hp = None
    you_hp = None
    # The walk from the stile to the briar-woods is ~30 ticks on its own;
    # the budget has to cover that plus the fight.
    for _ in range(70):
        st = await read_until(ws, "state")
        you_hp = st["you"].get("hp")
        seen = [n for n in st.get("npcs") or [] if n["id"] == thorn["id"]]
        if not seen:
            fell = True
            break
        last_hp = seen[0].get("hp")
        if st["you"].get("action") == "fight" or st["you"].get("target") == thorn["id"]:
            print("tick", st["n"], "you", you_hp, "target", last_hp, "action", st["you"].get("action"))
    assert fell, "thornkin did not fall (last hp=%s you=%s)" % (last_hp, you_hp)
    assert you_hp and you_hp > 0, you_hp
    print("SOUTH FIGHT PASS thornkin fell, heart", you_hp)
    sk = st["you"].get("skills") or {}
    for name in ("melee", "defense"):
        if name in sk:
            print("  %s lv %s (%s xp)" % (name, sk[name]["lv"], sk[name]["xp"]))
    await ws.close()


if __name__ == "__main__":
    asyncio.run(main())
