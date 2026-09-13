#!/usr/bin/env python3
"""Two-client walk + chat + forage + reconnect smoke test."""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = sys.argv[1] if len(sys.argv) > 1 else "ws://127.0.0.1:8080/ws"


async def read_until(ws, typ, timeout=8):
    last = None
    async def _wait():
        nonlocal last
        while True:
            last = json.loads(await ws.recv())
            if last.get("t") == typ:
                return last
    return await asyncio.wait_for(_wait(), timeout)


async def client(name):
    """Log in as `name` and walk through the stile."""
    ws, _ = await hollow_auth.join(URL, name)
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    return ws, welcome, state


async def main():
    # Fresh accounts each run. Packs persist now, so reusing a name would
    # carry the previous run's berries into the counts asserted below.
    ash = hollow_auth.fresh_name("Ash")
    briar = hollow_auth.fresh_name("Briar")
    a_ws, a_w, a_s = await client(ash)
    b_ws, b_w, b_s = await client(briar)
    print("joined", a_w["username"], b_w["username"], "tickMs", a_w["tickMs"])

    # Peers are described by an opaque handle, never by a real player id.
    assert a_w["handle"] and a_w["handle"] != b_w["handle"], (a_w, b_w)
    for peer in a_s.get("players", []):
        assert peer["id"] != a_w["handle"], "own handle leaked into the peer list"
    print("handles ok", a_w["handle"], b_w["handle"])

    await a_ws.send(json.dumps({"t": "chat", "text": "the hearth is warm"}))
    chat = await read_until(b_ws, "chat")
    assert chat["from"] == ash and "hearth" in chat["text"], chat
    print("chat ok", chat)

    bush = next(n for n in a_s["nodes"] if n["kind"] == "bush")
    await a_ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
    berries = 0
    # Work nodes block now, so the walk to the thicket is 15-17 ticks from
    # the stile plus 2 to channel. Budget generously; this is a smoke test,
    # not a timing assertion.
    for _ in range(45):
        st = await read_until(a_ws, "state")
        berries = sum(it["n"] for it in st["you"]["inv"] if it["id"] == "berry")
        if berries >= 1:
            break
    assert berries >= 1, "forage did not yield a berry"
    print("forage ok berries", berries)

    others = [p["name"] for p in st["players"]]
    assert briar in others, (briar, others)
    print("others-from-ash", others)

    await a_ws.close()
    await b_ws.close()

    a2, w2, s2 = await client(ash)
    berries2 = sum(it["n"] for it in s2["you"]["inv"] if it["id"] == "berry")
    assert berries2 == berries, (berries, berries2)
    print("reconnect persist ok berries", berries2)
    assert w2["handle"] != a_w["handle"], "handle must rotate on rejoin"
    print("handle rotated on rejoin")
    await a2.close()
    print("SMOKE PASS")


if __name__ == "__main__":
    asyncio.run(main())
