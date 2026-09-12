package world

// Original-IP hamlet plus southern briar-woods. No third-party tilesets
// or borrowed monster names.
//
//	# wall   T tree   ~ water   P path
//	. grass  H house  * hearth  B bramble
//	Z hazel  M millstone
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
	bush, hazel, mill := 0, 0, 0
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
		{ID: "npc-marta", Name: "Marta", X: 4, Y: 5, HomeX: 4, HomeY: 5, WanderEvery: 4, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		{ID: "npc-fen", Name: "Old Fen", X: 18, Y: 2, HomeX: 18, HomeY: 2, WanderEvery: 5, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		{ID: "npc-pip", Name: "Pip", X: 9, Y: 8, HomeX: 9, HomeY: 8, WanderEvery: 2, MinX: 1, MaxX: 22, MinY: 1, MaxY: 14},
		{
			ID: "npc-thornkin-1", Name: "Thornkin", X: 10, Y: 22, HomeX: 10, HomeY: 22,
			WanderEvery: 3, Hostile: true, HP: thornkinHP, MaxHP: thornkinHP, Dmg: thornkinDmg,
			MinX: 6, MaxX: 16, MinY: 19, MaxY: 26,
		},
		{
			ID: "npc-thornkin-2", Name: "Thornkin", X: 18, Y: 27, HomeX: 18, HomeY: 27,
			WanderEvery: 4, Hostile: true, HP: thornkinHP, MaxHP: thornkinHP, Dmg: thornkinDmg,
			MinX: 12, MaxX: 24, MinY: 24, MaxY: 30,
		},
		{
			ID: "npc-brambleback", Name: "Brambleback", X: 16, Y: 26, HomeX: 16, HomeY: 26,
			WanderEvery: 5, Hostile: true, HP: bramblebackHP, MaxHP: bramblebackHP, Dmg: bramblebackDmg,
			MinX: 10, MaxX: 22, MinY: 24, MaxY: 30,
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
