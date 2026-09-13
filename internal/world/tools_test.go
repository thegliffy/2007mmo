package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Without an axe a tree is just a tree.
func TestCannotChopWithoutAnAxe(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, tree)

	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemLog) != 0 {
		t.Fatal("chopped a tree bare-handed")
	}
	if tree.Remaining != treeYield {
		t.Fatalf("the tree lost %d boughs to someone with no axe", treeYield-tree.Remaining)
	}
	if note := w.TakeNote("p1"); note == "" {
		t.Error("the player was told nothing about why nothing happened")
	}
}

// And with one, the same click works.
func TestAxeMakesTheDifference(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemAxe)
	standBeside(t, w, p, tree)
	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemLog) != 1 {
		t.Fatalf("an axe-carrying player got no log: %+v", p.Inv)
	}
}

func TestCannotLightWithoutFlint(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)
	msg, ok := w.LightFire("p1", g.ID)
	if ok {
		t.Fatal("lit a fire with no flint and steel")
	}
	if msg == "" {
		t.Error("the player was told nothing")
	}
	if w.groundByID(g.ID) == nil {
		t.Fatal("the logs were consumed by a fire that never lit")
	}
}

// A blade adds levels and a flat point on top.
func TestSwordAddsAttackAndDamage(t *testing.T) {
	w := testWorld(t, newMem())
	hit := func(withSword bool) int {
		npc := w.npcByID("npc-thornkin-1")
		npc.HP = npc.MaxHP
		npc.Target = ""
		id := "bare"
		if withSword {
			id = "armed"
		}
		p := w.UpsertPlayer(NewPlayerRec(id, "Kyle"), true)
		if withSword {
			withTools(p, protocol.ItemSword)
		}
		p.X, p.Y = npc.X, npc.Y-1
		w.SetAttack(id, npc.ID)
		before := npc.HP
		w.tickCombat(context.Background(), p)
		return before - npc.HP
	}
	bare := hit(false)
	armed := hit(true)
	if armed <= bare {
		t.Fatalf("the sword did nothing: %d damage bare, %d armed", bare, armed)
	}
	info := protocol.Catalog()[protocol.ItemSword]
	want := meleeDamage(1+info.Attack) + info.Damage
	if armed != want {
		t.Fatalf("armed hit for %d, expected %d (level 1 + %d attack, +%d damage)",
			armed, want, info.Attack, info.Damage)
	}
}

// Tools take a slot each and never stack, so a pack of axes costs what it
// looks like it costs.
func TestToolsDoNotStack(t *testing.T) {
	var inv []ItemStack
	inv = addItem(inv, protocol.ItemAxe, 1)
	inv = addItem(inv, protocol.ItemAxe, 1)
	if len(inv) != 2 {
		t.Fatalf("two axes took %d slots, want 2", len(inv))
	}
	if countItem(inv, protocol.ItemAxe) != 2 {
		t.Fatalf("counted %d axes", countItem(inv, protocol.ItemAxe))
	}
	// Berries still stack.
	var berries []ItemStack
	berries = addItem(berries, protocol.ItemBerry, 1)
	berries = addItem(berries, protocol.ItemBerry, 4)
	if len(berries) != 1 || berries[0].N != 5 {
		t.Fatalf("berries stopped stacking: %+v", berries)
	}
}

// Dropping one tool must take one slot, not clear every copy.
func TestRemovingOneToolLeavesTheRest(t *testing.T) {
	var inv []ItemStack
	inv = addItem(inv, protocol.ItemAxe, 3)
	inv = removeItem(inv, protocol.ItemAxe, 1)
	if countItem(inv, protocol.ItemAxe) != 2 {
		t.Fatalf("removing one axe left %d", countItem(inv, protocol.ItemAxe))
	}
	if len(inv) != 2 {
		t.Fatalf("slots = %d, want 2", len(inv))
	}
}

// A full pack cannot take another tool.
func TestToolsRespectThePackLimit(t *testing.T) {
	var inv []ItemStack
	for i := 0; i < protocol.InvSlots+5; i++ {
		inv = addItem(inv, protocol.ItemAxe, 1)
	}
	if len(inv) != protocol.InvSlots {
		t.Fatalf("pack grew to %d slots, limit is %d", len(inv), protocol.InvSlots)
	}
}

// The pedlar has to actually stock the tools, or they are unobtainable.
func TestPedlarStocksEveryTool(t *testing.T) {
	for id, info := range protocol.Catalog() {
		if !info.Tool {
			continue
		}
		o, ok := offerFor(id)
		if !ok || o.Costs <= 0 {
			t.Errorf("%s (%s) is a tool nobody sells, so it can never be got", id, info.Name)
		}
	}
}

// Each tool verb must map to exactly one tool, or hasTool is ambiguous.
func TestEveryVerbHasOneTool(t *testing.T) {
	seen := map[string]string{}
	for id, info := range protocol.Catalog() {
		if !info.Tool || info.Verb == "" {
			continue
		}
		if other, clash := seen[info.Verb]; clash {
			t.Errorf("verb %q is claimed by both %s and %s", info.Verb, other, id)
		}
		seen[info.Verb] = id
	}
	for _, verb := range []string{"chop", "light"} {
		if protocol.ToolFor(verb) == "" {
			t.Errorf("no tool provides %q", verb)
		}
	}
	// Work with no tool requirement stays unblocked.
	if !hasTool(nil, "forage") {
		t.Error("foraging should need no tool")
	}
}
