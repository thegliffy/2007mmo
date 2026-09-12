package world

import "github.com/thegliffy/2007mmo/internal/protocol"

const (
	playerMaxHP     = 10
	playerDmg       = 2
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

func (w *World) tickCombat(p *Player) {
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

	npc.HP -= playerDmg
	if npc.HP <= 0 {
		w.fellNPC(npc, p)
		return
	}
	p.HP -= npc.Dmg
	if p.HP <= 0 {
		w.defeat(p)
	}
}

func (w *World) fellNPC(npc *NPC, p *Player) {
	npc.HP = 0
	npc.Target = ""
	npc.RespawnIn = npcRespawnTicks
	if p != nil {
		p.Target = ""
		p.Action = "idle"
		w.note(p.ID, "The "+npc.Name+" slumps into the bracken.")
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
