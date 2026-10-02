#!/usr/bin/env python3
"""Chunky early-2000s isometric sprites for Hollowmere.

Flat fills, a one-pixel dark outline, and a short palette. Draw sizes match
the pack the client already plants (terrain 64, props 96, figures 64×96,
cottage 192, paperdoll 128×192). Depleted gatherables are siblings of the
full plates so the client can swap them when a node is not ready.
"""
from __future__ import annotations

import json
import os
from PIL import Image, ImageDraw

ROOT = os.path.join(os.path.dirname(__file__), "..", "client", "assets")
INK = (16, 12, 8, 255)

# Limited woodland palette. Two or three flat tones, no gradients.
G_TOP, G_MARK, G_LEFT, G_RIGHT = (62, 128, 42, 255), (46, 104, 32, 255), (36, 82, 24, 255), (26, 60, 18, 255)
P_TOP, P_MARK, P_LEFT, P_RIGHT = (186, 154, 90, 255), (156, 124, 68, 255), (132, 100, 54, 255), (104, 76, 40, 255)
W_TOP, W_MARK, W_LEFT, W_RIGHT = (56, 112, 184, 255), (168, 208, 232, 255), (36, 78, 148, 255), (24, 52, 112, 255)
S_TOP, S_MARK, S_LEFT, S_RIGHT = (128, 102, 66, 255), (96, 74, 48, 255), (92, 70, 44, 255), (70, 52, 32, 255)
WALL_TOP, WALL_MARK = (132, 124, 112, 255), (96, 90, 80, 255)
WALL_LEFT, WALL_RIGHT = (88, 80, 70, 255), (62, 56, 48, 255)

WOOD_D, WOOD_M, WOOD_L = (92, 52, 24, 255), (140, 84, 40, 255), (176, 122, 66, 255)
LEAF_D, LEAF_M, LEAF_L = (26, 72, 22, 255), (42, 112, 34, 255), (78, 150, 52, 255)
DEAD = (112, 96, 56, 255)
STONE_D, STONE_M, STONE_L = (62, 56, 50, 255), (118, 110, 100, 255), (176, 168, 154, 255)
COPPER, COPPER_D = (188, 96, 36, 255), (140, 64, 24, 255)
TIN, TIN_D = (206, 210, 214, 255), (150, 156, 164, 255)
BERRY = (176, 36, 36, 255)
NUT = (156, 104, 48, 255)
PLASTER, PLASTER_D = (214, 186, 132, 255), (176, 140, 88, 255)
THATCH, THATCH_L = (148, 108, 48, 255), (184, 140, 64, 255)
FIRE_C, FIRE_Y = (208, 72, 24, 255), (240, 196, 64, 255)
PERCH, PERCH_D = (214, 226, 232, 255), (120, 156, 186, 255)
RIPPLE = (186, 220, 236, 255)

SKIN = {
    "fair": (236, 206, 168, 255),
    "tan": (204, 148, 96, 255),
    "olive": (164, 116, 70, 255),
    "deep": (102, 62, 40, 255),
}
HAIR_C = {
    "umber": (72, 40, 22, 255),
    "straw": (212, 176, 88, 255),
    "soot": (36, 28, 20, 255),
    "russet": (148, 58, 28, 255),
    "snow": (232, 226, 214, 255),
}
TOP_C = {
    "moss": (58, 96, 40, 255),
    "clay": (168, 84, 44, 255),
    "ink": (36, 52, 92, 255),
    "cream": (220, 196, 140, 255),
    "berry": (122, 36, 56, 255),
}


def shade(c, mul):
    return tuple(max(0, min(255, int(c[i] * mul))) for i in range(3)) + (255,)


def new(w, h):
    return Image.new("RGBA", (w, h), (0, 0, 0, 0))


def outline_outward(im, color=INK):
    w, h = im.size
    src = im.load()
    add = []
    for y in range(h):
        for x in range(w):
            if src[x, y][3] > 16:
                continue
            for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                nx, ny = x + dx, y + dy
                if 0 <= nx < w and 0 <= ny < h and src[nx, ny][3] > 16:
                    add.append((x, y))
                    break
    for x, y in add:
        src[x, y] = color
    return im


def up(im, scale):
    return im.resize((im.width * scale, im.height * scale), Image.Resampling.NEAREST)


def save(im, rel):
    path = os.path.join(ROOT, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    im.save(path, "PNG")
    webp = os.path.splitext(path)[0] + ".webp"
    if os.path.exists(webp) or rel.startswith(("terrain/", "props/", "gather/", "npcs/", "hostiles/", "buildings/", "paperdoll/")):
        im.save(webp, "WEBP", lossless=True)
    return path


def ink_poly(d, pts):
    d.line(pts + [pts[0]], fill=INK)


def cube(top, mark, left, right, marks, drop=5, logical=32):
    im = new(logical, logical)
    d = ImageDraw.Draw(im)
    cx, cy, hw, hh = 16, 12, 13, 6
    n, e, s, w = (cx, cy - hh), (cx + hw, cy), (cx, cy + hh), (cx - hw, cy)
    sd = (s[0], s[1] + drop)
    wd = (w[0], w[1] + drop)
    ed = (e[0], e[1] + drop)
    d.polygon([w, s, sd, wd], fill=left)
    d.polygon([e, s, sd, ed], fill=right)
    d.polygon([n, e, s, w], fill=top)
    for x, y, rw, rh in marks:
        d.rectangle([x, y, x + rw - 1, y + rh - 1], fill=mark)
    ink_poly(d, [n, e, s, w])
    d.line([w, s, sd, wd, w], fill=INK)
    d.line([e, s, sd, ed, e], fill=INK)
    return outline_outward(im)


def terrain_set():
    grass = cube(G_TOP, G_MARK, G_LEFT, G_RIGHT, [(8, 10, 4, 2), (16, 12, 5, 2), (12, 15, 3, 2)])
    path = cube(P_TOP, P_MARK, P_LEFT, P_RIGHT, [(9, 11, 8, 2), (14, 15, 6, 2)])
    water = cube(W_TOP, W_MARK, W_LEFT, W_RIGHT, [(10, 11, 5, 1), (16, 14, 4, 1)])
    scar = cube(S_TOP, S_MARK, S_LEFT, S_RIGHT, [(9, 11, 3, 2), (18, 13, 4, 2), (13, 16, 3, 2)], drop=5)
    wall = cube(WALL_TOP, WALL_MARK, WALL_LEFT, WALL_RIGHT, [(8, 9, 4, 2), (15, 11, 5, 2), (11, 14, 3, 2)], drop=8)
    save(up(grass, 2), "terrain/grass.png")
    save(up(path, 2), "terrain/path.png")
    save(up(water, 2), "terrain/water.png")
    save(up(scar, 2), "terrain/scar.png")
    save(up(wall, 2), "terrain/wall.png")


def flat_tile(base, mark, blobs):
    im = new(32, 16)
    d = ImageDraw.Draw(im)
    d.rectangle([0, 0, 31, 15], fill=base)
    for x, y, w, h in blobs:
        d.rectangle([x, y, x + w - 1, y + h - 1], fill=mark)
    return up(im, 2)


def fallback_tiles():
    # Legacy 64×32 stand-ins. The iso painter prefers terrain/ cubes.
    pairs = {
        "tiles/grass.png": (G_TOP, G_MARK, [(2, 2, 4, 2), (14, 6, 5, 2), (24, 3, 3, 2)]),
        "tiles/grass-scar.png": (S_TOP, S_MARK, [(4, 3, 4, 2), (18, 8, 4, 2)]),
        "tiles/path.png": (P_TOP, P_MARK, [(6, 4, 10, 2), (16, 9, 8, 2)]),
        "tiles/path-scar.png": (S_TOP, P_MARK, [(6, 4, 8, 2), (18, 9, 6, 2)]),
        "tiles/water.png": (W_TOP, W_MARK, [(8, 4, 6, 1), (16, 9, 5, 1)]),
        "tiles/earth.png": (S_TOP, S_MARK, [(5, 5, 4, 2), (20, 8, 5, 2)]),
        "tiles/wall.png": (WALL_TOP, WALL_MARK, [(2, 2, 6, 3), (12, 6, 8, 3), (22, 2, 6, 3)]),
        "tiles/house.png": (PLASTER, THATCH, [(0, 0, 32, 5), (4, 8, 6, 6)]),
    }
    for rel, (base, mark, blobs) in pairs.items():
        im = flat_tile(base, mark, blobs)
        # These are not in the webp twin set; write PNG only.
        path = os.path.join(ROOT, rel)
        im.save(path, "PNG")


def prop(draw_fn, scale=3, logical=32):
    im = new(logical, logical)
    d = ImageDraw.Draw(im)
    draw_fn(d)
    return up(outline_outward(im), scale)


def tree(d):
    d.ellipse([3, 1, 28, 18], fill=LEAF_D)
    d.ellipse([6, 3, 22, 14], fill=LEAF_M)
    d.ellipse([8, 4, 15, 9], fill=LEAF_L)
    d.rectangle([14, 15, 18, 29], fill=WOOD_D)
    d.rectangle([15, 15, 18, 29], fill=WOOD_M)


def stump(d):
    # Chips widen the plate so the stump itself stays narrower than a canopy.
    d.rectangle([3, 26, 6, 28], fill=WOOD_M)
    d.rectangle([26, 25, 29, 27], fill=WOOD_D)
    d.rectangle([6, 29, 8, 30], fill=WOOD_L)
    d.rectangle([23, 28, 26, 30], fill=WOOD_M)
    d.rectangle([11, 20, 21, 29], fill=WOOD_D)
    d.rectangle([13, 20, 21, 29], fill=WOOD_M)
    d.ellipse([11, 17, 21, 23], fill=WOOD_L)
    d.ellipse([13, 18, 19, 22], fill=WOOD_M)
    d.rectangle([15, 19, 17, 21], fill=WOOD_D)


def bramble(d, bare=False):
    if bare:
        d.line([(5, 27), (8, 16), (3, 12)], fill=WOOD_D, width=1)
        d.line([(16, 29), (16, 12), (28, 9)], fill=WOOD_M, width=1)
        d.line([(10, 28), (20, 16), (27, 20)], fill=WOOD_D, width=1)
        d.line([(8, 20), (4, 22)], fill=WOOD_M, width=1)
        d.rectangle([3, 11, 5, 13], fill=DEAD)
        d.rectangle([26, 8, 28, 10], fill=DEAD)
        d.rectangle([15, 11, 17, 13], fill=DEAD)
        d.ellipse([12, 26, 20, 30], fill=S_LEFT)
        return
    d.ellipse([2, 8, 29, 30], fill=LEAF_D)
    d.ellipse([6, 11, 24, 26], fill=LEAF_M)
    d.ellipse([9, 13, 18, 20], fill=LEAF_L)
    for x, y in ((7, 16), (18, 14), (13, 21), (22, 22), (10, 25)):
        d.rectangle([x, y, x + 1, y + 1], fill=BERRY)


def hazel(d, bare=False):
    d.rectangle([15, 18, 18, 30], fill=WOOD_D)
    d.rectangle([16, 18, 18, 30], fill=WOOD_M)
    if bare:
        d.line([(16, 20), (6, 10)], fill=WOOD_D, width=1)
        d.line([(17, 18), (26, 8)], fill=WOOD_M, width=1)
        d.line([(16, 16), (16, 6)], fill=WOOD_D, width=1)
        d.line([(16, 14), (10, 8)], fill=WOOD_M, width=1)
        d.line([(17, 14), (24, 12)], fill=WOOD_D, width=1)
        d.rectangle([5, 9, 7, 11], fill=DEAD)
        d.rectangle([25, 7, 27, 9], fill=DEAD)
        d.rectangle([15, 5, 17, 7], fill=DEAD)
        return
    d.ellipse([5, 3, 27, 20], fill=LEAF_D)
    d.ellipse([8, 5, 22, 16], fill=LEAF_M)
    d.ellipse([10, 6, 16, 11], fill=LEAF_L)
    for x, y in ((9, 12), (18, 10), (14, 15)):
        d.rectangle([x, y, x + 1, y + 1], fill=NUT)


def ore(d, spots, color, empty=False, hollow=None):
    d.polygon([(4, 22), (9, 14), (16, 11), (24, 14), (28, 22), (24, 29), (8, 29)], fill=STONE_M)
    d.polygon([(8, 22), (12, 16), (18, 15), (16, 24), (8, 26)], fill=STONE_L)
    d.polygon([(18, 16), (25, 16), (26, 24), (18, 27)], fill=STONE_D)
    d.polygon([(10, 24), (16, 20), (14, 28)], fill=STONE_D)
    if empty:
        d.rectangle([14, 18, 19, 23], fill=hollow or STONE_D)
        d.rectangle([15, 19, 18, 22], fill=shade(hollow or STONE_D, 0.75))
        return
    for x, y in spots:
        d.rectangle([x, y, x + 1, y + 1], fill=color)


def fish_spot(d, empty=False):
    # Pool sits low so the plate's ground diamond lands on the water tile.
    d.ellipse([3, 16, 29, 30], fill=W_TOP)
    d.ellipse([6, 18, 26, 28], fill=W_LEFT)
    d.arc([7, 18, 25, 28], 200, 340, fill=RIPPLE)
    d.arc([10, 20, 22, 27], 210, 330, fill=W_MARK)
    d.rectangle([8, 22, 11, 23], fill=RIPPLE)
    d.rectangle([19, 23, 23, 24], fill=W_MARK)
    if empty:
        return
    d.polygon([(9, 24), (16, 21), (20, 23), (16, 26)], fill=PERCH)
    d.polygon([(16, 21), (23, 19), (21, 23), (23, 26), (16, 26)], fill=PERCH_D)
    d.rectangle([11, 22, 12, 23], fill=INK)
    d.rectangle([14, 15, 15, 18], fill=RIPPLE)
    d.rectangle([16, 14, 18, 15], fill=W_MARK)


def hearth(d):
    d.ellipse([6, 18, 26, 30], fill=STONE_D)
    d.ellipse([8, 19, 24, 28], fill=STONE_M)
    d.rectangle([11, 22, 14, 25], fill=WOOD_D)
    d.rectangle([16, 21, 21, 25], fill=WOOD_M)
    d.polygon([(16, 10), (11, 22), (21, 22)], fill=FIRE_C)
    d.polygon([(16, 13), (13, 21), (19, 21)], fill=FIRE_Y)


def stile(d):
    d.rectangle([6, 14, 9, 30], fill=WOOD_D)
    d.rectangle([22, 12, 25, 30], fill=WOOD_D)
    d.rectangle([7, 14, 9, 30], fill=WOOD_M)
    d.rectangle([23, 12, 25, 30], fill=WOOD_M)
    d.rectangle([6, 16, 25, 19], fill=WOOD_L)
    d.rectangle([6, 22, 25, 25], fill=WOOD_M)
    d.rectangle([6, 16, 25, 17], fill=WOOD_D)


def millstone(d):
    d.ellipse([5, 16, 27, 30], fill=STONE_D)
    d.ellipse([6, 12, 26, 26], fill=STONE_M)
    d.ellipse([8, 10, 24, 22], fill=STONE_L)
    d.ellipse([13, 13, 19, 19], fill=STONE_D)
    d.line([(16, 11), (16, 21)], fill=STONE_D)
    d.line([(9, 16), (23, 16)], fill=STONE_D)


def chest(d):
    d.rectangle([6, 16, 26, 29], fill=WOOD_D)
    d.rectangle([7, 17, 25, 28], fill=WOOD_M)
    d.rectangle([6, 14, 26, 19], fill=WOOD_L)
    d.rectangle([6, 18, 26, 19], fill=WOOD_D)
    d.rectangle([14, 20, 18, 24], fill=(212, 176, 72, 255))


def kiln(d):
    d.polygon([(8, 28), (6, 16), (12, 8), (20, 8), (26, 16), (24, 28)], fill=STONE_D)
    d.polygon([(10, 26), (9, 16), (14, 10), (20, 12), (22, 26)], fill=STONE_M)
    d.rectangle([12, 16, 20, 24], fill=(48, 28, 20, 255))
    d.rectangle([14, 18, 18, 22], fill=FIRE_C)
    d.rectangle([15, 19, 17, 21], fill=FIRE_Y)
    d.rectangle([14, 6, 18, 10], fill=STONE_L)


def anvil(d):
    d.rectangle([12, 22, 20, 30], fill=WOOD_D)
    d.rectangle([13, 22, 20, 30], fill=WOOD_M)
    d.polygon([(6, 18), (8, 14), (24, 14), (26, 18), (22, 21), (10, 21)], fill=STONE_D)
    d.polygon([(8, 15), (18, 15), (20, 18), (10, 18)], fill=STONE_L)
    d.rectangle([20, 16, 26, 19], fill=(48, 46, 50, 255))


def stall(d):
    d.rectangle([6, 16, 9, 30], fill=WOOD_D)
    d.rectangle([22, 16, 25, 30], fill=WOOD_D)
    d.polygon([(4, 16), (16, 6), (28, 16)], fill=BERRY)
    d.polygon([(8, 16), (16, 9), (24, 16)], fill=(212, 176, 72, 255))
    d.rectangle([8, 18, 23, 21], fill=WOOD_L)
    d.rectangle([10, 22, 13, 25], fill=LEAF_M)
    d.rectangle([16, 22, 20, 25], fill=COPPER)


def rocks_prop(d):
    ore(d, (), STONE_L, empty=True, hollow=STONE_D)


def house(d):
    # 48×48 logical cottage. Mass sits west/north; the SE hearth cell is
    # punched out of this plate at draw time.
    # Left (west) wall.
    d.polygon([(10, 34), (24, 42), (24, 26), (10, 18)], fill=PLASTER_D)
    # Right (east) wall, stopping short of the punched hearth corner.
    d.polygon([(24, 42), (36, 34), (36, 22), (24, 26)], fill=PLASTER)
    # Timber on the west face.
    d.polygon([(10, 24), (24, 32), (24, 29), (10, 21)], fill=WOOD_D)
    d.line([(16, 22), (16, 36)], fill=WOOD_D)
    # Door on the west face.
    d.polygon([(14, 36), (20, 39), (20, 30), (14, 27)], fill=WOOD_D)
    d.polygon([(15, 35), (19, 37), (19, 31), (15, 28)], fill=WOOD_M)
    # Window on the east face.
    d.polygon([(28, 30), (33, 27), (33, 24), (28, 27)], fill=(72, 116, 156, 255))
    # Roof.
    d.polygon([(6, 20), (24, 6), (42, 18), (24, 28)], fill=THATCH)
    d.polygon([(12, 20), (24, 10), (32, 16), (24, 24)], fill=THATCH_L)
    d.line([(24, 6), (24, 28)], fill=WOOD_D)
    # Chimney.
    d.rectangle([30, 8, 35, 16], fill=STONE_D)
    d.rectangle([31, 9, 34, 15], fill=STONE_M)


def gather_and_props():
    specs = {
        "props/tree.png": tree,
        "props/stump.png": stump,
        "props/hearth.png": hearth,
        "props/stile.png": stile,
        "props/millstone.png": millstone,
        "props/oak_chest.png": chest,
        "props/kiln.png": kiln,
        "props/anvil.png": anvil,
        "props/pedlar_stall.png": stall,
        "props/rocks.png": rocks_prop,
        "gather/bramble.png": lambda d: bramble(d, False),
        "gather/bramble_bare.png": lambda d: bramble(d, True),
        "gather/hazel.png": lambda d: hazel(d, False),
        "gather/hazel_bare.png": lambda d: hazel(d, True),
        "gather/ore_copper.png": lambda d: ore(d, ((12, 18), (20, 17), (16, 23)), COPPER),
        "gather/ore_copper_empty.png": lambda d: ore(d, (), COPPER, empty=True, hollow=COPPER_D),
        "gather/ore_tin.png": lambda d: ore(d, ((13, 17), (21, 18), (15, 23)), TIN),
        "gather/ore_tin_empty.png": lambda d: ore(d, (), TIN, empty=True, hollow=TIN_D),
        "gather/fish_spot.png": lambda d: fish_spot(d, False),
        "gather/fish_spot_empty.png": lambda d: fish_spot(d, True),
    }
    for rel, fn in specs.items():
        save(prop(fn), rel)
    save(up(outline_outward(draw_house_canvas()), 4), "buildings/house.png")
    # Legacy fallback plate. Same cottage, smaller canvas, still simple.
    small = draw_house_canvas().resize((32, 32), Image.Resampling.NEAREST)
    save(up(outline_outward(small), 3), "props/house.png")


def draw_house_canvas():
    im = new(48, 48)
    house(ImageDraw.Draw(im))
    return im


def rect(d, box, color):
    x, y, w, h = box
    d.rectangle([x, y, x + w - 1, y + h - 1], fill=color)


def layout(short=False, robe=False):
    if short:
        return {
            "head": (13, 18, 7, 7),
            "neck": (15, 24, 3, 2),
            "arm_l": (10, 25, 3, 7),
            "arm_r": (19, 25, 3, 7),
            "hand_l": (10, 31, 3, 2),
            "hand_r": (19, 31, 3, 2),
            "leg_l": (13, 34, 3, 8),
            "leg_r": (17, 34, 3, 8),
            "foot_l": (12, 42, 4, 3),
            "foot_r": (17, 42, 4, 3),
            "torso": (12, 25, 8, 8),
            "skirt": (12, 32, 8, 5),
            "eyes": ((15, 21), (18, 21)),
        }
    skirt_h = 14 if robe else 6
    return {
        "head": (12, 6, 8, 8),
        "neck": (14, 13, 4, 2),
        "arm_l": (8, 15, 4, 9),
        "arm_r": (20, 15, 4, 9),
        "hand_l": (8, 23, 4, 3),
        "hand_r": (20, 23, 4, 3),
        "leg_l": (12, 28, 4, 12),
        "leg_r": (16, 28, 4, 12),
        "foot_l": (11, 40, 5, 4),
        "foot_r": (16, 40, 5, 4),
        "torso": (11, 15, 10, 12),
        "skirt": (11, 25, 10, skirt_h),
        "eyes": ((14, 9), (17, 9)),
    }


def paint_skin(d, lay, skin):
    dark = shade(skin, 0.78)
    for key in ("head", "neck", "arm_l", "arm_r", "hand_l", "hand_r", "leg_l", "leg_r", "foot_l", "foot_r"):
        rect(d, lay[key], skin)
    # Flat shadow on the left side of the head, arm, and leg.
    x, y, w, h = lay["head"]
    rect(d, (x, y, 3, h), dark)
    x, y, w, h = lay["arm_l"]
    rect(d, (x, y, max(1, w // 2), h), dark)
    x, y, w, h = lay["leg_l"]
    rect(d, (x, y, max(1, w // 2), h), dark)
    for ex, ey in lay["eyes"]:
        d.point((ex, ey), fill=INK)


def paint_tunic(d, lay, color):
    dark = shade(color, 0.72)
    rect(d, lay["torso"], color)
    rect(d, lay["skirt"], color)
    x, y, w, h = lay["torso"]
    rect(d, (x, y, 3, h), dark)
    x, y, w, h = lay["skirt"]
    rect(d, (x, y, 3, h), dark)


def paint_hair(d, lay, style, color):
    dark = shade(color, 0.72)
    hx, hy, hw, hh = lay["head"]
    if style == "cropped":
        rect(d, (hx - 1, hy - 3, hw + 2, 4), color)
        rect(d, (hx - 1, hy - 3, 3, 4), dark)
        rect(d, (hx, hy, hw, 2), color)
    elif style == "short":
        rect(d, (hx - 1, hy - 3, hw + 2, 5), color)
        rect(d, (hx - 1, hy + 1, 3, 5), color)
        rect(d, (hx + hw - 2, hy + 1, 3, 5), color)
        rect(d, (hx - 1, hy - 3, 3, 8), dark)
    elif style == "tied":
        rect(d, (hx - 1, hy - 3, hw + 2, 5), color)
        rect(d, (hx - 1, hy + 1, 3, 4), color)
        rect(d, (hx + hw - 1, hy + 1, 3, 4), color)
        rect(d, (hx + hw, hy + 2, 4, 4), color)
        rect(d, (hx - 1, hy - 3, 3, 7), dark)
    elif style == "long":
        rect(d, (hx - 1, hy - 3, hw + 2, 5), color)
        rect(d, (hx - 2, hy + 2, 3, 18), color)
        rect(d, (hx + hw - 1, hy + 2, 3, 16), color)
        rect(d, (hx - 2, hy - 3, 3, 20), dark)
    else:
        rect(d, (hx - 1, hy - 3, hw + 2, 4), color)


def figure_image(spec, layers):
    """layers is a subset of {'skin','tunic','hair'}."""
    im = new(32, 48)
    d = ImageDraw.Draw(im)
    lay = layout(short=spec.get("short"), robe=spec.get("robe"))
    if "skin" in layers:
        paint_skin(d, lay, spec["skin"])
    if "tunic" in layers:
        paint_tunic(d, lay, spec["tunic"])
    if "hair" in layers:
        paint_hair(d, lay, spec["hair"], spec["hair_c"])
    if spec.get("staff") and "skin" in layers and "tunic" in layers:
        d.rectangle([24, 8, 25, 44], fill=WOOD_D)
        d.rectangle([25, 8, 26, 44], fill=WOOD_M)
    if spec.get("pack") and "tunic" in layers:
        d.rectangle([21, 16, 26, 24], fill=WOOD_D)
        d.rectangle([22, 17, 26, 23], fill=WOOD_M)
    if spec.get("apron") and "tunic" in layers:
        d.rectangle([13, 20, 19, 30], fill=(236, 228, 210, 255))
    return outline_outward(im)


def npc_sprites():
    specs = {
        "npcs/marta.png": dict(skin=SKIN["olive"], hair="tied", hair_c=HAIR_C["russet"], tunic=TOP_C["cream"], apron=True),
        "npcs/old_fen.png": dict(skin=SKIN["deep"], hair="long", hair_c=HAIR_C["snow"], tunic=TOP_C["ink"], robe=True, staff=True),
        "npcs/pip.png": dict(skin=SKIN["fair"], hair="cropped", hair_c=HAIR_C["umber"], tunic=TOP_C["berry"], short=True),
        "npcs/wend.png": dict(skin=SKIN["tan"], hair="tied", hair_c=HAIR_C["straw"], tunic=TOP_C["clay"], pack=True),
    }
    for rel, spec in specs.items():
        save(up(figure_image(spec, {"skin", "tunic", "hair"}), 2), rel)


def hostile_sprites():
    kin = new(32, 48)
    d = ImageDraw.Draw(kin)
    d.ellipse([10, 28, 22, 42], fill=(48, 78, 32, 255))
    d.ellipse([12, 24, 20, 34], fill=(70, 104, 42, 255))
    d.rectangle([15, 18, 17, 26], fill=WOOD_D)
    d.rectangle([8, 30, 11, 38], fill=WOOD_D)
    d.rectangle([21, 28, 24, 36], fill=WOOD_D)
    d.rectangle([11, 22, 13, 28], fill=WOOD_M)
    d.point((14, 27), fill=INK)
    d.point((18, 27), fill=INK)
    d.rectangle([9, 40, 13, 44], fill=(36, 56, 24, 255))
    d.rectangle([19, 40, 23, 44], fill=(36, 56, 24, 255))
    save(up(outline_outward(kin), 2), "hostiles/thornkin.png")

    bb = new(32, 48)
    d = ImageDraw.Draw(bb)
    d.ellipse([3, 24, 26, 42], fill=(58, 40, 24, 255))
    d.ellipse([5, 26, 20, 38], fill=(92, 62, 36, 255))
    d.ellipse([18, 16, 30, 30], fill=(74, 48, 28, 255))
    d.rectangle([22, 12, 24, 18], fill=WOOD_D)
    d.rectangle([8, 14, 10, 24], fill=WOOD_D)
    d.rectangle([12, 16, 14, 26], fill=WOOD_M)
    d.rectangle([4, 38, 8, 45], fill=WOOD_D)
    d.rectangle([10, 38, 14, 45], fill=WOOD_D)
    d.rectangle([18, 38, 22, 45], fill=WOOD_D)
    d.rectangle([24, 36, 28, 44], fill=WOOD_D)
    d.point((22, 21), fill=INK)
    d.point((26, 21), fill=INK)
    d.rectangle([27, 24, 30, 26], fill=(40, 24, 16, 255))
    save(up(outline_outward(bb), 2), "hostiles/brambleback.png")


def paperdoll():
    # Shared 32×48 grid, scaled ×4 to 128×192, so every layer shares a foot.
    base = dict(skin=SKIN["tan"], hair="short", hair_c=HAIR_C["umber"], tunic=TOP_C["moss"])
    white = (255, 255, 255, 255)

    body = figure_image(dict(base, skin=white, tunic=white, hair_c=white), {"skin"})
    save(up(body, 4), "paperdoll/body_mask.png")
    tunic_mask = figure_image(dict(base, skin=white, tunic=white, hair_c=white), {"tunic"})
    save(up(tunic_mask, 4), "paperdoll/tunic_mask.png")

    for name, col in SKIN.items():
        im = figure_image(dict(base, skin=col), {"skin"})
        save(up(im, 4), f"paperdoll/skin_{name}.png")
    for name, col in TOP_C.items():
        im = figure_image(dict(base, tunic=col), {"tunic"})
        save(up(im, 4), f"paperdoll/tunic_{name}.png")
    for style in ("cropped", "short", "tied", "long"):
        im = figure_image(dict(base, hair=style, hair_c=white), {"hair"})
        save(up(im, 4), f"paperdoll/hair_{style}_mask.png")

    # Sample the creator default: tan / short / umber / moss, at world size.
    spec = dict(skin=SKIN["tan"], hair="short", hair_c=HAIR_C["umber"], tunic=TOP_C["moss"])
    preview = up(figure_image(spec, {"skin", "tunic", "hair"}), 2)
    save(preview, "paperdoll/preview_default_looks.png")


def bounds_of(path):
    im = Image.open(path).convert("RGBA")
    w, h = im.size
    px = im.load()
    minx, miny, maxx, maxy = w, h, -1, -1
    for y in range(h):
        for x in range(w):
            if px[x, y][3] > 16:
                if x < minx: minx = x
                if y < miny: miny = y
                if x > maxx: maxx = x
                if y > maxy: maxy = y
    if maxx < 0:
        return None
    return {"x": minx, "y": miny, "w": maxx - minx + 1, "h": maxy - miny + 1, "img_w": w, "img_h": h}


def refresh_manifest():
    manifest_path = os.path.join(ROOT, "manifest.json")
    with open(manifest_path) as f:
        manifest = json.load(f)
    notes = {entry["path"]: entry.get("notes", "") for entry in manifest["files"]}
    extras = {
        "gather/bramble_bare.png": "node bush spent — bare twigs",
        "gather/hazel_bare.png": "node hazel spent — bare branches",
        "gather/ore_copper_empty.png": "node copper spent — empty rock",
        "gather/ore_tin_empty.png": "node tin spent — empty rock",
        "gather/fish_spot.png": "node fish — ripples and a perch",
        "gather/fish_spot_empty.png": "node fish spent — empty ripples",
    }
    notes.update(extras)
    # Rewrite draw sizes from the files we know.
    files = []
    for dirpath, _, names in os.walk(ROOT):
        for name in sorted(names):
            if not name.endswith(".png"):
                continue
            rel = os.path.relpath(os.path.join(dirpath, name), ROOT).replace(os.sep, "/")
            if rel.startswith("tiles/"):
                continue
            im = Image.open(os.path.join(dirpath, name))
            kind = rel.split("/")[0]
            kind = {"props": "prop", "npcs": "npc", "buildings": "building"}.get(kind, kind.rstrip("s") if kind != "hostiles" else "hostile")
            if rel.startswith("paperdoll/"):
                kind = "paperdoll"
            if rel.startswith("gather/"):
                kind = "gather"
            if rel.startswith("terrain/"):
                kind = "terrain"
            if rel.startswith("hostiles/"):
                kind = "hostile"
            files.append({
                "path": rel,
                "kind": kind,
                "draw_px": [im.width, im.height],
                "notes": notes.get(rel, rel),
                "bytes": os.path.getsize(os.path.join(dirpath, name)),
            })
    files.sort(key=lambda e: e["path"])
    manifest["files"] = files
    with open(manifest_path, "w") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")


def contact_sheet():
    names = []
    for dirpath, _, files in os.walk(ROOT):
        for name in files:
            if name.endswith(".png") and "tiles" not in dirpath:
                names.append(os.path.join(dirpath, name))
    names.sort()
    cell = 104
    cols = 8
    rows = (len(names) + cols - 1) // cols
    sheet = Image.new("RGBA", (cols * cell, rows * (cell + 14)), (28, 24, 20, 255))
    draw = ImageDraw.Draw(sheet)
    for i, path in enumerate(names):
        im = Image.open(path).convert("RGBA")
        im.thumbnail((cell - 8, cell - 8), Image.Resampling.NEAREST)
        x = (i % cols) * cell + (cell - im.width) // 2
        y = (i // cols) * (cell + 14) + 4
        sheet.alpha_composite(im, (x, y))
        label = os.path.relpath(path, ROOT).replace(".png", "")[-22:]
        draw.text(((i % cols) * cell + 2, y + im.height + 1), label, fill=(240, 220, 180, 255))
    out = "/tmp/rs_contact.png"
    sheet.save(out)
    print("contact", out)


def main():
    terrain_set()
    fallback_tiles()
    gather_and_props()
    npc_sprites()
    hostile_sprites()
    paperdoll()
    refresh_manifest()
    contact_sheet()
    keys = [
        "terrain/grass.png", "terrain/path.png", "terrain/water.png", "terrain/scar.png", "terrain/wall.png",
        "props/tree.png", "props/stump.png", "props/stile.png", "props/hearth.png",
        "buildings/house.png", "npcs/marta.png", "paperdoll/skin_tan.png",
        "gather/bramble.png", "gather/bramble_bare.png", "gather/ore_copper.png", "gather/ore_copper_empty.png",
        "gather/fish_spot.png", "gather/fish_spot_empty.png",
    ]
    for rel in keys:
        b = bounds_of(os.path.join(ROOT, rel))
        print(f"{rel:40} {b}")


if __name__ == "__main__":
    main()
