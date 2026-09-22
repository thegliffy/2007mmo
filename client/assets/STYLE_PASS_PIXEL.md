# Pixel style lock (Kyle reference 2026-09-13)

Reference: `docs/kyle_style_reference.png` (chunky ¾ isometric pixel rocks).

## Rules
- Chunky **¾ / isometric pixel** props
- Limited palette, **clean dark outlines**, readable silhouette
- Early **2007 browser MMO** — not painterly / modern 3D sheets
- Client: ¾ + chase; HUD wood+parchment; original IP

## Pack
All files under this folder restyled to match. Draw sizes unchanged (64 / 96 / 64×96, paperdoll layers 128×192).
Camera diamond is 128×64 (+33% vs 96×48) so the same plates read closer.
Spent gather plates: `props/stump.png`, `gather/bramble_bare.png`, `gather/hazel_bare.png`, `gather/ore_copper_empty.png`, `gather/ore_tin_empty.png`.
Coder: re-push `client/assets/` from this pack (supersedes PR #10 painterly/flat passes). Regenerate with `scripts/gen-rs-sprites.py`.
