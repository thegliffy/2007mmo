package world

import (
	"context"
	"fmt"
	"sort"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// The pedlar. She buys most of what the hamlet produces and sells cooked
// food, which is the only thing coins were ever going to be for: fighting
// pays in coin, and coin buys the food that lets you fight something
// bigger.
//
// Every exchange moves items and coins together, so it goes through
// CommitAction the same way a forage does — Postgres first, memory second.
// A crash between the two loses the trade rather than paying twice.

// TradeOffer is one line of the pedlar's board.
type TradeOffer struct {
	Item string
	// Pays is what she gives you for one, zero if she does not want it.
	Pays int
	// Costs is what she charges for one, zero if she does not stock it.
	Costs int
}

// tradeBoard is the whole price list. She pays less than she charges,
// because she has to eat too.
var tradeBoard = []TradeOffer{
	{Item: protocol.ItemBerry, Pays: 1},
	{Item: protocol.ItemNut, Pays: 1},
	{Item: protocol.ItemLog, Pays: 2},
	{Item: protocol.ItemPulp, Pays: 3},
	{Item: protocol.ItemPaper, Pays: 8},
	{Item: protocol.ItemRoast, Pays: 5, Costs: 12},
	{Item: protocol.ItemTart, Pays: 7, Costs: 16},
	{Item: protocol.ItemLeather, Pays: 25},
}

func offerFor(item string) (TradeOffer, bool) {
	for _, o := range tradeBoard {
		if o.Item == item {
			return o, true
		}
	}
	return TradeOffer{}, false
}

// TradeBoard is the price list as the client sees it.
func TradeBoard() []protocol.TradeOffer {
	out := make([]protocol.TradeOffer, 0, len(tradeBoard))
	for _, o := range tradeBoard {
		out = append(out, protocol.TradeOffer{Item: o.Item, Pays: o.Pays, Costs: o.Costs})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Costs > out[j].Costs })
	return out
}

// traderNear returns the pedlar if the player is standing beside one.
func (w *World) traderNear(p *Player) *NPC {
	if p == nil {
		return nil
	}
	for _, n := range w.NPCs {
		if !n.Trader {
			continue
		}
		if adjacent(p.X, p.Y, n.X, n.Y) || (p.X == n.X && p.Y == n.Y) {
			return n
		}
	}
	return nil
}

// SetTrade walks a player to the pedlar and marks the intent to trade.
func (w *World) SetTrade(id, npcID string) {
	p := w.Players[id]
	npc := w.npcByID(npcID)
	if p == nil || npc == nil || !npc.Trader {
		return
	}
	tx, ty, ok := w.nearestAdjacent(p.X, p.Y, npc.X, npc.Y)
	if !ok {
		w.note(id, "There is no room to stand beside "+npc.Name+".")
		return
	}
	p.Target = ""
	p.ActionNode = ""
	p.ActionGround = ""
	p.ActionTrader = npcID
	p.HasDest = true
	p.DestX, p.DestY = tx, ty
	p.Path = nil
	if channeling(p.Action) {
		p.Action = "idle"
		p.ActionTicks = 0
		p.ActionItem = ""
	}
}

// tickTrade opens the board once the player has arrived.
func (w *World) tickTrade(p *Player) {
	if p.ActionTrader == "" || p.HasDest {
		return
	}
	npc := w.npcByID(p.ActionTrader)
	p.ActionTrader = ""
	if npc == nil || !npc.Trader {
		return
	}
	if !adjacent(p.X, p.Y, npc.X, npc.Y) && (p.X != npc.X || p.Y != npc.Y) {
		return // wandered off on the way
	}
	if w.tradeOpen == nil {
		w.tradeOpen = make(map[string]string)
	}
	w.tradeOpen[p.ID] = npc.Name
}

// TakeTradeOpen reports whether a player just reached the pedlar, so the
// hub can push the board. Drained like a note.
func (w *World) TakeTradeOpen(id string) string {
	if w.tradeOpen == nil {
		return ""
	}
	name := w.tradeOpen[id]
	delete(w.tradeOpen, id)
	return name
}

// Sell hands one item to the pedlar for coin.
func (w *World) Sell(ctx context.Context, id, itemID string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	if w.traderNear(p) == nil {
		return "There is nobody here to trade with.", false
	}
	offer, ok := offerFor(itemID)
	if !ok || offer.Pays <= 0 {
		return "She has no use for that.", false
	}
	if countItem(p.Inv, itemID) < 1 {
		return "You have none of those.", false
	}

	next := recFromPlayer(p)
	next.Inv = removeItem(next.Inv, itemID, 1)
	next.Coins += offer.Pays
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Coins = next.Coins
	p.Dirty = false
	return fmt.Sprintf("You sell one %s for %s.", itemName(itemID), coinWord(offer.Pays)), true
}

// Buy takes one item from the pedlar for coin.
func (w *World) Buy(ctx context.Context, id, itemID string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	if w.traderNear(p) == nil {
		return "There is nobody here to trade with.", false
	}
	offer, ok := offerFor(itemID)
	if !ok || offer.Costs <= 0 {
		return "She does not stock that.", false
	}
	if p.Coins < offer.Costs {
		return fmt.Sprintf("That costs %s and you have %d.", coinWord(offer.Costs), p.Coins), false
	}

	next := recFromPlayer(p)
	before := countItem(next.Inv, itemID)
	next.Inv = addItem(next.Inv, itemID, 1)
	if countItem(next.Inv, itemID) == before {
		return "Your pack is full.", false
	}
	next.Coins -= offer.Costs
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Coins = next.Coins
	p.Dirty = false
	return fmt.Sprintf("You buy one %s for %s.", itemName(itemID), coinWord(offer.Costs)), true
}

// coinWord keeps the pedlar from saying "1 coins".
func coinWord(n int) string {
	if n == 1 {
		return "1 coin"
	}
	return fmt.Sprintf("%d coins", n)
}

func itemName(id string) string {
	if info, ok := protocol.Catalog()[id]; ok {
		return lowerFirst(info.Name)
	}
	return id
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}
