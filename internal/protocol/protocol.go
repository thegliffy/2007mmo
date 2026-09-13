// Package protocol is the shared JSON WebSocket contract between
// the browser client, the world server, and the headless bot harness.
package protocol

const (
	TickMs     = 600
	WorldName  = "hollowmere"
	MaxNameLen = 16
	MaxChatLen = 80
	InvSlots   = 28

	MsgHello    = "hello"
	MsgMove     = "move"
	MsgInteract = "interact"
	MsgAttack   = "attack"
	MsgUse      = "use"
	MsgChat     = "chat"
	MsgDrop     = "drop"
	MsgLight    = "light"
	MsgBuy      = "buy"
	MsgSell     = "sell"
	MsgPing     = "ping"

	MsgWelcome = "welcome"
	MsgState   = "state"
	// Same wire tag as MsgChat on purpose: direction disambiguates. A
	// client sends {"t":"chat"} to speak, the server sends {"t":"chat"}
	// to relay. Named twice so call sites read correctly.
	MsgSaid  = "chat"
	MsgEvent = "evt"
	MsgPong  = "pong"
	MsgErr   = "err"
	MsgTrade = "trade"

	// SessionCookie carries the login session. HttpOnly: script must not
	// be able to read or forward it.
	SessionCookie = "hollowmere_session"

	ItemBerry = "berry"
	ItemPulp  = "pulp"
	ItemTart  = "tart"
	ItemNut   = "nut"
	ItemRoast = "roast"
	// Dropped by southern beasts. Coins are deliberately not an item:
	// they live in a purse and never cost a pack slot.
	ItemLeather = "leather"
	ItemLog     = "log"
	ItemPaper   = "paper"

	SkillForage  = "forage"
	SkillCook    = "cook"
	SkillMelee   = "melee"
	SkillDefense = "defense"
	SkillWood    = "wood"

	KindBush  = "bush"
	KindHazel = "hazel"
	KindMill  = "mill"
	KindFire  = "fire"
	KindTree  = "tree"

	ActionForage = "forage"
	ActionMill   = "mill"
	ActionCook   = "cook"
	ActionRoast  = "roast"
	ActionChop   = "chop"
	ActionPaper  = "paper"
	ActionFight  = "fight"
)

// In is every client → server frame. Unused fields stay empty.
//
// It deliberately carries no identity. Who you are is decided by the
// session cookie on the WebSocket upgrade, so a client cannot name a
// player id or a session token and be believed.
type In struct {
	T    string `json:"t"`
	X    int    `json:"x,omitempty"`
	Y    int    `json:"y,omitempty"`
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
	Ts   int64  `json:"ts,omitempty"`
}

type Item struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

type Skill struct {
	XP int `json:"xp"`
	Lv int `json:"lv"`
}

// PlayerView is what one player is allowed to know about another.
//
// ID is an opaque per-session handle, not the player's real id. Peers
// need something stable across ticks to interpolate sprites; they must
// never learn an identifier that could be replayed as a credential.
type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Action string `json:"action,omitempty"`
}

type YouView struct {
	PlayerView
	Inv    []Item           `json:"inv"`
	Skills map[string]Skill `json:"skills"`
	HP     int              `json:"hp"`
	MaxHP  int              `json:"maxHp"`
	Coins  int              `json:"coins"`
	Target string           `json:"target,omitempty"`
}

// GroundView is a pile lying in the grass.
// TradeOffer is one line of the pedlar's board. Pays is what she gives
// for one; Costs is what she charges. Zero means she will not deal in it
// that direction.
type TradeOffer struct {
	Item  string `json:"item"`
	Pays  int    `json:"pays,omitempty"`
	Costs int    `json:"costs,omitempty"`
}

// Trade is pushed when a player reaches the pedlar.
type Trade struct {
	T      string       `json:"t"`
	With   string       `json:"with"`
	Offers []TradeOffer `json:"offers"`
}

type GroundView struct {
	ID    string `json:"id"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Label string `json:"label"`
	Coins int    `json:"coins,omitempty"`
	// Mine marks a pile still reserved for the player being sent it.
	Mine bool `json:"mine,omitempty"`
}

// NodeView is a work node. Position and kind never change, so the full
// list ships once in the welcome and state frames carry only the nodes
// that are NOT in their resting state — which is nearly none of them.
// With 75 choppable trees on the map, sending all of them 1.67 times a
// second to every client was most of the state frame for no reason.
type NodeView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind,omitempty"`
	X     int    `json:"x,omitempty"`
	Y     int    `json:"y,omitempty"`
	Ready bool   `json:"ready"`
	Left  int    `json:"left,omitempty"`
	// Fire marks a campfire a player lit; Burns is how many ticks it has
	// left before it dies down.
	Burns int `json:"burns,omitempty"`
}

type NPCView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	HP      int    `json:"hp,omitempty"`
	MaxHP   int    `json:"maxHp,omitempty"`
	Hostile bool   `json:"hostile,omitempty"`
	Trader  bool   `json:"trader,omitempty"`
}

type TileMap struct {
	W     int      `json:"w"`
	H     int      `json:"h"`
	Tile  int      `json:"tile"`
	Tiles []string `json:"tiles"`
}

type ItemInfo struct {
	Name  string `json:"name"`
	Glyph string `json:"glyph"`
}

type Welcome struct {
	T        string               `json:"t"`
	Handle   string               `json:"handle"`
	Username string               `json:"username"`
	TickMs   int                  `json:"tickMs"`
	World    string               `json:"world"`
	Map      TileMap              `json:"map"`
	Nodes    []NodeView           `json:"nodes"`
	You      YouView              `json:"you"`
	Items    map[string]ItemInfo  `json:"items"`
	Skills   map[string]SkillInfo `json:"skillInfo"`
}

// Auth frames are HTTP JSON, not WebSocket, but they live here so the
// browser client, the bot harness, and the smoke scripts agree.

type AuthRequest struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// Current and Next are used by POST /auth/password.
	Current string `json:"current,omitempty"`
	Next    string `json:"next,omitempty"`
}

type AuthResponse struct {
	Username string `json:"username"`
	World    string `json:"world"`
}

type AuthError struct {
	Error string `json:"error"`
}

type State struct {
	T       string       `json:"t"`
	N       uint64       `json:"n"`
	Ms      float64      `json:"ms"`
	Online  int          `json:"online"`
	You     YouView      `json:"you"`
	Players []PlayerView `json:"players"`
	NPCs    []NPCView    `json:"npcs"`
	// Nodes carries only what is not at rest. Anything absent is ready.
	Nodes  []NodeView   `json:"nodes"`
	Ground []GroundView `json:"ground"`
}

type Chat struct {
	T    string `json:"t"`
	From string `json:"from"`
	Text string `json:"text"`
	Kind string `json:"kind,omitempty"`
}

type Event struct {
	T    string `json:"t"`
	Text string `json:"text"`
}

type Err struct {
	T   string `json:"t"`
	Msg string `json:"msg"`
}

type Pong struct {
	T  string `json:"t"`
	Ts int64  `json:"ts"`
}

type Stats struct {
	World     string  `json:"world"`
	Tick      uint64  `json:"tick"`
	TickMs    int     `json:"tickMs"`
	TickP50Ms float64 `json:"tickP50Ms"`
	TickP99Ms float64 `json:"tickP99Ms"`
	TickMaxMs float64 `json:"tickMaxMs"`
	// Loop covers the tick plus fanning state out to every client; Lag is
	// how late each tick fired. Tick* alone excludes the fan-out.
	LoopP50Ms     float64 `json:"loopP50Ms"`
	LoopP99Ms     float64 `json:"loopP99Ms"`
	LoopMaxMs     float64 `json:"loopMaxMs"`
	LagP50Ms      float64 `json:"lagP50Ms"`
	LagP99Ms      float64 `json:"lagP99Ms"`
	LagMaxMs      float64 `json:"lagMaxMs"`
	FramesDropped uint64  `json:"framesDropped"`
	Samples       int     `json:"samples"`
	Online        int     `json:"online"`
	WS            int     `json:"ws"`
	PlayersMem    int     `json:"playersMem"`
	Joins         uint64  `json:"joins"`
	Chats         uint64  `json:"chats"`
	Actions       uint64  `json:"actions"`
	LimitedHello  uint64  `json:"limitedHello"`
	LimitedWS     uint64  `json:"limitedWS"`
	LimitedChat   uint64  `json:"limitedChat"`
	LimitedConn   uint64  `json:"limitedConn"`
	UnauthWS      uint64  `json:"unauthWS"`
	LimitedLogin  uint64  `json:"limitedLogin"`
	LoginFails    uint64  `json:"loginFails"`
}

// SkillInfo lets the client render skills it was not compiled with. The
// pack had the same problem: a hardcoded list meant adding something
// server-side silently failed to show up.
type SkillInfo struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

func SkillCatalog() map[string]SkillInfo {
	return map[string]SkillInfo{
		SkillForage:  {Name: "Foraging", Order: 1},
		SkillCook:    {Name: "Cooking", Order: 2},
		SkillMelee:   {Name: "Melee", Order: 3},
		SkillDefense: {Name: "Defense", Order: 4},
		SkillWood:    {Name: "Woodcutting", Order: 5},
	}
}

func Catalog() map[string]ItemInfo {
	return map[string]ItemInfo{
		ItemBerry:   {Name: "Brambleberry", Glyph: "Bb"},
		ItemPulp:    {Name: "Bramble pulp", Glyph: "Pp"},
		ItemTart:    {Name: "Hearth tart", Glyph: "Ht"},
		ItemNut:     {Name: "Hazel nut", Glyph: "Hz"},
		ItemRoast:   {Name: "Roast hazel", Glyph: "Rh"},
		ItemLeather: {Name: "Goblin leather", Glyph: "Gl"},
		ItemLog:     {Name: "Log", Glyph: "Lg"},
		ItemPaper:   {Name: "Paper", Glyph: "Pa"},
	}
}
