#!/usr/bin/env python3
"""Measure opaque bounds + anchor points for every sprite in client/assets/
and print the BOUNDS entries for client/sprites.js.

Anchor convention (see destAtFeet / destAtGroundDiamond in sprites.js):
- ax: opaque mid-x at the opaque bottom row (feet anchor for figures)
- ay: opaque bottom row
- terrain plates: bounds = the full diamond (w=128,h=64) so the plate's
  top face lands exactly on the cell diamond.
"""
import os
from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "client", "assets")

KEYS = [
    "terrain/grass", "terrain/grass-scar", "terrain/path", "terrain/scar",
    "terrain/water", "terrain/wall",
    "props/tree", "props/tree-scar", "props/stump", "props/stile",
    "props/bush", "props/bush-scar", "props/hazel", "props/hazel-scar",
    "props/millstone", "props/oak_chest", "props/kiln", "props/anvil",
    "props/copper", "props/copper-scar", "props/tin", "props/tin-scar",
    "props/hearth", "props/scar",
    "buildings/house",
    "npcs/marta", "npcs/old_fen", "npcs/pip", "npcs/wend",
    "hostiles/thornkin", "hostiles/brambleback",
    "paperdoll/skin", "paperdoll/tunic", "paperdoll/hair",
]

SHADOW = (36, 30, 22)


def measure(path, skip_shadow=True):
    im = Image.open(path).convert("RGBA")
    px = im.load()
    w, h = im.size
    xs0, ys0, xs1, ys1 = w, h, -1, -1
    for y in range(h):
        for x in range(w):
            r, g, b, a = px[x, y]
            if a == 0:
                continue
            if skip_shadow and (r, g, b) == SHADOW:
                continue
            xs0 = min(xs0, x); ys0 = min(ys0, y)
            xs1 = max(xs1, x); ys1 = max(ys1, y)
    if xs1 < 0:
        return None
    # anchor: opaque mid-x at bottom row (skip shadow pixels too)
    row = [x for x in range(xs0, xs1 + 1)
           if px[x, ys1][3] and px[x, ys1][:3] != SHADOW]
    ax = (row[0] + row[-1]) // 2 if row else (xs0 + xs1) // 2
    return dict(x=xs0, y=ys0, w=xs1 - xs0 + 1, h=ys1 - ys0 + 1, ax=ax, ay=ys1)


for key in KEYS:
    rel = key + ".png"
    p = os.path.join(OUT, rel)
    if not os.path.exists(p):
        print(f'    // MISSING {key}')
        continue
    m = measure(p)
    if not m:
        print(f'    // EMPTY {key}')
        continue
    if key.startswith("terrain/"):
        # full-diamond bounds: plate top face == cell diamond
        im = Image.open(p)
        print(f'    "{key}": {{ x: 0, y: 0, w: {im.width}, h: {im.height} }},')
    else:
        print(f'    "{key}": {{ x: {m["x"]}, y: {m["y"]}, w: {m["w"]}, h: {m["h"]}, '
              f'ax: {m["ax"]}, ay: {m["ay"]} }},')
