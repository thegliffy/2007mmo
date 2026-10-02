#!/usr/bin/env python3
"""OSRS-style re-cut of the sprite generator (2026-10-02, rev 2).

User-locked rules for this pass:
- Ground is FLAT: no extruded cube sides, no black tile outlines. Tiles are
  seamless 2:1 diamonds that butt edge-to-edge (the client draws them at
  2x = 128x64, so plates are authored at 128x64).
- All shading is checkerboard DITHER / irregular fleck clusters between
  muted tones; no gradients, no feathered shadows.
- Palette pulled toward 2007 OSRS: muddy olive greens, desaturated
  steel-teal water, warm stone.
- Props/characters: hard dark outline, 2-3 tone shading, FLAT hard-edged
  ground shadow ellipse, proper OSRS silhouettes (anvil horn, ore rock
  boulders, chest with lid+lock, goblin ears+nose, spiked beast).
- Characters: 3-head-tall, arms at sides, faces, feet.

Run:  python3 scripts/gen-rs-sprites-osrs.py
Writes straight into client/assets/ (same layout as gen-rs-sprites.py).
"""
import hashlib
import math
import os
import random

from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "client", "assets")

# ---------------------------------------------------------------- palette --
GRASS = dict(base=(86, 116, 60), shade=(70, 98, 48), light=(102, 132, 74),
             mark=(58, 82, 40))
GRASS_SCAR = dict(base=(96, 108, 62), shade=(78, 90, 50), light=(112, 124, 76),
                  mark=(64, 74, 44))
PATH = dict(base=(146, 128, 96), shade=(122, 106, 78), light=(162, 144, 110),
            mark=(104, 90, 66))
EARTH = dict(base=(112, 92, 66), shade=(92, 74, 52), light=(128, 106, 78),
             mark=(76, 60, 42))
WATER = dict(base=(64, 104, 128), shade=(50, 86, 110), light=(88, 128, 150),
             mark=(110, 150, 170))
WALL = dict(base=(128, 122, 110), shade=(104, 98, 88), light=(146, 140, 126),
            mark=(86, 80, 72))
PLASTER = dict(base=(196, 176, 140), shade=(168, 150, 118), light=(212, 194, 158))
THATCH = dict(base=(122, 96, 58), shade=(100, 78, 46), light=(140, 112, 70))
TIMBER = dict(base=(104, 78, 50), shade=(84, 62, 38), light=(122, 94, 62))
STONE = dict(base=(120, 116, 108), shade=(96, 92, 86), light=(140, 136, 126))
LEAF = dict(base=(62, 96, 48), shade=(46, 76, 36), light=(84, 118, 64))
BARK = dict(base=(84, 62, 40), shade=(64, 46, 28), light=(102, 78, 54))
SKIN = dict(base=(216, 178, 140), shade=(186, 148, 112), light=(232, 200, 166))
HAIR_UMBER = (61, 36, 18)
OUTLINE = (26, 18, 12)          # soft dark brown, NOT pure black
SHADOW = (36, 30, 22)

CELL = 128                     # terrain plate width (drawn 1:1 at TW)
CELL_H = 64


def dither(px, py, a, b):
    return a if (px + py) % 2 == 0 else b


def in_diamond(x, y, w, h):
    cx, cy = w / 2.0, h / 2.0
    return abs(x - cx) / (w / 2.0) + abs(y - cy) / (h / 2.0) <= 1.0


def hash_int(*parts):
    h = hashlib.sha256(("|".join(str(p) for p in parts)).encode()).digest()
    return int.from_bytes(h[:8], "big")


# ------------------------------------------------------------- terrain -----
def terrain_plate(fam, seed, w=CELL, h=CELL_H, water=False):
    """Flat seamless 2:1 diamond: base fill, irregular fleck CLUSTERS
    (2-4px) of shade/light, water gets horizontal ripple lines."""
    img = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    px = img.load()
    rng = random.Random(hash_int(seed, w, h))
    # pre-scatter fleck clusters so texture is irregular, not noise
    flecks = []
    for _ in range(10):
        fx = rng.randrange(8, w - 8)
        fy = rng.randrange(6, h - 6)
        if not in_diamond(fx, fy, w, h):
            continue
        tone = rng.choice([fam["shade"], fam["light"], fam["mark"]])
        for _ in range(rng.randrange(2, 5)):
            ox, oy = fx + rng.randrange(-2, 3), fy + rng.randrange(-1, 2)
            flecks.append((ox, oy, tone))
    for y in range(h):
        for x in range(w):
            if not in_diamond(x, y, w, h):
                continue
            px[x, y] = fam["base"] + (255,)
    for (fx, fy, tone) in flecks:
        if 0 <= fx < w and 0 <= fy < h and in_diamond(fx, fy, w, h):
            px[fx, fy] = tone + (255,)
    if water:
        # horizontal ripple lines, 2 tones, deterministic wobble
        for row in range(8, h - 8, 10):
            for x in range(6, w - 6):
                y = row + int(2 * math.sin(x / 9.0 + hash_int(seed, row) % 7))
                if 0 < y < h and in_diamond(x, y, w, h):
                    px[x, y] = WATER["light"] + (255,)
                    if x % 3 == 0 and y + 1 < h:
                        px[x, y + 1] = WATER["shade"] + (255,)
    return img


def wall_plate(fam, seed):
    """Wall tile: the client stretches this over a raised box, so it must
    read as a vertical stone face: running-bond bricks + mortar."""
    img = Image.new("RGBA", (CELL, CELL), (0, 0, 0, 0))
    px = img.load()
    for y in range(CELL):
        for x in range(CELL):
            if not in_diamond(x, y, CELL, CELL):
                continue
            row = y // 8
            off = 10 if row % 2 else 0
            if (y % 8) == 0 or ((x + off) % 16) == 0:
                px[x, y] = fam["mark"] + (255,)
            else:
                px[x, y] = dither(x, y, fam["base"], fam["light"]) + (255,)
    return img


# ------------------------------------------------------------ sprite kit ---
def shadow_ellipse(img, cx, cy, rx, ry, alpha=110):
    """FLAT hard-edged shadow — no feathering."""
    px = img.load()
    for y in range(int(cy - ry), int(cy + ry) + 1):
        for x in range(int(cx - rx), int(cx + rx) + 1):
            dx, dy = (x - cx) / rx, (y - cy) / ry
            if dx * dx + dy * dy <= 1.0 and 0 <= x < img.width and 0 <= y < img.height:
                r, g, b, a = px[x, y]
                if a == 0:
                    px[x, y] = SHADOW + (alpha,)


def outline_pass(img, color=OUTLINE):
    """1px hard dark outline on the silhouette (OSRS separation)."""
    px = img.load()
    w, h = img.width, img.height
    edge = []
    for y in range(h):
        for x in range(w):
            if px[x, y][3]:
                continue
            for (ox, oy) in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                nx, ny = x + ox, y + oy
                if 0 <= nx < w and 0 <= ny < h and px[nx, ny][3]:
                    edge.append((x, y))
                    break
    for (x, y) in edge:
        px[x, y] = color + (255,)


def simple_prop(draw_fn, w=96, h=96, shadow=(48, None), outlined=True):
    img = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    # outline FIRST, then stamp the flat shadow: the outline pass skips
    # shadow pixels, but it must not run over an already-painted shadow
    # or it draws a dark ring around it.
    if outlined:
        outline_pass(img)
    if shadow and shadow[1] is not None:
        shadow_ellipse(img, shadow[0], shadow[1], int(w * 0.28), 6)
    draw_fn(img)
    return img


def iso_box(img, x0, y0, x1, y1, top_rise, fam):
    """Simple 3-face iso box: top face (light), front-left (base),
    front-right (shade). y0 is the top-front corner."""
    px = img.load()
    w = x1 - x0
    # front face
    for y in range(y0, y1):
        for x in range(x0, x1):
            px[x, y] = dither(x, y, fam["base"], fam["shade"]) + (255,)
    # top face (parallelogram rising to the back)
    for t in range(top_rise):
        for x in range(x0, x1):
            y = y0 - 1 - t
            if y >= 0:
                px[x, y] = dither(x, y, fam["light"], fam["base"]) + (255,)
    # right side hint
    for y in range(y0, y1):
        for x in range(x1, min(x1 + top_rise // 2, img.width)):
            px[x, y] = fam["shade"] + (255,)


def draw_chest(img):
    fam = dict(base=(150, 112, 66), shade=(118, 86, 48), light=(172, 134, 84),
               mark=(90, 64, 36))
    iso_box(img, 28, 56, 68, 84, 10, fam)
    px = img.load()
    for y in range(56, 84):                    # vertical planks
        for x in (38, 50, 62):
            px[x, y] = fam["mark"] + (255,)
    for x in range(28, 68):                    # lid seam
        px[x, 62] = fam["mark"] + (255,)
    for y in range(64, 72):                    # lock plate
        for x in range(44, 52):
            px[x, y] = (86, 82, 76, 255)
    px[47, 67] = (40, 38, 34, 255)


def draw_millstone(img):
    px = img.load()
    cx, cy, rx, ry = 48, 62, 24, 14
    # wheel: ellipse with thickness (side rim below)
    for y in range(cy - ry, cy + ry + 8):
        for x in range(cx - rx, cx + rx):
            dx, dy = (x - cx) / rx, (y - cy) / ry
            if dx * dx + dy * dy <= 1.0:
                px[x, y] = dither(x, y, STONE["light"], STONE["base"]) + (255,)
            elif cy + ry - 2 < y <= cy + ry + 6 and abs(dx) < 0.96:
                px[x, y] = STONE["shade"] + (255,)
    # central hole
    for y in range(cy - 3, cy + 3):
        for x in range(cx - 4, cx + 4):
            if ((x - cx) / 4) ** 2 + ((y - cy) / 3) ** 2 <= 1.0:
                px[x, y] = (48, 44, 40, 255)
    # radial grooves
    for a in range(0, 360, 60):
        gx, gy = math.cos(math.radians(a)), math.sin(math.radians(a)) * 0.58
        for t in range(7, 22, 3):
            x, y = int(cx + gx * t), int(cy + gy * t)
            if 0 <= x < img.width and 0 <= y < img.height:
                px[x, y] = STONE["shade"] + (255,)


def draw_ore(img, tint):
    """Irregular boulder with embedded ore chunks."""
    px = img.load()
    rng = random.Random(hash_int(tint))
    pts = []
    for a in range(0, 360, 30):
        r = rng.randrange(20, 30)
        pts.append((48 + r * math.cos(math.radians(a)) * 1.1,
                    68 + r * math.sin(math.radians(a)) * 0.8))
    for y in range(38, 90):
        for x in range(16, 80):
            inside = True
            n = len(pts)
            sign = 0
            for i in range(n):
                x1, y1 = pts[i]
                x2, y2 = pts[(i + 1) % n]
                d = (x2 - x1) * (y - y1) - (y2 - y1) * (x - x1)
                s = 1 if d > 0 else -1
                if sign == 0:
                    sign = s
                elif s != sign:
                    inside = False
                    break
            if inside:
                px[x, y] = dither(x, y, STONE["base"], STONE["shade"]) + (255,)
    # ore chunks (2-3 blobs of 3-5px)
    for _ in range(3):
        ox, oy = rng.randrange(30, 66), rng.randrange(52, 78)
        if px[ox, oy][3] == 0:
            continue
        for _ in range(rng.randrange(3, 6)):
            x, y = ox + rng.randrange(-2, 3), oy + rng.randrange(-2, 3)
            if 0 <= x < img.width and 0 <= y < img.height and px[x, y][3]:
                px[x, y] = tint + (255,)


def draw_bush(img, berries=False):
    px = img.load()
    for (lx, ly, lrw, lrh) in [(36, 66, 17, 13), (60, 66, 17, 13), (48, 54, 17, 13)]:
        for y in range(ly - lrh, ly + lrh):
            for x in range(lx - lrw, lx + lrw):
                dx, dy = (x - lx) / lrw, (y - ly) / lrh
                if dx * dx + dy * dy <= 1.0:
                    if dx * dx + dy * dy > 0.7:
                        px[x, y] = LEAF["shade"] + (255,)
                    else:
                        px[x, y] = dither(x, y, LEAF["base"], LEAF["light"]) \
                            if (x + y) % 3 else LEAF["base"]
    if berries:
        for (bx, by) in [(38, 58), (56, 62), (48, 48), (62, 70), (34, 70)]:
            px[bx, by] = (150, 40, 60, 255)
            px[bx + 1, by] = (120, 30, 48, 255)


def draw_hazel(img):
    px = img.load()
    # taller shrub with nut clusters
    for (lx, ly, lrw, lrh) in [(34, 60, 16, 13), (62, 60, 16, 13), (48, 46, 18, 14)]:
        for y in range(ly - lrh, ly + lrh):
            for x in range(lx - lrw, lx + lrw):
                dx, dy = (x - lx) / lrw, (y - ly) / lrh
                if dx * dx + dy * dy <= 1.0:
                    if dx * dx + dy * dy > 0.7:
                        px[x, y] = LEAF["shade"] + (255,)
                    else:
                        px[x, y] = dither(x, y, LEAF["base"], LEAF["light"]) \
                            if (x + y) % 3 else LEAF["base"]
    for (nx, ny) in [(40, 52), (56, 56), (48, 40), (62, 66), (32, 66)]:
        px[nx, ny] = (150, 120, 70, 255)
        px[nx + 1, ny] = (120, 94, 52, 255)


def draw_bare_branches(img):
    px = img.load()
    def stalk(x0, y0, x1, y1):
        steps = max(abs(x1 - x0), abs(y1 - y0))
        for i in range(steps + 1):
            x = int(x0 + (x1 - x0) * i / steps)
            y = int(y0 + (y1 - y0) * i / steps)
            if 0 <= x < img.width and 0 <= y < img.height:
                px[x, y] = BARK["shade"] + (255,)
    stalk(48, 88, 48, 52)
    stalk(48, 64, 30, 44); stalk(48, 60, 66, 42)
    stalk(48, 52, 38, 34); stalk(48, 52, 60, 32)


def draw_fire(img):
    px = img.load()
    # logs crossed
    for (x0, y0, x1, y1) in [(28, 84, 66, 76), (30, 76, 68, 84)]:
        steps = max(abs(x1 - x0), abs(y1 - y0))
        for i in range(steps + 1):
            x = int(x0 + (x1 - x0) * i / steps)
            y = int(y0 + (y1 - y0) * i / steps)
            for dy in range(3):
                px[x, y + dy] = dither(x, y, BARK["base"], BARK["shade"]) + (255,)
    # irregular flames: dark orange outer, orange mid, yellow core
    def flame(cx, base, ht, col):
        for i in range(ht):
            y = base - i
            hw = max(1, int((ht - i) * 0.45))
            wob = int(2 * math.sin(i / 2.0 + cx))
            for x in range(cx - hw + wob, cx + hw + wob):
                if 0 <= x < img.width and 0 <= y < img.height and px[x, y][3] == 0:
                    px[x, y] = col + (255,)
    flame(40, 74, 22, (190, 90, 40))
    flame(56, 74, 26, (190, 90, 40))
    flame(48, 74, 30, (222, 140, 50))
    flame(48, 72, 18, (236, 200, 96))


def draw_anvil(img):
    px = img.load()
    def rect(x0, y0, x1, y1, col):
        for y in range(y0, y1):
            for x in range(x0, x1):
                px[x, y] = col + (255,)
    # base block
    rect(30, 78, 66, 88, dither(0, 0, STONE["shade"], STONE["base"]))
    for y in range(78, 88):
        for x in range(30, 66):
            px[x, y] = dither(x, y, (84, 80, 74), (64, 60, 56)) + (255,)
    # waisted body
    for y in range(62, 78):
        hw = 8 - (y - 62) // 6
        for x in range(48 - hw - 2, 48 + hw + 2):
            px[x, y] = dither(x, y, (96, 92, 86), (70, 66, 62)) + (255,)
    # top face (light) + side
    for y in range(52, 62):
        for x in range(28, 62):
            px[x, y] = dither(x, y, (130, 126, 118), (96, 92, 86)) + (255,)
    # horn: tapered to the right
    for i in range(14):
        y0 = 54 + i // 4
        for x in range(62, 76 - i):
            px[x, y0] = (96, 92, 86, 255)
            px[x, y0 + 1] = (84, 80, 74, 255)


def draw_stump(img):
    px = img.load()
    for y in range(62, 90):
        for x in range(34, 62):
            px[x, y] = dither(x, y, BARK["base"], BARK["shade"]) + (255,)
    for y in range(54, 64):          # cut top (light)
        for x in range(34, 62):
            px[x, y] = dither(x, y, (170, 140, 100), (150, 120, 84)) + (255,)
    for x in range(38, 60, 6):       # bark notches
        px[x, 54] = BARK["shade"] + (255,)


def draw_stile(img):
    fam = dict(base=(122, 94, 62), shade=(96, 72, 46), light=(140, 110, 74),
               mark=(76, 56, 36))
    iso_box(img, 32, 52, 64, 90, 8, fam)
    px = img.load()
    for y in range(52, 90, 9):
        for x in range(32, 64):
            px[x, y] = fam["mark"] + (255,)


def draw_kiln(img):
    px = img.load()
    # domed stone kiln with a fire mouth
    for y in range(36, 90):
        half = int(24 * math.sqrt(max(0.0, 1 - ((y - 90) / 54.0) ** 2))) + 4
        for x in range(48 - half, 48 + half):
            if y < 46:
                px[x, y] = dither(x, y, STONE["light"], STONE["base"]) + (255,)
            else:
                px[x, y] = dither(x, y, STONE["base"], STONE["shade"]) + (255,)
    for y in range(68, 84):          # mouth
        for x in range(40, 56):
            px[x, y] = (40, 26, 18, 255)
    for y in range(68, 74):
        for x in range(42, 54):
            px[x, y] = (190, 90, 40, 255)


def draw_scar(img):
    px = img.load()
    rng = random.Random(hash_int("scar"))
    for _ in range(6):
        cx, cy = rng.randrange(20, 76), rng.randrange(40, 84)
        r = rng.randrange(6, 14)
        for y in range(cy - r, cy + r):
            for x in range(cx - r, cx + r):
                if (x - cx) ** 2 + (y - cy) ** 2 <= r * r and 0 <= x < 96 and 0 <= y < 96:
                    if px[x, y][3] == 0:
                        px[x, y] = dither(x, y, (70, 58, 44), (56, 46, 34)) + (255,)


def tree_plate(seed, variant="oak"):
    w, h = 96, 128
    img = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    shadow_ellipse(img, 48, 120, 20, 6)
    px = img.load()
    rng = random.Random(hash_int(seed, variant))
    # trunk with value ramp (light left, dark right) + root flare
    for y in range(70, 120):
        tw = 4 + (y - 70) // 22
        for x in range(48 - tw, 48 + tw):
            if x < 48:
                px[x, y] = dither(x, y, BARK["light"], BARK["base"]) + (255,)
            else:
                px[x, y] = dither(x, y, BARK["base"], BARK["shade"]) + (255,)
    for x in range(38, 58):          # roots
        px[x, 116 + (x % 3)] = BARK["shade"] + (255,)
    # canopy: blocky lobes, dark underside band, flat light top
    lobes = [(48, 34, 25, 19), (29, 48, 19, 15), (67, 48, 19, 15), (48, 56, 23, 13)]
    for (lx, ly, lrw, lrh) in lobes:
        for y in range(int(ly - lrh), int(ly + lrh) + 1):
            for x in range(int(lx - lrw), int(lx + lrw) + 1):
                d = ((x - lx) / lrw) ** 2 + ((y - ly) / lrh) ** 2
                if d <= 1.0:
                    if y > ly + lrh * 0.45:
                        px[x, y] = LEAF["shade"] + (255,)
                    elif y < ly - lrh * 0.3:
                        px[x, y] = dither(x, y, LEAF["light"], LEAF["base"]) + (255,)
                    else:
                        px[x, y] = LEAF["base"] + (255,)
    # blocky bite notches so the silhouette is lumpy, not vector-round
    for _ in range(10):
        a = rng.randrange(0, 360)
        x = int(48 + 27 * math.cos(math.radians(a)))
        y = int(42 + 24 * math.sin(math.radians(a)))
        for yy in range(y - 2, y + 3):
            for xx in range(x - 2, x + 3):
                if 0 <= xx < w and 0 <= yy < h:
                    px[xx, yy] = (0, 0, 0, 0)
    # a few highlight pixels on top
    for _ in range(8):
        x, y = rng.randrange(28, 68), rng.randrange(16, 34)
        if px[x, y][3]:
            px[x, y] = LEAF["light"] + (255,)
    outline_pass(img)
    return img


# ------------------------------------------------------- characters --------
def figure(w=64, h=96, skin=SKIN, hair=HAIR_UMBER, tunic=(74, 106, 50),
           legs=(96, 82, 60), face=True, seed=0, body="human", tint=None,
           club=False, spikes=False):
    """3-head-tall figure, arms at sides, face, feet, hard outline,
    flat ground shadow. body: human | goblin | beast."""
    img = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    px = img.load()
    shadow_ellipse(img, w // 2, h - 5, 15, 5, alpha=110)
    cx = w // 2
    body_col = tint or tunic
    body_sh = tuple(max(0, c - 30) for c in body_col)
    body_lt = tuple(min(255, c + 18) for c in body_col)

    def put(x, y, col):
        if 0 <= x < w and 0 <= y < h:
            px[x, y] = col + (255,)

    def rect(x0, y0, x1, y1, col, shade=None):
        for y in range(y0, y1):
            for x in range(x0, x1):
                c = shade if (shade and x >= (x0 + x1) // 2) else col
                put(x, y, c)

    if body == "human":
        hx0, hy0, hx1, hy1 = cx - 9, 8, cx + 9, 26
        rect(hx0, hy0, hx1, hy1, skin["base"], skin["shade"])
        rect(hx0 - 1, hy0 - 2, hx1 + 1, hy0 + 6, hair)
        for y in range(hy0 + 2, hy1 - 2):
            put(hx0 - 1, y, hair); put(hx1, y, hair)
        if face:
            put(cx - 4, hy0 + 9, OUTLINE); put(cx + 3, hy0 + 9, OUTLINE)
            put(cx - 1, hy0 + 14, skin["shade"]); put(cx, hy0 + 14, skin["shade"])
        rect(cx - 8, 28, cx + 8, 54, body_col, body_sh)
        rect(cx - 11, 30, cx - 8, 52, body_col, body_sh)      # arms
        rect(cx + 8, 30, cx + 11, 52, body_col, body_sh)
        rect(cx - 11, 52, cx - 8, 55, skin["base"])           # hands
        rect(cx + 8, 52, cx + 11, 55, skin["base"])
        rect(cx - 8, 52, cx + 8, 54, (70, 52, 34))            # belt
        rect(cx - 7, 54, cx - 1, 84, legs, tuple(max(0, c - 24) for c in legs))
        rect(cx + 1, 54, cx + 7, 84, legs, tuple(max(0, c - 24) for c in legs))
        rect(cx - 8, 84, cx - 1, 88, (64, 48, 32))            # boots
        rect(cx + 1, 84, cx + 8, 88, (64, 48, 32))
    elif body == "goblin":
        g = tint or (96, 122, 62)
        g_sh = tuple(max(0, c - 28) for c in g)
        g_lt = tuple(min(255, c + 18) for c in g)
        rect(cx - 10, 18, cx + 10, 40, g, g_sh)               # big head
        for y in range(22, 32):                               # pointy ears
            for x in range(cx - 15, cx - 10):
                put(x, y, g_lt if y - 22 < (32 - y) else g_sh)
            for x in range(cx + 10, cx + 15):
                put(x, y, g_lt if y - 22 < (32 - y) else g_sh)
        put(cx - 4, 26, OUTLINE); put(cx + 3, 26, OUTLINE)    # eyes
        for y in range(30, 34):                               # big nose
            hw = 3 - (y - 30)
            for x in range(cx - hw, cx + hw):
                put(x, y, g_sh)
        rect(cx - 3, 36, cx + 3, 37, OUTLINE)                 # mouth
        rect(cx - 8, 40, cx + 8, 60, body_col, body_sh)       # loincloth tunic
        rect(cx - 11, 42, cx - 8, 58, g, g_sh)                # arms
        rect(cx + 8, 42, cx + 11, 58, g, g_sh)
        rect(cx - 8, 58, cx + 8, 64, (110, 92, 60), (88, 72, 46))  # cloth
        rect(cx - 7, 64, cx - 2, 84, g, g_sh)                 # legs
        rect(cx + 2, 64, cx + 7, 84, g, g_sh)
        rect(cx - 8, 84, cx - 1, 88, (40, 32, 24))            # feet
        rect(cx + 1, 84, cx + 8, 88, (40, 32, 24))
        if club:
            for y in range(40, 66):                           # club in right hand
                for x in range(cx + 12, cx + 15):
                    put(x, y, BARK["shade"])
            for y in range(34, 42):
                for x in range(cx + 11, cx + 17):
                    put(x, y, BARK["base"])
    else:  # beast: low quadruped, thorn spikes along the back
        b = tint or (70, 88, 52)
        b_sh = tuple(max(0, c - 24) for c in b)
        b_lt = tuple(min(255, c + 16) for c in b)
        rect(cx - 18, 50, cx + 12, 72, b, b_sh)               # body
        rect(cx + 8, 38, cx + 22, 56, b, b_sh)                # head
        rect(cx + 20, 46, cx + 26, 52, b_lt)                  # snout
        put(cx + 14, 44, OUTLINE); put(cx + 19, 44, OUTLINE)  # eyes
        rect(cx + 9, 34, cx + 12, 38, b_sh)                   # ear
        if spikes:
            for sx in range(cx - 16, cx + 10, 5):             # bramble spurs
                put(sx, 48, (30, 26, 20)); put(sx, 47, (30, 26, 20))
                put(sx + 1, 46, (30, 26, 20))
        for lx in (-16, -9, 4, 10):                           # four legs
            rect(cx + lx, 72, cx + lx + 4, 84, b_sh)
            rect(cx + lx, 84, cx + lx + 5, 86, (40, 32, 24))  # feet
        for i in range(8):                                     # tail
            put(cx - 19 - i, 52 - i // 2, b_sh)
            put(cx - 19 - i, 53 - i // 2, b_sh)
    outline_pass(img)
    return img


# ------------------------------------------------------------- main --------
def main():
    written = []

    def save(rel, img):
        path = os.path.join(OUT, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        img.save(path)
        written.append(rel)

    # terrain: flat seamless plates at world size (128x64)
    save("terrain/grass.png", terrain_plate(GRASS, "grass"))
    save("terrain/grass-scar.png", terrain_plate(GRASS_SCAR, "grass-scar"))
    save("terrain/path.png", terrain_plate(PATH, "path"))
    save("terrain/scar.png", terrain_plate(EARTH, "scar"))
    save("terrain/water.png", terrain_plate(WATER, "water", water=True))
    save("terrain/wall.png", wall_plate(WALL, "wall"))

    # props
    save("props/tree.png", tree_plate("tree"))
    save("props/tree-scar.png", tree_plate("tree-scar", variant="scar"))
    save("props/bush.png", simple_prop(lambda i: draw_bush(i, True), shadow=(48, 86)))
    save("props/bush-scar.png", simple_prop(lambda i: draw_bare_branches(i), shadow=(48, 86)))
    save("props/hazel.png", simple_prop(lambda i: draw_bush(i, False), shadow=(48, 86)))
    save("props/hazel-scar.png", simple_prop(lambda i: draw_bare_branches(i), shadow=(48, 86)))
    save("props/millstone.png", simple_prop(draw_millstone, shadow=(48, 84)))
    save("props/oak_chest.png", simple_prop(draw_chest, shadow=(48, 86)))
    save("props/kiln.png", simple_prop(draw_kiln, shadow=(48, 90)))
    save("props/anvil.png", simple_prop(draw_anvil, shadow=(48, 88)))
    save("props/copper.png", simple_prop(lambda i: draw_ore(i, (176, 100, 60)), shadow=(48, 86)))
    save("props/copper-scar.png", simple_prop(lambda i: draw_ore(i, (120, 116, 108)), shadow=(48, 86)))
    save("props/tin.png", simple_prop(lambda i: draw_ore(i, (168, 170, 166)), shadow=(48, 86)))
    save("props/tin-scar.png", simple_prop(lambda i: draw_ore(i, (120, 116, 108)), shadow=(48, 86)))
    save("props/hearth.png", simple_prop(draw_fire, shadow=(48, 88)))
    save("props/stump.png", simple_prop(draw_stump, shadow=(48, 88)))
    save("props/stile.png", simple_prop(draw_stile, shadow=(48, 90)))
    save("props/scar.png", simple_prop(draw_scar, shadow=None))

    # gather nodes (96x96 plates the client plants on the ground diamond)
    save("gather/bramble.png", simple_prop(lambda i: draw_bush(i, True), shadow=(48, 86)))
    save("gather/bramble_bare.png", simple_prop(lambda i: draw_bare_branches(i), shadow=(48, 86)))
    save("gather/hazel.png", simple_prop(lambda i: draw_hazel(i), shadow=(48, 86)))
    save("gather/hazel_bare.png", simple_prop(lambda i: draw_bare_branches(i), shadow=(48, 86)))
    save("gather/ore_copper.png", simple_prop(lambda i: draw_ore(i, (176, 100, 60)), shadow=(48, 86)))
    save("gather/ore_copper_empty.png", simple_prop(lambda i: draw_ore(i, (120, 116, 108)), shadow=(48, 86)))
    save("gather/ore_tin.png", simple_prop(lambda i: draw_ore(i, (168, 170, 166)), shadow=(48, 86)))
    save("gather/ore_tin_empty.png", simple_prop(lambda i: draw_ore(i, (120, 116, 108)), shadow=(48, 86)))

    # buildings: iso house — two wall faces, two roof planes, door+window.
    # The plate is a 2x2 footprint: its ground diamond must be exactly
    # 128x64 source px (2:1) centred at the canvas bottom, so the client
    # plants the walls flush on the tile grid.
    house = Image.new("RGBA", (192, 192), (0, 0, 0, 0))
    hp = house.load()

    def hput(x, y, col):
        if 0 <= x < 192 and 0 <= y < 192:
            hp[x, y] = col + (255,)

    GX, GY = 96, 160          # ground-diamond centre (x, y)
    GHW, GHH = 64, 32         # half-extents of the 128x64 ground diamond
    # walls sit on the diamond's south half: base rows from GY up
    WALL_TOP = 92

    def edge_at(x):
        return GY + GHH - (abs(x - GX) / GHW) * GHH

    for x in range(GX - GHW, GX + GHW):
        for y in range(WALL_TOP, int(edge_at(x))):
            side = x < GX
            base = PLASTER["base"] if side else PLASTER["shade"]
            alt = PLASTER["shade"] if side else PLASTER["base"]
            hput(x, y, dither(x, y, base, alt))
    # timber posts at the wall corners and mid
    for x in (GX - GHW, GX - 1, GX + GHW - 1):
        for yy in range(WALL_TOP, int(edge_at(x))):
            hput(x, yy, TIMBER["shade"])
    for y in (WALL_TOP, 124, GY + GHH - 4):
        for x in range(GX - GHW, GX + GHW):
            if y < edge_at(x):
                hput(x, y, TIMBER["shade"])
    # roof: two planes meeting at a ridge above the diamond's east-west axis
    RIDGE_Y = 36
    for y in range(RIDGE_Y, WALL_TOP + 4):
        t = y - RIDGE_Y
        x0 = max(0, GX - 16 - t * 2)
        x1 = min(192, GX + 16 + t * 2)
        for x in range(x0, GX):
            hput(x, y, dither(x, y, THATCH["base"], THATCH["shade"]))
        for x in range(GX, x1):
            hput(x, y, dither(x, y, THATCH["shade"], THATCH["base"]))
        if y % 7 == 0:
            for x in range(x0, x1):
                hput(x, y, THATCH["light"])
    for y in range(RIDGE_Y, WALL_TOP + 4):
        hput(GX - 1, y, THATCH["shade"])
        hput(GX, y, THATCH["shade"])
    # door on the left face, window on the right
    for y in range(128, GY + GHH - 4):
        for x in range(GX - 40, GX - 16):
            hput(x, y, dither(x, y, BARK["base"], BARK["shade"]))
    for x in (GX - 40, GX - 17):
        for y in range(128, GY + GHH - 4):
            hput(x, y, BARK["shade"])
    hput(GX - 20, 150, (60, 50, 36))
    for y in range(112, 130):
        for x in range(GX + 16, GX + 38):
            hput(x, y, (40, 34, 26))
    for x in range(GX + 16, GX + 38, 5):
        for y in range(112, 130):
            hput(x, y, TIMBER["base"])
    outline_pass(house)
    shadow_ellipse(house, GX, GY + GHH - 2, 52, 8, alpha=110)
    save("buildings/house.png", house)

    # characters
    save("npcs/marta.png", figure(hair=(90, 40, 24), tunic=(140, 70, 60), seed=1))
    save("npcs/fen.png", figure(hair=(120, 116, 108), tunic=(80, 90, 70), seed=2))
    save("npcs/pip.png", figure(hair=(212, 180, 106), tunic=(60, 90, 130), seed=3))
    save("npcs/wend.png", figure(hair=(61, 36, 18), tunic=(120, 90, 140), seed=4))
    save("hostiles/thornkin.png", figure(body="goblin", tunic=(70, 60, 44),
                                         club=True, seed=5))
    save("hostiles/brambleback.png", figure(body="beast", spikes=True, seed=6))

    # paperdoll layers from the new doll
    doll = figure(hair=(0, 0, 0), tunic=(255, 255, 255), seed=7)
    dp = doll.load()
    skin_img = Image.new("RGBA", (64, 96), (0, 0, 0, 0))
    sp = skin_img.load()
    tunic_img = Image.new("RGBA", (64, 96), (0, 0, 0, 0))
    tp = tunic_img.load()
    hair_img = Image.new("RGBA", (64, 96), (0, 0, 0, 0))
    hgp = hair_img.load()
    for y in range(96):
        for x in range(64):
            r, g, b, a = dp[x, y]
            if a == 0:
                continue
            torso = 28 <= y < 55 and 17 <= x < 47
            if y < 28:
                hgp[x, y] = (255, 255, 255, a)
            elif torso:
                tp[x, y] = (255, 255, 255, a)
            else:
                sp[x, y] = (r, g, b, a)
    save("paperdoll/skin.png", skin_img)
    save("paperdoll/tunic.png", tunic_img)
    save("paperdoll/hair.png", hair_img)

    print(f"wrote {len(written)} sprites")
    for rel in written:
        print(" ", rel)


if __name__ == "__main__":
    main()
