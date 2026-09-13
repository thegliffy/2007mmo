package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Things left lying in the grass: what a beast was carrying, and whatever
// players throw away.
//
// Ground piles are deliberately NOT persisted, and that is what keeps the
// no-dupe rule intact rather than breaking it.
//
// A pickup has to move an item from the world into a pack. Commit the pack
// first and remove the pile second, and a crash between the two leaves the
// item in Postgres AND still on the ground — a duplicate, the one outcome
// the whole design exists to prevent. So the pile is emptied first and the
// pack committed second: a failure there loses the item, which is the safe
// direction, and the pile is put back if the commit merely errors rather
// than dying.
//
// Because piles live only in memory, a crash takes every pile with it. The
// pack is whatever Postgres last agreed to, so there is nothing left to
// duplicate against. Persisting them would mean a second transaction in
// the same tick and a genuinely hard ordering problem, to make dropped
// items survive a restart that loses nothing else.
const (
	groundExpireTicks = 200 // ~2 minutes at a 600ms tick
	maxGroundPiles    = 64  // a cap, so a bored player cannot grow the map forever
)

type GroundItem struct {
	ID       string
	X, Y     int
	Inv      []ItemStack
	Coins    int
	ExpireIn int
}

func (g *GroundItem) empty() bool {
	return g == nil || (g.Coins == 0 && len(g.Inv) == 0)
}

// label describes a pile in one short phrase, for the client and for
// flavour text.
func (g *GroundItem) label(items map[string]protocol.ItemInfo) string {
	if g == nil {
		return ""
	}
	parts := make([]string, 0, len(g.Inv)+1)
	for _, it := range g.Inv {
		name := it.ID
		if info, ok := items[it.ID]; ok {
			name = info.Name
		}
		if it.N > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", name, it.N))
		} else {
			parts = append(parts, name)
		}
	}
	if g.Coins > 0 {
		parts = append(parts, fmt.Sprintf("%d coins", g.Coins))
	}
	return strings.Join(parts, ", ")
}

func (w *World) groundByID(id string) *GroundItem {
	if w.Ground == nil {
		return nil
	}
	return w.Ground[id]
}

// dropPile leaves a pile on the given tile, merging into one already there
// so a busy spot does not sprout a dozen separate piles.
func (w *World) dropPile(x, y int, inv []ItemStack, coins int) *GroundItem {
	if len(inv) == 0 && coins == 0 {
		return nil
	}
	if w.Ground == nil {
		w.Ground = make(map[string]*GroundItem)
	}
	for _, g := range w.Ground {
		if g.X == x && g.Y == y {
			for _, it := range inv {
				g.Inv = addItem(g.Inv, it.ID, it.N)
			}
			g.Coins += coins
			g.ExpireIn = groundExpireTicks
			return g
		}
	}
	if len(w.Ground) >= maxGroundPiles {
		w.expireOldestPile()
	}
	w.groundSeq++
	g := &GroundItem{
		ID:       fmt.Sprintf("ground-%d", w.groundSeq),
		X:        x,
		Y:        y,
		Inv:      append([]ItemStack(nil), inv...),
		Coins:    coins,
		ExpireIn: groundExpireTicks,
	}
	w.Ground[g.ID] = g
	return g
}

// expireOldestPile makes room once the cap is reached, taking whichever
// pile was closest to vanishing anyway.
func (w *World) expireOldestPile() {
	var oldest *GroundItem
	for _, g := range w.Ground {
		if oldest == nil || g.ExpireIn < oldest.ExpireIn {
			oldest = g
		}
	}
	if oldest != nil {
		delete(w.Ground, oldest.ID)
	}
}

func (w *World) tickGround() {
	for id, g := range w.Ground {
		g.ExpireIn--
		if g.ExpireIn <= 0 || g.empty() {
			delete(w.Ground, id)
		}
	}
}

// DropItem throws one of an item out of the pack and onto the floor.
//
// The pack is committed without the item before the pile exists, so a
// crash in between loses it rather than leaving it in two places.
func (w *World) DropItem(ctx context.Context, id, itemID string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	if countItem(p.Inv, itemID) < 1 {
		return "You have none of those.", false
	}
	if !w.Walkable(p.X, p.Y) {
		return "There is nowhere here to set it down.", false
	}

	next := recFromPlayer(p)
	next.Inv = removeItem(next.Inv, itemID, 1)
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Dirty = false

	w.dropPile(p.X, p.Y, []ItemStack{{ID: itemID, N: 1}}, 0)
	name := itemID
	if info, ok := protocol.Catalog()[itemID]; ok {
		name = info.Name
	}
	return "You set down the " + strings.ToLower(name) + ".", true
}

// SetPickup walks a player to a pile and marks it for collection.
func (w *World) SetPickup(id, groundID string) {
	p := w.Players[id]
	g := w.groundByID(groundID)
	if p == nil || g == nil {
		return
	}
	p.Target = ""
	p.ActionNode = ""
	p.ActionGround = groundID
	p.HasDest = true
	p.DestX, p.DestY = g.X, g.Y
	p.Path = nil
	if channeling(p.Action) {
		p.Action = "idle"
		p.ActionTicks = 0
		p.ActionItem = ""
	}
}

// tickPickup collects a pile once the player is standing on it.
func (w *World) tickPickup(ctx context.Context, p *Player) {
	if p.ActionGround == "" || p.HasDest {
		return
	}
	g := w.groundByID(p.ActionGround)
	p.ActionGround = ""
	if g == nil {
		return
	}
	if p.X != g.X || p.Y != g.Y {
		return // wandered off, or never made it
	}

	// Empty the pile first, then commit the pack. The other order is the
	// one that can duplicate; see the note at the top of this file.
	taken := append([]ItemStack(nil), g.Inv...)
	coins := g.Coins
	delete(w.Ground, g.ID)

	next := recFromPlayer(p)
	next.Coins += coins
	left := make([]ItemStack, 0, len(taken))
	got := make([]ItemStack, 0, len(taken))
	for _, it := range taken {
		before := countItem(next.Inv, it.ID)
		next.Inv = addItem(next.Inv, it.ID, it.N)
		gained := countItem(next.Inv, it.ID) - before
		if gained > 0 {
			got = append(got, ItemStack{ID: it.ID, N: gained})
		}
		if gained < it.N {
			left = append(left, ItemStack{ID: it.ID, N: it.N - gained})
		}
	}

	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			// Nothing was taken; put the pile back exactly as it was.
			w.Ground[g.ID] = g
			w.note(p.ID, "The hamlet hiccuped. The pile is still there.")
			return
		}
	}
	p.Inv = next.Inv
	p.Coins = next.Coins
	p.Dirty = false

	// Whatever would not fit goes back on the floor.
	if len(left) > 0 {
		w.dropPile(g.X, g.Y, left, 0)
	}

	picked := &GroundItem{Inv: got, Coins: coins}
	switch {
	case picked.empty():
		w.note(p.ID, "Your pack is too full to lift any of it.")
	case len(left) > 0:
		w.note(p.ID, "You take "+picked.label(protocol.Catalog())+". The rest will not fit.")
	default:
		w.note(p.ID, "You take "+picked.label(protocol.Catalog())+".")
	}
}
