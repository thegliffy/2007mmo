package world

// Original-IP hamlet plus southern briar-woods. No third-party tilesets
// or borrowed monster names.
//
//	# wall   T tree   ~ water   P path
//	. grass  H house  * hearth  B bramble
//	Z hazel  M millstone
//
// Everything a player cannot stand on blocks: walls, trees, water, and
// the work nodes themselves. You reach a bramble or the hearth from an
// adjacent tile, not by standing inside it.
//
// House tiles ('H') stay walkable on purpose: the hearth sits inside the
// house, and blocking them would seal it off entirely. See
// TestEveryNodeIsReachable.
//
// The northern 15 rows keep the Week 1 hamlet coordinates (hearth, mill,
// bramble, hazel, stile spawn). Rows below open into a woods and clearing.
var mapRows = []string{
	"############################",
	"#TTT....PPPP....TTTTTTT...T#",
	"#T......P..P......B.B.T..T.#",
	"#.......P..P......B.B......#",
	"#..HHH..PPPP...............#",
	"#..H*H..P...M.........~~~..#",
	"#..HHH..P.....~~~.....~~~..#",
	"#.......P.....~~~..........#",
	"#..TTT..PPPPPPPPP....TTT...#",
	"#.......P............T.T...#",
	"#.......P....~~~...........#",
	"#TTT....P....~~~..TTT......#",
	"#T.ZZ...P..........T.......#",
	"#.......P..................#",
	"#TTTT...P.........TTTTT....#",
	"#.......P..................#",
	"#.......PPPP...............#",
	"#..TTT..P..P.....TTTT......#",
	"#.......P..P.....T..T......#",
	"#.......PPPP.......P.......#",
	"#..TTT.............P...TTT.#",
	"#..............PPPPP.......#",
	"#..TTT.........P...P...TT..#",
	"#..............P.~~~.P.....#",
	"#..T...........P.~~~.P..T..#",
	"#..............PPPPPPP.....#",
	"#.....TTT......P.....P.TTT.#",
	"#..............P...........#",
	"#.....TTT......P.....P.TTT.#",
	"#..............PPPPPPP.....#",
	"#..T...........P.....P..T..#",
	"############################",
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
			switch c {
			case '#', 'T', '~', 'B', 'Z', 'M', '*':
				block[y][x] = true
			}
		}
	}
	return w, h, tiles, block
}

func seedNodes() []*Node {
	var out []*Node
	bush, hazel, mill, tree := 0, 0, 0, 0
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
			case '*':
				out = append(out, &Node{
					ID:        "fire-1",
					Kind:      KindFire,
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
		switch row[nx] {
		case '#', 'T', '~', 'B', 'Z', 'M', '*':
			// blocked
		default:
			return true
		}
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
