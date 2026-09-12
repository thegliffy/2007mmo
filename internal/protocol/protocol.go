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
	MsgUse      = "use"
	MsgChat     = "chat"
	MsgPing     = "ping"

	MsgWelcome = "welcome"
	MsgState   = "state"
	MsgSaid    = "chat"
	MsgEvent   = "evt"
	MsgPong    = "pong"
	MsgErr     = "err"

	ItemBerry = "berry"
	ItemPulp  = "pulp"
	ItemTart  = "tart"
	ItemNut   = "nut"
	ItemRoast = "roast"

	SkillForage = "forage"
	SkillCook   = "cook"

	KindBush  = "bush"
	KindHazel = "hazel"
	KindMill  = "mill"
	KindFire  = "fire"

	ActionForage = "forage"
	ActionMill   = "mill"
	ActionCook   = "cook"
	ActionRoast  = "roast"
)

// In is every client → server frame. Unused fields stay empty.
type In struct {
	T        string `json:"t"`
	PlayerID string `json:"playerId,omitempty"`
	Name     string `json:"name,omitempty"`
	Session  string `json:"session,omitempty"`
	X        int    `json:"x,omitempty"`
	Y        int    `json:"y,omitempty"`
	ID       string `json:"id,omitempty"`
	Text     string `json:"text,omitempty"`
	Ts       int64  `json:"ts,omitempty"`
}

type Item struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

type Skill struct {
	XP int `json:"xp"`
	Lv int `json:"lv"`
}

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
}

type NodeView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Ready bool   `json:"ready"`
	Left  int    `json:"left,omitempty"`
}

type NPCView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
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
	T        string              `json:"t"`
	PlayerID string              `json:"playerId"`
	Session  string              `json:"session"`
	TickMs   int                 `json:"tickMs"`
	World    string              `json:"world"`
	Map      TileMap             `json:"map"`
	You      YouView             `json:"you"`
	Items    map[string]ItemInfo `json:"items"`
}

type State struct {
	T       string       `json:"t"`
	N       uint64       `json:"n"`
	Ms      float64      `json:"ms"`
	Online  int          `json:"online"`
	You     YouView      `json:"you"`
	Players []PlayerView `json:"players"`
	NPCs    []NPCView    `json:"npcs"`
	Nodes   []NodeView   `json:"nodes"`
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
	World        string  `json:"world"`
	Tick         uint64  `json:"tick"`
	TickMs       int     `json:"tickMs"`
	TickP50Ms    float64 `json:"tickP50Ms"`
	TickP99Ms    float64 `json:"tickP99Ms"`
	TickMaxMs    float64 `json:"tickMaxMs"`
	Samples      int     `json:"samples"`
	Online       int     `json:"online"`
	WS           int     `json:"ws"`
	PlayersMem   int     `json:"playersMem"`
	Joins        uint64  `json:"joins"`
	Chats        uint64  `json:"chats"`
	Actions      uint64  `json:"actions"`
	LimitedHello uint64  `json:"limitedHello"`
	LimitedWS    uint64  `json:"limitedWS"`
	LimitedChat  uint64  `json:"limitedChat"`
	LimitedConn  uint64  `json:"limitedConn"`
}

func Catalog() map[string]ItemInfo {
	return map[string]ItemInfo{
		ItemBerry: {Name: "Brambleberry", Glyph: "Bb"},
		ItemPulp:  {Name: "Bramble pulp", Glyph: "Pp"},
		ItemTart:  {Name: "Hearth tart", Glyph: "Ht"},
		ItemNut:   {Name: "Hazel nut", Glyph: "Hz"},
		ItemRoast: {Name: "Roast hazel", Glyph: "Rh"},
	}
}
