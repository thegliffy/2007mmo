package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// standOn puts a player on a tile with a pile already there.
func pileAtPlayer(t *testing.T, w *World, p *Player, inv []ItemStack, coins int) *GroundItem {
	t.Helper()
	g := w.dropPile(p.X, p.Y, inv, coins, p.ID)
	if g == nil {
		t.Fatal("setup: no pile was created")
	}
	return g
}

func TestDroppingAnItemLeavesItOnTheGround(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 2}}

	msg, ok := w.DropItem(context.Background(), "p1", protocol.ItemBerry)
	if !ok {
		t.Fatalf("drop refused: %s", msg)
	}
	if got := countItem(p.Inv, protocol.ItemBerry); got != 1 {
		t.Fatalf("pack holds %d berries, want 1", got)
	}
	if len(w.Ground) != 1 {
		t.Fatalf("expected one pile, got %d", len(w.Ground))
	}
	for _, g := range w.Ground {
		if g.X != p.X || g.Y != p.Y {
			t.Fatalf("pile at (%d,%d), player at (%d,%d)", g.X, g.Y, p.X, p.Y)
		}
		if countItem(g.Inv, protocol.ItemBerry) != 1 {
			t.Fatalf("pile holds %+v", g.Inv)
		}
	}
	// And the store agrees the pack shrank.
	rec, _ := st.LoadPlayer(context.Background(), "p1")
	if countItem(rec.Inv, protocol.ItemBerry) != 1 {
		t.Fatalf("store still shows %d berries", countItem(rec.Inv, protocol.ItemBerry))
	}
}

func TestDroppingWhatYouDoNotHaveIsRefused(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	_ = p
	if _, ok := w.DropItem(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("dropped a tart that was never held")
	}
	if len(w.Ground) != 0 {
		t.Fatal("a pile appeared from nothing")
	}
}

// A failed commit must leave the pack alone and put nothing on the floor,
// or dropping would be a way to duplicate.
func TestDropIsAbandonedIfTheStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	st.fail = true

	if _, ok := w.DropItem(context.Background(), "p1", protocol.ItemBerry); ok {
		t.Fatal("drop reported success while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatal("the berry left the pack without a commit")
	}
	if len(w.Ground) != 0 {
		t.Fatal("a pile was created without a commit — that is a duplicate")
	}
}

func TestWalkingOntoAPileCollectsIt(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := pileAtPlayer(t, w, p, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 12)

	w.SetPickup("p1", g.ID)
	for i := 0; i < 6 && len(w.Ground) > 0; i++ {
		w.Tick(context.Background())
	}
	if len(w.Ground) != 0 {
		t.Fatalf("pile still there after collecting: %d", len(w.Ground))
	}
	if p.Coins != 12 {
		t.Fatalf("coins = %d, want 12", p.Coins)
	}
	if countItem(p.Inv, protocol.ItemLeather) != 1 {
		t.Fatalf("leather not in the pack: %+v", p.Inv)
	}
}

// The pile is emptied before the pack is committed, so a store failure
// loses nothing and duplicates nothing: the pile comes back intact.
func TestFailedPickupLeavesThePileIntact(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := pileAtPlayer(t, w, p, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 9)

	st.fail = true
	w.SetPickup("p1", g.ID)
	for i := 0; i < 6; i++ {
		w.Tick(context.Background())
	}
	if p.Coins != 0 || countItem(p.Inv, protocol.ItemLeather) != 0 {
		t.Fatalf("player gained %d coins and %d leather without a commit",
			p.Coins, countItem(p.Inv, protocol.ItemLeather))
	}
	if len(w.Ground) != 1 {
		t.Fatalf("the pile was lost on a failed pickup: %d piles", len(w.Ground))
	}
	back := w.groundByID(g.ID)
	if back == nil || back.Coins != 9 || countItem(back.Inv, protocol.ItemLeather) != 1 {
		t.Fatalf("the pile came back changed: %+v", back)
	}
}

// You have to actually be standing on it.
func TestPickupNeedsYouOnTheTile(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := w.dropPile(p.X+3, p.Y, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 5, p.ID)

	p.ActionGround = g.ID
	p.HasDest = false // pretend we arrived somewhere else
	w.tickPickup(context.Background(), p)
	if p.Coins != 0 {
		t.Fatal("collected a pile from across the hamlet")
	}
	if len(w.Ground) != 1 {
		t.Fatal("the pile vanished without being collected")
	}
}

func TestPilesMergeOnTheSameTile(t *testing.T) {
	w := testWorld(t, newMem())
	w.dropPile(9, 9, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 5, "p1")
	w.dropPile(9, 9, []ItemStack{{ID: protocol.ItemBerry, N: 2}}, 7, "p1")
	if len(w.Ground) != 1 {
		t.Fatalf("expected one merged pile, got %d", len(w.Ground))
	}
	for _, g := range w.Ground {
		if g.Coins != 12 || countItem(g.Inv, protocol.ItemBerry) != 3 {
			t.Fatalf("merge went wrong: %d coins, %d berries", g.Coins, countItem(g.Inv, protocol.ItemBerry))
		}
	}
}

func TestPilesExpire(t *testing.T) {
	w := testWorld(t, newMem())
	w.dropPile(9, 9, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 0, "p1")
	for i := 0; i < groundExpireTicks+2; i++ {
		w.tickGround()
	}
	if len(w.Ground) != 0 {
		t.Fatal("pile outlived its expiry")
	}
}

// Piles must not grow without bound, or a bored player could fill memory.
func TestPileCountIsCapped(t *testing.T) {
	w := testWorld(t, newMem())
	for i := 0; i < maxGroundPiles*2; i++ {
		w.dropPile(1+i%20, 1+(i/20)%20, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 0, "p1")
	}
	if len(w.Ground) > maxGroundPiles {
		t.Fatalf("%d piles, cap is %d", len(w.Ground), maxGroundPiles)
	}
}

// A felled beast leaves its loot where it died rather than teleporting it
// into the killer's pack.
func TestBeastLeavesItsLootOnTheGround(t *testing.T) {
	w := testWorld(t, newMem())
	w.SeedLoot(7)
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20)
	p.X, p.Y = npc.X, npc.Y-1
	deadX, deadY := npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	for i := 0; i < 12 && npc.Living(); i++ {
		w.tickCombat(context.Background(), p)
	}
	if npc.Living() {
		t.Fatal("setup: the thornkin did not fall")
	}
	if p.Coins != 0 {
		t.Fatalf("loot went straight into the purse (%d coins); it should be on the ground", p.Coins)
	}
	if len(w.Ground) != 1 {
		t.Fatalf("expected a pile where it fell, got %d", len(w.Ground))
	}
	for _, g := range w.Ground {
		if g.X != deadX || g.Y != deadY {
			t.Fatalf("pile at (%d,%d), beast fell at (%d,%d)", g.X, g.Y, deadX, deadY)
		}
		if g.Coins <= 0 {
			t.Fatalf("pile has no coins: %+v", g)
		}
	}
}

// Piles are visible to everyone, so they belong in the snapshot.
func TestPilesAppearInTheSnapshot(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLeather, N: 2}}, 30, "p1")
	snap := w.Snapshot("p1")
	if len(snap.Ground) != 1 {
		t.Fatalf("snapshot carried %d piles", len(snap.Ground))
	}
	g := snap.Ground[0]
	if g.Coins != 30 {
		t.Fatalf("snapshot coins = %d", g.Coins)
	}
	if g.Label == "" {
		t.Fatal("pile has no label for the client to show")
	}
}

// orderSpy records what the world looked like at the instant the pack was
// committed.
type orderSpy struct {
	*mem
	w             *World
	watch         string
	pileStillHere bool
	sawCommit     bool
}

func (o *orderSpy) CommitAction(ctx context.Context, p *PlayerRec, n *NodeRec) error {
	o.sawCommit = true
	if _, ok := o.w.Ground[o.watch]; ok {
		o.pileStillHere = true
	}
	return o.mem.CommitAction(ctx, p, n)
}

// The ordering that actually matters, and which a failing store cannot
// reveal: at the moment the pack write lands, the pile must already be
// gone. If it is still there, a crash in that gap leaves the item in
// Postgres AND on the ground — a duplicate, which is the one thing the
// persistence rule exists to prevent.
func TestPileIsGoneBeforeThePackIsCommitted(t *testing.T) {
	base := newMem()
	w := testWorld(t, base)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	g := pileAtPlayer(t, w, p, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 20)

	spy := &orderSpy{mem: base, w: w, watch: g.ID}
	w.Store = spy

	p.ActionGround = g.ID
	p.HasDest = false
	w.tickPickup(context.Background(), p)

	if !spy.sawCommit {
		t.Fatal("the pickup never committed anything")
	}
	if spy.pileStillHere {
		t.Fatal("the pile was still on the ground when the pack was written; " +
			"a crash there would duplicate the item")
	}
	if p.Coins != 20 {
		t.Fatalf("coins = %d, want 20", p.Coins)
	}
}

// two returns a world with two players standing on the same tile.
func twoPlayers(t *testing.T, w *World) (*Player, *Player) {
	t.Helper()
	a := w.UpsertPlayer(NewPlayerRec("a", "Ash"), true)
	b := w.UpsertPlayer(NewPlayerRec("b", "Briar"), true)
	b.X, b.Y = a.X, a.Y
	return a, b
}

// For the first minute the pile is not merely refused to others — they are
// never told it is there.
func TestReservedPileIsInvisibleToEveryoneElse(t *testing.T) {
	w := testWorld(t, newMem())
	a, b := twoPlayers(t, w)
	w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 25, a.ID)

	mine := w.Snapshot(a.ID)
	if len(mine.Ground) != 1 {
		t.Fatalf("the owner cannot see their own pile: %d", len(mine.Ground))
	}
	if !mine.Ground[0].Mine {
		t.Error("the owner's pile should be marked as theirs")
	}
	theirs := w.Snapshot(b.ID)
	if len(theirs.Ground) != 0 {
		t.Fatalf("a reserved pile leaked into someone else's snapshot: %+v", theirs.Ground)
	}
}

// And it cannot be taken by someone who never saw it, even if they guess
// the id — the server must not trust the client.
func TestReservedPileCannotBeTakenByAnotherPlayer(t *testing.T) {
	w := testWorld(t, newMem())
	a, b := twoPlayers(t, w)
	g := w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemLeather, N: 1}}, 25, a.ID)

	w.SetPickup(b.ID, g.ID)
	for i := 0; i < 6; i++ {
		w.Tick(context.Background())
	}
	if b.Coins != 0 || countItem(b.Inv, protocol.ItemLeather) != 0 {
		t.Fatal("another player took a reserved pile")
	}
	if w.groundByID(g.ID) == nil {
		t.Fatal("the pile vanished when someone else grabbed at it")
	}
}

// After the reserved window it becomes everyone's.
func TestPileGoesPublicAfterTheReservedWindow(t *testing.T) {
	w := testWorld(t, newMem())
	a, b := twoPlayers(t, w)
	g := w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 11, a.ID)

	for i := 0; i < groundPrivateTicks+1; i++ {
		w.tickGround()
	}
	if w.groundByID(g.ID) == nil {
		t.Fatal("the pile expired during its reserved window")
	}
	if len(w.Snapshot(b.ID).Ground) != 1 {
		t.Fatal("the pile is still hidden after the reserved window")
	}
	if w.Snapshot(a.ID).Ground[0].Mine {
		t.Error("a public pile should no longer be marked as the owner's")
	}

	w.SetPickup(b.ID, g.ID)
	for i := 0; i < 6 && w.groundByID(g.ID) != nil; i++ {
		w.Tick(context.Background())
	}
	if b.Coins != 11 {
		t.Fatalf("the other player collected %d coins, want 11", b.Coins)
	}
}

// Total life is the reserved window plus the public one.
func TestPileLivesForBothWindowsThenGoes(t *testing.T) {
	w := testWorld(t, newMem())
	g := w.dropPile(9, 9, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 0, "a")

	for i := 0; i < groundPrivateTicks+groundPublicTicks-2; i++ {
		w.tickGround()
	}
	if w.groundByID(g.ID) == nil {
		t.Fatalf("the pile went early, before %d ticks", groundPrivateTicks+groundPublicTicks)
	}
	for i := 0; i < 4; i++ {
		w.tickGround()
	}
	if w.groundByID(g.ID) != nil {
		t.Fatal("the pile outlived both windows")
	}
}

// Two people dropping on one tile must not pool into one reserved pile.
func TestSeparatePilesWhileReservedToDifferentPeople(t *testing.T) {
	w := testWorld(t, newMem())
	a, b := twoPlayers(t, w)
	w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 5, a.ID)
	w.dropPile(b.X, b.Y, []ItemStack{{ID: protocol.ItemNut, N: 1}}, 7, b.ID)

	if len(w.Ground) != 2 {
		t.Fatalf("expected two separate piles, got %d", len(w.Ground))
	}
	for _, id := range []string{a.ID, b.ID} {
		snap := w.Snapshot(id)
		if len(snap.Ground) != 1 {
			t.Fatalf("%s sees %d piles, should see only their own", id, len(snap.Ground))
		}
	}
	// Once both go public they tidy into one sack.
	for i := 0; i < groundPrivateTicks+1; i++ {
		w.tickGround()
	}
	if len(w.Ground) != 1 {
		t.Fatalf("public piles on one tile did not merge: %d", len(w.Ground))
	}
	for _, g := range w.Ground {
		if g.Coins != 12 {
			t.Fatalf("merged coins = %d, want 12", g.Coins)
		}
	}
}

// Topping up a pile that has already gone public must not reserve it
// again, or somebody could hold a tile forever with a berry a minute.
func TestToppingUpAPublicPileDoesNotReserveIt(t *testing.T) {
	w := testWorld(t, newMem())
	a, b := twoPlayers(t, w)
	g := w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemBerry, N: 1}}, 0, a.ID)
	for i := 0; i < groundPrivateTicks+1; i++ {
		w.tickGround()
	}
	if g.private() {
		t.Fatal("setup: the pile is still reserved")
	}
	w.dropPile(a.X, a.Y, []ItemStack{{ID: protocol.ItemNut, N: 1}}, 0, a.ID)
	if g.private() {
		t.Fatal("adding to a public pile made it reserved again")
	}
	if len(w.Snapshot(b.ID).Ground) != 1 {
		t.Fatal("the pile disappeared from the other player's view")
	}
}

// A beast's loot belongs to whoever felled it.
func TestBeastLootIsReservedForTheKiller(t *testing.T) {
	w := testWorld(t, newMem())
	w.SeedLoot(7)
	npc := w.npcByID("npc-thornkin-1")
	killer := w.UpsertPlayer(NewPlayerRec("killer", "Ash"), true)
	bystander := w.UpsertPlayer(NewPlayerRec("bystander", "Briar"), true)
	killer.Skills[protocol.SkillMelee] = atLevel(20)
	killer.X, killer.Y = npc.X, npc.Y-1
	w.SetAttack(killer.ID, npc.ID)
	for i := 0; i < 12 && npc.Living(); i++ {
		w.tickCombat(context.Background(), killer)
	}
	if npc.Living() {
		t.Fatal("setup: the thornkin did not fall")
	}
	if len(w.Snapshot(killer.ID).Ground) != 1 {
		t.Fatal("the killer cannot see what they felled")
	}
	if len(w.Snapshot(bystander.ID).Ground) != 0 {
		t.Fatal("a bystander can see the killer's loot")
	}
}
