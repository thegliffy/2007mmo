package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Phase 2 P0: no-dupe coverage for the paths that grew after Week 1 —
// metal (mine / smelt / forge), loot piles, and the pedlar. The rule is
// the same everywhere: Postgres (or the store double) wins; a crash or a
// double-click must not mint a second item.

func persist(t *testing.T, st *mem, p *Player) {
	t.Helper()
	if err := st.SavePlayer(context.Background(), recFromPlayer(p)); err != nil {
		t.Fatal(err)
	}
}

func rebuildFromStore(t *testing.T, st *mem) *World {
	t.Helper()
	w := testWorld(t, st)
	nodes, err := st.LoadNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.RestoreNodes(nodes)
	return w
}

func loadInto(t *testing.T, w *World, st *mem, id string) *Player {
	t.Helper()
	rec, err := st.LoadPlayer(context.Background(), id)
	if err != nil || rec == nil {
		t.Fatalf("load %s: %v %v", id, rec, err)
	}
	return w.UpsertPlayer(rec, true)
}

func startChannel(t *testing.T, w *World, p *Player, nodeID string) {
	t.Helper()
	w.SetInteract(p.ID, nodeID)
	w.Tick(context.Background())
	if !channeling(p.Action) {
		t.Fatalf("setup: expected a channel, action=%q note=%q", p.Action, w.TakeNote(p.ID))
	}
}

func TestCrashMidForageChannelGrantsNothing(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	bush := firstKind(w, KindBush)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, bush)
	persist(t, st, p)
	if err := st.UpsertNode(context.Background(), *recFromNode(bush)); err != nil {
		t.Fatal(err)
	}

	startChannel(t, w, p, bush.ID)
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("mid-channel crash minted a berry: %v", p2.Inv)
	}
	bush2 := w2.Nodes[bush.ID]
	if bush2 == nil || bush2.Remaining != bush.Remaining {
		t.Fatalf("vein/bush remaining changed without a commit: %+v", bush2)
	}
}

func TestCrashMidMineChannelGrantsNothing(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	persist(t, st, p)
	_ = st.UpsertNode(context.Background(), *recFromNode(vein))

	startChannel(t, w, p, vein.ID)
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemCopper) != 0 {
		t.Fatalf("mid-mine crash minted copper: %v", p2.Inv)
	}
}

func TestCrashMidSmeltLeavesTheOres(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	kiln := firstNodeOfKind(w, KindKiln)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}, {ID: protocol.ItemTin, N: 1}}
	standBeside(t, w, p, kiln)
	persist(t, st, p)

	startChannel(t, w, p, kiln.ID)
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemBar) != 0 {
		t.Fatal("mid-smelt crash minted a bar")
	}
	if countItem(p2.Inv, protocol.ItemCopper) != 1 || countItem(p2.Inv, protocol.ItemTin) != 1 {
		t.Fatalf("ores vanished mid-channel: %v", p2.Inv)
	}
}

func TestCrashMidForgeLeavesTheBar(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	anvil := firstNodeOfKind(w, KindAnvil)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBar, N: 1}}
	standBeside(t, w, p, anvil)
	persist(t, st, p)

	startChannel(t, w, p, anvil.ID)
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemKnife) != 0 {
		t.Fatal("mid-forge crash minted a knife")
	}
	if countItem(p2.Inv, protocol.ItemBar) != 1 {
		t.Fatalf("bar vanished mid-channel: %v", p2.Inv)
	}
}

func TestCrashAfterMineNoDupe(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 {
		t.Fatalf("setup: %v", p.Inv)
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemCopper) != 1 {
		t.Fatalf("dupe or loss after mine recovery: %v", p2.Inv)
	}
}

func TestCrashAfterSmeltNoDupe(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	kiln := firstNodeOfKind(w, KindKiln)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}, {ID: protocol.ItemTin, N: 1}}
	standBeside(t, w, p, kiln)
	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 1 {
		t.Fatalf("setup: %v", p.Inv)
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemBar) != 1 || countItem(p2.Inv, protocol.ItemCopper) != 0 {
		t.Fatalf("dupe or loss after smelt recovery: %v", p2.Inv)
	}
}

func TestCrashAfterForgeNoDupe(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	anvil := firstNodeOfKind(w, KindAnvil)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBar, N: 1}}
	standBeside(t, w, p, anvil)
	w.SetInteract("p1", anvil.ID)
	for i := 0; i < forgeTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemKnife) != 1 {
		t.Fatalf("setup: %v", p.Inv)
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemKnife) != 1 || countItem(p2.Inv, protocol.ItemBar) != 0 {
		t.Fatalf("dupe or loss after forge recovery: %v", p2.Inv)
	}
}

func TestDoubleClickForageStillOneBerry(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	bush := firstKind(w, KindBush)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, bush)
	w.SetInteract("p1", bush.ID)
	w.Tick(context.Background())
	w.SetInteract("p1", bush.ID) // restart the channel, do not grant twice
	for i := 0; i < forageTicks+3; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("double-click forage: %v", p.Inv)
	}
	if countItem(st.players["p1"].Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("store disagrees: %v", st.players["p1"].Inv)
	}
}

func TestDoubleClickMineStillOneOre(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	vein := firstNodeOfKind(w, KindCopper)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	w.SetInteract("p1", vein.ID)
	w.Tick(context.Background())
	w.SetInteract("p1", vein.ID)
	for i := 0; i < mineTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemCopper) != 1 {
		t.Fatalf("double-click mine: %v", p.Inv)
	}
}

func TestDoubleClickSmeltStillOneBar(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	kiln := firstNodeOfKind(w, KindKiln)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}, {ID: protocol.ItemTin, N: 1}}
	standBeside(t, w, p, kiln)
	w.SetInteract("p1", kiln.ID)
	w.Tick(context.Background())
	w.SetInteract("p1", kiln.ID)
	for i := 0; i < smeltTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBar) != 1 {
		t.Fatalf("double-click smelt: %v", p.Inv)
	}
	if countItem(p.Inv, protocol.ItemCopper) != 0 || countItem(p.Inv, protocol.ItemTin) != 0 {
		t.Fatalf("ores survived a double-click smelt: %v", p.Inv)
	}
}

func TestDisconnectMidChannelDoesNotCompleteOffline(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	bush := firstKind(w, KindBush)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	standBeside(t, w, p, bush)
	startChannel(t, w, p, bush.ID)
	w.SetOnline("p1", false)
	persist(t, st, p)
	for i := 0; i < forageTicks+6; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemBerry) != 0 {
		t.Fatal("an offline channel completed and granted a berry")
	}
	if countItem(st.players["p1"].Inv, protocol.ItemBerry) != 0 {
		t.Fatal("store gained a berry while the socket was down")
	}
}

func TestDisconnectMidChannelThenCrashLosesTheChannel(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	vein := firstNodeOfKind(w, KindTin)
	p := withPick(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true))
	standBeside(t, w, p, vein)
	persist(t, st, p)
	startChannel(t, w, p, vein.ID)
	w.SetOnline("p1", false)
	persist(t, st, p)

	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemTin) != 0 {
		t.Fatalf("reconnect-after-crash mid-mine duplicated tin: %v", p2.Inv)
	}
	for i := 0; i < mineTicks+4; i++ {
		w2.Tick(context.Background())
	}
	if countItem(p2.Inv, protocol.ItemTin) != 0 {
		t.Fatal("a restored player finished a channel that never committed")
	}
}

func TestDoublePickupOfOnePile(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := pileAtPlayer(t, w, p, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 8)
	w.SetPickup("p1", g.ID)
	w.SetPickup("p1", g.ID)
	for i := 0; i < 8; i++ {
		w.Tick(context.Background())
	}
	if p.Coins != 8 || countItem(p.Inv, protocol.ItemLeather) != 1 {
		t.Fatalf("double pickup: coins=%d inv=%v", p.Coins, p.Inv)
	}
	if len(w.Ground) != 0 {
		t.Fatalf("pile still there: %d", len(w.Ground))
	}
}

func TestTwoPlayersCannotBothTakeAPublicPile(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	a, b := twoPlayers(t, w)
	g := w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 15, a.ID)
	for i := 0; i < groundPrivateTicks+1; i++ {
		w.tickGround()
	}
	w.SetPickup(a.ID, g.ID)
	w.SetPickup(b.ID, g.ID)
	for i := 0; i < 8; i++ {
		w.Tick(context.Background())
	}
	gotCoins := a.Coins + b.Coins
	gotLeather := countItem(a.Inv, protocol.ItemLeather) + countItem(b.Inv, protocol.ItemLeather)
	if gotCoins != 15 || gotLeather != 1 {
		t.Fatalf("pile split: a coins=%d leather=%d; b coins=%d leather=%d",
			a.Coins, countItem(a.Inv, protocol.ItemLeather),
			b.Coins, countItem(b.Inv, protocol.ItemLeather))
	}
}

func TestCrashDropsPilesAndKeepsThePack(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 2}}
	persist(t, st, p)
	if _, ok := w.DropItem(context.Background(), "p1", protocol.ItemBerry); !ok {
		t.Fatal("setup drop")
	}
	if len(w.Ground) != 1 {
		t.Fatal("setup: no pile")
	}
	// Crash: piles are memory-only. The pack in the store is the truth.
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("pack after crash: %v", p2.Inv)
	}
	if len(w2.Ground) != 0 {
		t.Fatal("a pile survived a process death — that is a dupe waiting to happen")
	}
}

func TestCrashAfterSellNoDupeCoins(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atPedlar(t, w, 0)
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}}
	msg, ok := w.Sell(context.Background(), "p1", protocol.ItemCopper)
	if !ok {
		t.Fatalf("setup sell: %s", msg)
	}
	want := p.Coins
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if p2.Coins != want || countItem(p2.Inv, protocol.ItemCopper) != 0 {
		t.Fatalf("sell recovery coins=%d inv=%v", p2.Coins, p2.Inv)
	}
}

func TestCrashAfterBuyNoDupeItem(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	offer, _ := offerFor(protocol.ItemTart)
	_ = atPedlar(t, w, offer.Costs)
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemTart); !ok {
		t.Fatal("setup buy")
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemTart) != 1 || p2.Coins != 0 {
		t.Fatalf("buy recovery coins=%d inv=%v", p2.Coins, p2.Inv)
	}
}

func TestDoubleSellOneItemSellsOnce(t *testing.T) {
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 0)
	p.Inv = []ItemStack{{ID: protocol.ItemTin, N: 1}}
	if _, ok := w.Sell(context.Background(), "p1", protocol.ItemTin); !ok {
		t.Fatal("first sale")
	}
	if _, ok := w.Sell(context.Background(), "p1", protocol.ItemTin); ok {
		t.Fatal("sold the same tin twice")
	}
	offer, _ := offerFor(protocol.ItemTin)
	if p.Coins != offer.Pays || countItem(p.Inv, protocol.ItemTin) != 0 {
		t.Fatalf("double sell: coins=%d inv=%v", p.Coins, p.Inv)
	}
}

func TestDoubleBuyWithExactCoinsBuysOnce(t *testing.T) {
	w := testWorld(t, newMem())
	offer, _ := offerFor(protocol.ItemKnife)
	p := atPedlar(t, w, offer.Costs)
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemKnife); !ok {
		t.Fatal("first buy")
	}
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemKnife); ok {
		t.Fatal("bought a second knife without the coin")
	}
	if countItem(p.Inv, protocol.ItemKnife) != 1 || p.Coins != 0 {
		t.Fatalf("double buy: coins=%d inv=%v", p.Coins, p.Inv)
	}
}
