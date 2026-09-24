package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func withRod(t *testing.T, w *World, p *Player) *Player {
	t.Helper()
	return armedWith(t, w, p, protocol.ItemRod)
}

func hearthNode(w *World) *Node {
	return w.Nodes["fire-1"]
}

func TestFishingYieldsAPerch(t *testing.T) {
	w := testWorld(t, newMem())
	spot := firstNodeOfKind(w, KindFish)
	if spot == nil {
		t.Fatal("no fishing spots on the map")
	}
	p := withRod(t, w, w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, spot)

	w.SetInteract("p1", spot.ID)
	for i := 0; i < fishTicks+3; i++ {
		w.Tick(context.Background())
	}
	if got := countItem(p.Inv, protocol.ItemPerch); got != 1 {
		t.Fatalf("pack holds %d perch, want 1 (%+v)", got, p.Inv)
	}
	if sk := p.Skills[protocol.SkillFish]; sk.XP != fishXP {
		t.Fatalf("fishing xp = %d, want %d", sk.XP, fishXP)
	}
	if spot.Remaining != fishYield-1 {
		t.Fatalf("spot has %d left, want %d", spot.Remaining, fishYield-1)
	}
	if p.Skills[protocol.SkillForage].XP != 0 || p.Skills[protocol.SkillWood].XP != 0 || p.Skills[protocol.SkillMine].XP != 0 {
		t.Fatalf("fishing trained another skill: %+v", p.Skills)
	}
}

func TestCannotFishWithoutARod(t *testing.T) {
	w := testWorld(t, newMem())
	spot := firstNodeOfKind(w, KindFish)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, spot)

	w.SetInteract("p1", spot.ID)
	for i := 0; i < fishTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPerch) != 0 {
		t.Fatal("fished bare-handed")
	}
	if spot.Remaining != fishYield {
		t.Fatalf("the spot lost %d fish to someone with no rod", fishYield-spot.Remaining)
	}
	if note := w.TakeNote("p1"); note == "" {
		t.Error("the player was told nothing about why nothing happened")
	}
}

// A rod in the pack is not a rod in the hand. Same rule as the axe.
func TestRodInThePackDoesNotFish(t *testing.T) {
	w := testWorld(t, newMem())
	spot := firstNodeOfKind(w, KindFish)
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemRod)
	standBeside(t, w, p, spot)

	w.SetInteract("p1", spot.ID)
	for i := 0; i < fishTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemPerch) != 0 {
		t.Fatal("fished with the rod still in the pack")
	}
	if spot.Remaining != fishYield {
		t.Fatal("the spot depleted for a rod that was not in hand")
	}
}

func TestFishingSpotDepletesAndReturns(t *testing.T) {
	w := testWorld(t, newMem())
	spot := firstNodeOfKind(w, KindFish)
	p := withRod(t, w, w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, spot)

	for i := 0; i < fishYield; i++ {
		w.SetInteract("p1", spot.ID)
		for j := 0; j < fishTicks+3; j++ {
			w.Tick(context.Background())
		}
	}
	if spot.Remaining != 0 {
		t.Fatalf("spot still has %d after %d casts", spot.Remaining, fishYield)
	}
	if nodeReady(spot) {
		t.Fatal("a spent fishing spot should not read as ready")
	}
	snap := w.Snapshot("p1")
	var onWire bool
	for _, n := range snap.Nodes {
		if n.ID != spot.ID {
			continue
		}
		onWire = true
		if n.Ready {
			t.Fatal("spent spot was sent ready")
		}
	}
	if !onWire {
		t.Fatal("spent spot was left out of the state frame")
	}

	for i := 0; i < fishCD+2; i++ {
		w.Tick(context.Background())
	}
	if spot.Remaining != fishYield || spot.Cooldown != 0 {
		t.Fatalf("spot did not restore: remaining=%d cooldown=%d", spot.Remaining, spot.Cooldown)
	}
	if !nodeReady(spot) {
		t.Fatal("restored spot should read as ready")
	}
	for _, n := range w.Snapshot("p1").Nodes {
		if n.ID == spot.ID {
			t.Fatal("a spot back at rest should be omitted from the state frame")
		}
	}
}

func TestHearthFriesAPerch(t *testing.T) {
	w := testWorld(t, newMem())
	fire := hearthNode(w)
	if fire == nil || fire.Kind != KindFire {
		t.Fatal("the hearth is missing")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemPerch, N: 1}}
	standBeside(t, w, p, fire)

	w.SetInteract("p1", fire.ID)
	for i := 0; i < fryTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemFried) != 1 || countItem(p.Inv, protocol.ItemPerch) != 0 {
		t.Fatalf("perch did not fry: %+v", p.Inv)
	}
	if p.Skills[protocol.SkillCook].XP != fryXP {
		t.Fatalf("cooking xp = %d, want %d", p.Skills[protocol.SkillCook].XP, fryXP)
	}
	if p.Skills[protocol.SkillFish].XP != 0 {
		t.Fatal("frying trained fishing")
	}
}

// Pulp still goes first, the way berries still beat logs at the millstone.
func TestHearthPrefersPulpOverPerch(t *testing.T) {
	w := testWorld(t, newMem())
	fire := hearthNode(w)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemPulp, N: 1}, {ID: protocol.ItemPerch, N: 1}}
	standBeside(t, w, p, fire)

	w.SetInteract("p1", fire.ID)
	for i := 0; i < cookTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemTart) != 1 {
		t.Fatalf("expected a tart from the pulp: %+v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemPerch) != 1 {
		t.Fatal("the perch should be untouched while pulp is waiting")
	}
}

func TestRawPerchIsNotFood(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemPerch, N: 1}}
	if _, ok := w.UseItem(context.Background(), "p1", protocol.ItemPerch); ok {
		t.Fatal("ate a raw perch")
	}
	if countItem(p.Inv, protocol.ItemPerch) != 1 {
		t.Fatal("the raw perch left the pack")
	}
}

func TestFriedPerchHeals(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.HP = 4
	p.Inv = []ItemStack{{ID: protocol.ItemFried, N: 1}}
	msg, ok := w.UseItem(context.Background(), "p1", protocol.ItemFried)
	if !ok || countItem(p.Inv, protocol.ItemFried) != 0 {
		t.Fatalf("did not eat the fried perch: %q %+v", msg, p.Inv)
	}
	if p.HP != 4+friedHeal {
		t.Fatalf("hp = %d, want %d", p.HP, 4+friedHeal)
	}
}

func TestReedwaterOpensEastOfTheHamlet(t *testing.T) {
	w := testWorld(t, newMem())
	if w.W < 60 || w.H < 46 {
		t.Fatalf("reedwater needs a wider map than 40×32, got %dx%d", w.W, w.H)
	}
	if w.Tiles[1][1] != 'T' {
		t.Fatal("hamlet tree at 1,1 moved")
	}
	if w.Tiles[5][4] != '*' {
		t.Fatal("hearth moved")
	}
	if len(w.FindPath(spawnX, spawnY, 16, 26)) == 0 {
		t.Fatal("south path into the clearing broke")
	}
	kiln := firstNodeOfKind(w, KindKiln)
	if kiln == nil || kiln.X < 27 || kiln.X >= 40 {
		t.Fatalf("kiln should stay in the eastern scars, got %+v", kiln)
	}
	// The briar-woods stay sealed from the new shore.
	if w.Tiles[26][39] != '#' || w.Tiles[30][39] != '#' {
		t.Fatal("south-east wall opened into the reedwater")
	}

	spots := 0
	var one *Node
	for _, n := range w.Nodes {
		if n.Kind != KindFish {
			continue
		}
		spots++
		one = n
		if n.X < 40 {
			t.Fatalf("%s sits in the old map at (%d,%d)", n.ID, n.X, n.Y)
		}
		if w.Tiles[n.Y][n.X] != 'F' {
			t.Fatalf("%s is not on a fishing tile", n.ID)
		}
	}
	if spots < 6 {
		t.Fatalf("want several fishing spots, got %d", spots)
	}
	sx, sy, ok := w.nearestAdjacent(spawnX, spawnY, one.X, one.Y)
	if !ok {
		t.Fatalf("%s has nowhere to stand", one.ID)
	}
	if len(w.FindPath(spawnX, spawnY, sx, sy)) == 0 && (sx != spawnX || sy != spawnY) {
		t.Fatalf("no path from the stile to fishing spot %s at (%d,%d)", one.ID, one.X, one.Y)
	}
}
