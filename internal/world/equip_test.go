package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Equipping takes the item out of the pack. That is the whole point: a
// sword that cost no space was strictly better than carrying it.
func TestEquippingLeavesThePack(t *testing.T) {
	w := testWorld(t, newMem())
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemSword)
	if countItem(p.Inv, protocol.ItemSword) != 1 {
		t.Fatal("setup")
	}
	if msg, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); !ok {
		t.Fatalf("equip refused: %s", msg)
	}
	if countItem(p.Inv, protocol.ItemSword) != 0 {
		t.Fatal("the sword is equipped and still in the pack")
	}
	if p.Equipped[protocol.SlotHand] != protocol.ItemSword {
		t.Fatalf("hand holds %q", p.Equipped[protocol.SlotHand])
	}
	// And back again.
	if _, ok := w.Unequip(context.Background(), "p1", protocol.SlotHand); !ok {
		t.Fatal("unequip refused")
	}
	if countItem(p.Inv, protocol.ItemSword) != 1 || p.Equipped[protocol.SlotHand] != "" {
		t.Fatalf("unequip went wrong: inv=%+v equipped=%+v", p.Inv, p.Equipped)
	}
}

// One hand, one thing. Equipping the axe stows the sword.
func TestOneThingInHandAtATime(t *testing.T) {
	w := testWorld(t, newMem())
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true),
		protocol.ItemSword, protocol.ItemAxe)
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); !ok {
		t.Fatal("could not wield the sword")
	}
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemAxe); !ok {
		t.Fatal("could not take up the axe")
	}
	if p.Equipped[protocol.SlotHand] != protocol.ItemAxe {
		t.Fatalf("hand holds %q, want the axe", p.Equipped[protocol.SlotHand])
	}
	if countItem(p.Inv, protocol.ItemSword) != 1 {
		t.Fatal("the sword did not go back in the pack when swapped out")
	}
}

// The trade-off the slot exists to create: carrying an axe is no longer
// enough to chop with it.
func TestCarryingAnAxeIsNotWieldingOne(t *testing.T) {
	w := testWorld(t, newMem())
	tree := firstNodeOfKind(w, KindTree)
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true),
		protocol.ItemAxe, protocol.ItemSword)
	// Sword in hand, axe merely in the pack.
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); !ok {
		t.Fatal("could not wield the sword")
	}
	standBeside(t, w, p, tree)
	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemLog) != 0 {
		t.Fatal("chopped a tree with a sword in hand and the axe in the pack")
	}

	// Swap, and the same click works.
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemAxe); !ok {
		t.Fatal("could not take up the axe")
	}
	w.SetInteract("p1", tree.ID)
	for i := 0; i < chopTicks+4; i++ {
		w.Tick(context.Background())
	}
	if countItem(p.Inv, protocol.ItemLog) != 1 {
		t.Fatalf("axe in hand still would not chop: %+v", p.Inv)
	}
}

// Flint is used from the pack, not worn — it has no slot.
func TestFlintNeedsNoSlot(t *testing.T) {
	if protocol.EquipSlot(protocol.ItemFlint) != "" {
		t.Fatal("flint and steel should not be equipment")
	}
	w := testWorld(t, newMem())
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemFlint)
	g := w.dropPile(p.X, p.Y, []ItemStack{{ID: protocol.ItemLog, N: 1}}, 0, p.ID)
	if msg, ok := w.LightFire("p1", g.ID); !ok {
		t.Fatalf("flint in the pack should light a fire: %s", msg)
	}
}

// Worn armour absorbs blows.
func TestJerkinReducesDamageTaken(t *testing.T) {
	w := testWorld(t, newMem())
	took := func(worn bool) int {
		npc := w.npcByID("npc-brambleback")
		npc.HP = npc.MaxHP
		npc.Target = ""
		id := "bare"
		if worn {
			id = "clad"
		}
		p := w.UpsertPlayer(NewPlayerRec(id, "Kyle"), true)
		if worn {
			withTools(p, protocol.ItemJerkin)
			if _, ok := w.Equip(context.Background(), id, protocol.ItemJerkin); !ok {
				t.Fatal("could not wear the jerkin")
			}
		}
		p.X, p.Y = npc.X, npc.Y-1
		w.SetAttack(id, npc.ID)
		before := p.HP
		w.tickCombat(context.Background(), p)
		return before - p.HP
	}
	bare, clad := took(false), took(true)
	if clad >= bare {
		t.Fatalf("the jerkin absorbed nothing: %d damage bare, %d clad", bare, clad)
	}
}

// Worn gear survives a logout like everything else on the record.
func TestEquipmentPersists(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemSword)
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); !ok {
		t.Fatal("equip failed")
	}
	w.PersistPlayer(context.Background(), "p1")

	rec, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || rec == nil {
		t.Fatalf("load: %v %v", rec, err)
	}
	if rec.Equipped[protocol.SlotHand] != protocol.ItemSword {
		t.Fatalf("stored equipment = %+v", rec.Equipped)
	}
	w2 := testWorld(t, st)
	if got := w2.UpsertPlayer(rec, true).Equipped[protocol.SlotHand]; got != protocol.ItemSword {
		t.Fatalf("after rejoin the hand holds %q", got)
	}
	_ = p
}

// A stored record must not share its map with the live player, or one
// mutates the other behind its back.
func TestEquipmentIsCopiedNotShared(t *testing.T) {
	w := testWorld(t, newMem())
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemSword)
	w.Equip(context.Background(), "p1", protocol.ItemSword)
	rec := recFromPlayer(p)
	rec.Equipped[protocol.SlotHand] = "moonbeam"
	if p.Equipped[protocol.SlotHand] != protocol.ItemSword {
		t.Fatal("changing the record changed the live player")
	}
}

// Equipping what is already in the slot used to swap the item with
// itself: one out of the pack, the identical one back in, a Postgres
// write for nothing, and "You take up the briar sword, stowing the briar
// sword" in the log.
func TestEquippingWhatYouAlreadyHoldIsANoop(t *testing.T) {
	w := testWorld(t, newMem())
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true),
		protocol.ItemSword, protocol.ItemSword)
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); !ok {
		t.Fatal("could not wield the sword")
	}
	spare := countItem(p.Inv, protocol.ItemSword)
	msg, ok := w.Equip(context.Background(), "p1", protocol.ItemSword)
	if ok {
		t.Fatal("equipped the sword twice")
	}
	if countItem(p.Inv, protocol.ItemSword) != spare {
		t.Fatal("the spare sword moved")
	}
	if msg == "" {
		t.Fatal("no word about it either way")
	}
}

// You cannot wear a brambleberry.
func TestOnlyEquipmentCanBeEquipped(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemBerry); ok {
		t.Fatal("equipped a brambleberry")
	}
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); ok {
		t.Fatal("equipped a sword that was never held")
	}
}

// A store failure must leave both the pack and the slots alone.
func TestEquipIsAbandonedIfTheStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := withTools(w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true), protocol.ItemSword)
	st.fail = true
	if _, ok := w.Equip(context.Background(), "p1", protocol.ItemSword); ok {
		t.Fatal("equipped while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemSword) != 1 {
		t.Fatal("the sword left the pack without a commit")
	}
	if p.Equipped[protocol.SlotHand] != "" {
		t.Fatal("the hand was filled without a commit")
	}
}

// Every slot the client shows must be one some item can go in.
func TestEverySlotHasSomethingToPutInIt(t *testing.T) {
	for _, slot := range protocol.EquipSlots() {
		found := false
		for id := range protocol.Catalog() {
			if protocol.EquipSlot(id) == slot {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("slot %q has no item that fits it", slot)
		}
	}
}
