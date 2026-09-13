#!/usr/bin/env python3
"""T3 helper: forage, print pack, exit. Re-run after world restart with same id."""
import asyncio
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import hollow_auth

URL = os.environ.get("WS_URL", "ws://127.0.0.1:8080/ws")
# The account name is now the identity that survives a restart: run this
# twice with the same NAME to prove the pack persisted across a crash.
NAME = os.environ.get("NAME", "KyleT3")
PASSWORD = os.environ.get("PASSWORD", hollow_auth.DEFAULT_PASSWORD)


async def main():
    ws, _ = await hollow_auth.join(URL, NAME, PASSWORD)
    you = None
    nodes = []
    for _ in range(8):
        msg = json.loads(await asyncio.wait_for(ws.recv(), 8))
        if msg.get("t") in ("welcome", "state"):
            you = msg.get("you") or you
            # Only the welcome lists every node; state frames carry just the
            # ones that are not at rest.
            if msg.get("t") == "welcome":
                nodes = msg.get("nodes") or nodes
            if you and nodes:
                break
    berries = sum(it["n"] for it in (you or {}).get("inv", []) if it["id"] == "berry")
    tarts = sum(it["n"] for it in (you or {}).get("inv", []) if it["id"] == "tart")
    print(f"account={NAME} berries={berries} tarts={tarts} action={you.get('action') if you else None}")
    if os.environ.get("FORAGE") == "1":
        bush = next((n for n in nodes if n["kind"] == "bush"), None)
        if not bush:
            print("no ready bush")
            await ws.close()
            return
        await ws.send(json.dumps({"t": "interact", "id": bush["id"]}))
        for _ in range(24):
            msg = json.loads(await asyncio.wait_for(ws.recv(), 8))
            if msg.get("t") != "state":
                continue
            you = msg["you"]
            berries = sum(it["n"] for it in you.get("inv", []) if it["id"] == "berry")
            if berries > 0 and you.get("action") in ("", "idle", None, "walk"):
                break
        print(f"after_forage berries={berries} inv={you.get('inv')}")
    await ws.close()
    print("NAME=" + NAME)


if __name__ == "__main__":
    asyncio.run(main())
