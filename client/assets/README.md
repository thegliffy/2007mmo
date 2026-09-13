# Hollowmere art drop

Presentation only. The hamlet stays **top-down**; these files are drawn in the existing tile cells. Drop Art Director PNGs on these paths — same filenames — and they replace the placeholders. Missing files keep the canvas fallback.

## Anchors

| Kind | Placeholder size | How it is drawn |
|------|------------------|-----------------|
| Tiles | 64×32 | Stretched to fill the tile cell. |
| Props / hostiles | 64×96 | Stand in the cell, feet toward the bottom. |

No isometric engine. No paperdoll layer compositor. Players still use the existing appearance paperdoll (skin, hair, tunic colours).

## Tiles — `assets/tiles/`

| File | Tile |
|------|------|
| `grass.png` | `.` hamlet grass (and under trees) |
| `grass-scar.png` | `.` eastern scars |
| `path.png` | `P` packed earth |
| `path-scar.png` | `P` in the scars |
| `water.png` | `~` |
| `wall.png` | `#` |
| `house.png` | `H` / `*` cottage floor |
| `earth.png` | `C` `N` `K` `A` scar ground |

## Props — `assets/props/`

| File | What |
|------|------|
| `stile.png` | Wooden crossing at spawn (8, 8) — scenery, not a node |
| `hearth.png` | Cottage hearth (`fire-1`) |
| `fire.png` | Player campfire |
| `mill.png` | Millstone |
| `chest.png` | Oak chest |
| `kiln.png` | Smelting kiln |
| `anvil.png` | Forge anvil |
| `bush.png` / `bush-spent.png` | Bramble, ready / picked |
| `hazel.png` / `hazel-spent.png` | Hazel, ready / picked |
| `tree.png` / `stump.png` | Tree vs spent |
| `copper.png` / `tin.png` | Ore veins |

## Hostiles — `assets/hostiles/`

| File | Who |
|------|-----|
| `thornkin.png` | Thornkin |
| `brambleback.png` | Brambleback |

Villagers and players stay on the in-client paperdoll until Kyle asks for more.

## Locked look

Carved wood + parchment HUD. Woodland life-skills. Original Hollowmere IP — no borrowed studio chrome.
