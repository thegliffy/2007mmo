package world

type Point struct {
	X, Y int
}

func (w *World) inBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < w.W && y < w.H
}

func (w *World) Walkable(x, y int) bool {
	return w.inBounds(x, y) && !w.Block[y][x]
}

// FindPath is a short BFS. Returns steps after start, including dest.
func (w *World) FindPath(sx, sy, tx, ty int) []Point {
	if !w.Walkable(sx, sy) || !w.Walkable(tx, ty) {
		return nil
	}
	if sx == tx && sy == ty {
		return nil
	}
	type node struct{ x, y, i int }
	max := w.W * w.H
	if max < 256 {
		max = 256
	}
	qx := make([]int, 0, 64)
	qy := make([]int, 0, 64)
	prev := make([]int, w.W*w.H)
	for i := range prev {
		prev[i] = -2
	}
	start := sy*w.W + sx
	goal := ty*w.W + tx
	prev[start] = -1
	qx = append(qx, sx)
	qy = append(qy, sy)
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	found := false
	for head := 0; head < len(qx) && head < max; head++ {
		x, y := qx[head], qy[head]
		if x == tx && y == ty {
			found = true
			break
		}
		for _, d := range dirs {
			nx, ny := x+d[0], y+d[1]
			if !w.Walkable(nx, ny) {
				continue
			}
			idx := ny*w.W + nx
			if prev[idx] != -2 {
				continue
			}
			prev[idx] = y*w.W + x
			qx = append(qx, nx)
			qy = append(qy, ny)
		}
	}
	if !found || prev[goal] == -2 {
		return nil
	}
	var rev []Point
	for idx := goal; idx != start && idx >= 0; idx = prev[idx] {
		rev = append(rev, Point{idx % w.W, idx / w.W})
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func (w *World) nearestAdjacent(fromX, fromY, tx, ty int) (int, int, bool) {
	if fromX == tx && fromY == ty {
		return fromX, fromY, true
	}
	dirs := [5][2]int{{0, 0}, {0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	best := 1 << 30
	bx, by := tx, ty
	ok := false
	for _, d := range dirs {
		x, y := tx+d[0], ty+d[1]
		if !w.Walkable(x, y) {
			continue
		}
		dist := abs(x-fromX) + abs(y-fromY)
		if dist < best {
			best = dist
			bx, by = x, y
			ok = true
		}
	}
	return bx, by, ok
}

func adjacent(ax, ay, bx, by int) bool {
	dx := abs(ax - bx)
	dy := abs(ay - by)
	return dx+dy <= 1
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
