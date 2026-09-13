package world

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"unicode"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

const (
	KindBush   = protocol.KindBush
	KindHazel  = protocol.KindHazel
	KindMill   = protocol.KindMill
	KindFire   = protocol.KindFire
	KindTree   = protocol.KindTree
	KindCopper = protocol.KindCopper
	KindTin    = protocol.KindTin
	KindKiln   = protocol.KindKiln
	KindAnvil  = protocol.KindAnvil
	KindChest  = protocol.KindChest

	forageTicks = 2
	millTicks   = 2
	cookTicks   = 3
	roastTicks  = 2
	chopTicks   = 3
	paperTicks  = 3
	mineTicks   = 3
	smeltTicks  = 3
	forgeTicks  = 3
	forageXP    = 12
	millXP      = 10
	cookXP      = 18
	roastXP     = 14
	chopXP      = 15
	paperXP     = 20
	mineXP      = 16
	smeltXP     = 22
	forgeXP     = 28
	treeYield   = 3
	treeCD      = 20
	oreYield    = 3
	oreCD       = 22
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
	ActionGround string
	ActionTrader string
	ActionChest  string
	ActionItem   string
	Inv          []ItemStack
	Bank         []ItemStack
	Skills       map[string]SkillState
	HP           int
	MaxHP        int
	Coins        int
	BankCoins    int
	Looks        protocol.Looks
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
	// Burns counts down for a campfire a player lit. Zero means the node
	// is part of the map and lives forever.
	Burns int
}

// temporary reports whether this node will burn out and vanish.
func (n *Node) temporary() bool { return n != nil && n.Burns > 0 }

type NPC struct {
	ID          string
	Name        string
	X, Y        int
	HomeX       int
	HomeY       int
	WanderEvery int
	Hostile     bool
	Trader      bool
	HP          int
	MaxHP       int
	Dmg         int
	MinX, MaxX  int
	MinY, MaxY  int
	Target      string
	RespawnIn   int
	// What it is worth. CoinsMin/Max are inclusive; LeatherOdds is a one-in-N
	// chance, zero meaning it never drops any.
	CoinsMin    int
	CoinsMax    int
	LeatherOdds int
}

type World struct {
	W, H      int
	Tiles     [][]byte
	Block     [][]bool
	Rows      []string
	Players   map[string]*Player
	NPCs      []*NPC
	Nodes     map[string]*Node
	Ground    map[string]*GroundItem
	Store     Store
	TickN     uint64
	LastMs    float64
	groundSeq int
	notes     map[string]string
	tradeOpen map[string]string
	bankOpen  map[string]string
	// rng rolls loot. The simulation itself stays deterministic — this is
	// only ever consulted for drops, which no crash-recovery guarantee
	// depends on. Owned by the World rather than global so a test can pin
	// the seed.
	rng *rand.Rand
	// handles maps a real player id to the opaque per-session handle that
	// peers are allowed to see. Rotated on every join so a handle cannot
	// be used to follow someone across sessions.
	handles map[string]string
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
		Ground:  make(map[string]*GroundItem),
		Store:   store,
		notes:   make(map[string]string),
		handles: make(map[string]string),
		rng:     rand.New(rand.NewSource(seedFromCrypto())),
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
	for id := range protocol.SkillCatalog() {
		ensureSkill(rec.Skills, id)
	}
	// Health survives a logout. Without this, disconnecting mid-fight was
	// a free full heal while the beast kept its wounds.
	hp := playerMaxHP
	if rec.HP != nil && *rec.HP > 0 && *rec.HP <= playerMaxHP {
		hp = *rec.HP
	}
	p := &Player{
		ID:     rec.ID,
		Name:   SanitizeName(rec.Name),
		X:      rec.X,
		Y:      rec.Y,
		Inv:       append([]ItemStack(nil), rec.Inv...),
		Bank:      append([]ItemStack(nil), rec.Bank...),
		Skills:    rec.Skills,
		HP:        hp,
		MaxHP:     playerMaxHP,
		Coins:     rec.Coins,
		BankCoins: rec.BankCoins,
		Looks:     rec.Looks,
		Online: online,
	}
	if !w.Walkable(p.X, p.Y) {
		p.X, p.Y = spawnX, spawnY
	}
	w.Players[p.ID] = p
	return p
}

func NewPlayerRec(id, name string) *PlayerRec {
	full := playerMaxHP
	return &PlayerRec{
		ID:     id,
		Name:   SanitizeName(name),
		X:      spawnX,
		Y:      spawnY,
		HP:     &full,
		Inv:    nil,
		Skills: newSkillSet(),
	}
}

func (w *World) SetOnline(id string, on bool) {
	if p := w.Players[id]; p != nil {
		p.Online = on
	}
	if !on {
		delete(w.handles, id)
	}
}

// RotateHandle mints a fresh peer-visible handle for a player and returns
// it. Called on join, so the handle a peer saw last session is useless.
func (w *World) RotateHandle(id string) string {
	if w.handles == nil {
		w.handles = make(map[string]string)
	}
	h := newHandle()
	w.handles[id] = h
	return h
}

// HandleFor returns the peer-visible handle for a player, minting one if
// the player is somehow live without a handle.
func (w *World) HandleFor(id string) string {
	if h := w.handles[id]; h != "" {
		return h
	}
	return w.RotateHandle(id)
}

// SeedLoot pins the loot roller, so a test can assert on a drop instead
// of on a probability.
func (w *World) SeedLoot(seed int64) { w.rng = rand.New(rand.NewSource(seed)) }

// seedFromCrypto starts the loot roller somewhere unpredictable.
func seedFromCrypto() int64 {
	var b [8]byte
	var n int64
	if _, err := cryptorand.Read(b[:]); err != nil {
		return 1
	}
	for _, v := range b {
		n = n<<8 | int64(v)
	}
	if n < 0 {
		n = -n
	}
	return n
}

// newHandle is 96 bits of randomness: unguessable, but carries no
// meaning and grants nothing on its own.
func newHandle() string {
	b := make([]byte, 12)
	if _, err := cryptorand.Read(b); err != nil {
		panic("world: crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
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
	if npc := w.npcByID(nodeID); npc != nil {
		if npc.Trader {
			w.SetTrade(id, nodeID)
			return
		}
		w.SetAttack(id, nodeID)
		return
	}
	if w.groundByID(nodeID) != nil {
		w.SetPickup(id, nodeID)
		return
	}
	p := w.Players[id]
	n := w.Nodes[nodeID]
	if p == nil || n == nil {
		return
	}
	if n.Kind == KindChest {
		w.SetChest(id, nodeID)
		return
	}
	p.ActionChest = ""
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
	w.tickNodes(ctx)
	w.tickGround()
	w.tickNPCs()

	ids := make([]string, 0, len(w.Players))
	for id, p := range w.Players {
		if p.Online {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	for _, id := range ids {
		p := w.Players[id]
		w.tickChase(p)
		w.tickMove(p)
		w.tickCombat(ctx, p)
		w.tickAction(ctx, p)
		w.tickPickup(ctx, p)
		w.tickTrade(p)
		w.tickChest(p)
		if w.TickN%10 == 0 && p.Dirty {
			_ = w.Store.SavePlayer(ctx, recFromPlayer(p))
			p.Dirty = false
		}
	}
}

// tickNodes runs inside the authoritative step, so its store write is
// bounded: a stalled Postgres must degrade into a skipped node refresh,
// never into a frozen world.
func (w *World) tickNodes(ctx context.Context) {
	for id, n := range w.Nodes {
		// A campfire burns down and goes, whatever kind it is. This runs
		// before the gathering check because a fire is not a gathering
		// node and would otherwise never be ticked at all.
		if n.temporary() {
			n.Burns--
			if n.Burns <= 0 {
				delete(w.Nodes, id)
				continue
			}
		}
		if !gathers(n.Kind) {
			continue
		}
		if n.Cooldown > 0 {
			n.Cooldown--
			if n.Cooldown == 0 && n.Remaining == 0 {
				n.Remaining = n.Max
				if w.Store != nil && !n.temporary() {
					_ = w.Store.UpsertNode(ctx, *recFromNode(n))
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
	case KindBush, KindHazel, KindTree, KindCopper, KindTin:
		if n.Remaining <= 0 || n.Cooldown > 0 {
			p.ActionNode = ""
			return
		}
		if n.Kind == KindTree {
			if !hasTool(p.Inv, "chop") {
				w.note(p.ID, "You would need an axe for that.")
				p.ActionNode = ""
				return
			}
			p.Action = protocol.ActionChop
			p.ActionTicks = chopTicks
		} else if n.Kind == KindCopper || n.Kind == KindTin {
			if !hasTool(p.Inv, "mine") {
				w.note(p.ID, "You would need a pick for that.")
				p.ActionNode = ""
				return
			}
			p.Action = protocol.ActionMine
			p.ActionTicks = mineTicks
		} else {
			p.Action = protocol.ActionForage
			p.ActionTicks = forageTicks
		}
		p.ActionItem = forageItem(n.Kind)
	case KindMill:
		// The millstone crushes berries into pulp and pulps logs into
		// paper, whichever you are carrying.
		if countItem(p.Inv, protocol.ItemBerry) >= 1 {
			p.Action = protocol.ActionMill
			p.ActionTicks = millTicks
			p.ActionItem = protocol.ItemBerry
			return
		}
		if countItem(p.Inv, protocol.ItemLog) >= 1 {
			p.Action = protocol.ActionPaper
			p.ActionTicks = paperTicks
			p.ActionItem = protocol.ItemLog
			return
		}
		w.note(p.ID, "The millstone waits for brambleberries, or a log to pulp.")
		p.ActionNode = ""
		return
	case KindKiln:
		// The kiln is already hot, like the hearth. Copper and tin
		// together make a bronze bar — one beginner alloy, not a menu.
		if countItem(p.Inv, protocol.ItemCopper) >= 1 && countItem(p.Inv, protocol.ItemTin) >= 1 {
			p.Action = protocol.ActionSmelt
			p.ActionTicks = smeltTicks
			p.ActionItem = protocol.ItemBar
			return
		}
		if countItem(p.Inv, protocol.ItemCopper) >= 1 {
			w.note(p.ID, "The kiln wants tin as well. Copper alone will not run.")
		} else if countItem(p.Inv, protocol.ItemTin) >= 1 {
			w.note(p.ID, "The kiln wants copper as well. Tin alone will not run.")
		} else {
			w.note(p.ID, "The kiln is hot. Bring copper and tin together.")
		}
		p.ActionNode = ""
		return
	case KindAnvil:
		if countItem(p.Inv, protocol.ItemBar) >= 1 {
			p.Action = protocol.ActionForge
			p.ActionTicks = forgeTicks
			p.ActionItem = protocol.ItemBar
			return
		}
		w.note(p.ID, "The anvil waits for a bronze bar.")
		p.ActionNode = ""
		return
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
	var lv int
	var leveled bool

	switch action {
	case protocol.ActionForage, protocol.ActionChop, protocol.ActionMine:
		want := forageItem(n.Kind)
		if want == "" || n.Remaining <= 0 {
			return
		}
		before := countItem(next.Inv, want)
		next.Inv = addItem(next.Inv, want, 1)
		if countItem(next.Inv, want) == before {
			w.note(p.ID, "Your pack is full.")
			return
		}
		switch {
		case want == protocol.ItemLog:
			lv, leveled = addSkillXP(next.Skills, protocol.SkillWood, chopXP)
		case want == protocol.ItemCopper || want == protocol.ItemTin:
			lv, leveled = addSkillXP(next.Skills, protocol.SkillMine, mineXP)
		default:
			lv, leveled = addSkillXP(next.Skills, protocol.SkillForage, forageXP)
		}
		nr := *recFromNode(n)
		nr.Remaining--
		if nr.Remaining <= 0 {
			nr.Remaining = 0
			nr.Cooldown = gatherCD(n.Kind)
		}
		nodeRec = &nr
		switch want {
		case protocol.ItemNut:
			flavor = "A hazel nut comes free of its husk."
		case protocol.ItemLog:
			flavor = "The bough comes away. A good log."
		case protocol.ItemCopper:
			flavor = "The vein gives up a lump of copper, still cold."
		case protocol.ItemTin:
			flavor = "A pale chip of tin comes free of the rock."
		default:
			flavor = "You pick a brambleberry, still warm from the sun."
		}
	case protocol.ActionPaper:
		if n.Kind != KindMill || item != protocol.ItemLog || countItem(next.Inv, protocol.ItemLog) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemLog, 1), protocol.ItemPaper, 1)
		lv, leveled = addSkillXP(next.Skills, protocol.SkillWood, paperXP)
		flavor = "The millstone worries the log to pulp, and the pulp dries to paper."
	case protocol.ActionMill:
		if n.Kind != KindMill || countItem(next.Inv, protocol.ItemBerry) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemBerry, 1), protocol.ItemPulp, 1)
		lv, leveled = addSkillXP(next.Skills, protocol.SkillCook, millXP)
		flavor = "You crush the berries on the millstone. The pulp smells of late summer."
	case protocol.ActionCook:
		if n.Kind != KindFire || item != protocol.ItemPulp || countItem(next.Inv, protocol.ItemPulp) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemPulp, 1), protocol.ItemTart, 1)
		lv, leveled = addSkillXP(next.Skills, protocol.SkillCook, cookXP)
		flavor = "The hearth gives you a tart, glazed and crumbling."
	case protocol.ActionRoast:
		if n.Kind != KindFire || item != protocol.ItemNut || countItem(next.Inv, protocol.ItemNut) < 1 {
			return
		}
		next.Inv = addItem(removeItem(next.Inv, protocol.ItemNut, 1), protocol.ItemRoast, 1)
		lv, leveled = addSkillXP(next.Skills, protocol.SkillCook, roastXP)
		flavor = "The hazel nut pops. A little smoke, a little sweetness."
	case protocol.ActionSmelt:
		if n.Kind != KindKiln || countItem(next.Inv, protocol.ItemCopper) < 1 || countItem(next.Inv, protocol.ItemTin) < 1 {
			return
		}
		next.Inv = removeItem(next.Inv, protocol.ItemCopper, 1)
		next.Inv = removeItem(next.Inv, protocol.ItemTin, 1)
		before := countItem(next.Inv, protocol.ItemBar)
		next.Inv = addItem(next.Inv, protocol.ItemBar, 1)
		if countItem(next.Inv, protocol.ItemBar) == before {
			w.note(p.ID, "Your pack is full.")
			return
		}
		lv, leveled = addSkillXP(next.Skills, protocol.SkillSmith, smeltXP)
		flavor = "The kiln drinks both ores. A bronze bar, dull and heavy."
	case protocol.ActionForge:
		if n.Kind != KindAnvil || item != protocol.ItemBar || countItem(next.Inv, protocol.ItemBar) < 1 {
			return
		}
		next.Inv = removeItem(next.Inv, protocol.ItemBar, 1)
		before := countItem(next.Inv, protocol.ItemKnife)
		next.Inv = addItem(next.Inv, protocol.ItemKnife, 1)
		if countItem(next.Inv, protocol.ItemKnife) == before {
			w.note(p.ID, "Your pack is full.")
			return
		}
		lv, leveled = addSkillXP(next.Skills, protocol.SkillSmith, forgeXP)
		flavor = "The hammer rings. A bronze knife, rough but keen."
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
	if leveled {
		flavor = strings.TrimSpace(flavor + " " + levelUpLine(action, lv))
	}
	if flavor != "" {
		w.note(p.ID, flavor)
	}
}

// levelUpLine names the skill that moved, since one action can feed
// either Foraging or Cooking.
func levelUpLine(action string, lv int) string {
	name := "Cooking"
	switch action {
	case protocol.ActionForage:
		name = "Foraging"
	case protocol.ActionChop, protocol.ActionPaper:
		name = "Woodcutting"
	case protocol.ActionMine:
		name = "Mining"
	case protocol.ActionSmelt, protocol.ActionForge:
		name = "Smithing"
	}
	return fmt.Sprintf("(%s is now level %d.)", name, lv)
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
	case protocol.ItemCopper, protocol.ItemTin:
		return "Ore wants the kiln, not a pocket to rattle in.", false
	case protocol.ItemBar:
		return "A bar is for the anvil.", false
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
		p.Dirty = true
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
		you = w.youView(p)
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
			ID: w.HandleFor(p.ID), Name: p.Name, X: p.X, Y: p.Y, Action: p.Action,
			Looks: looksView(p.Looks),
		})
	}
	npcs := make([]protocol.NPCView, 0, len(w.NPCs))
	for _, n := range w.NPCs {
		if !n.Living() {
			continue
		}
		npcs = append(npcs, protocol.NPCView{
			ID: n.ID, Name: n.Name, X: n.X, Y: n.Y,
			HP: n.HP, MaxHP: n.MaxHP, Hostile: n.Hostile, Trader: n.Trader,
		})
	}
	// Only what is not at rest. The client already has every node's
	// position and kind from the welcome, and with 75 trees on the map
	// resending all of them 1.67 times a second to every client was most
	// of the frame for nothing. A campfire is always included: the client
	// was never told about it, because it did not exist at welcome time.
	nodes := make([]protocol.NodeView, 0, 8)
	for _, n := range w.Nodes {
		atRest := nodeReady(n) && n.Remaining == n.Max
		if atRest && !n.temporary() {
			continue
		}
		nodes = append(nodes, w.nodeView(n))
	}
	// A reserved pile is left out of everyone else's snapshot entirely,
	// rather than sent and then refused: nobody should be able to see that
	// someone nearby just got lucky.
	ground := make([]protocol.GroundView, 0, len(w.Ground))
	items := protocol.Catalog()
	for _, g := range w.Ground {
		if !g.visibleTo(id) {
			continue
		}
		ground = append(ground, protocol.GroundView{
			ID: g.ID, X: g.X, Y: g.Y, Label: g.label(items), Coins: g.Coins,
			Mine: g.private() && g.Owner == id,
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
		Ground:  ground,
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

func (w *World) youView(p *Player) protocol.YouView {
	inv := make([]protocol.Item, 0, len(p.Inv))
	for _, it := range p.Inv {
		inv = append(inv, protocol.Item{ID: it.ID, N: it.N})
	}
	sk := make(map[string]protocol.Skill, len(p.Skills))
	for k, v := range p.Skills {
		sk[k] = protocol.Skill{XP: v.XP, Lv: v.Lv}
	}
	return protocol.YouView{
		PlayerView: protocol.PlayerView{
			ID: w.HandleFor(p.ID), Name: p.Name, X: p.X, Y: p.Y, Action: p.Action,
			Looks: looksView(p.Looks),
		},
		Inv:       inv,
		Skills:    sk,
		HP:        p.HP,
		MaxHP:     p.MaxHP,
		Coins:     p.Coins,
		Bank:      bankView(p.Bank),
		BankCoins: p.BankCoins,
		Target:    p.Target,
	}
}

// looksView ships a finished creator pass to peers. Unset stays omitted
// so a load bot without a face does not look like a chosen palette.
func looksView(l protocol.Looks) *protocol.Looks {
	if !l.Set() {
		return nil
	}
	cp := l
	return &cp
}

// nodeView is the full description, used in the welcome and for campfires
// the client has not seen before.
func (w *World) nodeView(n *Node) protocol.NodeView {
	return protocol.NodeView{
		ID:    n.ID,
		Kind:  n.Kind,
		X:     n.X,
		Y:     n.Y,
		Ready: nodeReady(n),
		Left:  n.Remaining,
		Burns: n.Burns,
	}
}

// AllNodes is every node with its position, sent once at join.
func (w *World) AllNodes() []protocol.NodeView {
	out := make([]protocol.NodeView, 0, len(w.Nodes))
	for _, n := range w.Nodes {
		out = append(out, w.nodeView(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func nodeReady(n *Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == KindFire || n.Kind == KindMill || n.Kind == KindKiln || n.Kind == KindAnvil || n.Kind == KindChest {
		return true
	}
	return n.Remaining > 0 && n.Cooldown == 0
}

func gathers(kind string) bool {
	return kind == KindBush || kind == KindHazel || kind == KindTree ||
		kind == KindCopper || kind == KindTin
}

func channeling(action string) bool {
	switch action {
	case protocol.ActionForage, protocol.ActionMill, protocol.ActionCook, protocol.ActionRoast,
		protocol.ActionChop, protocol.ActionPaper, protocol.ActionMine, protocol.ActionSmelt,
		protocol.ActionForge:
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
	case KindTree:
		return protocol.ItemLog
	case KindCopper:
		return protocol.ItemCopper
	case KindTin:
		return protocol.ItemTin
	default:
		return ""
	}
}

func gatherCD(kind string) int {
	switch kind {
	case KindHazel:
		return hazelCD
	case KindTree:
		return treeCD
	case KindCopper, KindTin:
		return oreCD
	}
	return bushCD
}

// addSkillXP grants xp and reports whether that crossed a level.
func addSkillXP(skills map[string]SkillState, id string, xp int) (level int, leveled bool) {
	sk := skills[id]
	before := sk.Lv
	sk.XP += xp
	sk.Lv = LevelFromXP(sk.XP)
	skills[id] = sk
	return sk.Lv, sk.Lv > before
}

// newSkillSet gives a fresh character every skill the world knows about,
// so a character made before a skill existed and one made after look the
// same to everything downstream.
func newSkillSet() map[string]SkillState {
	out := make(map[string]SkillState, len(protocol.SkillCatalog()))
	for id := range protocol.SkillCatalog() {
		out[id] = SkillState{Lv: 1}
	}
	return out
}

func ensureSkill(m map[string]SkillState, id string) {
	if _, ok := m[id]; !ok {
		m[id] = SkillState{Lv: 1}
	}
}

func countItem(inv []ItemStack, id string) int {
	n := 0
	for _, it := range inv {
		if it.ID == id {
			n += it.N
		}
	}
	return n
}

// hasTool reports whether the pack holds the tool a piece of work needs.
// Carrying it is enough — there is no equip slot, so a tool in the pack is
// a tool in the hand.
func hasTool(inv []ItemStack, verb string) bool {
	need := protocol.ToolFor(verb)
	return need == "" || countItem(inv, need) > 0
}

// bestWeapon returns the attack and damage of the best blade in the pack.
func bestWeapon(inv []ItemStack) (attack, damage int) {
	cat := protocol.Catalog()
	for _, it := range inv {
		info, ok := cat[it.ID]
		if !ok || !info.Tool {
			continue
		}
		if info.Attack+info.Damage > attack+damage {
			attack, damage = info.Attack, info.Damage
		}
	}
	return attack, damage
}

// addItem puts n of an item in the pack. Tools take a slot each and never
// stack — two axes are two axes, in two slots, so a pack of tools costs
// you the room it looks like it costs.
func addItem(inv []ItemStack, id string, n int) []ItemStack {
	return addItemCapped(inv, id, n, protocol.InvSlots)
}

func addItemCapped(inv []ItemStack, id string, n, cap int) []ItemStack {
	if protocol.IsTool(id) {
		for i := 0; i < n; i++ {
			if len(inv) >= cap {
				return inv
			}
			inv = append(inv, ItemStack{ID: id, N: 1})
		}
		return inv
	}
	for i := range inv {
		if inv[i].ID == id {
			inv[i].N += n
			if inv[i].N > maxInvStack {
				inv[i].N = maxInvStack
			}
			return inv
		}
	}
	if len(inv) >= cap {
		return inv
	}
	return append(inv, ItemStack{ID: id, N: n})
}

// removeItem returns a new slice. It used to filter in place through
// inv[:0], which was safe only because every caller happened to pass a
// fresh copy — a nasty thing to rely on when this is the code path where
// aliasing would mean duplicated items.
func removeItem(inv []ItemStack, id string, n int) []ItemStack {
	out := make([]ItemStack, 0, len(inv))
	left := n
	for _, it := range inv {
		if it.ID == id && left > 0 {
			if protocol.IsTool(id) {
				// One slot, one tool: drop this entry and move on.
				left--
				continue
			}
			take := left
			if take > it.N {
				take = it.N
			}
			it.N -= take
			left -= take
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
	p.ActionGround = ""
	p.ActionTrader = ""
	p.ActionChest = ""
	p.ActionItem = ""
}
