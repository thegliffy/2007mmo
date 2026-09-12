package world

// Original-IP hamlet. No third-party tilesets or place names.
//
//	# wall   T tree   ~ water   P path
//	. grass  H house  * fire    B bush
var mapRows = []string{
	"########################",
	"#TTT....PPPP....TTTTTTT#",
	"#T......P..P......B.B.T#",
	"#.......P..P......B.B..#",
	"#..HHH..PPPP...........#",
	"#..H*H..P..............#",
	"#..HHH..P.....~~~......#",
	"#.......P.....~~~......#",
	"#..TTT..PPPPPPPPP......#",
	"#.......P..............#",
	"#.......P....~~~.......#",
	"#TTT....P....~~~..TTT..#",
	"#T......P..........T...#",
	"#.......P..............#",
	"#TTTT...P.........TTTTT#",
	"########################",
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
			case '#', 'T', '~':
				block[y][x] = true
			}
		}
	}
	return w, h, tiles, block
}

func walkableGlyph(c byte) bool {
	switch c {
	case '#', 'T', '~':
		return false
	default:
		return true
	}
}

func seedNodes() []*Node {
	var out []*Node
	bush := 0
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

func seedNPCs() []*NPC {
	return []*NPC{
		{ID: "npc-marta", Name: "Marta", X: 4, Y: 5, WanderEvery: 4},
		{ID: "npc-fen", Name: "Old Fen", X: 18, Y: 2, WanderEvery: 5},
		{ID: "npc-pip", Name: "Pip", X: 9, Y: 8, WanderEvery: 2},
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
