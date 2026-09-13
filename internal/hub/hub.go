package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/store"
	"github.com/thegliffy/2007mmo/internal/world"
)

// cmdKind identifies a queued client command.

type cmdKind int

const (
	cmdHello cmdKind = iota
	cmdMove
	cmdInteract
	cmdAttack
	cmdUse
	cmdChat
	cmdLeave
)

type cmd struct {
	kind     cmdKind
	client   *Client
	playerID string
	x, y     int
	id       string
	text     string
}

// Client is one authenticated WebSocket. Every identity field is set
// before readLoop starts and never written again, so the reader and the
// hub goroutine can both read them without synchronization.
type Client struct {
	playerID  string
	accountID string
	username  string
	session   string
	ip        string
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	hub       *Hub
	closeOnce sync.Once
}

// stop signals writeLoop to finish. The send channel is deliberately
// never closed: sendJSON can race with a takeover, and sending on a
// closed channel would panic the connection's goroutine.
func (c *Client) stop() {
	c.closeOnce.Do(func() { close(c.done) })
}

type Hub struct {
	World   *world.World
	PG      *store.Postgres
	Redis   *store.Redis
	Auth    *auth.Service
	Tick    time.Duration
	cmds    chan cmd
	mu      sync.Mutex
	clients map[string]*Client // playerID -> client
	metrics *Metrics
	limits  *limits

	proxy   *proxyTrust
	upgrade websocket.Upgrader

	// muted is accountID -> when the mute lifts, refreshed by the
	// heartbeat loop. Cached rather than read per message: cmdChat runs on
	// the world goroutine, and a Redis round-trip there is exactly the
	// cost that was just taken off the tick.
	muted map[string]time.Time
}

// newUpgrader wires the origin check into the WebSocket handshake. This
// matters far more now that the credential is a cookie: without it, any
// page could open an authenticated socket as whoever is logged in.
func newUpgrader(pt *proxyTrust) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     pt.allowOrigin,
	}
}

func New(w *world.World, pg *store.Postgres, rd *store.Redis, a *auth.Service, tick time.Duration) *Hub {
	if tick <= 0 {
		tick = time.Duration(protocol.TickMs) * time.Millisecond
	}
	pt := loadProxyTrust()
	return &Hub{
		World:   w,
		PG:      pg,
		Redis:   rd,
		Auth:    a,
		Tick:    tick,
		proxy:   pt,
		upgrade: newUpgrader(pt),
		cmds:    make(chan cmd, 1024),
		clients: make(map[string]*Client),
		metrics: NewMetrics(),
		limits:  limitsFromEnv(),
		muted:   make(map[string]time.Time),
	}
}

// isMuted reports whether an account is currently quiet.
func (h *Hub) isMuted(accountID string) bool {
	if accountID == "" {
		return false
	}
	h.mu.Lock()
	until, ok := h.muted[accountID]
	h.mu.Unlock()
	return ok && time.Now().Before(until)
}

func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(h.Tick)
	defer t.Stop()
	// Presence is a 30s-TTL heartbeat, not simulation state. Writing it
	// per client per tick put N synchronous Redis round-trips on the
	// world's critical path; it lives on its own schedule now, alongside
	// the session re-check.
	go h.heartbeatLoop(ctx)
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.cmds:
			h.handle(ctx, c)
		case now := <-t.C:
			// Three separate numbers, because they fail differently:
			//   sim   - the authoritative step alone
			//   loop  - sim plus fanning state out to every client
			//   lag   - how late this tick fired against its schedule
			// Gating on sim alone hid the fan-out, which is the half that
			// grows with player count.
			if !last.IsZero() {
				h.metrics.ObserveLag(now.Sub(last) - h.Tick)
			}
			last = now

			start := time.Now()
			h.World.Tick(ctx)
			sim := time.Since(start)
			h.World.LastMs = float64(sim.Microseconds()) / 1000.0
			h.metrics.Observe(sim)
			h.metrics.SetTick(h.World.TickN)
			h.metrics.SetOnline(h.World.OnlineCount(), len(h.World.Players))
			h.broadcastState()
			h.flushNotes()
			h.metrics.ObserveLoop(time.Since(start))
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
		h.metrics.AddAction()
		h.pushNoteAndState(c.client, c.playerID)
	case cmdAttack:
		h.World.SetAttack(c.playerID, c.id)
		h.metrics.AddAction()
		h.pushNoteAndState(c.client, c.playerID)
	case cmdUse:
		if text, ok := h.World.UseItem(ctx, c.playerID, c.id); text != "" {
			h.sendJSON(c.client, protocol.Event{T: protocol.MsgEvent, Text: text})
			if ok {
				h.metrics.AddAction()
				h.sendJSON(c.client, h.World.Snapshot(c.playerID))
			}
		}
	case cmdChat:
		if c.client != nil && h.isMuted(c.client.accountID) {
			h.sendJSON(c.client, protocol.Err{T: protocol.MsgErr, Msg: "The hamlet cannot hear you just now."})
			return
		}
		from, text, ok := h.World.TryChat(c.playerID, c.text)
		if !ok {
			return
		}
		h.metrics.AddChat()
		h.broadcastJSON(protocol.Chat{T: protocol.MsgSaid, From: from, Text: text, Kind: "say"})
	case cmdLeave:
		h.onLeave(ctx, c.playerID, c.client)
	}
}

// onHello completes the join for an already-authenticated socket. It
// takes no identity from the client: playerID and username were fixed by
// the session cookie during the upgrade.
func (h *Hub) onHello(ctx context.Context, c cmd) {
	cl := c.client
	if cl == nil || cl.playerID == "" {
		return
	}
	id := cl.playerID

	p := h.World.Players[id]
	if p == nil {
		rec, err := h.World.Store.LoadPlayer(ctx, id)
		if err != nil {
			log.Printf("load player: %v", err)
			h.sendJSON(cl, protocol.Err{T: protocol.MsgErr, Msg: "the hamlet could not read your pack"})
			return
		}
		if rec == nil {
			// CreateAccount writes account and player together, so this
			// should be unreachable. Heal instead of refusing entry.
			log.Printf("account %s had no player row; recreating", cl.accountID)
			rec = world.NewPlayerRec(id, cl.username)
			if err := h.World.Store.SavePlayer(ctx, rec); err != nil {
				log.Printf("create player: %v", err)
				h.sendJSON(cl, protocol.Err{T: protocol.MsgErr, Msg: "could not enter the hamlet"})
				return
			}
		}
		// The account username is the only source of a display name.
		rec.Name = cl.username
		p = h.World.UpsertPlayer(rec, true)
	} else {
		p.Online = true
		p.Name = world.SanitizeName(cl.username)
	}

	// A fresh handle per join: a peer who noted your handle last session
	// cannot use it to recognize you in this one.
	handle := h.World.RotateHandle(p.ID)

	if h.Redis != nil {
		_ = h.Redis.SetPresence(ctx, p.ID)
	}

	h.mu.Lock()
	if old, ok := h.clients[p.ID]; ok && old != cl {
		old.stop()
	}
	h.clients[p.ID] = cl
	h.metrics.SetWS(len(h.clients))
	h.mu.Unlock()

	h.metrics.AddJoin()
	h.sendJSON(cl, protocol.Welcome{
		T:        protocol.MsgWelcome,
		Handle:   handle,
		Username: p.Name,
		TickMs:   int(h.Tick / time.Millisecond),
		World:    protocol.WorldName,
		Map:      h.World.MapInfo(),
		You:      h.World.Snapshot(p.ID).You,
		Items:    protocol.Catalog(),
		Skills:   protocol.SkillCatalog(),
	})
	h.sendJSON(cl, h.World.Snapshot(p.ID))
}

func (h *Hub) pushNoteAndState(cl *Client, playerID string) {
	if text := h.World.TakeNote(playerID); text != "" {
		h.sendJSON(cl, protocol.Event{T: protocol.MsgEvent, Text: text})
	}
	if cl != nil && playerID != "" {
		h.sendJSON(cl, h.World.Snapshot(playerID))
	}
}

func (h *Hub) flushNotes() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		text := h.World.TakeNote(id)
		if text == "" {
			continue
		}
		h.mu.Lock()
		cl := h.clients[id]
		h.mu.Unlock()
		h.sendJSON(cl, protocol.Event{T: protocol.MsgEvent, Text: text})
	}
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

// heartbeatLoop refreshes presence keys and re-checks that every open
// socket still has a live session.
//
// Sessions are only verified once, at the upgrade. Without this, revoking
// a session — an admin password reset, a logout from another device —
// would not disturb a connection that is already established, so the
// thing you were trying to evict keeps playing until it disconnects on
// its own. The presence write already visits each client on this cadence,
// so the check costs one more Redis read per client per 10s.
func (h *Hub) heartbeatLoop(ctx context.Context) {
	if h.Redis == nil {
		return
	}
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.mu.Lock()
			live := make([]*Client, 0, len(h.clients))
			for _, cl := range h.clients {
				live = append(live, cl)
			}
			h.mu.Unlock()

			for _, cl := range live {
				stepCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, playerID, _, err := h.Auth.Resolve(stepCtx, cl.session)
				if err != nil || playerID != cl.playerID {
					cancel()
					h.metrics.AddLimited("auth")
					h.sendJSON(cl, protocol.Err{T: protocol.MsgErr, Msg: "Your session ended. Log in again."})
					cl.stop()
					continue
				}
				_ = h.Redis.SetPresence(stepCtx, cl.playerID)
				if left, mErr := h.Redis.MuteRemaining(stepCtx, cl.accountID); mErr == nil {
					h.mu.Lock()
					if left > 0 {
						h.muted[cl.accountID] = time.Now().Add(left)
					} else {
						delete(h.muted, cl.accountID)
					}
					h.mu.Unlock()
				}
				cancel()
			}
		}
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
		case <-cl.done:
			continue
		default:
		}
		select {
		case cl.send <- b:
		default:
			h.metrics.AddDroppedFrame()
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
	case <-cl.done:
		return
	default:
	}
	select {
	case cl.send <- b:
	default:
		// The client is not draining fast enough. Dropping a state frame
		// is survivable (the next one is a full snapshot), but silently
		// dropping them meant "0 drops" in the load gate measured only
		// whether sockets stayed open, not whether data arrived.
		h.metrics.AddDroppedFrame()
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	ip := h.proxy.clientIP(r)
	if !h.limits.conn.allow(ip) {
		h.metrics.AddLimited("conn")
		http.Error(w, "the gate is crowded", http.StatusTooManyRequests)
		return
	}

	// Identity is settled before the upgrade. An unauthenticated socket
	// never exists, so no frame can ever arrive from an unknown player.
	token := sessionToken(r)
	accountID, playerID, username, err := h.Auth.Resolve(r.Context(), token)
	if err != nil || playerID == "" {
		h.metrics.AddLimited("auth")
		http.Error(w, "the gate is shut: log in first", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cl := &Client{
		playerID:  playerID,
		accountID: accountID,
		username:  username,
		session:   token,
		ip:        ip,
		conn:      conn,
		send:      make(chan []byte, 16),
		done:      make(chan struct{}),
		hub:       h,
	}
	if h.Redis != nil {
		if left, mErr := h.Redis.MuteRemaining(r.Context(), accountID); mErr == nil && left > 0 {
			h.mu.Lock()
			h.muted[accountID] = time.Now().Add(left)
			h.mu.Unlock()
		}
	}

	go cl.writeLoop()
	cl.readLoop()
}

func (c *Client) readLoop() {
	defer func() {
		c.hub.cmds <- cmd{kind: cmdLeave, client: c, playerID: c.playerID}
		_ = c.conn.Close()
		c.stop()
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
		key := c.playerID
		if key == "" {
			key = c.ip
		}
		if in.T != protocol.MsgPing && !c.hub.limits.ws.allow(key) {
			c.hub.metrics.AddLimited("ws")
			continue
		}
		switch in.T {
		case protocol.MsgHello:
			if !c.hub.limits.hello.allow(c.ip) {
				c.hub.metrics.AddLimited("hello")
				c.hub.sendJSON(c, protocol.Err{T: protocol.MsgErr, Msg: "The gate is crowded. Wait a breath."})
				continue
			}
			c.hub.cmds <- cmd{kind: cmdHello, client: c, playerID: c.playerID}
		case protocol.MsgMove:
			c.hub.cmds <- cmd{kind: cmdMove, client: c, playerID: c.playerID, x: in.X, y: in.Y}
		case protocol.MsgInteract:
			c.hub.cmds <- cmd{kind: cmdInteract, client: c, playerID: c.playerID, id: in.ID}
		case protocol.MsgAttack:
			c.hub.cmds <- cmd{kind: cmdAttack, client: c, playerID: c.playerID, id: in.ID}
		case protocol.MsgUse:
			c.hub.cmds <- cmd{kind: cmdUse, client: c, playerID: c.playerID, id: in.ID}
		case protocol.MsgChat:
			if !c.hub.limits.chat.allow(key) {
				c.hub.metrics.AddLimited("chat")
				c.hub.sendJSON(c, protocol.Err{T: protocol.MsgErr, Msg: "You are speaking too quickly."})
				continue
			}
			c.hub.cmds <- cmd{kind: cmdChat, client: c, playerID: c.playerID, text: in.Text}
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
		case <-c.done:
			// Flush whatever is already queued first. Eviction sends a
			// "why" frame and then stops the client; closing straight away
			// threw that frame away and the player just saw the socket
			// vanish. writeLoop owns the write side, so it closes the
			// connection itself once the close frame is out.
			for drained := true; drained; {
				select {
				case msg := <-c.send:
					_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
					if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
						_ = c.conn.Close()
						return
					}
				default:
					drained = false
				}
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			_ = c.conn.Close()
			return
		case msg := <-c.send:
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
	writeJSON(w, h.stats())
}

func (h *Hub) stats() protocol.Stats {
	p50, p99, max, n := h.metrics.Percentiles()
	tick, ws, online, mem := h.metrics.Snapshot()
	joins, chats, actions, lh, lw, lc, ln, la, ll, lf := h.metrics.Counters()
	loopP50, loopP99, loopMax, lagP50, lagP99, lagMax, dropped := h.metrics.LoopPercentiles()
	return protocol.Stats{
		World:         protocol.WorldName,
		Tick:          tick,
		TickMs:        int(h.Tick / time.Millisecond),
		TickP50Ms:     p50,
		TickP99Ms:     p99,
		TickMaxMs:     max,
		LoopP50Ms:     loopP50,
		LoopP99Ms:     loopP99,
		LoopMaxMs:     loopMax,
		LagP50Ms:      lagP50,
		LagP99Ms:      lagP99,
		LagMaxMs:      lagMax,
		FramesDropped: dropped,
		Samples:       n,
		Online:        online,
		WS:            ws,
		PlayersMem:    mem,
		Joins:         joins,
		Chats:         chats,
		Actions:       actions,
		LimitedHello:  lh,
		LimitedWS:     lw,
		LimitedChat:   lc,
		LimitedConn:   ln,
		UnauthWS:      la,
		LimitedLogin:  ll,
		LoginFails:    lf,
	}
}

func (h *Hub) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	s := h.stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP hollowmere_tick The latest completed world tick.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_tick gauge\n")
	fmt.Fprintf(w, "hollowmere_tick %d\n", s.Tick)
	fmt.Fprintf(w, "# HELP hollowmere_tick_p50_ms Tick duration p50 over the rolling sample window.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_tick_p50_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_tick_p50_ms %.4f\n", s.TickP50Ms)
	fmt.Fprintf(w, "# HELP hollowmere_tick_p99_ms Tick duration p99 over the rolling sample window.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_tick_p99_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_tick_p99_ms %.4f\n", s.TickP99Ms)
	fmt.Fprintf(w, "# HELP hollowmere_tick_max_ms Max tick duration in the rolling sample window.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_tick_max_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_tick_max_ms %.4f\n", s.TickMaxMs)
	fmt.Fprintf(w, "# HELP hollowmere_loop_p99_ms Full cycle: tick plus the state fan-out to every client.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_loop_p99_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_loop_p99_ms %.4f\n", s.LoopP99Ms)
	fmt.Fprintf(w, "# HELP hollowmere_loop_p50_ms Full cycle, median.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_loop_p50_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_loop_p50_ms %.4f\n", s.LoopP50Ms)
	fmt.Fprintf(w, "# HELP hollowmere_tick_lag_p99_ms How late a tick fired against its schedule.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_tick_lag_p99_ms gauge\n")
	fmt.Fprintf(w, "hollowmere_tick_lag_p99_ms %.4f\n", s.LagP99Ms)
	fmt.Fprintf(w, "# HELP hollowmere_frames_dropped_total State frames discarded because a client was not draining.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_frames_dropped_total counter\n")
	fmt.Fprintf(w, "hollowmere_frames_dropped_total %d\n", s.FramesDropped)
	fmt.Fprintf(w, "# HELP hollowmere_online Players currently in the hamlet.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_online gauge\n")
	fmt.Fprintf(w, "hollowmere_online %d\n", s.Online)
	fmt.Fprintf(w, "# HELP hollowmere_ws Open WebSocket clients.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_ws gauge\n")
	fmt.Fprintf(w, "hollowmere_ws %d\n", s.WS)
	fmt.Fprintf(w, "# HELP hollowmere_players_mem Player records held in memory.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_players_mem gauge\n")
	fmt.Fprintf(w, "hollowmere_players_mem %d\n", s.PlayersMem)
	fmt.Fprintf(w, "# HELP hollowmere_joins_total Successful hamlet joins.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_joins_total counter\n")
	fmt.Fprintf(w, "hollowmere_joins_total %d\n", s.Joins)
	fmt.Fprintf(w, "# HELP hollowmere_chats_total Public chat lines accepted.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_chats_total counter\n")
	fmt.Fprintf(w, "hollowmere_chats_total %d\n", s.Chats)
	fmt.Fprintf(w, "# HELP hollowmere_actions_total Interact and successful use commands.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_actions_total counter\n")
	fmt.Fprintf(w, "hollowmere_actions_total %d\n", s.Actions)
	fmt.Fprintf(w, "# HELP hollowmere_rate_limited_total Frames or connections refused by rate limits.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_rate_limited_total counter\n")
	fmt.Fprintf(w, "hollowmere_rate_limited_total{kind=\"hello\"} %d\n", s.LimitedHello)
	fmt.Fprintf(w, "hollowmere_rate_limited_total{kind=\"ws\"} %d\n", s.LimitedWS)
	fmt.Fprintf(w, "hollowmere_rate_limited_total{kind=\"chat\"} %d\n", s.LimitedChat)
	fmt.Fprintf(w, "hollowmere_rate_limited_total{kind=\"conn\"} %d\n", s.LimitedConn)
	fmt.Fprintf(w, "hollowmere_rate_limited_total{kind=\"login\"} %d\n", s.LimitedLogin)
	fmt.Fprintf(w, "# HELP hollowmere_unauthenticated_ws_total WebSocket upgrades refused for a missing or dead session.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_unauthenticated_ws_total counter\n")
	fmt.Fprintf(w, "hollowmere_unauthenticated_ws_total %d\n", s.UnauthWS)
	fmt.Fprintf(w, "# HELP hollowmere_login_failures_total Rejected register, login, and password-change attempts.\n")
	fmt.Fprintf(w, "# TYPE hollowmere_login_failures_total counter\n")
	fmt.Fprintf(w, "hollowmere_login_failures_total %d\n", s.LoginFails)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type Metrics struct {
	mu           sync.Mutex
	samples      []float64
	i            int
	full         bool
	loop         *ring
	lag          *ring
	dropped      uint64
	tick         uint64
	ws           int
	online       int
	mem          int
	joins        uint64
	chats        uint64
	actions      uint64
	limitedHello uint64
	limitedWS    uint64
	limitedChat  uint64
	limitedConn  uint64
	limitedAuth  uint64
	limitedLogin uint64
	loginFails   uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		samples: make([]float64, 512),
		loop:    newRing(512),
		lag:     newRing(512),
	}
}

// ring is a fixed-size window of millisecond samples.
type ring struct {
	vals []float64
	i    int
	full bool
}

func newRing(n int) *ring { return &ring{vals: make([]float64, n)} }

func (r *ring) add(ms float64) {
	r.vals[r.i] = ms
	r.i = (r.i + 1) % len(r.vals)
	if r.i == 0 {
		r.full = true
	}
}

// percentiles returns p50, p99 and max over the window.
func (r *ring) percentiles() (p50, p99, max float64) {
	n := r.i
	if r.full {
		n = len(r.vals)
	}
	if n == 0 {
		return 0, 0, 0
	}
	cp := make([]float64, n)
	copy(cp, r.vals[:n])
	if r.full {
		copy(cp, r.vals)
	}
	sort.Float64s(cp)
	idx := n * 99 / 100
	if idx >= n {
		idx = n - 1
	}
	return cp[n*50/100], cp[idx], cp[n-1]
}

// ObserveLoop records a full cycle: simulation plus the state fan-out.
func (m *Metrics) ObserveLoop(d time.Duration) {
	m.mu.Lock()
	m.loop.add(float64(d.Microseconds()) / 1000.0)
	m.mu.Unlock()
}

// ObserveLag records how late a tick fired against its schedule. This is
// the number that actually says whether the world is keeping up.
func (m *Metrics) ObserveLag(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.mu.Lock()
	m.lag.add(float64(d.Microseconds()) / 1000.0)
	m.mu.Unlock()
}

func (m *Metrics) AddDroppedFrame() {
	m.mu.Lock()
	m.dropped++
	m.mu.Unlock()
}

// LoopPercentiles reports loop and lag windows together.
func (m *Metrics) LoopPercentiles() (loopP50, loopP99, loopMax, lagP50, lagP99, lagMax float64, dropped uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	loopP50, loopP99, loopMax = m.loop.percentiles()
	lagP50, lagP99, lagMax = m.lag.percentiles()
	return loopP50, loopP99, loopMax, lagP50, lagP99, lagMax, m.dropped
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

func (m *Metrics) AddJoin() {
	m.mu.Lock()
	m.joins++
	m.mu.Unlock()
}

func (m *Metrics) AddChat() {
	m.mu.Lock()
	m.chats++
	m.mu.Unlock()
}

func (m *Metrics) AddAction() {
	m.mu.Lock()
	m.actions++
	m.mu.Unlock()
}

// AddLoginFail counts a credential rejection. Kept apart from the
// throttle counter: a spike in throttling means someone is guessing, a
// spike here without throttling usually means people forgot a password.
func (m *Metrics) AddLoginFail() {
	m.mu.Lock()
	m.loginFails++
	m.mu.Unlock()
}

func (m *Metrics) AddLimited(kind string) {
	m.mu.Lock()
	switch kind {
	case "hello":
		m.limitedHello++
	case "ws":
		m.limitedWS++
	case "chat":
		m.limitedChat++
	case "conn":
		m.limitedConn++
	case "auth":
		m.limitedAuth++
	case "login":
		m.limitedLogin++
	}
	m.mu.Unlock()
}

func (m *Metrics) Counters() (joins, chats, actions, hello, ws, chat, conn, unauth, limitedLogin, loginFails uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.joins, m.chats, m.actions, m.limitedHello, m.limitedWS, m.limitedChat,
		m.limitedConn, m.limitedAuth, m.limitedLogin, m.loginFails
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
	idx := n * 99 / 100
	if idx >= n {
		idx = n - 1
	}
	p99 = cp[idx]
	max = cp[n-1]
	return p50, p99, max, n
}
