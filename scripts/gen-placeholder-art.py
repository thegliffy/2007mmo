#!/usr/bin/env python3
"""Write clean geometric PNGs into client/assets/ for the art drop.

Same filenames the client loads. Art Director PNGs replace these in place.
Uses only the stdlib (struct + zlib) so CI and a bare checkout can regenerate.
"""
from __future__ import annotations

import struct
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "client" / "assets"


def png(w: int, h: int, rgba: bytes) -> bytes:
    def chunk(tag: bytes, data: bytes) -> bytes:
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    raw = b"".join(b"\x00" + rgba[y * w * 4 : (y + 1) * w * 4] for y in range(h))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(raw, 9))
        + chunk(b"IEND", b"")
    )


class Buf:
    def __init__(self, w: int, h: int):
        self.w, self.h = w, h
        self.px = bytearray(w * h * 4)

    def set(self, x: int, y: int, c: tuple[int, int, int, int]) -> None:
        if 0 <= x < self.w and 0 <= y < self.h:
            i = (y * self.w + x) * 4
            self.px[i : i + 4] = bytes(c)

    def blend(self, x: int, y: int, c: tuple[int, int, int, int]) -> None:
        if not (0 <= x < self.w and 0 <= y < self.h):
            return
        i = (y * self.w + x) * 4
        sa = c[3] / 255.0
        if sa <= 0:
            return
        da = self.px[i + 3] / 255.0
        out_a = sa + da * (1 - sa)
        if out_a <= 0:
            return
        for k in range(3):
            self.px[i + k] = int(
                (c[k] * sa + self.px[i + k] * da * (1 - sa)) / out_a
            )
        self.px[i + 3] = int(out_a * 255)

    def fill(self, c: tuple[int, int, int, int]) -> None:
        for y in range(self.h):
            for x in range(self.w):
                self.set(x, y, c)

    def rect(self, x: int, y: int, w: int, h: int, c: tuple[int, int, int, int]) -> None:
        for yy in range(y, y + h):
            for xx in range(x, x + w):
                self.blend(xx, yy, c)

    def ellipse(self, cx: float, cy: float, rx: float, ry: float, c: tuple[int, int, int, int]) -> None:
        for y in range(int(cy - ry) - 1, int(cy + ry) + 2):
            for x in range(int(cx - rx) - 1, int(cx + rx) + 2):
                if rx <= 0 or ry <= 0:
                    continue
                if ((x + 0.5 - cx) / rx) ** 2 + ((y + 0.5 - cy) / ry) ** 2 <= 1:
                    self.blend(x, y, c)

    def diamond(self, cx: float, cy: float, hw: float, hh: float, c: tuple[int, int, int, int]) -> None:
        for y in range(int(cy - hh) - 1, int(cy + hh) + 2):
            for x in range(int(cx - hw) - 1, int(cx + hw) + 2):
                if hw <= 0 or hh <= 0:
                    continue
                if abs(x + 0.5 - cx) / hw + abs(y + 0.5 - cy) / hh <= 1:
                    self.blend(x, y, c)

    def write(self, path: Path) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(png(self.w, self.h, bytes(self.px)))


def rgba(hex_color: str, a: int = 255) -> tuple[int, int, int, int]:
    h = hex_color.lstrip("#")
    if len(h) == 3:
        h = "".join(ch * 2 for ch in h)
    return int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16), a


def tile(top: str, shade: str, speckle: str | None = None) -> Buf:
    b = Buf(64, 32)
    b.rect(0, 0, 64, 32, rgba(shade))
    b.rect(1, 1, 62, 30, rgba(top))
    if speckle:
        for x, y in ((22, 12), (36, 18), (28, 20), (40, 11), (18, 16)):
            b.blend(x, y, rgba(speckle, 180))
            b.blend(x + 1, y, rgba(speckle, 120))
    return b


def prop(w: int = 64, h: int = 96) -> Buf:
    return Buf(w, h)


def save(rel: str, buf: Buf) -> None:
    path = ROOT / rel
    buf.write(path)
    print("wrote", path.relative_to(ROOT.parent.parent))


def main() -> None:
    save("tiles/grass.png", tile("#5a8f3c", "#3d6a28", "#7aaa48"))
    save("tiles/grass-scar.png", tile("#6e5a40", "#4a3c28", "#8a7350"))
    save("tiles/path.png", tile("#c2a36b", "#8a6a3a", "#d4b88a"))
    save("tiles/path-scar.png", tile("#8a7350", "#5a4a30", "#b08950"))
    save("tiles/water.png", tile("#3a7ab0", "#1e4a78", "#8ec4e8"))
    save("tiles/wall.png", tile("#6b5340", "#3a2a1c", "#c08950"))
    save("tiles/house.png", tile("#b08968", "#6b4423", "#d4b08a"))
    save("tiles/earth.png", tile("#6a5a3c", "#4a3c28", "#8a7350"))

    # --- stile ---
    s = prop()
    s.rect(18, 70, 6, 22, rgba("#6b3e1a"))
    s.rect(40, 70, 6, 22, rgba("#6b3e1a"))
    s.rect(16, 74, 32, 5, rgba("#8a5a28"))
    s.rect(16, 82, 32, 4, rgba("#c08950"))
    s.rect(22, 88, 20, 4, rgba("#5a381c"))
    save("props/stile.png", s)

    # --- hearth ---
    h = prop()
    h.rect(16, 48, 32, 40, rgba("#5a4a40"))
    h.rect(20, 52, 24, 28, rgba("#2a1c14"))
    h.rect(24, 58, 16, 16, rgba("#e07020", 230))
    h.rect(14, 44, 36, 8, rgba("#8a7350"))
    save("props/hearth.png", h)

    # --- campfire base ---
    f = prop()
    f.rect(18, 82, 28, 6, rgba("#5a3a18"))
    f.ellipse(32, 78, 8, 14, rgba("#e07020", 220))
    f.ellipse(32, 72, 4, 10, rgba("#f0d060", 200))
    save("props/fire.png", f)

    # --- mill ---
    m = prop()
    m.ellipse(32, 78, 20, 10, rgba("#8a8070"))
    m.ellipse(32, 76, 14, 7, rgba("#4a4035"))
    m.rect(30, 58, 4, 20, rgba("#6b3e1a"))
    save("props/mill.png", m)

    # --- chest ---
    c = prop()
    c.rect(14, 68, 36, 22, rgba("#6b3e1a"))
    c.rect(14, 62, 36, 10, rgba("#8a5a28"))
    c.rect(28, 74, 8, 6, rgba("#c4a36a"))
    save("props/chest.png", c)

    # --- kiln ---
    k = prop()
    k.rect(18, 40, 28, 50, rgba("#3a2a22"))
    k.rect(24, 56, 16, 22, rgba("#1a140e"))
    k.rect(26, 60, 12, 12, rgba("#e07020", 220))
    k.rect(26, 28, 12, 14, rgba("#5a3a28"))
    save("props/kiln.png", k)

    # --- anvil ---
    a = prop()
    a.rect(20, 78, 24, 12, rgba("#4a3a28"))
    a.rect(24, 70, 16, 10, rgba("#2a2a2a"))
    a.rect(12, 64, 40, 8, rgba("#8a8a8a"))
    save("props/anvil.png", a)

    # --- bush ---
    bu = prop()
    bu.ellipse(32, 80, 18, 12, rgba("#2f5a22"))
    bu.ellipse(24, 76, 10, 8, rgba("#3d6a28"))
    bu.ellipse(40, 78, 10, 8, rgba("#245218"))
    bu.rect(24, 76, 3, 3, rgba("#bb3333"))
    bu.rect(38, 80, 3, 3, rgba("#bb3333"))
    save("props/bush.png", bu)
    bs = prop()
    bs.ellipse(32, 80, 16, 10, rgba("#3a4a30"))
    save("props/bush-spent.png", bs)

    # --- hazel ---
    hz = prop()
    hz.rect(30, 62, 5, 26, rgba("#6b3e1a"))
    hz.ellipse(32, 64, 16, 14, rgba("#3d5a22"))
    hz.ellipse(26, 70, 3, 3, rgba("#c4a36a"))
    hz.ellipse(38, 72, 3, 3, rgba("#c4a36a"))
    save("props/hazel.png", hz)
    hs = prop()
    hs.rect(30, 70, 5, 18, rgba("#5a381c"))
    hs.ellipse(32, 70, 12, 10, rgba("#3a4a30"))
    save("props/hazel-spent.png", hs)

    # --- tree / stump ---
    tr = prop()
    tr.rect(28, 58, 8, 34, rgba("#6b3e1a"))
    tr.ellipse(32, 48, 22, 20, rgba("#245218"))
    tr.ellipse(24, 42, 12, 10, rgba("#2f5a22"))
    tr.ellipse(40, 44, 12, 10, rgba("#3d6a28"))
    save("props/tree.png", tr)
    st = prop()
    st.ellipse(32, 86, 12, 6, rgba("#6b3e1a"))
    st.ellipse(32, 84, 8, 4, rgba("#4a2a10"))
    save("props/stump.png", st)

    # --- ore ---
    cu = prop()
    cu.diamond(32, 80, 18, 12, rgba("#6a5340"))
    cu.rect(26, 74, 4, 4, rgba("#c46a32"))
    cu.rect(36, 80, 3, 3, rgba("#c46a32"))
    save("props/copper.png", cu)
    tn = prop()
    tn.diamond(32, 80, 18, 12, rgba("#6a5340"))
    tn.rect(26, 74, 4, 4, rgba("#c8c4b0"))
    tn.rect(36, 80, 3, 3, rgba("#c8c4b0"))
    save("props/tin.png", tn)

    # --- hostiles ---
    th = prop()
    th.ellipse(32, 88, 12, 5, rgba("#000000", 50))
    th.rect(24, 62, 16, 22, rgba("#3a4a22"))
    th.rect(20, 66, 6, 14, rgba("#2a3a18"))
    th.rect(38, 66, 6, 14, rgba("#2a3a18"))
    th.ellipse(32, 56, 9, 8, rgba("#4a5a28"))
    th.rect(30, 46, 4, 10, rgba("#5a2a18"))
    th.rect(22, 50, 3, 8, rgba("#5a2a18"))
    th.rect(39, 50, 3, 8, rgba("#5a2a18"))
    save("hostiles/thornkin.png", th)

    br = prop()
    br.ellipse(32, 90, 16, 6, rgba("#000000", 50))
    br.ellipse(32, 78, 20, 12, rgba("#3a2a18"))
    br.rect(14, 74, 8, 16, rgba("#2a1c10"))
    br.rect(42, 74, 8, 16, rgba("#2a1c10"))
    br.ellipse(44, 68, 10, 8, rgba("#4a3420"))
    br.rect(28, 62, 5, 10, rgba("#5a2a18"))
    br.rect(36, 60, 4, 8, rgba("#5a2a18"))
    save("hostiles/brambleback.png", br)


if __name__ == "__main__":
    main()
