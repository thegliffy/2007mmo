#!/usr/bin/env python3
"""Buy, wear, and swap gear over WS.

The point of the hand slot is that carrying a tool is no longer enough, so
this script proves the refusal as carefully as it proves the success: it
chops with the sword in hand and the axe in the pack (nothing happens),
then swaps and chops again with the same click.

Needs the local stack. Pass a purse-filling hook as argv[2] (a shell
command taking one SQL statement) because a legitimate 200 coins is a long
grind and this is testing equipment, not the economy.
"""
import asyncio
import json
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"
PURSE = sys.argv[2] if len(sys.argv) > 2 else ""
NAME = None  # a fresh account per run; see main()


async def read_until(ws, typ, timeout=15):
    async def _wait():
        while True:
            msg = json.loads(await ws.recv())
            if msg.get("t") == typ:
                return msg

    return await asyncio.wait_for(_wait(), timeout)


async def settle(ws, ticks=3):
    """Advance a few ticks, collecting any notes the world sends."""
    notes = []
    last = None
    for _ in range(ticks):
        while True:
            msg = json.loads(await asyncio.wait_for(ws.recv(), 15))
            if msg.get("t") in ("evt", "err"):
                notes.append(msg.get("text", ""))
            if msg.get("t") == "state":
                last = msg
                break
    return last, notes


def pack(you):
    return {s["id"]: s["n"] for s in (you.get("inv") or []) if s.get("id")}


async def main():
    # A fresh account per run: the world keeps a player in memory after
    # the first join, so filling the purse of a name it has already seen
    # would write to a row nobody reads.
    name = hollow_auth.fresh_name("KyleEquip")
    cookie = hollow_auth.login(URL, name)
    hollow_auth.carve_looks(URL, cookie)
    if PURSE:
        # Registration wrote the player row; fill the purse before the
        # world loads it into memory at hello.
        subprocess.run(
            PURSE + " \"update players set coins=200 where name='%s'\"" % name,
            shell=True, check=True, stdout=subprocess.DEVNULL,
        )
    ws = await hollow_auth.connect(URL, cookie)
    await ws.send(json.dumps({"t": "hello"}))
    welcome = await read_until(ws, "welcome")
    slots = welcome.get("equipSlots") or []
    assert "hand" in slots and "body" in slots, slots
    items = welcome.get("items") or {}
    assert items["sword"].get("slot") == "hand", items["sword"]
    assert items["jerkin"].get("slot") == "body", items["jerkin"]
    assert not items["flint"].get("slot"), "flint is used from the pack, not worn"
    print("slots", slots)

    state = await read_until(ws, "state")
    pedlar = next(n for n in state["npcs"] if n.get("trader"))
    trees = [n for n in welcome["nodes"] if n["kind"] == "tree"]
    assert trees, "no trees in the hamlet"

    # 1. Buy the kit.
    # Clicking the pedlar is an "interact", the same click that attacks a
    # goblin or chops a tree; the world decides what it means.
    await ws.send(json.dumps({"t": "interact", "id": pedlar["id"]}))
    board = await read_until(ws, "trade", timeout=45)
    assert any(o["item"] == "sword" for o in board["offers"]), board
    for item in ("axe", "sword", "jerkin"):
        await ws.send(json.dumps({"t": "buy", "id": item}))
    st, notes = await settle(ws, 3)
    held = pack(st["you"])
    assert held.get("axe") and held.get("sword") and held.get("jerkin"), (held, notes)
    print("bought", held, "purse", st["you"].get("coins"))

    # 2. Wear the jerkin and take up the sword.
    await ws.send(json.dumps({"t": "equip", "id": "jerkin"}))
    await ws.send(json.dumps({"t": "equip", "id": "sword"}))
    st, notes = await settle(ws, 3)
    worn = st["you"].get("equipped") or {}
    assert worn.get("hand") == "sword" and worn.get("body") == "jerkin", (worn, notes)
    assert not pack(st["you"]).get("sword"), "the sword is worn and still in the pack"
    print("worn", worn, "pack", pack(st["you"]))

    # 3. Chop with the sword in hand. The axe is right there in the pack
    #    and that is deliberately not enough.
    tree = trees[0]
    await ws.send(json.dumps({"t": "interact", "id": tree["id"]}))
    st, notes = await settle(ws, 45)
    assert not pack(st["you"]).get("log"), "chopped with a sword in hand"
    assert any("axe in hand" in n for n in notes), notes
    print("refused:", next(n for n in notes if "axe in hand" in n))

    # 4. Swap to the axe and click the same tree.
    await ws.send(json.dumps({"t": "equip", "id": "axe"}))
    st, notes = await settle(ws, 3)
    worn = st["you"].get("equipped") or {}
    assert worn.get("hand") == "axe", (worn, notes)
    assert pack(st["you"]).get("sword") == 1, "the sword did not go back in the pack"
    await ws.send(json.dumps({"t": "interact", "id": tree["id"]}))
    logs = 0
    for _ in range(50):
        st, _ = await settle(ws, 1)
        logs = pack(st["you"]).get("log", 0)
        if logs:
            break
    assert logs, "axe in hand still would not chop"
    print("chopped", logs, "log(s) with", worn)

    # 5. Put the axe away again.
    await ws.send(json.dumps({"t": "unequip", "id": "hand"}))
    st, notes = await settle(ws, 3)
    assert not (st["you"].get("equipped") or {}).get("hand"), st["you"].get("equipped")
    assert pack(st["you"]).get("axe") == 1, pack(st["you"])
    print("EQUIP PASS", (st["you"].get("equipped") or {}), pack(st["you"]))
    await ws.close()


asyncio.run(main())
