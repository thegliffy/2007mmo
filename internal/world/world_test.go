package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

type mem struct {
	players map[string]PlayerRec
	nodes   map[string]NodeRec
	fail    bool
}

func newMem() *mem {
	return &mem{players: map[string]PlayerRec{}, nodes: map[string]NodeRec{}}
}

func (m *mem) LoadPlayer(_ context.Context, id string) (*PlayerRec, error) {
	p, ok := m.players[id]
	if !ok {
		return nil, nil
	}
	cp := p
	return &cp, nil
}
func (m *mem) SavePlayer(_ context.Context, p *PlayerRec) error {
	if m.fail {
		return errFail
	}
	m.players[p.ID] = *p
	return nil
}
func (m *mem) CommitAction(_ context.Context, p *PlayerRec, n *NodeRec) error {
	if m.fail {
		return errFail
	}
	m.players[p.ID] = *p
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
	var bush *Node
	for _, n := range w.Nodes {
		if n.Kind == KindBush {
			bush = n
			break
		}
	}
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
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	fire := w.Nodes["fire-1"]
	p.X, p.Y = fire.X, fire.Y
	w.SetInteract("p1", fire.ID)
	for i := 0; i < cookTicks+2; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 0 || countItem(p.Inv, protocol.ItemTart) != 1 {
		t.Fatalf("cook failed: %v", p.Inv)
	}
	msg, ok := w.UseItem(context.Background(), "p1", protocol.ItemTart)
	if !ok || countItem(p.Inv, protocol.ItemTart) != 0 {
		t.Fatalf("use failed ok=%v msg=%s inv=%v", ok, msg, p.Inv)
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
