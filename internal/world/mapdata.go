package world

// Original-IP hamlet, southern briar-woods, the eastern scars, and the
// reedwater beyond them. No third-party tilesets or borrowed monster names.
//
//	# wall   T tree   ~ water   P path
//	. grass  H house  * hearth  B bramble
//	Z hazel  M millstone
//	C copper N tin    K kiln    A anvil
//	E oak chest (personal bank, by the stile)
//	F fishing spot (reedwater; the client paints it as water)
//
// Everything a player cannot stand on blocks: walls, trees, water, and
// the work nodes themselves. You reach a bramble, a fishing spot, or the
// hearth from an adjacent tile, not by standing inside it.
//
// House tiles ('H') stay walkable on purpose: the hearth sits inside the
// house, and blocking them would seal it off entirely. See
// TestEveryNodeIsReachable.
//
// Columns 0–38 keep every Week 1 / Week 2 / scars coordinate (hearth,
// mill, bramble, hazel, stile spawn, southern clearing, veins, kiln,
// anvil). Column 39 was the east wall. Three gates — rows 4, 7, and 13,
// where the grass already reached the wall — open onto the reedwater,
// which runs to x=59 and y=45. The briar-woods stay walled off from
// that shore.
var mapRows = []string{
	"############################################################",
	"#TTT....PPPP....TTTTTTT...TTTTT.TTTTTTT#..TTT......~~~~~~~~#",
	"#T......P..P......B.B.T..T..T.C..C.TTTT#....T......~~~~~~~~#",
	"#.......P..P......B.B..............C..T#..PPPP.....~~~~~~~~#",
	"#..HHH..PPPP...............PPPP.C......PPPP........~~~~~~~~#",
	"#..H*H..P...M.........~~~..P..P..C.C..T#..P..TTT...~~~~~~~~#",
	"#..HHH..P.....~~~.....~~~..P.KP.......T#..P........~~~~~~~~#",
	"#.......P.....~~~..........P..P.N.N....P.PPPPP.....~~~~~~~~#",
	"#..TTT.EPPPPPPPPP....TTT...PPPPP....N.T#.....TTT..F~~~~~~~~#",
	"#.......P............T.T.....A.P.N....T#..........P~~~~~~~~#",
	"#.......P....~~~...............P...N..T#..TTT....P.~~~~~~~~#",
	"#TTT....P....~~~..TTT......TT..PPP....T#.........P..~~~~~~~#",
	"#T.ZZ...P..........T.......T.....P.C.TT#......PPPP.F~~~~~~~#",
	"#.......P....................N....P....PP.....P....~~~~~~~~#",
	"#TTTT...P.........TTTTT....TTT...P.TTTT#..TTT.P.TTT~~~~~~~~#",
	"#.......P........................P.....#......PPPP.~~~~~~~~#",
	"#.......PPPP................TTT.PP.TT.T#....PPPP.~~~~~~~~~~#",
	"#..TTT..P..P.....TTTT.......T...P...C.T#..TTT..P.F~~~~~~~~~#",
	"#.......P..P.....T..T.........C.P.N...T#.......P..~~~~~~~~~#",
	"#.......PPPP.......P..............P...T#....PPPP.~~~~~~~~~~#",
	"#..TTT.............P...TTT.TTT..PPP.TTT#..TTT..P.~~~~~~~~~~#",
	"#..............PPPPP.................P.#.......P.F~~~~~~~~~#",
	"#..TTT.........P...P...TT..TT.N...P.C.T#..TTT..PP~~~~~~~~~~#",
	"#..............P.~~~.P.............P..T#.......P.PPF~~~~~~~#",
	"#..T...........P.~~~.P..T...T.C...P.N.T#..........P~~~~~~~~#",
	"#..............PPPPPPP..........PPP....#..TTT....P.~~~~~~~~#",
	"#.....TTT......P.....P.TTT.TTT..P...TTT#.......PP.F~~~~~~~~#",
	"#..............P.................P.C..T#....PPPP.~~~~~~~~~~#",
	"#.....TTT......P.....P.TTT.TT.N.P...N.T#..TTT..P.~~~~~~~~~~#",
	"#..............PPPPPPP..........PPP....#.......P.F~~~~~~~~~#",
	"#..T...........P.....P..T..T.C......C.T#....PPPP.~~~~~~~~~~#",
	"########################################.......P.~~~~~~~~~~#",
	"########################################..TTT..P.F~~~~~~~~~#",
	"########################################.......P..~~~~~~~~~#",
	"########################################....PPPP.F~~~~~~~~~#",
	"########################################..TTT..P.~~~~~~~~~~#",
	"########################################.......P.~~~~~~~~~~#",
	"########################################..TTT..P.F~~~~~~~~~#",
	"########################################.......PP~~~~~~~~~~#",
	"########################################..TTT..P.~~~~~~~~~~#",
	"########################################.......P.F~~~~~~~~~#",
	"########################################....PPPP.~~~~~~~~~~#",
	"########################################..TTT.....~~~~~~~~~#",
	"########################################..........~~~~~~~~~#",
	"########################################..TTT.....~~~~~~~~~#",
	"############################################################",
}

const tileSize = 32

func parseMap() (w, h int, tiles [][]byte, block [][]bool) {
	h = len(mapRows)
	w = len(mapRows[0])
	tiles = make([][]byte, h)
	block = make([][]bool, h)
	for y, row := range mapRows {
		if len(row) != w {
			panic("map row width mismatch")
		}
		tiles[y] = []byte(row)
		block[y] = make([]bool, w)
		for x, c := range row {
			if tileBlocks(byte(c)) {
				block[y][x] = true
			}
		}
	}
	return w, h, tiles, block
}

func seedNodes() []*Node {
	var out []*Node
	bush, hazel, mill, tree := 0, 0, 0, 0
	copper, tin, kiln, anvil, chest, fish := 0, 0, 0, 0, 0, 0
	for y, row := range mapRows {
		for x, c := range row {
			switch c {
			case 'B':
				bush++
				out = append(out, &Node{
					ID:        "bush-" + itoa(bush),
					Kind:      KindBush,
					X:         x,
					Y:         y,
					Remaining: bushYield,
					Max:       bushYield,
				})
			case 'Z':
				hazel++
				out = append(out, &Node{
					ID:        "hazel-" + itoa(hazel),
					Kind:      KindHazel,
					X:         x,
					Y:         y,
					Remaining: hazelYield,
					Max:       hazelYield,
				})
			case 'M':
				mill++
				out = append(out, &Node{
					ID:        "mill-" + itoa(mill),
					Kind:      KindMill,
					X:         x,
					Y:         y,
					Remaining: 1,
					Max:       1,
				})
			case 'T':
				// Only trees you can actually stand beside. A tree buried
				// inside a clump has no walkable neighbour, so it can never
				// be chopped and has no business being a node — it stays
				// scenery, and the node list stays smaller for it.
				if !hasWalkableNeighbour(x, y) {
					continue
				}
				tree++
				out = append(out, &Node{
					ID:        "tree-" + itoa(tree),
					Kind:      KindTree,
					X:         x,
					Y:         y,
					Remaining: treeYield,
					Max:       treeYield,
				})
			case 'C', 'N':
				// Same rule as trees: a vein you can never stand beside
				// is scenery, not a node.
				if !hasWalkableNeighbour(x, y) {
					continue
				}
				kind, prefix := KindCopper, "copper-"
				n := &copper
				if c == 'N' {
					kind, prefix, n = KindTin, "tin-", &tin
				}
				*n++
				out = append(out, &Node{
					ID:        prefix + itoa(*n),
					Kind:      kind,
					X:         x,
					Y:         y,
					Remaining: oreYield,
					Max:       oreYield,
				})
			case 'K':
				kiln++
				out = append(out, &Node{
					ID:        "kiln-" + itoa(kiln),
					Kind:      KindKiln,
					X:         x,
					Y:         y,
					Remaining: 1,
					Max:       1,
				})
			case 'A':
				anvil++
				out = append(out, &Node{
					ID:        "anvil-" + itoa(anvil),
					Kind:      KindAnvil,
					X:         x,
					Y:         y,
					Remaining: 1,
					Max:       1,
				})
			case 'F':
				// Same rule as trees: a spot you can never stand beside
				// is a ripple, not a node.
				if !hasWalkableNeighbour(x, y) {
					continue
				}
				fish++
				out = append(out, &Node{
					ID:        "fish-" + itoa(fish),
					Kind:      KindFish,
					X:         x,
					Y:         y,
					Remaining: fishYield,
					Max:       fishYield,
				})
			case '*':
				out = append(out, &Node{
					ID:        "fire-1",
					Kind:      KindFire,
					X:         x,
					Y:         y,
					Remaining: 1,
					Max:       1,
				})
			case 'E':
				chest++
				out = append(out, &Node{
					ID:        "chest-" + itoa(chest),
					Kind:      KindChest,
					X:         x,
					Y:         y,
					Remaining: 1,
					Max:       1,
				})
			}
		}
	}
	return out
}

// hasWalkableNeighbour reports whether any of the four tiles around
// (x,y) can be stood on.
func hasWalkableNeighbour(x, y int) bool {
	for _, d := range [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		nx, ny := x+d[0], y+d[1]
		if ny < 0 || ny >= len(mapRows) {
			continue
		}
		row := mapRows[ny]
		if nx < 0 || nx >= len(row) {
			continue
		}
		if tileBlocks(row[nx]) {
			continue
		}
		return true
	}
	return false
}

func seedNPCs() []*NPC {
	return []*NPC{
		// Beside the hearth, not in it: (4,5) is the fire tile itself.
		{ID: "npc-marta", Name: "Marta", X: 3, Y: 5, HomeX: 3, HomeY: 5, WanderEvery: 4, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		// Beside the brambles rather than inside one: (18,2) is a bush tile.
		{ID: "npc-fen", Name: "Old Fen", X: 17, Y: 2, HomeX: 17, HomeY: 2, WanderEvery: 5, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		// The pedlar keeps to the path by the millstone, so she is on the
		// way to everything rather than tucked in a corner.
		{ID: "npc-wend", Name: "Wend the Pedlar", X: 10, Y: 5, HomeX: 10, HomeY: 5,
			WanderEvery: 0, Trader: true, MinX: 10, MaxX: 10, MinY: 5, MaxY: 5},
		{ID: "npc-pip", Name: "Pip", X: 9, Y: 8, HomeX: 9, HomeY: 8, WanderEvery: 2, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		{
			ID: "npc-thornkin-1", Name: "Thornkin", X: 10, Y: 22, HomeX: 10, HomeY: 22,
			WanderEvery: 3, Hostile: true, HP: thornkinHP, MaxHP: thornkinHP, Dmg: thornkinDmg,
			MinX: 6, MaxX: 16, MinY: 19, MaxY: 26,
			CoinsMin: 3, CoinsMax: 8, LeatherOdds: 16,
		},
		{
			ID: "npc-thornkin-2", Name: "Thornkin", X: 18, Y: 27, HomeX: 18, HomeY: 27,
			WanderEvery: 4, Hostile: true, HP: thornkinHP, MaxHP: thornkinHP, Dmg: thornkinDmg,
			MinX: 12, MaxX: 24, MinY: 24, MaxY: 30,
			CoinsMin: 3, CoinsMax: 8, LeatherOdds: 16,
		},
		{
			ID: "npc-brambleback", Name: "Brambleback", X: 16, Y: 26, HomeX: 16, HomeY: 26,
			WanderEvery: 5, Hostile: true, HP: bramblebackHP, MaxHP: bramblebackHP, Dmg: bramblebackDmg,
			MinX: 10, MaxX: 22, MinY: 24, MaxY: 30,
			CoinsMin: 12, CoinsMax: 25, LeatherOdds: 6,
		},
	}
}

// tileBlocks is everything a player cannot stand on. House tiles stay
// walkable so the hearth is not sealed inside its own walls.
func tileBlocks(c byte) bool {
	switch c {
	case '#', 'T', '~', 'B', 'Z', 'M', '*', 'C', 'N', 'K', 'A', 'E', 'F':
		return true
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
