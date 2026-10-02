#!/usr/bin/env python3
"""Regenerate the 128x192 paperdoll layers to match the new 64x96 doll.

Layers (skin_<tone>, tunic_<top>, hair_<style>_mask, body_mask,
tunic_mask, jerkin_mask, sword_mask) all share the 128x192 canvas with
the foot at bottom-centre (DOLL_AX=64, DOLL_AY=192 in sprites.js).
The 64x96 doll is scaled 2x and centred: doll (x,y) -> canvas (2x+0, 2y+0)
with the doll's foot row (y=95) landing on canvas row 190..191.
"""
import os

from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "client", "assets", "paperdoll")

SKINS = {"fair": (232, 200, 166), "tan": (216, 178, 140),
         "olive": (176, 140, 98), "deep": (110, 72, 44)}
TUNICS = {"moss": (74, 106, 50), "clay": (168, 90, 50), "ink": (42, 58, 90),
          "cream": (232, 212, 168), "berry": (122, 42, 64)}
HAIRS = {"cropped": 4, "short": 6, "tied": 8, "long": 12}  # extra length px


def load_doll():
    # Rebuild the doll from the fresh NPC plate: marta is the plain doll
    # with hair; instead of that, regenerate via the generator's figure().
    import importlib.util
    spec = importlib.util.spec_from_file_location(
        "gen", os.path.join(ROOT, "scripts", "gen-rs-sprites-osrs.py"))
    gen = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(gen)
    return gen.figure(hair=(0, 0, 0), tunic=(255, 255, 255), seed=7)


def to_canvas(img64):
    c = Image.new("RGBA", (128, 192), (0, 0, 0, 0))
    big = img64.resize((128, 192), Image.NEAREST)
    c.alpha_composite(big, (0, 0))
    return c


def main():
    doll = load_doll()
    dp = doll.load()

    # split the doll into regions on the 64x96 grid
    def region(fn):
        m = Image.new("RGBA", (64, 96), (0, 0, 0, 0))
        mp = m.load()
        for y in range(96):
            for x in range(64):
                r, g, b, a = dp[x, y]
                if a and fn(x, y, r, g, b):
                    mp[x, y] = (r, g, b, a)
        return to_canvas(m)

    def torso(x, y):
        return 28 <= y < 55 and 17 <= x < 47

    def head(x, y):
        return y < 28

    skin = region(lambda x, y, r, g, b: not torso(x, y) and not head(x, y)
                  or (head(x, y) and (y >= 12 or b > 200)))
    # simpler: skin = everything except hair cap (top 12 rows of head) and torso
    skin = region(lambda x, y, r, g, b: not torso(x, y) and not (head(x, y) and y < 14))
    tunic = region(lambda x, y, r, g, b: torso(x, y))
    hair = region(lambda x, y, r, g, b: head(x, y) and y < 14)

    # solid-tone variants
    def tint(img, color):
        out = Image.new("RGBA", img.size, (0, 0, 0, 0))
        op = out.load()
        ip = img.load()
        for y in range(img.height):
            for x in range(img.width):
                if ip[x, y][3]:
                    op[x, y] = color + (ip[x, y][3],)
        return out

    for name, col in SKINS.items():
        tint(skin, col).save(os.path.join(OUT, f"skin_{name}.png"))
    for name, col in TUNICS.items():
        tint(tunic, col).save(os.path.join(OUT, f"tunic_{name}.png"))

    # hair masks: white shape, longer styles extend down the sides
    for style, extra in HAIRS.items():
        hm = hair.copy()
        hp = hm.load()
        if extra:
            for y in range(28, 28 + extra * 2):
                for x in range(40, 52):
                    hp[x, y] = (255, 255, 255, 255)
        tint(hm, (255, 255, 255)).save(os.path.join(OUT, f"hair_{style}_mask.png"))

    # masks used by worn gear: body_mask = whole doll silhouette,
    # tunic_mask = torso, jerkin_mask = torso+arms, sword_mask = right arm+hand
    body = region(lambda x, y, r, g, b: True)
    tint(body, (255, 255, 255)).save(os.path.join(OUT, "body_mask.png"))
    tint(tunic, (255, 255, 255)).save(os.path.join(OUT, "tunic_mask.png"))
    jerkin = region(lambda x, y, r, g, b: 28 <= y < 55 and 17 <= x < 47)
    tint(jerkin, (255, 255, 255)).save(os.path.join(OUT, "jerkin_mask.png"))
    sword = region(lambda x, y, r, g, b: 30 <= y < 56 and 45 <= x < 56)
    tint(sword, (255, 255, 255)).save(os.path.join(OUT, "sword_mask.png"))

    print("paperdoll layers regenerated")


if __name__ == "__main__":
    main()
