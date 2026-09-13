package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

const (
	playerMaxHP = 10

	// Melee raises the damage you deal, Defense lowers the damage you take.
	// Both are flat and deterministic: the world has no randomness anywhere
	// (even NPC wander is derived from the tick number), and a swing that
	// sometimes misses would be the first thing to break that. It also
	// means a fight's outcome can be worked out on paper, which is the
	// property that makes the numbers below arguable.
	//
	//   melee  lv 1..3 -> 2   4..7 -> 3   8..11 -> 4  12..15 -> 5  16..19 -> 6  20 -> 7
	//   defense lv 1..5 -> 0  6..11 -> 1  12..17 -> 2  18..20 -> 3
	//
	// Damage taken never falls below minDamageTaken, so no amount of
	// Defense makes you safe to stand still in.
	baseMeleeDmg     = 2
	meleeDmgPerLevel = 4 // levels needed per extra point of damage
	defensePerLevel  = 6 // levels needed per point of damage absorbed
	minDamageTaken   = 1

	// XP. Melee pays for damage dealt, Defense for the raw force of what
	// hit you — raw, not what got through, or training Defense would slow
	// down exactly as it started working.
	meleeXPPerDamage   = 4
	defenseXPPerDamage = 6

	playerDmg       = baseMeleeDmg // starting damage, kept for tests
	thornkinHP      = 6
	thornkinDmg     = 1
	bramblebackHP   = 14
	bramblebackDmg  = 2
	npcRespawnTicks = 10
	tartHeal        = 4
	roastHeal       = 3
)

func (n *NPC) Living() bool {
	if n == nil || n.RespawnIn > 0 {
		return false
	}
	if n.Hostile {
		return n.HP > 0
	}
	return true
}

func (n *NPC) allows(x, y int) bool {
	if n.MaxX <= n.MinX && n.MaxY <= n.MinY {
		return true
	}
	return x >= n.MinX && x <= n.MaxX && y >= n.MinY && y <= n.MaxY
}

func (w *World) npcByID(id string) *NPC {
	for _, n := range w.NPCs {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (w *World) SetAttack(id, npcID string) {
	p := w.Players[id]
	npc := w.npcByID(npcID)
	if p == nil || npc == nil {
		return
	}
	if !npc.Hostile {
		w.note(id, npc.Name+" is no enemy of yours.")
		return
	}
	if !npc.Living() {
		w.note(id, "Only crushed bracken remains.")
		return
	}
	if npc.Target != "" && npc.Target != id {
		if other := w.Players[npc.Target]; other != nil && other.Online && other.Target == npc.ID {
			w.note(id, "The "+npc.Name+" is already locked in a fight.")
			return
		}
	}
	tx, ty, ok := w.nearestAdjacent(p.X, p.Y, npc.X, npc.Y)
	if !ok {
		w.note(id, "There is no ground beside the "+npc.Name+" to stand on.")
		return
	}
	first := p.Target != npcID
	p.Target = npcID
	p.HasDest = true
	p.DestX, p.DestY = tx, ty
	p.Path = nil
	p.ActionNode = ""
	p.ActionItem = ""
	if channeling(p.Action) {
		p.Action = "idle"
		p.ActionTicks = 0
	}
	if first {
		w.note(id, "You set upon the "+npc.Name+".")
	}
}

// destKeepsThreat reports whether a walk dest should leave the current
// fight lock alone. WASD and a missed click both arrive as SetDest; those
// used to wipe Target via cancelAction, so a player could take one swing
// of damage and then be unable to stay locked (or look unlocked).
// Keep the lock when the step is still beside the beast or not farther
// from it than we already are (closing in). Walking strictly away breaks off.
func (w *World) destKeepsThreat(p *Player, x, y int) bool {
	if p == nil || p.Target == "" {
		return false
	}
	npc := w.npcByID(p.Target)
	if !npc.Living() {
		return false
	}
	dDest := chebyshev(x, y, npc.X, npc.Y)
	dNow := chebyshev(p.X, p.Y, npc.X, npc.Y)
	return dDest <= 1 || dDest <= dNow
}

func (w *World) tickChase(p *Player) {
	if p.Target == "" {
		return
	}
	npc := w.npcByID(p.Target)
	if !npc.Living() {
		p.Target = ""
		if p.Action == protocol.ActionFight {
			p.Action = "idle"
		}
		return
	}
	if adjacent(p.X, p.Y, npc.X, npc.Y) {
		p.HasDest = false
		p.Path = nil
		return
	}
	tx, ty, ok := w.nearestAdjacent(p.X, p.Y, npc.X, npc.Y)
	if !ok {
		p.Target = ""
		return
	}
	if !p.HasDest || p.DestX != tx || p.DestY != ty {
		p.HasDest = true
		p.DestX, p.DestY = tx, ty
		p.Path = nil
	}
}

func (w *World) tickCombat(ctx context.Context, p *Player) {
	if p.Target == "" {
		if p.Action == protocol.ActionFight {
			p.Action = "idle"
		}
		return
	}
	npc := w.npcByID(p.Target)
	if !npc.Living() {
		p.Target = ""
		if p.Action == protocol.ActionFight {
			p.Action = "idle"
		}
		return
	}
	if !adjacent(p.X, p.Y, npc.X, npc.Y) {
		if p.Action == protocol.ActionFight {
			p.Action = "idle"
		}
		return
	}
	if channeling(p.Action) {
		return
	}
	p.HasDest = false
	p.Path = nil
	p.Action = protocol.ActionFight
	npc.Target = p.ID

	// You swing first. Landing the killing blow means taking nothing back,
	// which is why a fight you can only just win is still worth having.
	dealt := meleeDamage(skillLevel(p, protocol.SkillMelee))
	if dealt > npc.HP {
		dealt = npc.HP
	}
	npc.HP -= dealt
	if lv, up := addSkillXP(p.Skills, protocol.SkillMelee, dealt*meleeXPPerDamage); up {
		w.note(p.ID, fmt.Sprintf("Melee is now level %d.", lv))
	}
	p.Dirty = true
	if npc.HP <= 0 {
		w.fellNPC(ctx, npc, p)
		return
	}

	// Defense trains on the raw force of the blow, not on what got past
	// it, so getting better at absorbing does not slow down the learning.
	taken := damageAfterDefense(npc.Dmg, skillLevel(p, protocol.SkillDefense))
	if lv, up := addSkillXP(p.Skills, protocol.SkillDefense, npc.Dmg*defenseXPPerDamage); up {
		w.note(p.ID, fmt.Sprintf("Defense is now level %d.", lv))
	}
	p.HP -= taken
	if p.HP <= 0 {
		w.defeat(p)
	}
}

func (w *World) fellNPC(_ context.Context, npc *NPC, p *Player) {
	npc.HP = 0
	npc.Target = ""
	npc.RespawnIn = npcRespawnTicks
	if p != nil {
		p.Target = ""
		p.Action = "idle"
		w.note(p.ID, strings.TrimSpace("The "+npc.Name+" slumps into the bracken. "+w.spillDrops(npc, p.ID)))
	}
	for _, other := range w.Players {
		if other != nil && other.Target == npc.ID {
			other.Target = ""
			if other.Action == protocol.ActionFight {
				other.Action = "idle"
			}
		}
	}
}

// spillDrops rolls what a fallen beast was carrying and leaves it where it
// fell. Nothing is committed here: a pile is memory-only, and the item
// only becomes real when somebody picks it up and that write lands.
func (w *World) spillDrops(npc *NPC, killer string) string {
	coins := 0
	if npc.CoinsMax > 0 {
		lo, hi := npc.CoinsMin, npc.CoinsMax
		if lo < 0 {
			lo = 0
		}
		if hi < lo {
			hi = lo
		}
		coins = lo + w.roll(hi-lo+1)
	}
	var inv []ItemStack
	if npc.LeatherOdds > 0 && w.roll(npc.LeatherOdds) == 0 {
		inv = append(inv, ItemStack{ID: protocol.ItemLeather, N: 1})
	}
	g := w.dropPile(npc.X, npc.Y, inv, coins, killer)
	if g == nil {
		return ""
	}
	return "It leaves " + g.label(protocol.Catalog()) + " in the grass, yours for a minute."
}

// roll returns a value in [0,n). A world without a seeded roller — one
// built straight from a struct literal in a test — never drops.
func (w *World) roll(n int) int {
	if w.rng == nil || n <= 1 {
		return 0
	}
	return w.rng.Intn(n)
}

func (w *World) defeat(p *Player) {
	for _, npc := range w.NPCs {
		if npc.Target == p.ID {
			npc.Target = ""
		}
	}
	p.X, p.Y = spawnX, spawnY
	p.HP = p.MaxHP
	p.HasDest = false
	p.Path = nil
	p.Target = ""
	p.Action = "idle"
	p.ActionTicks = 0
	p.ActionNode = ""
	p.ActionItem = ""
	p.Dirty = true
	w.note(p.ID, "The world tilts. You wake at the stile, pack still on your shoulder.")
}

func (w *World) stepToward(npc *NPC, tx, ty int) {
	dx, dy := 0, 0
	if npc.X < tx {
		dx = 1
	} else if npc.X > tx {
		dx = -1
	}
	if npc.Y < ty {
		dy = 1
	} else if npc.Y > ty {
		dy = -1
	}
	try := [][2]int{{dx, 0}, {0, dy}, {dx, dy}}
	if abs(npc.Y-ty) > abs(npc.X-tx) {
		try = [][2]int{{0, dy}, {dx, 0}, {dx, dy}}
	}
	for _, d := range try {
		if d[0] == 0 && d[1] == 0 {
			continue
		}
		nx, ny := npc.X+d[0], npc.Y+d[1]
		if w.Walkable(nx, ny) && npc.allows(nx, ny) {
			npc.X, npc.Y = nx, ny
			return
		}
	}
}

// meleeDamage is what a player of this Melee level hits for.
func meleeDamage(level int) int {
	if level < 1 {
		level = 1
	}
	return baseMeleeDmg + level/meleeDmgPerLevel
}

// defenseReduction is how much of each incoming hit a player of this
// Defense level absorbs.
func defenseReduction(level int) int {
	if level < 1 {
		level = 1
	}
	return level / defensePerLevel
}

// damageAfterDefense applies that absorption, never below the floor.
func damageAfterDefense(raw, level int) int {
	got := raw - defenseReduction(level)
	if got < minDamageTaken {
		got = minDamageTaken
	}
	return got
}

func skillLevel(p *Player, id string) int {
	if p == nil || p.Skills == nil {
		return 1
	}
	if sk, ok := p.Skills[id]; ok && sk.Lv > 0 {
		return sk.Lv
	}
	return 1
}

func foodHeal(itemID string) int {
	switch itemID {
	case protocol.ItemTart:
		return tartHeal
	case protocol.ItemRoast:
		return roastHeal
	default:
		return 0
	}
}

func applyHeal(p *Player, n int) int {
	if p == nil || n <= 0 {
		return 0
	}
	if p.MaxHP <= 0 {
		p.MaxHP = playerMaxHP
	}
	before := p.HP
	p.HP += n
	if p.HP > p.MaxHP {
		p.HP = p.MaxHP
	}
	return p.HP - before
}
