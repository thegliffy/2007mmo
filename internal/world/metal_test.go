package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func withPick(p *Player) *Player {
	return withTools(p, protocol.ItemPick)
}

// Without a pick a vein is just a rock.
func TestCannotMineWithoutAPick(t *testing.T) {
	w := testWorld(t, newMem())
	vein := firstNodeOfKind(w, KindCopper)
	if vein == nil {
		t.Fatal("no copper veins on the map")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, vein)

	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 0 {
		t.Fatal("mined copper bare-handed")
	}
	if vein.Remaining != oreYield {
		t.Fatalf("the vein lost %d lumps to someone with no pick", oreYield-vein.Remaining)
	}
	if note := w.TakeNote("p1"); note == "" {
		t.Error("the player was told nothing about why nothing happened")
	}
}

func TestPickMakesTheDifference(t *testing.T) {
	w := testWorld(t, newMem())
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 {
		t.Fatalf("a pick-carrying player got no copper: %+v", p.Inv)
	}
	if sk := p.Skills[protocol.SkillMine]; sk.XP != mineXP {
		t.Fatalf("mining xp = %d, want %d", sk.XP, mineXP)
	}
}

func TestMiningDoesNotTrainWoodcutting(t *testing.T) {
	w := testWorld(t, newMem())
	vein := firstNodeOfKind(w, KindTin)
	if vein == nil {
		t.Fatal("no tin veins on the map")
	}
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemTin) != 1 {
		t.Fatalf("no tin: %+v", p.Inv)
	}
	if p.Skills[protocol.SkillWood].XP != 0 {
		t.Fatal("mining gave woodcutting experience")
	}
	if p.Skills[protocol.SkillForage].XP != 0 {
		t.Fatal("mining gave foraging experience")
	}
}

func TestVeinDepletesAndRegrows(t *testing.T) {
	w := testWorld(t, newMem())
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)

	for i := 0; i < oreYield; i++ {
		w.SetInteract("p1", vein.ID)
		for j := 0; j < mineTicks+3; j++ {
			w.Tick(context.Background())
		}
	}
	if vein.Remaining != 0 {
		t.Fatalf("vein still has %d after %d swings", vein.Remaining, oreYield)
	}
	if nodeReady(vein) {
		t.Fatal("a spent vein should not read as ready")
	}
	for i := 0; i < oreCD+2; i++ {
		w.Tick(context.Background())
	}
	if vein.Remaining != oreYield {
		t.Fatalf("vein did not come back: %d", vein.Remaining)
	}
}

func TestKilnNeedsBothOres(t *testing.T) {
	w := testWorld(t, newMem())
	kiln := firstNodeOfKind(w, KindKiln)
	if kiln == nil {
		t.Fatal("no kiln on the map")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}}
	standBeside(t, w, p, kiln)

	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 0 {
		t.Fatal("smelted a bar from copper alone")
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 {
		t.Fatal("the copper was taken for a smelt that did not happen")
	}
	if w.TakeNote("p1") == "" {
		t.Error("the kiln said nothing")
	}
}

func TestKilnSmeltsBronze(t *testing.T) {
	w := testWorld(t, newMem())
	kiln := firstNodeOfKind(w, KindKiln)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}, {ID: protocol.ItemTin, N: 1}}
	standBeside(t, w, p, kiln)

	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 1 {
		t.Fatalf("no bar: %+v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemCopper) != 0 || countItem(p.Inv, protocol.ItemTin) != 0 {
		t.Fatalf("the ores survived the kiln: %+v", p.Inv)
	}
	if p.Skills[protocol.SkillSmith].XP != smeltXP {
		t.Fatalf("smithing xp = %d, want %d", p.Skills[protocol.SkillSmith].XP, smeltXP)
	}
}

func TestAnvilForgesAKnife(t *testing.T) {
	w := testWorld(t, newMem())
	anvil := firstNodeOfKind(w, KindAnvil)
	if anvil == nil {
		t.Fatal("no anvil on the map")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBar, N: 1}}
	standBeside(t, w, p, anvil)

	w.SetInteract("p1", anvil.ID)
	for i := 0; i < forgeTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemKnife) != 1 {
		t.Fatalf("no knife: %+v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemBar) != 0 {
		t.Fatal("the bar was not consumed")
	}
	if p.Skills[protocol.SkillSmith].XP != forgeXP {
		t.Fatalf("smithing xp = %d, want %d", p.Skills[protocol.SkillSmith].XP, forgeXP)
	}
}

func TestAnvilWaitsForABar(t *testing.T) {
	w := testWorld(t, newMem())
	anvil := firstNodeOfKind(w, KindAnvil)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}}
	standBeside(t, w, p, anvil)
	w.SetInteract("p1", anvil.ID)
	for i := 0; i < forgeTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemKnife) != 0 {
		t.Fatal("forged a knife from an ore")
	}
	if w.TakeNote("p1") == "" {
		t.Error("the anvil said nothing")
	}
}

// The whole beginner loop, one sitting: pick, both ores, bar, knife.
func TestBeginnerMetalLoop(t *testing.T) {
	w := testWorld(t, newMem())
	copper := firstNodeOfKind(w, KindCopper)
	tin := firstNodeOfKind(w, KindTin)
	kiln := firstNodeOfKind(w, KindKiln)
	anvil := firstNodeOfKind(w, KindAnvil)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))

	standBeside(t, w, p, copper)
	w.SetInteract("p1", copper.ID)
	for i := 0; i < mineTicks+3; i++ {
		w.Tick(context.Background())
	}
	standBeside(t, w, p, tin)
	w.SetInteract("p1", tin.ID)
	for i := 0; i < mineTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 || countItem(p.Inv, protocol.ItemTin) != 1 {
		t.Fatalf("expected both ores: %+v", p.Inv)
	}

	standBeside(t, w, p, kiln)
	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 1 {
		t.Fatalf("expected a bar: %+v", p.Inv)
	}

	standBeside(t, w, p, anvil)
	w.SetInteract("p1", anvil.ID)
	for i := 0; i < forgeTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemKnife) != 1 {
		t.Fatalf("expected a knife: %+v", p.Inv)
	}
	if p.Skills[protocol.SkillMine].XP != mineXP*2 {
		t.Fatalf("mining xp = %d, want %d", p.Skills[protocol.SkillMine].XP, mineXP*2)
	}
	if p.Skills[protocol.SkillSmith].XP != smeltXP+forgeXP {
		t.Fatalf("smithing xp = %d, want %d", p.Skills[protocol.SkillSmith].XP, smeltXP+forgeXP)
	}
}

// A store failure must not mint the ore or spend the vein.
func TestMineAbortedIfStoreFails(t *testing.T) {
	st := newMem()
	st.fail = true
	w := testWorld(t, st)
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	before := vein.Remaining
	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 0 {
		t.Fatalf("memory should be unchanged on persist fail, inv=%v", p.Inv)
	}
	if vein.Remaining != before {
		t.Fatal("the vein mutated without a commit")
	}
}

func TestSmeltAbortedIfStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	kiln := firstNodeOfKind(w, KindKiln)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}, {ID: protocol.ItemTin, N: 1}}
	standBeside(t, w, p, kiln)
	st.fail = true
	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 0 {
		t.Fatal("a bar appeared while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 || countItem(p.Inv, protocol.ItemTin) != 1 {
		t.Fatalf("the ores left the pack without a commit: %+v", p.Inv)
	}
}

func TestForgeAbortedIfStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	anvil := firstNodeOfKind(w, KindAnvil)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBar, N: 1}}
	standBeside(t, w, p, anvil)
	st.fail = true
	w.SetInteract("p1", anvil.ID)
	for i := 0; i < forgeTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemKnife) != 0 {
		t.Fatal("a knife appeared while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemBar) != 1 {
		t.Fatal("the bar left the pack without a commit")
	}
}

func TestKnifeAddsAttackAndDamage(t *testing.T) {
	w := testWorld(t, newMem())
	hit := func(withKnife bool) int {
		npc := w.npcByID("npc-thornkin-1")
		npc.HP = npc.MaxHP
		npc.Target = ""
		id := "bare"
		if withKnife {
			id = "knifed"
		}
		p := w.UpsertPlayer(NewPlayerRec(id, "Kyle"), true)
		if withKnife {
			withTools(p, protocol.ItemKnife)
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
		t.Fatalf("the knife did nothing: %d damage bare, %d armed", bare, armed)
	}
	info := protocol.Catalog()[protocol.ItemKnife]
	want := meleeDamage(1+info.Attack) + info.Damage
	if armed != want {
		t.Fatalf("armed hit for %d, expected %d", armed, want)
	}
}

func TestPathFromStileToTheScars(t *testing.T) {
	w := testWorld(t, newMem())
	if w.W < 40 || w.H < 32 {
		t.Fatalf("the scars need a wider map than 28×32, got %dx%d", w.W, w.H)
	}
	kiln := firstNodeOfKind(w, KindKiln)
	anvil := firstNodeOfKind(w, KindAnvil)
	if kiln == nil || anvil == nil {
		t.Fatal("kiln or anvil missing")
	}
	if kiln.X < 27 || anvil.X < 27 {
		t.Fatalf("kiln/anvil should sit in the eastern scars, kiln=%d,%d anvil=%d,%d",
			kiln.X, kiln.Y, anvil.X, anvil.Y)
	}
	kx, ky, ok := w.nearestAdjacent(spawnX, spawnY, kiln.X, kiln.Y)
	if !ok {
		t.Fatal("nowhere to stand beside the kiln")
	}
	if len(w.FindPath(spawnX, spawnY, kx, ky)) == 0 && (kx != spawnX || ky != spawnY) {
		t.Fatal("no path from the stile to the kiln")
	}
	ax, ay, ok := w.nearestAdjacent(spawnX, spawnY, anvil.X, anvil.Y)
	if !ok {
		t.Fatal("nowhere to stand beside the anvil")
	}
	if len(w.FindPath(spawnX, spawnY, ax, ay)) == 0 && (ax != spawnX || ay != spawnY) {
		t.Fatal("no path from the stile to the anvil")
	}
	// Hamlet and south must still be where they were.
	if w.Tiles[1][1] != 'T' {
		t.Fatal("hamlet tree at 1,1 moved")
	}
	if w.Tiles[5][4] != '*' {
		t.Fatal("hearth moved")
	}
	if len(w.FindPath(spawnX, spawnY, 16, 26)) == 0 {
		t.Fatal("south path into the clearing broke")
	}
}

func TestUnreachableVeinsAreNotNodes(t *testing.T) {
	w := testWorld(t, newMem())
	nCopper, nTin := 0, 0
	for _, n := range w.Nodes {
		if n.Kind != KindCopper && n.Kind != KindTin {
			continue
		}
		if n.Kind == KindCopper {
			nCopper++
		} else {
			nTin++
		}
		if _, _, ok := w.nearestAdjacent(n.X, n.Y, n.X, n.Y); !ok {
			t.Fatalf("%s at (%d,%d) is a node but cannot be reached", n.ID, n.X, n.Y)
		}
	}
	if nCopper < 4 || nTin < 4 {
		t.Fatalf("the scars should have several of each vein, copper=%d tin=%d", nCopper, nTin)
	}
}
