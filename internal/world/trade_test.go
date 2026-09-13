package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func pedlar(w *World) *NPC {
	for _, n := range w.NPCs {
		if n.Trader {
			return n
		}
	}
	return nil
}

// atPedlar puts a player beside the pedlar with the given purse.
func atPedlar(t *testing.T, w *World, coins int) *Player {
	t.Helper()
	n := pedlar(w)
	if n == nil {
		t.Fatal("there is no pedlar in the hamlet")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	x, y, ok := w.nearestAdjacent(n.X, n.Y, n.X, n.Y)
	if !ok {
		t.Fatal("nowhere to stand beside the pedlar")
	}
	p.X, p.Y = x, y
	p.Coins = coins
	return p
}

func TestPedlarExistsAndIsNotAFight(t *testing.T) {
	w := testWorld(t, newMem())
	n := pedlar(w)
	if n == nil {
		t.Fatal("no trader")
	}
	if n.Hostile {
		t.Error("the pedlar should not be hostile")
	}
	if !w.Walkable(n.X, n.Y) {
		t.Errorf("the pedlar stands on blocked ground at (%d,%d)", n.X, n.Y)
	}
	// Clicking her must open trade, not start a brawl.
	p := atPedlar(t, w, 0)
	w.SetInteract("p1", n.ID)
	if p.Target != "" {
		t.Fatalf("clicking the pedlar set a combat target: %q", p.Target)
	}
	if p.ActionTrader != n.ID {
		t.Fatalf("clicking the pedlar did not open trade: %q", p.ActionTrader)
	}
}

func TestWalkingToThePedlarOpensTheBoard(t *testing.T) {
	w := testWorld(t, newMem())
	n := pedlar(w)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	w.SetInteract("p1", n.ID)
	for i := 0; i < 30 && w.TakeTradeOpen("p1") == ""; i++ {
		w.Tick(context.Background())
		if p.ActionTrader == "" && !p.HasDest {
			break
		}
	}
	// One more tick to let tickTrade fire on arrival.
	w.Tick(context.Background())
	if !adjacent(p.X, p.Y, n.X, n.Y) && (p.X != n.X || p.Y != n.Y) {
		t.Fatalf("player ended at (%d,%d), pedlar at (%d,%d)", p.X, p.Y, n.X, n.Y)
	}
}

func TestSellingPaysAndTakesTheItem(t *testing.T) {
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 0)
	p.Inv = []ItemStack{{ID: protocol.ItemLeather, N: 1}}

	msg, ok := w.Sell(context.Background(), "p1", protocol.ItemLeather)
	if !ok {
		t.Fatalf("sale refused: %s", msg)
	}
	offer, _ := offerFor(protocol.ItemLeather)
	if p.Coins != offer.Pays {
		t.Fatalf("purse = %d, want %d", p.Coins, offer.Pays)
	}
	if countItem(p.Inv, protocol.ItemLeather) != 0 {
		t.Fatal("the leather is still in the pack")
	}
}

func TestBuyingCostsAndGivesTheItem(t *testing.T) {
	w := testWorld(t, newMem())
	offer, _ := offerFor(protocol.ItemTart)
	p := atPedlar(t, w, offer.Costs+5)

	msg, ok := w.Buy(context.Background(), "p1", protocol.ItemTart)
	if !ok {
		t.Fatalf("purchase refused: %s", msg)
	}
	if p.Coins != 5 {
		t.Fatalf("purse = %d, want 5", p.Coins)
	}
	if countItem(p.Inv, protocol.ItemTart) != 1 {
		t.Fatalf("no tart: %+v", p.Inv)
	}
}

func TestCannotBuyWithoutTheCoin(t *testing.T) {
	w := testWorld(t, newMem())
	offer, _ := offerFor(protocol.ItemTart)
	p := atPedlar(t, w, offer.Costs-1)
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("bought a tart without enough coin")
	}
	if countItem(p.Inv, protocol.ItemTart) != 0 || p.Coins != offer.Costs-1 {
		t.Fatal("a refused purchase changed something")
	}
}

func TestCannotSellWhatYouDoNotHave(t *testing.T) {
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 0)
	if _, ok := w.Sell(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("sold a tart that was never held")
	}
	if p.Coins != 0 {
		t.Fatalf("purse grew to %d on a refused sale", p.Coins)
	}
}

// She does not deal in everything.
func TestPedlarRefusesGoodsSheDoesNotWant(t *testing.T) {
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 500)
	p.Inv = []ItemStack{{ID: "moonbeam", N: 1}}
	if _, ok := w.Sell(context.Background(), "p1", "moonbeam"); ok {
		t.Fatal("she bought something that is not on the board")
	}
	// And she stocks only what has a price.
	for _, o := range tradeBoard {
		if o.Costs == 0 {
			if _, ok := w.Buy(context.Background(), "p1", o.Item); ok {
				t.Fatalf("bought %s, which she does not stock", o.Item)
			}
		}
	}
}

// Trading at a distance is not trading.
func TestMustStandByThePedlar(t *testing.T) {
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 500)
	p.Inv = []ItemStack{{ID: protocol.ItemLeather, N: 1}}
	p.X, p.Y = spawnX, spawnY // back at the stile

	if _, ok := w.Sell(context.Background(), "p1", protocol.ItemLeather); ok {
		t.Fatal("sold from across the hamlet")
	}
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("bought from across the hamlet")
	}
}

// A trade moves items and coins together, so it must not touch memory
// unless the store took it — the same rule as every other item path.
func TestTradeIsAbandonedIfTheStoreFails(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	offer, _ := offerFor(protocol.ItemTart)
	p := atPedlar(t, w, offer.Costs)
	p.Inv = []ItemStack{{ID: protocol.ItemLeather, N: 1}}
	st.fail = true

	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("a purchase succeeded while the store was failing")
	}
	if p.Coins != offer.Costs || countItem(p.Inv, protocol.ItemTart) != 0 {
		t.Fatalf("the purchase half-happened: %d coins, %d tarts",
			p.Coins, countItem(p.Inv, protocol.ItemTart))
	}
	if _, ok := w.Sell(context.Background(), "p1", protocol.ItemLeather); ok {
		t.Fatal("a sale succeeded while the store was failing")
	}
	if countItem(p.Inv, protocol.ItemLeather) != 1 {
		t.Fatal("the leather left the pack without a commit")
	}
}

// A full pack must not swallow the coin.
func TestBuyingIntoAFullPackIsRefused(t *testing.T) {
	w := testWorld(t, newMem())
	offer, _ := offerFor(protocol.ItemTart)
	p := atPedlar(t, w, offer.Costs)
	for i := 0; i < protocol.InvSlots; i++ {
		p.Inv = append(p.Inv, ItemStack{ID: "filler" + itoa(i), N: 1})
	}
	if _, ok := w.Buy(context.Background(), "p1", protocol.ItemTart); ok {
		t.Fatal("bought into a full pack")
	}
	if p.Coins != offer.Costs {
		t.Fatalf("coins were taken for nothing: %d", p.Coins)
	}
}

// She pays less than she charges, or the hamlet has a money printer.
func TestSpreadIsNeverNegative(t *testing.T) {
	for _, o := range tradeBoard {
		if o.Costs > 0 && o.Pays >= o.Costs {
			t.Errorf("%s: pays %d but costs %d — buy low, sell high, forever",
				o.Item, o.Pays, o.Costs)
		}
	}
}

// Everything the board mentions must be a real item, or the client will
// render a blank row.
func TestBoardOnlyListsRealItems(t *testing.T) {
	cat := protocol.Catalog()
	for _, o := range tradeBoard {
		if _, ok := cat[o.Item]; !ok {
			t.Errorf("the board lists %q, which is not in the item catalog", o.Item)
		}
		if o.Pays == 0 && o.Costs == 0 {
			t.Errorf("%s is on the board but deals in neither direction", o.Item)
		}
	}
}

func TestCoinWordIsNotAlwaysPlural(t *testing.T) {
	if got := coinWord(1); got != "1 coin" {
		t.Errorf("coinWord(1) = %q", got)
	}
	for _, n := range []int{0, 2, 16} {
		if got := coinWord(n); got == "1 coin" {
			t.Errorf("coinWord(%d) = %q", n, got)
		}
	}
	// And the message a player actually reads.
	w := testWorld(t, newMem())
	p := atPedlar(t, w, 0)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	msg, ok := w.Sell(context.Background(), "p1", protocol.ItemBerry)
	if !ok {
		t.Fatalf("sale refused: %s", msg)
	}
	if want := "You sell one brambleberry for 1 coin."; msg != want {
		t.Fatalf("got %q, want %q", msg, want)
	}
}
