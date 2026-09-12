package world

import (
	"context"
	"strings"
	"unicode"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

const (
	KindBush  = protocol.KindBush
	KindHazel = protocol.KindHazel
	KindMill  = protocol.KindMill
	KindFire  = protocol.KindFire

	forageTicks = 2
	millTicks   = 2
	cookTicks   = 3
	roastTicks  = 2
	forageXP    = 12
	millXP      = 10
	cookXP      = 18
	roastXP     = 14
	bushYield   = 3
	bushCD      = 12
	hazelYield  = 2
	hazelCD     = 14
	maxInvStack = 99
	spawnX      = 8
	spawnY      = 8
)

type ItemStack struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

type SkillState struct {
	XP int `json:"xp"`
	Lv int `json:"lv"`
}

type Player struct {
	ID           string
	Name         string
	X, Y         int
	HasDest      bool
	DestX, DestY int
	Path         []Point
	Action       string
	ActionTicks  int
	ActionNode   string
	ActionItem   string
	Inv          []ItemStack
	Skills       map[string]SkillState
	HP           int
	MaxHP        int
	Target       string
	LastChat     uint64
	Dirty        bool
	Online       bool
}

type Node struct {
	ID        string
	Kind      string
	X, Y      int
	Remaining int
	Max       int
	Cooldown  int
}

type NPC struct {
	ID          string
	Name        string
	X, Y        int
	HomeX       int
	HomeY       int
	WanderEvery int
	Hostile     bool
	HP          int
	MaxHP       int
	Dmg         int
	MinX, MaxX  int
	MinY, MaxY  int
	Target      string
	RespawnIn   int
}

type World struct {
	W, H    int
	Tiles   [][]byte
	Block   [][]bool
	Rows    []string
	Players map[string]*Player
	NPCs    []*NPC
	Nodes   map[string]*Node
	Store   Store
	TickN   uint64
	LastMs  float64
	notes   map[string]string
}

func New(store Store) *World {
	w, h, tiles, block := parseMap()
	world := &World{
		W:       w,
		H:       h,
		Tiles:   tiles,
		Block:   block,
		Rows:    append([]string(nil), mapRows...),
		Players: make(map[string]*Player),
		NPCs:    seedNPCs(),
		Nodes:   make(map[string]*Node),
		Store:   store,
		notes:   make(map[string]string),
	}
	for _, n := range seedNodes() {
		world.Nodes[n.ID] = n
	}
	return world
}

func (w *World) RestoreNodes(recs []NodeRec) {
	for _, r := range recs {
		n, ok := w.Nodes[r.ID]
		if !ok {
			continue
		}
		n.Remaining = r.Remaining
		n.Cooldown = r.Cooldown
	}
}

func SanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= protocol.MaxNameLen {
			break
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "Wanderer"
	}
	return s
}

func LevelFromXP(xp int) int {
	lv := 1
	need := 25
	rest := xp
	for lv < 20 && rest >= need {
		rest -= need
		lv++
		need += 15
	}
	return lv
}

func (w *World) UpsertPlayer(rec *PlayerRec, online bool) *Player {
	if rec.Skills == nil {
		rec.Skills = map[string]SkillState{}
	}
	ensureSkill(rec.Skills, protocol.SkillForage)
	ensureSkill(rec.Skills, protocol.SkillCook)
	p := &Player{
		ID:     rec.ID,
		Name:   SanitizeName(rec.Name),
		X:      rec.X,
		Y:      rec.Y,
		Inv:    append([]ItemStack(nil), rec.Inv...),
		Skills: rec.Skills,
		HP:     playerMaxHP,
		MaxHP:  playerMaxHP,
		Online: online,
	}
	if !w.Walkable(p.X, p.Y) {
		p.X, p.Y = spawnX, spawnY
	}
	w.Players[p.ID] = p
	return p
}

func NewPlayerRec(id, name string) *PlayerRec {
	return &PlayerRec{
		ID:   id,
		Name: SanitizeName(name),
		X:    spawnX,
		Y:    spawnY,
		Inv:  nil,
		Skills: map[string]SkillState{
			protocol.SkillForage: {Lv: 1},
			protocol.SkillCook:   {Lv: 1},
		},
	}
}

func (w *World) SetOnline(id string, on bool) {
	if p := w.Players[id]; p != nil {
		p.Online = on
	}
}

func (w *World) SetDest(id string, x, y int) {
	p := w.Players[id]
	if p == nil || !w.Walkable(x, y) {
		return
	}
	if p.Target != "" && !w.destKeepsThreat(p, x, y) {
		p.Target = ""
	}
	p.HasDest = true
	p.DestX, p.DestY = x, y
	p.Path = nil
	cancelAction(p)
}

func (w *World) SetInteract(id, nodeID string) {
	if w.npcByID(nodeID) != nil {
		w.SetAttack(id, nodeID)
		return
	}
	p := w.Players[id]
	n := w.Nodes[nodeID]
	if p == nil || n == nil {
		return
	}
	tx, ty, ok := w.nearestAdjacent(p.X, p.Y, n.X, n.Y)
	if !ok {
		return
	}
	p.Target = ""
	p.ActionNode = nodeID
	p.HasDest = true
	p.DestX, p.DestY = tx, ty
	p.Path = nil
	if channeling(p.Action) {
		p.Action = "idle"
		p.ActionTicks = 0
		p.ActionItem = ""
	}
}

func (w *World) TryChat(id, text string) (from, clean string, ok bool) {
	p := w.Players[id]
	if p == nil {
		return "", "", false
	}
	if w.TickN > 0 && w.TickN == p.LastChat {
		return "", "", false
	}
	clean = sanitizeChat(text)
	if clean == "" {
		return "", "", false
	}
	p.LastChat = w.TickN
	return p.Name, clean, true
}

func sanitizeChat(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= protocol.MaxChatLen {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// Tick advances the authoritative 600ms step. Item outcomes persist
// before memory is updated so a killed container cannot duplicate loot.
func (w *World) Tick(ctx context.Context) {
	w.TickN++
	w.tickNodes()
	w.tickNPCs()

	ids := make([]string, 0, len(w.Players))
	for id, p := range w.Players {
		if p.Online {
			ids = append(ids, id)
		}
	}
	sortStrings(ids)

	for _, id := range ids {
		p := w.Players[id]
		w.tickChase(p)
		w.tickMove(p)
		w.tickCombat(p)
		w.tickAction(ctx, p)
		if w.TickN%10 == 0 && p.Dirty {
			_ = w.Store.SavePlayer(ctx, recFromPlayer(p))
			p.Dirty = false
		}
	}
}

func (w *World) tickNodes() {
	for _, n := range w.Nodes {
		if !gathers(n.Kind) {
			continue
		}
		if n.Cooldown > 0 {
			n.Cooldown--
			if n.Cooldown == 0 && n.Remaining == 0 {
				n.Remaining = n.Max
				if w.Store != nil {
					_ = w.Store.UpsertNode(context.Background(), *recFromNode(n))
				}
			}
		}
	}
}

func (w *World) tickNPCs() {
	if w.TickN == 0 {
		return
	}
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for _, npc := range w.NPCs {
		if npc.RespawnIn > 0 {
			npc.RespawnIn--
			if npc.RespawnIn == 0 {
				npc.HP = npc.MaxHP
				npc.X, npc.Y = npc.HomeX, npc.HomeY
				npc.Target = ""
			}
			continue
		}
		if npc.Target != "" {
			pl := w.Players[npc.Target]
			if pl == nil || !pl.Online || pl.Target != npc.ID {
				npc.Target = ""
			} else if !adjacent(npc.X, npc.Y, pl.X, pl.Y) {
				w.stepToward(npc, pl.X, pl.Y)
				continue
			} else {
				continue
			}
		}
		if npc.WanderEvery <= 0 || w.TickN%uint64(npc.WanderEvery) != 0 {
			continue
		}
		d := dirs[int(w.TickN+uint64(len(npc.Name)))%4]
		nx, ny := npc.X+d[0], npc.Y+d[1]
		if w.Walkable(nx, ny) && npc.allows(nx, ny) {
			npc.X, npc.Y = nx, ny
		}
	}
}

func (w *World) tickMove(p *Player) {
	if !p.HasDest {
		if p.Action == "walk" {
			p.Action = "idle"
		}
		return
	}
	if p.X == p.DestX && p.Y == p.DestY {
		p.HasDest = false
		p.Path = nil
		return
	}
	if len(p.Path) == 0 {
		p.Path = w.FindPath(p.X, p.Y, p.DestX, p.DestY)
		if len(p.Path) == 0 {
			p.HasDest = false
			return
		}
	}
	step := p.Path[0]
	p.Path = p.Path[1:]
	if !w.Walkable(step.X, step.Y) {
		p.Path = nil
		return
	}
	p.X, p.Y = step.X, step.Y
	p.Action = "walk"
	p.Dirty = true
}

func (w *World) tickAction(ctx context.Context, p *Player) {
	if channeling(p.Action) {
		p.ActionTicks--
		if p.ActionTicks > 0 {
			return
		}
		w.completeAction(ctx, p)
		return
	}
	if p.ActionNode == "" || p.HasDest {
		return
	}
	n := w.Nodes[p.ActionNode]
	if n == nil {
		p.ActionNode = ""
		return
	}
	if !adjacent(p.X, p.Y, n.X, n.Y) {
		return
	}
	switch n.Kind {
	case KindBush, KindHazel:
		if n.Remaining <= 0 || n.Cooldown > 0 {
			p.ActionNode = ""
			return
		}
		p.Action = protocol.ActionForage
		p.ActionTicks = forageTicks
		p.ActionItem = forageItem(n.Kind)
	case KindMill:
		if countItem(p.Inv, protocol.ItemBerry) < 1 {
			w.note(p.ID, "The millstone waits for brambleberries.")
			p.ActionNode = ""
			return
		}
		p.Action = protocol.ActionMill
		p.ActionTicks = millTicks
		p.ActionItem = protocol.ItemBerry
	case KindFire:
		if countItem(p.Inv, protocol.ItemPulp) >= 1 {
			p.Action = protocol.ActionCook
			p.ActionTicks = cookTicks
			p.ActionItem = protocol.ItemPulp
			return
		}
		if countItem(p.Inv, protocol.ItemNut) >= 1 {
			p.Action = protocol.ActionRoast
			p.ActionTicks = roastTicks
			p.ActionItem = protocol.ItemNut
			return
		}
		if countItem(p.Inv, protocol.ItemBerry) >= 1 {
			w.note(p.ID, "The hearth wants pulp. Crush the berries at the millstone first.")
		} else {
			w.note(p.ID, "The hearth is quiet. Bring pulp or a hazel nut.")
		}
		p.ActionNode = ""
	default:
		p.ActionNode = ""
	}
}

func (w *World) completeAction(ctx context.Context, p *Player) {
	n := w.Nodes[p.ActionNode]
	action := p.Action
	item := p.ActionItem
	p.Action = "idle"
	p.ActionTicks = 0
	p.ActionNode = ""
	p.ActionItem = ""
	if n == nil {
		return
	}

	next := recFromPlayer(p)
	var nodeRec *NodeRec
	var flavor string

	switch action {
	case protocol.ActionForage:
		want := forageItem(n.Kind)
		if want == "" || n.Remaining <= 0 {
			return
		}
		next.Inv = addItem(next.Inv, want, 1)
		addSkillXP(next.Skills, protocol.SkillForage, forageXP)
		nr := *recFromNode(n)
		nr.Remaining--
		if nr.Remaining <= 0 {
			nr.Remaining = 0
			nr.Cooldown = gatherCD(n.Kind)
		}
		nodeRec = &nr
		if want == protocol.ItemNut {
			flavor = "A hazel nut comes free of its husk."
		} else {
			flavor = "You pick a brambleberry, still warm from the sun."
		}
	case protocol.ActionMill:
		if n.Kind != KindMill || countItem(next.Inv, protocol.ItemBerry) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemBerry, 1), protocol.ItemPulp, 1)
		addSkillXP(next.Skills, protocol.SkillCook, millXP)
		flavor = "You crush the berries on the millstone. The pulp smells of late summer."
	case protocol.ActionCook:
		if n.Kind != KindFire || item != protocol.ItemPulp || countItem(next.Inv, protocol.ItemPulp) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemPulp, 1), protocol.ItemTart, 1)
		addSkillXP(next.Skills, protocol.SkillCook, cookXP)
		flavor = "The hearth gives you a tart, glazed and crumbling."
	case protocol.ActionRoast:
		if n.Kind != KindFire || item != protocol.ItemNut || countItem(next.Inv, protocol.ItemNut) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemNut, 1), protocol.ItemRoast, 1)
		addSkillXP(next.Skills, protocol.SkillCook, roastXP)
		flavor = "The hazel nut pops. A little smoke, a little sweetness."
	default:
		return
	}

	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nodeRec); err != nil {
			return
		}
	}
	p.Inv = next.Inv
	p.Skills = next.Skills
	p.Dirty = false
	if nodeRec != nil {
		n.Remaining = nodeRec.Remaining
		n.Cooldown = nodeRec.Cooldown
	}
	if flavor != "" {
		w.note(p.ID, flavor)
	}
}

func (w *World) UseItem(ctx context.Context, id, itemID string) (string, bool) {
	p := w.Players[id]
	if p == nil {
		return "", false
	}
	switch itemID {
	case protocol.ItemTart:
		if countItem(p.Inv, protocol.ItemTart) < 1 {
			return "You do not have a hearth tart.", false
		}
		return w.commitUse(ctx, p, protocol.ItemTart, "You eat a hearth tart. It tastes like late summer.")
	case protocol.ItemRoast:
		if countItem(p.Inv, protocol.ItemRoast) < 1 {
			return "You do not have a roast hazel.", false
		}
		return w.commitUse(ctx, p, protocol.ItemRoast, "You eat a roast hazel. The shell-sweetness lingers.")
	case protocol.ItemBerry:
		return "Raw brambleberries make the eyes water. Crush them at the millstone first.", false
	case protocol.ItemPulp:
		return "The pulp wants a hearth, not a mouthful.", false
	case protocol.ItemNut:
		return "Too hard to chew raw. The hearth would be kinder.", false
	default:
		return "That stays in the pack.", false
	}
}

func (w *World) commitUse(ctx context.Context, p *Player, itemID, flavor string) (string, bool) {
	next := recFromPlayer(p)
	next.Inv = removeItem(next.Inv, itemID, 1)
	if w.Store != nil {
		if err := w.Store.CommitAction(ctx, next, nil); err != nil {
			return "The hamlet hiccuped. Try again.", false
		}
	}
	p.Inv = next.Inv
	p.Dirty = false
	if healed := applyHeal(p, foodHeal(itemID)); healed > 0 {
		flavor = flavor + " Strength returns."
	}
	return flavor, true
}

func (w *World) TakeNote(id string) string {
	if w.notes == nil {
		return ""
	}
	s := w.notes[id]
	delete(w.notes, id)
	return s
}

func (w *World) note(id, text string) {
	if w.notes == nil {
		w.notes = map[string]string{}
	}
	w.notes[id] = text
}

func (w *World) PersistPlayer(ctx context.Context, id string) {
	p := w.Players[id]
	if p == nil || w.Store == nil {
		return
	}
	_ = w.Store.SavePlayer(ctx, recFromPlayer(p))
	p.Dirty = false
}

func (w *World) Snapshot(id string) protocol.State {
	you := protocol.YouView{}
	if p := w.Players[id]; p != nil {
		you = youView(p)
	}
	players := make([]protocol.PlayerView, 0, len(w.Players))
	online := 0
	for _, p := range w.Players {
		if !p.Online {
			continue
		}
		online++
		if p.ID == id {
			continue
		}
		players = append(players, protocol.PlayerView{
			ID: p.ID, Name: p.Name, X: p.X, Y: p.Y, Action: p.Action,
		})
	}
	npcs := make([]protocol.NPCView, 0, len(w.NPCs))
	for _, n := range w.NPCs {
		if !n.Living() {
			continue
		}
		npcs = append(npcs, protocol.NPCView{
			ID: n.ID, Name: n.Name, X: n.X, Y: n.Y,
			HP: n.HP, MaxHP: n.MaxHP, Hostile: n.Hostile,
		})
	}
	nodes := make([]protocol.NodeView, 0, len(w.Nodes))
	for _, n := range w.Nodes {
		nodes = append(nodes, protocol.NodeView{
			ID:    n.ID,
			Kind:  n.Kind,
			X:     n.X,
			Y:     n.Y,
			Ready: nodeReady(n),
			Left:  n.Remaining,
		})
	}
	return protocol.State{
		T:       protocol.MsgState,
		N:       w.TickN,
		Ms:      w.LastMs,
		Online:  online,
		You:     you,
		Players: players,
		NPCs:    npcs,
		Nodes:   nodes,
	}
}

func (w *World) MapInfo() protocol.TileMap {
	return protocol.TileMap{W: w.W, H: w.H, Tile: tileSize, Tiles: w.Rows}
}

func (w *World) OnlineCount() int {
	n := 0
	for _, p := range w.Players {
		if p.Online {
			n++
		}
	}
	return n
}

func youView(p *Player) protocol.YouView {
	inv := make([]protocol.Item, 0, len(p.Inv))
	for _, it := range p.Inv {
		inv = append(inv, protocol.Item{ID: it.ID, N: it.N})
	}
	sk := make(map[string]protocol.Skill, len(p.Skills))
	for k, v := range p.Skills {
		sk[k] = protocol.Skill{XP: v.XP, Lv: v.Lv}
	}
	return protocol.YouView{
		PlayerView: protocol.PlayerView{ID: p.ID, Name: p.Name, X: p.X, Y: p.Y, Action: p.Action},
		Inv:        inv,
		Skills:     sk,
		HP:         p.HP,
		MaxHP:      p.MaxHP,
		Target:     p.Target,
	}
}

func nodeReady(n *Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == KindFire || n.Kind == KindMill {
		return true
	}
	return n.Remaining > 0 && n.Cooldown == 0
}

func gathers(kind string) bool {
	return kind == KindBush || kind == KindHazel
}

func channeling(action string) bool {
	switch action {
	case protocol.ActionForage, protocol.ActionMill, protocol.ActionCook, protocol.ActionRoast:
		return true
	default:
		return false
	}
}

func forageItem(kind string) string {
	switch kind {
	case KindBush:
		return protocol.ItemBerry
	case KindHazel:
		return protocol.ItemNut
	default:
		return ""
	}
}

func gatherCD(kind string) int {
	if kind == KindHazel {
		return hazelCD
	}
	return bushCD
}

func addSkillXP(skills map[string]SkillState, id string, xp int) {
	sk := skills[id]
	sk.XP += xp
	sk.Lv = LevelFromXP(sk.XP)
	skills[id] = sk
}

func ensureSkill(m map[string]SkillState, id string) {
	if _, ok := m[id]; !ok {
		m[id] = SkillState{Lv: 1}
	}
}

func countItem(inv []ItemStack, id string) int {
	for _, it := range inv {
		if it.ID == id {
			return it.N
		}
	}
	return 0
}

func addItem(inv []ItemStack, id string, n int) []ItemStack {
	for i := range inv {
		if inv[i].ID == id {
			inv[i].N += n
			if inv[i].N > maxInvStack {
				inv[i].N = maxInvStack
			}
			return inv
		}
	}
	if len(inv) >= protocol.InvSlots {
		return inv
	}
	return append(inv, ItemStack{ID: id, N: n})
}

func removeItem(inv []ItemStack, id string, n int) []ItemStack {
	out := inv[:0]
	for _, it := range inv {
		if it.ID == id {
			it.N -= n
			if it.N <= 0 {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func cancelAction(p *Player) {
	p.Action = "idle"
	p.ActionTicks = 0
	p.ActionNode = ""
	p.ActionItem = ""
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}
