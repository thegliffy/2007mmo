package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func firstChest(w *World) *Node {
	for _, n := range w.Nodes {
		if n.Kind == KindChest {
			return n
		}
	}
	return nil
}

func atChest(t *testing.T, w *World, id, name string) *Player {
	t.Helper()
	n := firstChest(w)
	if n == nil {
		t.Fatal("there is no oak chest in the hamlet")
	}
	p := w.UpsertPlayer(NewPlayerRec(id, name), true)
	x, y, ok := w.nearestAdjacent(n.X, n.Y, n.X, n.Y)
	if !ok {
		t.Fatal("nowhere to stand beside the chest")
	}
	p.X, p.Y = x, y
	return p
}

func TestChestSitsByTheStileAndBlocks(t *testing.T) {
	w := testWorld(t, newMem())
	n := firstChest(w)
	if n == nil {
		t.Fatal("no chest node")
	}
	if n.X != 7 || n.Y != 8 {
		t.Fatalf("chest at (%d,%d), want (7,8) beside the stile", n.X, n.Y)
	}
	if w.Walkable(n.X, n.Y) {
		t.Fatal("the chest tile must block, like the millstone")
	}
	if w.Walkable(spawnX, spawnY) == false {
		t.Fatal("stile spawn must stay walkable")
	}
	if !adjacent(spawnX, spawnY, n.X, n.Y) {
		t.Fatalf("spawn (%d,%d) should stand beside the chest (%d,%d)", spawnX, spawnY, n.X, n.Y)
	}
}

func TestClickingTheChestOpensTheLid(t *testing.T) {
	w := testWorld(t, newMem())
	n := firstChest(w)
	p := atChest(t, w, "p1", "Kyle")
	w.SetInteract("p1", n.ID)
	if p.ActionChest != n.ID {
		t.Fatalf("clicking the chest did not mark it: %q", p.ActionChest)
	}
	if p.Target != "" || p.ActionTrader != "" {
		t.Fatal("the chest must not start a fight or a pedlar trade")
	}
	w.Tick(context.Background())
	if got := w.TakeBankOpen("p1"); got != chestName {
		t.Fatalf("lid did not open: %q", got)
	}
}

func TestWalkingToTheChestOpensTheLid(t *testing.T) {
	w := testWorld(t, newMem())
	n := firstChest(w)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = 12, 8
	w.SetInteract("p1", n.ID)
	opened := ""
	for i := 0; i < 20; i++ {
		w.Tick(context.Background())
		if opened = w.TakeBankOpen("p1"); opened != "" {
			break
		}
	}
	if opened != chestName {
		t.Fatalf("never opened the chest (at %d,%d)", p.X, p.Y)
	}
	if !adjacent(p.X, p.Y, n.X, n.Y) {
		t.Fatalf("opened from (%d,%d), chest at (%d,%d)", p.X, p.Y, n.X, n.Y)
	}
}

func TestDepositTakesFromPackAndStores(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 3}}

	msg, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1)
	if !ok {
		t.Fatalf("deposit refused: %s", msg)
	}
	if countItem(p.Inv, protocol.ItemBerry) != 2 {
		t.Fatalf("pack: %v", p.Inv)
	}
	if countItem(p.Bank, protocol.ItemBerry) != 1 {
		t.Fatalf("chest: %v", p.Bank)
	}
	stored := st.players["p1"]
	if countItem(stored.Inv, protocol.ItemBerry) != 2 || countItem(stored.Bank, protocol.ItemBerry) != 1 {
		t.Fatalf("store disagrees: inv=%v bank=%v", stored.Inv, stored.Bank)
	}
	if want := "You leave one brambleberry in the chest."; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}
}

func TestWithdrawReturnsToPack(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Bank = []ItemStack{{ID: protocol.ItemLog, N: 2}}

	msg, ok := w.Withdraw(context.Background(), "p1", protocol.ItemLog, 1)
	if !ok {
		t.Fatalf("withdraw refused: %s", msg)
	}
	if countItem(p.Inv, protocol.ItemLog) != 1 || countItem(p.Bank, protocol.ItemLog) != 1 {
		t.Fatalf("inv=%v bank=%v", p.Inv, p.Bank)
	}
	if want := "You take one log from the chest."; msg != want {
		t.Fatalf("got %q", msg)
	}
}

func TestCoinsMoveWithoutTakingASlot(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Coins = 9
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}

	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 5); !ok {
		t.Fatal("coin deposit")
	}
	if p.Coins != 4 || p.BankCoins != 5 {
		t.Fatalf("coins purse=%d chest=%d", p.Coins, p.BankCoins)
	}
	if len(p.Inv) != 1 || len(p.Bank) != 0 {
		t.Fatal("coins took a slot")
	}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.TokenCoins, 2); !ok {
		t.Fatal("coin withdraw")
	}
	if p.Coins != 6 || p.BankCoins != 3 {
		t.Fatalf("after take: purse=%d chest=%d", p.Coins, p.BankCoins)
	}
}

func TestMustStandByTheChest(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	p.Coins = 4
	p.X, p.Y = 16, 16

	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("deposited from across the hamlet")
	}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("withdrew from across the hamlet")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 1); ok {
		t.Fatal("left coins from across the hamlet")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 || p.Coins != 4 {
		t.Fatal("a distant refusal changed something")
	}
}

func TestDepositRefusesUnknownAndEmpty(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	if _, ok := w.Deposit(context.Background(), "p1", "moonbeam", 1); ok {
		t.Fatal("chest took an unknown item")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("deposited a berry that was never held")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 1); ok {
		t.Fatal("deposited coins from an empty purse")
	}
	p.Bank = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	p.Inv = nil
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemLog, 1); ok {
		t.Fatal("withdrew a log the chest never held")
	}
}

func TestFullChestRefusesAndLeavesThePack(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	for i := 0; i < protocol.BankSlots; i++ {
		p.Bank = append(p.Bank, ItemStack{ID: "filler" + itoa(i), N: 1})
	}
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("deposited into a full chest")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 {
		t.Fatal("the berry left the pack without a home")
	}
}

func TestFullPackRefusesWithdraw(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	for i := 0; i < protocol.InvSlots; i++ {
		p.Inv = append(p.Inv, ItemStack{ID: "filler" + itoa(i), N: 1})
	}
	p.Bank = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("withdrew into a full pack")
	}
	if countItem(p.Bank, protocol.ItemBerry) != 1 {
		t.Fatal("the berry left the chest without a home")
	}
}

func TestPeerCannotSeeOrTakeYourBank(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	a := atChest(t, w, "a", "Ash")
	b := atChest(t, w, "b", "Briar")
	a.Inv = []ItemStack{{ID: protocol.ItemLeather, N: 1}}
	a.Coins = 7
	if _, ok := w.Deposit(context.Background(), "a", protocol.ItemLeather, 1); !ok {
		t.Fatal("ash deposit")
	}
	if _, ok := w.Deposit(context.Background(), "a", protocol.TokenCoins, 7); !ok {
		t.Fatal("ash coins")
	}

	if countItem(b.Bank, protocol.ItemLeather) != 0 || b.BankCoins != 0 {
		t.Fatalf("briar can see ash's chest: bank=%v coins=%d", b.Bank, b.BankCoins)
	}
	if _, ok := w.Withdraw(context.Background(), "b", protocol.ItemLeather, 1); ok {
		t.Fatal("briar took ash's leather")
	}
	if _, ok := w.Withdraw(context.Background(), "b", protocol.TokenCoins, 1); ok {
		t.Fatal("briar took ash's coins")
	}

	w.RotateHandle("a")
	w.RotateHandle("b")
	snap := w.Snapshot("b")
	if countItemStacks(snap.You.Bank, protocol.ItemLeather) != 0 || snap.You.BankCoins != 0 {
		t.Fatal("briar's you-view carried ash's chest")
	}
	for _, peer := range snap.Players {
		if peer.Name == "Ash" {
			// PlayerView has no bank field; a leak would have to be invented.
			_ = peer
		}
	}
	if countItem(a.Bank, protocol.ItemLeather) != 1 || a.BankCoins != 7 {
		t.Fatalf("ash lost the chest to a peer: %v coins=%d", a.Bank, a.BankCoins)
	}
}

func countItemStacks(inv []protocol.Item, id string) int {
	n := 0
	for _, it := range inv {
		if it.ID == id {
			n += it.N
		}
	}
	return n
}

func TestBankRelogKeepsWhatYouLeft(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemKnife, N: 1}, {ID: protocol.ItemBerry, N: 2}}
	p.Coins = 11
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemKnife, 1); !ok {
		t.Fatal("knife")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 2); !ok {
		t.Fatal("berries")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 11); !ok {
		t.Fatal("coins")
	}

	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Bank, protocol.ItemKnife) != 1 || countItem(p2.Bank, protocol.ItemBerry) != 2 {
		t.Fatalf("relog chest: %v", p2.Bank)
	}
	if p2.BankCoins != 11 || p2.Coins != 0 {
		t.Fatalf("relog coins purse=%d chest=%d", p2.Coins, p2.BankCoins)
	}
	if countItem(p2.Inv, protocol.ItemKnife) != 0 || countItem(p2.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("relog pack still held what was left: %v", p2.Inv)
	}

	standBeside(t, w2, p2, firstChest(w2))
	if _, ok := w2.Withdraw(context.Background(), "p1", protocol.ItemKnife, 1); !ok {
		t.Fatal("withdraw after relog")
	}
	if countItem(p2.Inv, protocol.ItemKnife) != 1 || countItem(p2.Bank, protocol.ItemKnife) != 0 {
		t.Fatalf("after withdraw: inv=%v bank=%v", p2.Inv, p2.Bank)
	}
}

func TestBankMoveAbandonedIfStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	p.Coins = 4
	p.Bank = []ItemStack{{ID: protocol.ItemLog, N: 1}}
	p.BankCoins = 3
	st.fail = true

	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("deposit succeeded while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 1 || countItem(p.Bank, protocol.ItemBerry) != 0 {
		t.Fatal("the berry moved without a commit")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 2); ok {
		t.Fatal("coin deposit succeeded while the store was failing")
	}
	if p.Coins != 4 || p.BankCoins != 3 {
		t.Fatal("coins moved without a commit")
	}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemLog, 1); ok {
		t.Fatal("withdraw succeeded while the store was failing")
	}
	if countItem(p.Bank, protocol.ItemLog) != 1 || countItem(p.Inv, protocol.ItemLog) != 0 {
		t.Fatal("the log moved without a commit")
	}
}

func TestBankWriteKeepsLooksAndLooksWriteKeepsBank(t *testing.T) {
	st := newMem()
	rec := NewPlayerRec("p1", "Kyle")
	rec.Looks = testLooks()
	rec.Bank = []ItemStack{{ID: protocol.ItemBerry, N: 4}}
	rec.BankCoins = 9
	if err := st.SavePlayer(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	rec.Inv = []ItemStack{{ID: protocol.ItemNut, N: 1}}
	if err := st.SavePlayer(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	got, _ := st.LoadPlayer(context.Background(), "p1")
	if got.Looks != testLooks() {
		t.Fatalf("pack write wiped looks: %+v", got.Looks)
	}
	if countItem(got.Bank, protocol.ItemBerry) != 4 || got.BankCoins != 9 {
		t.Fatalf("pack write wiped the chest: bank=%v coins=%d", got.Bank, got.BankCoins)
	}

	other := protocol.Looks{
		Skin: protocol.SkinFair, Hair: protocol.HairLong,
		HairColor: protocol.HairSnow, Top: protocol.TopBerry,
	}
	_, _, err := ApplyLooksFirst(context.Background(), st, rec, other)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := st.LoadPlayer(context.Background(), "p1")
	if countItem(again.Bank, protocol.ItemBerry) != 4 || again.BankCoins != 9 {
		t.Fatalf("looks write wiped the chest: bank=%v coins=%d", again.Bank, again.BankCoins)
	}
}

func TestToolsStayUnstackedInTheChest(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemAxe, N: 1}, {ID: protocol.ItemAxe, N: 1}}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemAxe, 2); !ok {
		t.Fatal("axes")
	}
	if len(p.Bank) != 2 || p.Bank[0].N != 1 || p.Bank[1].N != 1 {
		t.Fatalf("axes stacked in the chest: %v", p.Bank)
	}
}

func TestDepositZeroMeansOne(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 3}}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 0); !ok {
		t.Fatal("n=0")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 2 || countItem(p.Bank, protocol.ItemBerry) != 1 {
		t.Fatalf("n=0 should move one: inv=%v bank=%v", p.Inv, p.Bank)
	}
}
