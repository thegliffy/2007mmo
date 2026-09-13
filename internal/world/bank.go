package world

import (
	"context"
	"fmt"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// The oak chest by the stile. One lid per villager — nobody else can
// see or take what you leave. Pack stays limited; this is the overflow.
//
// A move writes pack, purse, chest, and chest-purse together, through
// CommitAction the same way a forage or a pedlar trade does. Postgres
// first, memory second. A crash between the two loses the move rather
// than minting a second berry.

const chestName = "the oak chest"

func (w *World) chestByID(id string) *Node {
	n := w.Nodes[id]
	if n == nil || n.Kind != KindChest {
		return nil
	}
	return n
}

// chestNear returns the hamlet chest if the player is standing beside it.
func (w *World) chestNear(p *Player) *Node {
	if p == nil {
		return nil
	}
	for _, n := range w.Nodes {
		if n.Kind != KindChest {
			continue
		}
		if adjacent(p.X, p.Y, n.X, n.Y) || (p.X == n.X && p.Y == n.Y) {
			return n
		}
	}
	return nil
}

// SetChest walks a player to the oak chest and marks the intent to open it.
func (w *World) SetChest(id, nodeID string) {
	p := w.Players[id]
	n := w.chestByID(nodeID)
	if p == nil || n == nil {
		return
	}
	tx, ty, ok := w.nearestAdjacent(p.X, p.Y, n.X, n.Y)
	if !ok {
		w.note(id, "There is no room to stand beside the chest.")
		return
	}
	p.Target = ""
	p.ActionNode = ""
	p.ActionGround = ""
	p.ActionTrader = ""
	p.ActionChest = n.ID
	p.HasDest = true
	p.DestX, p.DestY = tx, ty
	p.Path = nil
	if channeling(p.Action) {
		p.Action = "idle"
		p.ActionTicks = 0
		p.ActionItem = ""
	}
}

// tickChest opens the lid once the player has arrived.
func (w *World) tickChest(p *Player) {
	if p.ActionChest == "" || p.HasDest {
		return
	}
	n := w.chestByID(p.ActionChest)
	p.ActionChest = ""
	if n == nil {
		return
	}
	if !adjacent(p.X, p.Y, n.X, n.Y) && (p.X != n.X || p.Y != n.Y) {
		return // wandered off on the way
	}
	if w.bankOpen == nil {
		w.bankOpen = make(map[string]string)
	}
	w.bankOpen[p.ID] = chestName
}

// TakeBankOpen reports whether a player just reached the chest, so the
// hub can push the lid. Drained like a note.
func (w *World) TakeBankOpen(id string) string {
	if w.bankOpen == nil {
		return ""
	}
	name := w.bankOpen[id]
	delete(w.bankOpen, id)
	return name
}

func moveQty(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// Deposit moves items or coins from the pack/purse into the personal chest.
func (w *World) Deposit(ctx context.Context, id, itemID string, n int) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	if w.chestNear(p) == nil {
		return "The chest is over there. Stand beside it.", false
	}
	n = moveQty(n)
	if itemID == protocol.TokenCoins {
		return w.moveCoins(ctx, p, n, true)
	}
	if _, ok := protocol.Catalog()[itemID]; !ok {
		return "The chest will not take that.", false
	}
	have := countItem(p.Inv, itemID)
	if have < 1 {
		return "You have none of those.", false
	}
	if n > have {
		n = have
	}

	next := recFromPlayer(p)
	before := countItem(next.Bank, itemID)
	next.Bank = addItemCapped(next.Bank, itemID, n, protocol.BankSlots)
	added := countItem(next.Bank, itemID) - before
	if added <= 0 {
		return "The chest is full.", false
	}
	next.Inv = removeItem(next.Inv, itemID, added)
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Bank = next.Bank
	p.Dirty = false
	return fmt.Sprintf("You leave %s in the chest.", itemPhrase(itemID, added)), true
}

// Withdraw moves items or coins from the personal chest into the pack/purse.
func (w *World) Withdraw(ctx context.Context, id, itemID string, n int) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	if w.chestNear(p) == nil {
		return "The chest is over there. Stand beside it.", false
	}
	n = moveQty(n)
	if itemID == protocol.TokenCoins {
		return w.moveCoins(ctx, p, n, false)
	}
	if _, ok := protocol.Catalog()[itemID]; !ok {
		return "There is nothing like that in the chest.", false
	}
	have := countItem(p.Bank, itemID)
	if have < 1 {
		return "The chest does not hold any of those.", false
	}
	if n > have {
		n = have
	}

	next := recFromPlayer(p)
	before := countItem(next.Inv, itemID)
	next.Inv = addItem(next.Inv, itemID, n)
	added := countItem(next.Inv, itemID) - before
	if added <= 0 {
		return "Your pack is full.", false
	}
	next.Bank = removeItem(next.Bank, itemID, added)
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Bank = next.Bank
	p.Dirty = false
	return fmt.Sprintf("You take %s from the chest.", itemPhrase(itemID, added)), true
}

func (w *World) moveCoins(ctx context.Context, p *Player, n int, intoChest bool) (string, bool) {
	next := recFromPlayer(p)
	if intoChest {
		if next.Coins < 1 {
			return "Your purse is empty.", false
		}
		if n > next.Coins {
			n = next.Coins
		}
		next.Coins -= n
		next.BankCoins += n
	} else {
		if next.BankCoins < 1 {
			return "The chest holds no coins.", false
		}
		if n > next.BankCoins {
			n = next.BankCoins
		}
		next.BankCoins -= n
		next.Coins += n
	}
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Coins = next.Coins
	p.BankCoins = next.BankCoins
	p.Dirty = false
	if intoChest {
		return fmt.Sprintf("You leave %s in the chest.", coinWord(n)), true
	}
	return fmt.Sprintf("You take %s from the chest.", coinWord(n)), true
}

func itemPhrase(id string, n int) string {
	name := itemName(id)
	if n == 1 {
		return "one " + name
	}
	return fmt.Sprintf("%d %s", n, name)
}

func bankView(bank []ItemStack) []protocol.Item {
	out := make([]protocol.Item, 0, len(bank))
	for _, it := range bank {
		out = append(out, protocol.Item{ID: it.ID, N: it.N})
	}
	return out
}
