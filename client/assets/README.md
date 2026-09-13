# Hollowmere production art pack

Original IP for `thegliffy/2007mmo`. North star: carved wood+parchment HUD (UI separate), stylized 3D paperdoll, woodland skills, **¾ hamlet** sprites.

## Draw sizes (intended)
| Kind | Size | Notes |
|---|---|---|
| Terrain tiles | **64×64** | Replace procedural fills; map advisory tileSize=32 — scale as needed |
| Props / gather / buildings | **96×96** (house **192×192**) | Anchor at tile bottom-center |
| Characters (NPC/hostile/player) | **64×96** | Feet near bottom; ¾ facing |
| Paperdoll masks | **128×~192** | Tint in client or use pre-tinted `skin_*` / `tunic_*` / hair masks |

## Map glyph → asset
| Glyph / node | File |
|---|---|
| `.` grass | `terrain/grass.png` |
| `P` path | `terrain/path.png` |
| `~` water | `terrain/water.png` |
| scars (x≥27) | `terrain/scar.png` |
| `#` wall | `terrain/wall.png` |
| `T` tree | `props/tree.png` (+ `props/stump.png` when spent) |
| `H` house | `buildings/house.png` |
| `*` hearth | `props/hearth.png` |
| `M` mill | `props/millstone.png` |
| `E` chest | `props/oak_chest.png` |
| `K` kiln | `props/kiln.png` |
| `A` anvil | `props/anvil.png` |
| bush / `B` | `gather/bramble.png` |
| hazel / `Z` | `gather/hazel.png` |
| copper / `C` | `gather/ore_copper.png` |
| tin / `N` | `gather/ore_tin.png` |
| stile | `props/stile.png` |
| pedlar | `props/pedlar_stall.png` |
| Marta / Old Fen / Pip / Wend | `npcs/*.png` |
| Thornkin / Brambleback | `hostiles/*.png` |

## Paperdoll axes (from protocol/looks.go)
- skin: fair tan olive deep → `paperdoll/skin_*.png`
- hair styles: cropped short tied long → `paperdoll/hair_*_mask.png` (+ hairColor tint)
- top: moss clay ink cream berry → `paperdoll/tunic_*.png`

See `manifest.json` for full file list. WebP twins sit beside PNGs where generated.

## Style pass (2026-09-13)
Kyle: ¾ + chase camera; **2007 browser MMO** chunky/flat readability (not modern painterly). See `STYLE_PASS_2007.md`. Terrain is flat procedural for chase-cam; props/NPCs/hostiles re-exported flatter.
