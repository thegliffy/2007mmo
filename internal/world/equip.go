package world

import (
	"context"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Worn gear. An equipped item leaves the pack and lives in its slot, which
// is the whole point: a sword that cost nothing to carry was strictly
// better than the space it took, so there was no decision in it.
//
// The hand holds one thing. A blade swings harder; an axe fells trees.
// Wanting both means choosing which one is in your hand right now, and
// that is the choice the slot exists to create.

// Equip moves an item out of the pack and into its slot, returning
// whatever was already there.
func (w *World) Equip(ctx context.Context, id, itemID string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	slot := protocol.EquipSlot(itemID)
	if slot == "" {
		return "That is not something you can wear or wield.", false
	}
	if p.Equipped[slot] == itemID {
		return "You are already wielding the " + itemName(itemID) + ".", false
	}
	if countItem(p.Inv, itemID) < 1 {
		return "You have none of those.", false
	}

	next := recFromPlayer(p)
	next.Inv = removeItem(next.Inv, itemID, 1)
	if next.Equipped == nil {
		next.Equipped = map[string]string{}
	}
	previous := next.Equipped[slot]
	if previous != "" {
		// Swapping: the old one goes back in the pack, and if there is no
		// room for it the swap does not happen at all.
		before := len(next.Inv)
		next.Inv = addItem(next.Inv, previous, 1)
		if len(next.Inv) == before && countItem(next.Inv, previous) == countItem(p.Inv, previous) {
			return "Your pack is too full to stow what you are already carrying.", false
		}
	}
	next.Equipped[slot] = itemID

	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Equipped = next.Equipped
	p.Dirty = false

	if previous != "" {
		return "You take up the " + itemName(itemID) + ", stowing the " + itemName(previous) + ".", true
	}
	return "You take up the " + itemName(itemID) + ".", true
}

// Unequip puts a worn item back in the pack.
func (w *World) Unequip(ctx context.Context, id, slot string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	itemID := p.Equipped[slot]
	if itemID == "" {
		return "", false
	}

	next := recFromPlayer(p)
	before := len(next.Inv)
	next.Inv = addItem(next.Inv, itemID, 1)
	if len(next.Inv) == before && countItem(next.Inv, itemID) == countItem(p.Inv, itemID) {
		return "Your pack is full.", false
	}
	if next.Equipped == nil {
		next.Equipped = map[string]string{}
	}
	delete(next.Equipped, slot)

	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Equipped = next.Equipped
	p.Dirty = false
	return "You put the " + itemName(itemID) + " away.", true
}

// equippedInfo describes whatever is in a slot.
func equippedInfo(p *Player, slot string) protocol.ItemInfo {
	if p == nil || p.Equipped == nil {
		return protocol.ItemInfo{}
	}
	return protocol.Catalog()[p.Equipped[slot]]
}

// weaponBonus is what the thing in your hand adds to a swing.
func weaponBonus(p *Player) (attack, damage int) {
	info := equippedInfo(p, protocol.SlotHand)
	return info.Attack, info.Damage
}

// armourBonus is what worn gear adds to Defense, in levels.
func armourBonus(p *Player) int {
	total := 0
	for _, slot := range protocol.EquipSlots() {
		total += equippedInfo(p, slot).Defense
	}
	return total
}

// wielding reports whether the thing in hand is the tool for this work.
// Carrying an axe is no longer enough — it has to be in your hand, which
// is what stops one pack from doing every job at once.
func wielding(p *Player, verb string) bool {
	need := protocol.ToolFor(verb)
	if need == "" {
		return true
	}
	if protocol.EquipSlot(need) == "" {
		// Not equipment at all — flint and steel and the like are used
		// from the pack.
		return countItem(p.Inv, need) > 0
	}
	return p != nil && p.Equipped[protocol.EquipSlot(need)] == need
}

// copyEquipped is a defensive copy, so a stored record never shares a map
// with a live player.
func copyEquipped(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
