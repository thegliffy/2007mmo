package hub

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/store"
	"github.com/thegliffy/2007mmo/internal/world"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type cmdKind int

const (
	cmdHello cmdKind = iota
	cmdMove
	cmdInteract
	cmdUse
	cmdChat
	cmdLeave
)

type cmd struct {
	kind     cmdKind
	client   *Client
	playerID string
	name     string
	session  string
	x, y     int
	id       string
	text     string
}

type Client struct {
	id        string
	conn      *websocket.Conn
	send      chan []byte
	hub       *Hub
	closeOnce sync.Once
}

type Hub struct {
	World  *world.World
	PG     *store.Postgres
	Redis  *store.Redis
	Tick   time.Duration
	cmds   chan cmd
	mu     sync.Mutex
	clients map[string]*Client // playerID -> client
	metrics *Metrics
}

func New(w *world.World, pg *store.Postgres, rd *store.Redis, tick time.Duration) *Hub {
	if tick <= 0 {
		tick = time.Duration(protocol.TickMs) * time.Millisecond
	}
	return &Hub{
		World:   w,
		PG:      pg,
		Redis:   rd,
		Tick:    tick,
		cmds:    make(chan cmd, 1024),
		clients: make(map[string]*Client),
		metrics: NewMetrics(),
	}
}

func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(h.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.cmds:
			h.handle(ctx, c)
		case <-t.C:
			start := time.Now()
			h.World.Tick(ctx)
			elapsed := time.Since(start)
			h.World.LastMs = float64(elapsed.Microseconds()) / 1000.0
			h.metrics.Observe(elapsed)
			h.metrics.SetTick(h.World.TickN)
			h.metrics.SetOnline(h.World.OnlineCount(), len(h.World.Players))
			h.broadcastState()
		}
	}
}

func (h *Hub) handle(ctx context.Context, c cmd) {
	switch c.kind {
	case cmdHello:
		h.onHello(ctx, c)
	case cmdMove:
		h.World.SetDest(c.playerID, c.x, c.y)
	case cmdInteract:
		h.World.SetInteract(c.playerID, c.id)
	case cmdUse:
		if text, ok := h.World.UseItem(ctx, c.playerID, c.id); text != "" {
			h.sendJSON(c.client, protocol.Event{T: protocol.MsgEvent, Text: text})
			if ok {
				h.sendJSON(c.client, h.World.Snapshot(c.playerID))
			}
		}
	case cmdChat:
		from, text, ok := h.World.TryChat(c.playerID, c.text)
		if !ok {
			return
		}
		h.broadcastJSON(protocol.Chat{T: protocol.MsgSaid, From: from, Text: text, Kind: "say"})
	case cmdLeave:
		h.onLeave(ctx, c.playerID, c.client)
	}
}

func (h *Hub) onHello(ctx context.Context, c cmd) {
	id := c.playerID
	if _, err := uuid.Parse(id); err != nil {
		id = uuid.NewString()
	}
	name := world.SanitizeName(c.name)

	if c.session != "" && h.Redis != nil {
		if sid, err := h.Redis.GetSession(ctx, c.session); err == nil && sid != "" {
			id = sid
		}
	}

	p := h.World.Players[id]
	if p == nil {
		rec, err := h.World.Store.LoadPlayer(ctx, id)
		if err != nil {
			log.Printf("load player: %v", err)
		}
		if rec == nil {
			rec = world.NewPlayerRec(id, name)
			if err := h.World.Store.SavePlayer(ctx, rec); err != nil {
				log.Printf("create player: %v", err)
				h.sendJSON(c.client, protocol.Err{T: protocol.MsgErr, Msg: "could not enter the hamlet"})
				return
			}
		} else if name != "Wanderer" {
			rec.Name = name
		}
		p = h.World.UpsertPlayer(rec, true)
	} else {
		p.Online = true
		if name != "Wanderer" {
			p.Name = name
		}
	}

	session := uuid.NewString()
	if h.Redis != nil {
		_ = h.Redis.SetSession(ctx, session, p.ID, 24*time.Hour)
		_ = h.Redis.SetPresence(ctx, p.ID)
	}

	h.mu.Lock()
	if old, ok := h.clients[p.ID]; ok && old != c.client {
		_ = old.conn.Close()
		old.closeSend()
	}
	c.client.id = p.ID
	h.clients[p.ID] = c.client
	h.metrics.SetWS(len(h.clients))
	h.mu.Unlock()

	h.sendJSON(c.client, protocol.Welcome{
		T:        protocol.MsgWelcome,
		PlayerID: p.ID,
		Session:  session,
		TickMs:   int(h.Tick / time.Millisecond),
		World:    protocol.WorldName,
		Map:      h.World.MapInfo(),
		You:      h.World.Snapshot(p.ID).You,
		Items:    protocol.Catalog(),
	})
	h.sendJSON(c.client, h.World.Snapshot(p.ID))
}

func (h *Hub) onLeave(ctx context.Context, id string, c *Client) {
	h.mu.Lock()
	cur := h.clients[id]
	same := cur == c
	if same {
		delete(h.clients, id)
	}
	h.metrics.SetWS(len(h.clients))
	h.mu.Unlock()
	if id == "" || !same {
		return
	}
	h.World.SetOnline(id, false)
	h.World.PersistPlayer(ctx, id)
	if h.Redis != nil {
		_ = h.Redis.ClearPresence(ctx, id)
	}
}

func (h *Hub) broadcastState() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		h.mu.Lock()
		cl := h.clients[id]
		h.mu.Unlock()
		if cl == nil {
			continue
		}
		h.sendJSON(cl, h.World.Snapshot(id))
		if h.Redis != nil {
			_ = h.Redis.SetPresence(context.Background(), id)
		}
	}
}

func (h *Hub) broadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, cl := range h.clients {
		select {
		case cl.send <- b:
		default:
		}
	}
}

func (h *Hub) sendJSON(cl *Client, v any) {
	if cl == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case cl.send <- b:
	default:
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cl := &Client{
		conn: conn,
		send: make(chan []byte, 16),
		hub:  h,
	}
	go cl.writeLoop()
	cl.readLoop()
}

func (c *Client) readLoop() {
	defer func() {
		c.hub.cmds <- cmd{kind: cmdLeave, client: c, playerID: c.id}
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var in protocol.In
		if err := json.Unmarshal(data, &in); err != nil {
			continue
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		switch in.T {
		case protocol.MsgHello:
			c.hub.cmds <- cmd{kind: cmdHello, client: c, playerID: in.PlayerID, name: in.Name, session: in.Session}
		case protocol.MsgMove:
			c.hub.cmds <- cmd{kind: cmdMove, client: c, playerID: c.id, x: in.X, y: in.Y}
		case protocol.MsgInteract:
			c.hub.cmds <- cmd{kind: cmdInteract, client: c, playerID: c.id, id: in.ID}
		case protocol.MsgUse:
			c.hub.cmds <- cmd{kind: cmdUse, client: c, playerID: c.id, id: in.ID}
		case protocol.MsgChat:
			c.hub.cmds <- cmd{kind: cmdChat, client: c, playerID: c.id, text: in.Text}
		case protocol.MsgPing:
			c.hub.sendJSON(c, protocol.Pong{T: protocol.MsgPong, Ts: in.Ts})
		}
	}
}

func (c *Client) writeLoop() {
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) ServeHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	out := map[string]any{"ok": true, "world": protocol.WorldName}
	if h.PG != nil {
		if err := h.PG.Ping(ctx); err != nil {
			out["ok"] = false
			out["postgres"] = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			out["postgres"] = "ok"
		}
	}
	if h.Redis != nil {
		if err := h.Redis.Ping(ctx); err != nil {
			out["ok"] = false
			out["redis"] = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			out["redis"] = "ok"
		}
	}
	writeJSON(w, out)
}

func (h *Hub) ServeStats(w http.ResponseWriter, r *http.Request) {
	p50, p99, max, n := h.metrics.Percentiles()
	tick, ws, online, mem := h.metrics.Snapshot()
	writeJSON(w, protocol.Stats{
		World:      protocol.WorldName,
		Tick:       tick,
		TickMs:     int(h.Tick / time.Millisecond),
		TickP50Ms:  p50,
		TickP99Ms:  p99,
		TickMaxMs:  max,
		Samples:    n,
		Online:     online,
		WS:         ws,
		PlayersMem: mem,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (c *Client) closeSend() {
	c.closeOnce.Do(func() { close(c.send) })
}

type Metrics struct {
	mu      sync.Mutex
	samples []float64
	i       int
	full    bool
	tick    uint64
	ws      int
	online  int
	mem     int
}

func NewMetrics() *Metrics {
	return &Metrics{samples: make([]float64, 512)}
}

func (m *Metrics) Observe(d time.Duration) {
	ms := float64(d.Microseconds()) / 1000.0
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples[m.i] = ms
	m.i = (m.i + 1) % len(m.samples)
	if m.i == 0 {
		m.full = true
	}
}

func (m *Metrics) SetTick(n uint64) {
	m.mu.Lock()
	m.tick = n
	m.mu.Unlock()
}

func (m *Metrics) SetWS(n int) {
	m.mu.Lock()
	m.ws = n
	m.mu.Unlock()
}

func (m *Metrics) SetOnline(online, mem int) {
	m.mu.Lock()
	m.online = online
	m.mem = mem
	m.mu.Unlock()
}

func (m *Metrics) Snapshot() (tick uint64, ws, online, mem int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tick, m.ws, m.online, m.mem
}

func (m *Metrics) Percentiles() (p50, p99, max float64, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n = m.i
	if m.full {
		n = len(m.samples)
	}
	if n == 0 {
		return 0, 0, 0, 0
	}
	cp := make([]float64, n)
	if m.full {
		copy(cp, m.samples)
	} else {
		copy(cp, m.samples[:n])
	}
	sort.Float64s(cp)
	p50 = cp[n*50/100]
	idx := n*99/100
	if idx >= n {
		idx = n - 1
	}
	p99 = cp[idx]
	max = cp[n-1]
	return p50, p99, max, n
}
