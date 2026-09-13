package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// mem is a local Store double. internal/store imports this package, so
// world tests cannot reuse store.Memory without an import cycle — but it
// must copy exactly as deeply, or the no-dupe tests would be proving the
// invariant against a store that aliases where Postgres does not.
type mem struct {
	players map[string]PlayerRec
	nodes   map[string]NodeRec
	fail    bool
}

func copyRec(p *PlayerRec) PlayerRec {
	cp := *p
	cp.Inv = append([]ItemStack(nil), p.Inv...)
	cp.Skills = make(map[string]SkillState, len(p.Skills))
	for k, v := range p.Skills {
		cp.Skills[k] = v
	}
	if p.HP != nil {
		hp := *p.HP
		cp.HP = &hp
	}
	return cp
}

func newMem() *mem {
	return &mem{players: map[string]PlayerRec{}, nodes: map[string]NodeRec{}}
}

func (m *mem) LoadPlayer(_ context.Context, id string) (*PlayerRec, error) {
	p, ok := m.players[id]
	if !ok {
		return nil, nil
	}
	cp := copyRec(&p)
	return &cp, nil
}
func (m *mem) SavePlayer(_ context.Context, p *PlayerRec) error {
	if m.fail {
		return errFail
	}
	m.players[p.ID] = copyRec(p)
	return nil
}
func (m *mem) CommitAction(_ context.Context, p *PlayerRec, n *NodeRec) error {
	if m.fail {
		return errFail
	}
	m.players[p.ID] = copyRec(p)
	if n != nil {
		m.nodes[n.ID] = *n
	}
	return nil
}
func (m *mem) LoadNodes(_ context.Context) ([]NodeRec, error) {
	var out []NodeRec
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return out, nil
}
func (m *mem) UpsertNode(_ context.Context, n NodeRec) error {
	m.nodes[n.ID] = n
	return nil
}

type failT string

func (f failT) Error() string { return string(f) }

var errFail failT = "fail"

func testWorld(t *testing.T, st Store) *World {
	t.Helper()
	w := New(st)
	return w
}

func TestWalkOneTilePerTick(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	startX, startY := p.X, p.Y
	w.SetDest("p1", startX+2, startY)
	w.Tick(context.Background())
	if p.X != startX+1 || p.Y != startY {
		t.Fatalf("expected one tile east, got %d,%d", p.X, p.Y)
	}
	w.Tick(context.Background())
	if p.X != startX+2 || p.Y != startY {
		t.Fatalf("expected dest, got %d,%d", p.X, p.Y)
	}
}

func TestCannotWalkThroughTrees(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	// (1,1) is a tree on the seeded map
	w.SetDest("p1", 1, 1)
	if p.HasDest {
		t.Fatal("dest should be rejected for blocked tile")
	}
}

func TestForageCommitsThenMemory(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	bush := firstKind(w, KindBush)
	if bush == nil {
		t.Fatal("no bush")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = bush.X, bush.Y
	w.SetInteract("p1", bush.ID)
	// arrive + start channel + complete
	for i := 0; i < forageTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("expected 1 berry, inv=%v", p.Inv)
	}
	saved := st.players["p1"]
	if countItem(saved.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("store missing berry: %+v", saved.Inv)
	}
	if st.nodes[bush.ID].Remaining != bushYield-1 {
		t.Fatalf("node remaining=%d", st.nodes[bush.ID].Remaining)
	}
}

func TestForageAbortedIfStoreFails(t *testing.T) {
	st := newMem()
	st.fail = true
	w := testWorld(t, st)
	var bush *Node
	for _, n := range w.Nodes {
		if n.Kind == KindBush {
			bush = n
			break
		}
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = bush.X, bush.Y
	before := bush.Remaining
	w.SetInteract("p1", bush.ID)
	for i := 0; i < forageTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("memory should be unchanged on persist fail, inv=%v", p.Inv)
	}
	if bush.Remaining != before {
		t.Fatalf("node mutated without commit")
	}
}

func TestCookAndUse(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemPulp, N: 1}}
	fire := w.Nodes["fire-1"]
	p.X, p.Y = fire.X, fire.Y
	w.SetInteract("p1", fire.ID)
	for i := 0; i < cookTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPulp) != 0 || countItem(p.Inv, protocol.ItemTart) != 1 {
		t.Fatalf("cook failed: %v", p.Inv)
	}
	msg, ok := w.UseItem(context.Background(), "p1", protocol.ItemTart)
	if !ok || countItem(p.Inv, protocol.ItemTart) != 0 {
		t.Fatalf("use failed ok=%v msg=%s inv=%v", ok, msg, p.Inv)
	}
}

func firstKind(w *World, kind string) *Node {
	for _, n := range w.Nodes {
		if n.Kind == kind {
			return n
		}
	}
	return nil
}

func TestBerryNeedsMillBeforeHearth(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	fire := w.Nodes["fire-1"]
	p.X, p.Y = fire.X, fire.Y
	w.SetInteract("p1", fire.ID)
	w.Tick(context.Background())
	if countItem(p.Inv, protocol.ItemTart) != 0 || countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("whole berries should not bake: %v", p.Inv)
	}
	if w.TakeNote("p1") == "" {
		t.Fatal("expected hearth hint")
	}
}

func TestMillThenCook(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	mill := firstKind(w, KindMill)
	if mill == nil {
		t.Fatal("no millstone")
	}
	p.X, p.Y = mill.X, mill.Y
	w.SetInteract("p1", mill.ID)
	for i := 0; i < millTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 0 || countItem(p.Inv, protocol.ItemPulp) != 1 {
		t.Fatalf("mill failed: %v", p.Inv)
	}
	saved := st.players["p1"]
	if countItem(saved.Inv, protocol.ItemPulp) != 1 || countItem(saved.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("store mill mismatch: %+v", saved.Inv)
	}
	fire := w.Nodes["fire-1"]
	p.X, p.Y = fire.X, fire.Y
	w.SetInteract("p1", fire.ID)
	for i := 0; i < cookTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemTart) != 1 || countItem(p.Inv, protocol.ItemPulp) != 0 {
		t.Fatalf("cook after mill failed: %v", p.Inv)
	}
}

func TestHazelForageAndRoast(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	hazel := firstKind(w, KindHazel)
	if hazel == nil {
		t.Fatal("no hazel")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = hazel.X, hazel.Y
	w.SetInteract("p1", hazel.ID)
	for i := 0; i < forageTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemNut) != 1 {
		t.Fatalf("expected 1 hazel nut, inv=%v", p.Inv)
	}
	if st.nodes[hazel.ID].Remaining != hazelYield-1 {
		t.Fatalf("hazel remaining=%d", st.nodes[hazel.ID].Remaining)
	}
	fire := w.Nodes["fire-1"]
	p.X, p.Y = fire.X, fire.Y
	w.SetInteract("p1", fire.ID)
	for i := 0; i < roastTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemNut) != 0 || countItem(p.Inv, protocol.ItemRoast) != 1 {
		t.Fatalf("roast failed: %v", p.Inv)
	}
	msg, ok := w.UseItem(context.Background(), "p1", protocol.ItemRoast)
	if !ok || countItem(p.Inv, protocol.ItemRoast) != 0 {
		t.Fatalf("eat roast failed ok=%v msg=%s inv=%v", ok, msg, p.Inv)
	}
}

func TestTwoForagersOneNut(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	hazel := firstKind(w, KindHazel)
	hazel.Remaining = 1
	a := w.UpsertPlayer(NewPlayerRec("a", "Ann"), true)
	b := w.UpsertPlayer(NewPlayerRec("b", "Bob"), true)
	a.X, a.Y = hazel.X, hazel.Y
	b.X, b.Y = hazel.X, hazel.Y
	w.SetInteract("a", hazel.ID)
	w.SetInteract("b", hazel.ID)
	for i := 0; i < forageTicks+2; i++ {
		w.Tick(context.Background())
	}
	got := countItem(a.Inv, protocol.ItemNut) + countItem(b.Inv, protocol.ItemNut)
	if got != 1 {
		t.Fatalf("expected exactly 1 nut across both, got %d (a=%v b=%v)", got, a.Inv, b.Inv)
	}
}

func TestCrashRecoveryNoPulpDupe(t *testing.T) {
	st := newMem()
	w1 := testWorld(t, st)
	p := w1.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	mill := firstKind(w1, KindMill)
	p.X, p.Y = mill.X, mill.Y
	w1.SetInteract("p1", mill.ID)
	for i := 0; i < millTicks+2; i++ {
		w1.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPulp) != 1 || countItem(p.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("setup mill: %v", p.Inv)
	}

	w2 := testWorld(t, st)
	rec, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || rec == nil {
		t.Fatalf("load after crash: %v %v", rec, err)
	}
	p2 := w2.UpsertPlayer(rec, true)
	if countItem(p2.Inv, protocol.ItemPulp) != 1 || countItem(p2.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("dupe or loss after mill recovery: %v", p2.Inv)
	}
}

func TestTwoForagersOneBerry(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	var bush *Node
	for _, n := range w.Nodes {
		if n.Kind == KindBush {
			bush = n
			break
		}
	}
	bush.Remaining = 1
	a := w.UpsertPlayer(NewPlayerRec("a", "Ann"), true)
	b := w.UpsertPlayer(NewPlayerRec("b", "Bob"), true)
	a.X, a.Y = bush.X, bush.Y
	b.X, b.Y = bush.X, bush.Y
	w.SetInteract("a", bush.ID)
	w.SetInteract("b", bush.ID)
	for i := 0; i < forageTicks+2; i++ {
		w.Tick(context.Background())
	}
	got := countItem(a.Inv, protocol.ItemBerry) + countItem(b.Inv, protocol.ItemBerry)
	if got != 1 {
		t.Fatalf("expected exactly 1 berry across both, got %d (a=%v b=%v)", got, a.Inv, b.Inv)
	}
	if bush.Remaining != 0 {
		t.Fatalf("bush should be empty, rem=%d", bush.Remaining)
	}
}

func TestLevelFromXP(t *testing.T) {
	if LevelFromXP(0) != 1 {
		t.Fatal("0 xp is level 1")
	}
	if LevelFromXP(25) != 2 {
		t.Fatalf("25 xp => lv 2, got %d", LevelFromXP(25))
	}
}

func TestSanitizeName(t *testing.T) {
	if SanitizeName("  ") != "Wanderer" {
		t.Fatal("blank")
	}
	if SanitizeName("Kyle<script>") != "Kylescript" {
		t.Fatalf("got %q", SanitizeName("Kyle<script>"))
	}
}

func TestCrashRecoveryNoItemDupe(t *testing.T) {
	st := newMem()
	w1 := testWorld(t, st)
	var bush *Node
	for _, n := range w1.Nodes {
		if n.Kind == KindBush {
			bush = n
			break
		}
	}
	p := w1.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = bush.X, bush.Y
	w1.SetInteract("p1", bush.ID)
	for i := 0; i < forageTicks+2; i++ {
		w1.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("setup: %v", p.Inv)
	}

	// Container killed: new process rebuilds only from the store.
	w2 := testWorld(t, st)
	nodes, _ := st.LoadNodes(context.Background())
	w2.RestoreNodes(nodes)
	rec, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || rec == nil {
		t.Fatalf("load after crash: %v %v", rec, err)
	}
	p2 := w2.UpsertPlayer(rec, true)
	if countItem(p2.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("dupe or loss after recovery: %v", p2.Inv)
	}
	var bush2 *Node
	for _, n := range w2.Nodes {
		if n.ID == bush.ID {
			bush2 = n
			break
		}
	}
	if bush2 == nil || bush2.Remaining != bushYield-1 {
		t.Fatalf("node not restored: %+v", bush2)
	}
}

// Work nodes block now, so each one must still have a walkable tile
// beside it. Blocking house tiles as well would seal the hearth inside
// its own walls; this is the test that catches that class of mistake.
func TestEveryNodeIsReachable(t *testing.T) {
	w := testWorld(t, newMem())
	for _, n := range w.Nodes {
		if w.Walkable(n.X, n.Y) {
			t.Errorf("%s at (%d,%d) is walkable; work nodes should block", n.ID, n.X, n.Y)
		}
		tx, ty, ok := w.nearestAdjacent(spawnX, spawnY, n.X, n.Y)
		if !ok {
			t.Fatalf("%s at (%d,%d) has no walkable tile beside it", n.ID, n.X, n.Y)
		}
		if !w.Walkable(tx, ty) {
			t.Fatalf("%s: nearestAdjacent returned unwalkable (%d,%d)", n.ID, tx, ty)
		}
		// And it must be reachable on foot from the stile.
		if len(w.FindPath(spawnX, spawnY, tx, ty)) == 0 && (tx != spawnX || ty != spawnY) {
			t.Fatalf("%s: no path from the stile to (%d,%d)", n.ID, tx, ty)
		}
	}
}

// Hostiles must not be stranded either.
func TestHostilesStandOnWalkableGround(t *testing.T) {
	w := testWorld(t, newMem())
	for _, n := range w.NPCs {
		if !w.Walkable(n.X, n.Y) {
			t.Errorf("%s spawns on blocked ground at (%d,%d)", n.ID, n.X, n.Y)
		}
		if !w.Walkable(n.HomeX, n.HomeY) {
			t.Errorf("%s has a blocked home tile (%d,%d)", n.ID, n.HomeX, n.HomeY)
		}
	}
}
