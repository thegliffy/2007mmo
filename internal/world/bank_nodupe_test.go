package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func TestDoubleDepositOneItemMovesOnce(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); !ok {
		t.Fatal("first deposit")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); ok {
		t.Fatal("deposited the same berry twice")
	}
	if countItem(p.Inv, protocol.ItemBerry) != 0 || countItem(p.Bank, protocol.ItemBerry) != 1 {
		t.Fatalf("double deposit: inv=%v bank=%v", p.Inv, p.Bank)
	}
	if countItem(st.players["p1"].Bank, protocol.ItemBerry) != 1 {
		t.Fatalf("store minted a second berry: %v", st.players["p1"].Bank)
	}
}

func TestDoubleWithdrawOneItemMovesOnce(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Bank = []ItemStack{{ID: protocol.ItemTin, N: 1}}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemTin, 1); !ok {
		t.Fatal("first withdraw")
	}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemTin, 1); ok {
		t.Fatal("withdrew the same tin twice")
	}
	if countItem(p.Inv, protocol.ItemTin) != 1 || countItem(p.Bank, protocol.ItemTin) != 0 {
		t.Fatalf("double withdraw: inv=%v bank=%v", p.Inv, p.Bank)
	}
}

func TestDoubleDepositCoinsMovesOnce(t *testing.T) {
	w := testWorld(t, newMem())
	p := atChest(t, w, "p1", "Kyle")
	p.Coins = 1
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 1); !ok {
		t.Fatal("first")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 1); ok {
		t.Fatal("left the same coin twice")
	}
	if p.Coins != 0 || p.BankCoins != 1 {
		t.Fatalf("double coin deposit: purse=%d chest=%d", p.Coins, p.BankCoins)
	}
}

func TestCrashAfterDepositNoDupe(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemCopper, N: 1}}
	p.Coins = 6
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemCopper, 1); !ok {
		t.Fatal("item")
	}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.TokenCoins, 6); !ok {
		t.Fatal("coins")
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemCopper) != 0 || countItem(p2.Bank, protocol.ItemCopper) != 1 {
		t.Fatalf("deposit recovery inv=%v bank=%v", p2.Inv, p2.Bank)
	}
	if p2.Coins != 0 || p2.BankCoins != 6 {
		t.Fatalf("deposit recovery coins purse=%d chest=%d", p2.Coins, p2.BankCoins)
	}
}

func TestCrashAfterWithdrawNoDupe(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Bank = []ItemStack{{ID: protocol.ItemBar, N: 1}}
	p.BankCoins = 4
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.ItemBar, 1); !ok {
		t.Fatal("item")
	}
	if _, ok := w.Withdraw(context.Background(), "p1", protocol.TokenCoins, 4); !ok {
		t.Fatal("coins")
	}
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Inv, protocol.ItemBar) != 1 || countItem(p2.Bank, protocol.ItemBar) != 0 {
		t.Fatalf("withdraw recovery inv=%v bank=%v", p2.Inv, p2.Bank)
	}
	if p2.Coins != 4 || p2.BankCoins != 0 {
		t.Fatalf("withdraw recovery coins purse=%d chest=%d", p2.Coins, p2.BankCoins)
	}
}

func TestDisconnectWalkingToChestDoesNotOpen(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	n := firstChest(w)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = 14, 8
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	persist(t, st, p)
	w.SetInteract("p1", n.ID)
	if p.ActionChest == "" {
		t.Fatal("setup: not walking to the chest")
	}
	w.SetOnline("p1", false)
	persist(t, st, p)
	for i := 0; i < 20; i++ {
		w.Tick(context.Background())
	}
	if w.TakeBankOpen("p1") != "" {
		t.Fatal("an offline walk opened the chest")
	}
	if countItem(p.Bank, protocol.ItemBerry) != 0 {
		t.Fatal("disconnect mid-walk deposited something")
	}

	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if p2.ActionChest != "" {
		t.Fatal("a restored player kept a walk-to-chest that never opened")
	}
	if countItem(p2.Inv, protocol.ItemBerry) != 1 || countItem(p2.Bank, protocol.ItemBerry) != 0 {
		t.Fatalf("crash mid-walk moved the berry: inv=%v bank=%v", p2.Inv, p2.Bank)
	}
	for i := 0; i < 20; i++ {
		w2.Tick(context.Background())
	}
	if w2.TakeBankOpen("p1") != "" {
		t.Fatal("a restored player opened a chest they never reached")
	}
}

func TestDisconnectMidDepositKeepsTheCommit(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := atChest(t, w, "p1", "Kyle")
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 1}}
	if _, ok := w.Deposit(context.Background(), "p1", protocol.ItemBerry, 1); !ok {
		t.Fatal("deposit")
	}
	w.SetOnline("p1", false)
	// Deposit already committed. Going offline must not roll it back
	// or mint a second berry on the next process.
	w2 := rebuildFromStore(t, st)
	p2 := loadInto(t, w2, st, "p1")
	if countItem(p2.Bank, protocol.ItemBerry) != 1 || countItem(p2.Inv, protocol.ItemBerry) != 0 {
		t.Fatalf("disconnect after deposit: inv=%v bank=%v", p2.Inv, p2.Bank)
	}
}
