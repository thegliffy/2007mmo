#!/usr/bin/env python3
"""Two-client walk + chat + forage + reconnect smoke test."""
import asyncio
import json
import sys
import uuid

import websockets

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


async def client(name, pid=None):
    pid = pid or str(uuid.uuid4())
    ws = await websockets.connect(URL)
    await ws.send(json.dumps({"t": "hello", "playerId": pid, "name": name}))
    welcome = await read_until(ws, "welcome")
    state = await read_until(ws, "state")
    return ws, welcome, state, pid


async def main():
    a_ws, a_w, a_s, a_id = await client("Ash")
    b_ws, b_w, b_s, b_id = await client("Briar")
    print("joined", a_w["playerId"][:8], b_w["playerId"][:8], "tickMs", a_w["tickMs"])

    await a_ws.send(json.dumps({"t": "chat", "text": "the hearth is warm"}))
    chat = await read_until(b_ws, "chat")
    assert chat["from"] == "Ash" and "hearth" in chat["text"], chat
    print("chat ok", chat)

    bush = next(n for n in a_s["nodes"] if n["kind"] == "bush")
    await a_ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
    berries = 0
    for _ in range(20):
        st = await read_until(a_ws, "state")
        berries = sum(it["n"] for it in st["you"]["inv"] if it["id"] == "berry")
        if berries >= 1:
            break
    assert berries >= 1, "forage did not yield a berry"
    print("forage ok berries", berries)

    others = [p["name"] for p in st["players"]]
    # Briar should be visible from Ash's snapshot
    print("others-from-ash", others)

    await a_ws.close()
    await b_ws.close()

    a2, w2, s2, _ = await client("Ash", a_id)
    berries2 = sum(it["n"] for it in s2["you"]["inv"] if it["id"] == "berry")
    assert berries2 == berries, (berries, berries2)
    print("reconnect persist ok berries", berries2)
    await a2.close()
    print("SMOKE PASS")


if __name__ == "__main__":
    asyncio.run(main())
