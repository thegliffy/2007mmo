# Hollowmere production art pack

Original IP for `thegliffy/2007mmo`. North star: carved wood+parchment HUD (UI separate), stylized 3D paperdoll, woodland skills, **¾ hamlet** sprites.

## Draw sizes (intended)
| Kind | Size | Notes |
|---|---|---|
| Terrain tiles | **64×64** | Replace procedural fills; map advisory tileSize=32 — scale as needed |
| Props / gather / buildings | **96×96** (house **192×192**) | Ground diamond of the plate maps onto the tile diamond (see `client/iso.js`) |
| Characters (NPC/hostile/player) | **64×96** | Opaque bottom-centre (feet) maps to iso project of tile centre |
| Paperdoll layers | **128×192** | Shared foot at canvas bottom-centre. Pre-tinted `skin_*` / `tunic_*`; hair masks tint in the client |

## Map glyph → asset
| Glyph / node | File |
|---|---|
| `.` grass | `terrain/grass.png` |
| `P` path | `terrain/path.png` |
| `~` water | `terrain/water.png` |
| scars (27≤x<40) | `terrain/scar.png` |
| `#` wall | `terrain/wall.png` |
| `T` tree | `props/tree.png` (+ `props/stump.png` when spent) |
| `H` house | `buildings/house.png` |
| `*` hearth | `props/hearth.png` |
| `M` mill | `props/millstone.png` |
| `E` chest | `props/oak_chest.png` |
| `K` kiln | `props/kiln.png` |
| `A` anvil | `props/anvil.png` |
| bush / `B` | `gather/bramble.png` (+ `gather/bramble_bare.png` when spent) |
| hazel / `Z` | `gather/hazel.png` (+ `gather/hazel_bare.png` when spent) |
| copper / `C` | `gather/ore_copper.png` (+ `gather/ore_copper_empty.png` when spent) |
| tin / `N` | `gather/ore_tin.png` (+ `gather/ore_tin_empty.png` when spent) |
| fish / `F` | `gather/fish_spot.png` (+ `gather/fish_spot_empty.png` when spent); ground tile is water |
| stile | `props/stile.png` |
| pedlar | `props/pedlar_stall.png` |
| Marta / Old Fen / Pip / Wend | `npcs/*.png` |
| Thornkin / Brambleback | `hostiles/*.png` |

## Paperdoll axes (from protocol/looks.go)
- skin: fair tan olive deep → `paperdoll/skin_*.png`
- hair styles: cropped short tied long → `paperdoll/hair_*_mask.png` (+ hairColor tint)
- top: moss clay ink cream berry → `paperdoll/tunic_*.png`

See `manifest.json` for full file list. WebP twins sit beside PNGs where generated.

## Style pass (2026-09-22)
Kyle: ¾ + chase camera, pulled in to a 128×64 tile diamond (+33% vs 96×48). Plates are chunky low-res pixels: flat two-tone fills, one dark outline, short palette. Gather nodes swap to a spent plate when `ready` is false (stump, bare bush, bare hazel, empty copper, empty tin, empty fishing ripples) and back when the node refills. Sprite URLs carry `?v=reedwater1`.
