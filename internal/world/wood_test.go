package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func firstNodeOfKind(w *World, kind string) *Node {
	for _, n := range w.Nodes {
		if n.Kind == kind {
			return n
		}
	}
	return nil
}

// standBeside puts a player on a walkable tile next to a node.
func standBeside(t *testing.T, w *World, p *Player, n *Node) {
	t.Helper()
	x, y, ok := w.nearestAdjacent(n.X, n.Y, n.X, n.Y)
	if !ok {
		t.Fatalf("%s has nowhere to stand beside it", n.ID)
	}
	p.X, p.Y = x, y
}

func TestChoppingATreeYieldsALog(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	if tree == nil {
		t.Fatal("no choppable trees on the map")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, tree)

	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+3; i++ {
		w.Tick(context.Background())
	}
	if got := countItem(p.Inv, protocol.ItemLog); got != 1 {
		t.Fatalf("pack holds %d logs, want 1 (%+v)", got, p.Inv)
	}
	if sk := p.Skills[protocol.SkillWood]; sk.XP != chopXP {
		t.Fatalf("woodcutting xp = %d, want %d", sk.XP, chopXP)
	}
	if tree.Remaining != treeYield-1 {
		t.Fatalf("tree has %d left, want %d", tree.Remaining, treeYield-1)
	}
}

// Chopping trains Woodcutting, not Foraging.
func TestChoppingDoesNotTrainForaging(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, tree)
	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+3; i++ {
		w.Tick(context.Background())
	}
	if p.Skills[protocol.SkillForage].XP != 0 {
		t.Fatalf("chopping gave foraging %d xp", p.Skills[protocol.SkillForage].XP)
	}
}

// A tree runs out and comes back, like a bramble.
func TestTreeDepletesAndRegrows(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, tree)

	for i := 0; i < treeYield; i++ {
		w.SetInteract("p1", tree.ID)
		for j := 0; j < chopTicks+3; j++ {
			w.Tick(context.Background())
		}
	}
	if tree.Remaining != 0 {
		t.Fatalf("tree still has %d after %d chops", tree.Remaining, treeYield)
	}
	if nodeReady(tree) {
		t.Fatal("a bare tree should not read as ready")
	}
	for i := 0; i < treeCD+2; i++ {
		w.Tick(context.Background())
	}
	if tree.Remaining != treeYield {
		t.Fatalf("tree did not regrow: %d", tree.Remaining)
	}
}

func TestMillstoneTurnsLogsIntoPaper(t *testing.T) {
	w := testWorld(t, newMem())
	mill := firstNodeOfKind(w, KindMill)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemLog, N: 1}}
	standBeside(t, w, p, mill)

	w.SetInteract("p1", mill.ID)
	for i := 0; i < paperTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPaper) != 1 {
		t.Fatalf("no paper: %+v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemLog) != 0 {
		t.Fatal("the log was not consumed")
	}
	if p.Skills[protocol.SkillWood].XP != paperXP {
		t.Fatalf("woodcutting xp = %d, want %d", p.Skills[protocol.SkillWood].XP, paperXP)
	}
}

// Carrying both, the millstone does the berry first — the older recipe
// keeps working exactly as it did.
func TestMillstonePrefersBerriesOverLogs(t *testing.T) {
	w := testWorld(t, newMem())
	mill := firstNodeOfKind(w, KindMill)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}, {ID: protocol.ItemLog, N: 1}}
	standBeside(t, w, p, mill)

	w.SetInteract("p1", mill.ID)
	for i := 0; i < millTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPulp) != 1 {
		t.Fatalf("expected pulp from the berry: %+v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemLog) != 1 {
		t.Fatal("the log should be untouched")
	}
}

func TestLightingALogPileMakesAFire(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)

	msg, ok := w.LightFire("p1", g.ID)
	if !ok {
		t.Fatalf("could not light it: %s", msg)
	}
	var fire *Node
	for _, n := range w.Nodes {
		if n.temporary() && n.X == p.X && n.Y == p.Y {
			fire = n
		}
	}
	if fire == nil {
		t.Fatal("no campfire appeared")
	}
	if fire.Kind != KindFire {
		t.Fatalf("campfire kind = %q", fire.Kind)
	}
	// The log is spent, so the pile is gone.
	if w.groundByID(g.ID) != nil {
		t.Fatal("the log pile survived being lit")
	}
}

func TestCannotLightWithoutALog(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 0, p.ID)
	if _, ok := w.LightFire("p1", g.ID); ok {
		t.Fatal("lit a fire from a brambleberry")
	}
}

// You cannot light somebody else's reserved pile.
func TestCannotLightAReservedPile(t *testing.T) {
	w := testWorld(t, newMem())
	a := w.UpsertPlayer(NewPlayerRec("a", "Ash"), true)
	b := w.UpsertPlayer(NewPlayerRec("b", "Briar"), true)
	b.X, b.Y = a.X, a.Y
	g := w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, a.ID)
	if _, ok := w.LightFire(b.ID, g.ID); ok {
		t.Fatal("lit someone else's reserved logs")
	}
}

// A campfire cooks like the hearth while it lasts.
func TestCampfireCooks(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemLog, N: 0}, {ID: protocol.ItemNut, N: 1}}
	g := w.dropPile(p.X+1, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)
	if _, ok := w.LightFire("p1", g.ID); !ok {
		t.Fatal("setup: could not light the fire")
	}
	var fire *Node
	for _, n := range w.Nodes {
		if n.temporary() {
			fire = n
		}
	}
	w.SetInteract("p1", fire.ID)
	for i := 0; i < roastTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemRoast) != 1 {
		t.Fatalf("the campfire did not roast the nut: %+v", p.Inv)
	}
}

func TestCampfireBurnsOut(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)
	w.LightFire("p1", g.ID)
	before := len(w.Nodes)
	for i := 0; i < campfireBurnTicks+2; i++ {
		w.Tick(context.Background())
	}
	if len(w.Nodes) != before-1 {
		t.Fatalf("the campfire outlived its logs: %d nodes, was %d", len(w.Nodes), before)
	}
}

// A campfire is a passing thing and must never be written to the store,
// where it would come back after a restart with nothing burning.
func TestCampfireIsNotPersisted(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)
	w.LightFire("p1", g.ID)
	for i := 0; i < 20; i++ {
		w.Tick(context.Background())
	}
	for id := range st.nodes {
		if len(id) >= 8 && id[:8] == "campfire" {
			t.Fatalf("a campfire was written to the store: %s", id)
		}
	}
}

// Trees walled in by other trees are scenery, not nodes.
func TestUnreachableTreesAreNotNodes(t *testing.T) {
	w := testWorld(t, newMem())
	for _, n := range w.Nodes {
		if n.Kind != KindTree {
			continue
		}
		if _, _, ok := w.nearestAdjacent(n.X, n.Y, n.X, n.Y); !ok {
			t.Fatalf("%s at (%d,%d) is a node but cannot be reached", n.ID, n.X, n.Y)
		}
	}
}

// The state frame carries only nodes that are not at rest. With 72 trees
// on the map, sending them all every tick was most of the frame.
func TestSnapshotOmitsNodesAtRest(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)

	if got := len(w.Snapshot("p1").Nodes); got != 0 {
		t.Fatalf("a quiet world sent %d nodes; it should send none", got)
	}
	if got := len(w.AllNodes()); got < 70 {
		t.Fatalf("the welcome carries %d nodes, expected the full map", got)
	}

	tree := firstNodeOfKind(w, KindTree)
	standBeside(t, w, p, tree)
	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+3; i++ {
		w.Tick(context.Background())
	}
	sent := w.Snapshot("p1").Nodes
	if len(sent) != 1 || sent[0].ID != tree.ID {
		t.Fatalf("expected just the chopped tree, got %+v", sent)
	}
}

// Foraging must still pay foraging. Chopping was bolted onto the same
// branch, and an earlier pass at it silently deleted the experience award
// for both.
func TestForagingStillTrainsForaging(t *testing.T) {
	w := testWorld(t, newMem())
	bush := firstNodeOfKind(w, KindBush)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, bush)
	w.SetInteract("p1", bush.ID)
	for i := 0; i < forageTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("no berry: %+v", p.Inv)
	}
	if got := p.Skills[protocol.SkillForage].XP; got != forageXP {
		t.Fatalf("foraging xp = %d, want %d", got, forageXP)
	}
	if p.Skills[protocol.SkillWood].XP != 0 {
		t.Fatal("foraging gave woodcutting experience")
	}
}
