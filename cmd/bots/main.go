// Headless WebSocket load harness. Not real browsers.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func main() {
	n := flag.Int("n", 50, "number of bot clients")
	addr := flag.String("addr", "ws://127.0.0.1:8080/ws", "world websocket URL")
	statsURL := flag.String("stats", "http://127.0.0.1:8080/stats", "world /stats URL")
	hotspot := flag.Bool("hotspot", false, "crowd bots onto the berry thicket")
	password := flag.String("password", "bramble-hollow-load-9", "password for the generated bot accounts")
	duration := flag.Duration("duration", 45*time.Second, "how long to run")
	flag.Parse()

	log.SetPrefix("bots ")
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	ctxDone := make(chan os.Signal, 1)
	signal.Notify(ctxDone, os.Interrupt, syscall.SIGTERM)

	var connected, drops, msgs, authFails, throttled, refused atomic.Int64
	var rttNanos atomic.Int64
	var rttN atomic.Int64

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runBot(i, *addr, *statsURL, *password, *hotspot, stop,
				&connected, &drops, &msgs, &authFails, &throttled, &refused, &rttNanos, &rttN)
		}(i)
		time.Sleep(8 * time.Millisecond)
	}

	deadline := time.After(*duration)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	printSnap := func(tag string) {
		var st protocol.Stats
		if resp, err := http.Get(*statsURL); err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			_ = json.Unmarshal(b, &st)
		}
		avgRTT := time.Duration(0)
		if rttN.Load() > 0 {
			avgRTT = time.Duration(rttNanos.Load() / rttN.Load())
		}
		fmt.Printf("%s bots=%d/%d drops=%d refused=%d authfail=%d throttled=%d msgs=%d rtt=%s | "+
			"tick p99=%.1fms loop p99=%.1fms lag p99=%.1fms framedrop=%d ws=%d\n",
			tag, connected.Load(), *n, drops.Load(), refused.Load(), authFails.Load(), throttled.Load(),
			msgs.Load(), avgRTT, st.TickP99Ms, st.LoopP99Ms, st.LagP99Ms, st.FramesDropped, st.WS)
	}

	running := true
	for running {
		select {
		case <-deadline:
			running = false
		case <-ctxDone:
			running = false
		case <-ticker.C:
			printSnap("run")
		}
	}
	close(stop)
	wg.Wait()
	printSnap("done")
	if refused.Load() > 0 {
		fmt.Printf("\n%d bots never got a socket: the per-IP connection budget ran out.\n"+
			"  Raise HOLLOWMERE_LIMIT_CONN_RATE / _BURST for load runs.\n", refused.Load())
	}
	if throttled.Load() > 0 {
		fmt.Printf("\n%d bots never got a welcome: the per-IP hello budget ran out.\n"+
			"  Every bot shares one address, so raise HOLLOWMERE_LIMIT_HELLO_RATE / _BURST\n"+
			"  for load runs (compose does). This is a harness limit, not a world failure.\n",
			throttled.Load())
	}
	if authFails.Load() > 0 {
		fmt.Printf("\n%d bots could not log in. Load runs need a generous login budget:\n"+
			"  HOLLOWMERE_LIMIT_LOGIN_RATE / _BURST on the world service (compose sets these).\n"+
			"  The live values in docs/ops.md are deliberately far tighter.\n", authFails.Load())
	}
	if st := lastStats(*statsURL); st.TickP99Ms > 0 {
		if st.TickP99Ms < 50 {
			fmt.Println("gate T1-ish: tick p99 < 50ms  PASS (check empty-world separately)")
		}
		if *n >= 200 {
			// Headroom, not an arbitrary millisecond count: the loop must
			// fit inside the tick with room to spare, and lag must stay
			// near zero. Lag only grows once loop time exceeds the tick,
			// so it is the last thing to move and the first thing to trust.
			budget := float64(st.TickMs)
			ok := st.LoopP99Ms < budget*0.5 && st.LagP99Ms < budget*0.1 &&
				drops.Load() == 0 && refused.Load() == 0
			verdict := "FAIL"
			if ok {
				verdict = "PASS"
			}
			fmt.Printf("gate T2 (%s): %d/%d joined | loop p99 %.1fms of %.0fms budget (%.0f%%) | "+
				"lag p99 %.1fms | tick p99 %.1fms | frames dropped %d | sockets lost %d\n",
				verdict, *n-int(refused.Load())-int(drops.Load()), *n,
				st.LoopP99Ms, budget, 100*st.LoopP99Ms/budget,
				st.LagP99Ms, st.TickP99Ms, st.FramesDropped, drops.Load())
			fmt.Println("  loop must fit the tick with margin; lag near zero means it did.")
		}
	}
}

func lastStats(url string) protocol.Stats {
	var st protocol.Stats
	resp, err := http.Get(url)
	if err != nil {
		return st
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&st)
	return st
}

func runBot(i int, addr, statsURL, password string, hotspot bool, stop <-chan struct{},
	connected, drops, msgs, authFails, throttled, refused, rttNanos, rttN *atomic.Int64) {

	// The world only upgrades authenticated sockets, so each bot needs a
	// real account and a real session cookie before it can dial.
	cookie, err := botSession(statsURL, fmt.Sprintf("bot%03d", i), password)
	if err != nil {
		authFails.Add(1)
		return
	}
	// A face so load figures are not nameless blobs. Idempotent: a
	// second run of the harness keeps the first carve.
	_ = botLooks(statsURL, cookie, i)

	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	hdr := http.Header{}
	hdr.Set("Cookie", cookie)

	// The per-IP connection budget sees every bot as the same caller, so a
	// 200-bot run is refused at the upgrade long before the world is under
	// any real load. Back off and ask again rather than counting it as a
	// lost socket: the limiter is doing its job, and a real crowd arriving
	// from 200 addresses would never hit it.
	var conn *websocket.Conn
	for attempt := 0; attempt < 12; attempt++ {
		c, resp, derr := dialer.Dial(addr, hdr)
		if derr == nil {
			conn = c
			break
		}
		code := 0
		if resp != nil {
			code = resp.StatusCode
			_ = resp.Body.Close()
		}
		if code != http.StatusTooManyRequests {
			drops.Add(1)
			return
		}
		select {
		case <-time.After(time.Duration(250*(attempt+1))*time.Millisecond +
			time.Duration(rand.Intn(500))*time.Millisecond):
		case <-stop:
			return
		}
	}
	if conn == nil {
		refused.Add(1)
		return
	}
	defer conn.Close()
	connected.Add(1)
	defer connected.Add(-1)

	hello, _ := json.Marshal(protocol.In{T: protocol.MsgHello})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		drops.Add(1)
		return
	}

	var mu sync.Mutex
	var youX, youY int
	nodes := []protocol.NodeView{}
	inWorld := make(chan struct{}, 1)
	helloRefused := make(chan struct{}, 1)

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			msgs.Add(1)
			var peek struct {
				T string `json:"t"`
			}
			if json.Unmarshal(data, &peek) != nil {
				continue
			}
			if peek.T == protocol.MsgWelcome || peek.T == protocol.MsgState {
				var st protocol.State
				if peek.T == protocol.MsgState {
					_ = json.Unmarshal(data, &st)
					mu.Lock()
					youX, youY = st.You.X, st.You.Y
					nodes = st.Nodes
					mu.Unlock()
				}
				select {
				case inWorld <- struct{}{}:
				default:
				}
			}
			// The world refuses a join when the per-IP hello budget is
			// spent. Every bot shares one address, so a big run hits that
			// routinely; tell the main loop to ask again.
			if peek.T == protocol.MsgErr {
				select {
				case helloRefused <- struct{}{}:
				default:
				}
			}
			if peek.T == protocol.MsgPong {
				var p protocol.Pong
				if json.Unmarshal(data, &p) == nil && p.Ts > 0 {
					d := time.Now().UnixMilli() - p.Ts
					if d >= 0 && d < 10_000 {
						rttNanos.Add(d * int64(time.Millisecond))
						rttN.Add(1)
					}
				}
			}
		}
	}()

	joined := false
	for attempt := 0; attempt < 16 && !joined; attempt++ {
		if attempt > 0 {
			// Back off, then ask again. Jittered so 200 bots do not all
			// retry on the same beat.
			wait := time.Duration(300*(attempt+1))*time.Millisecond +
				time.Duration(rand.Intn(400))*time.Millisecond
			select {
			case <-time.After(wait):
			case <-stop:
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
				drops.Add(1)
				return
			}
		}
		select {
		case <-inWorld:
			joined = true
		case <-helloRefused:
			// Throttled; loop around and retry.
		case <-time.After(8 * time.Second):
			drops.Add(1)
			return
		case <-stop:
			return
		}
	}
	if !joined {
		throttled.Add(1)
		return
	}

	tick := time.NewTicker(time.Duration(protocol.TickMs) * time.Millisecond)
	defer tick.Stop()
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))

	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if rng.Intn(8) == 0 {
				ping, _ := json.Marshal(protocol.In{T: protocol.MsgPing, Ts: time.Now().UnixMilli()})
				if err := conn.WriteMessage(websocket.TextMessage, ping); err != nil {
					drops.Add(1)
					return
				}
			}
			if rng.Intn(20) == 0 {
				chat, _ := json.Marshal(protocol.In{T: protocol.MsgChat, Text: "the thicket is lively"})
				_ = conn.WriteMessage(websocket.TextMessage, chat)
			}
			if hotspot {
				mu.Lock()
				localNodes := nodes
				mu.Unlock()
				tx, ty, nid := bushHotspot(localNodes)
				if nid != "" && rng.Intn(3) == 0 {
					b, _ := json.Marshal(protocol.In{T: protocol.MsgInteract, ID: nid})
					if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
						drops.Add(1)
						return
					}
					continue
				}
				if tx == 0 && ty == 0 {
					tx, ty = 18, 3
				}
				b, _ := json.Marshal(protocol.In{
					T: protocol.MsgMove,
					X: clamp(tx+rng.Intn(3)-1, 1, mapMaxX),
					Y: clamp(ty+rng.Intn(3)-1, 1, mapMaxY),
				})
				if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
					drops.Add(1)
					return
				}
				continue
			}
			mu.Lock()
			cx, cy := youX, youY
			mu.Unlock()
			x := clamp(cx+rng.Intn(7)-3, 1, mapMaxX)
			y := clamp(cy+rng.Intn(7)-3, 1, mapMaxY)
			b, _ := json.Marshal(protocol.In{T: protocol.MsgMove, X: x, Y: y})
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				drops.Add(1)
				return
			}
		}
	}
}

// botSession registers (or logs in) a bot account and returns the
// "name=value" session cookie to present on the WebSocket upgrade.
//
// Go's cookiejar refuses ws:// URLs, so the token is carried by hand
// rather than through a jar.
func botSession(statsURL, username, password string) (string, error) {
	base, err := url.Parse(statsURL)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(protocol.AuthRequest{Username: username, Password: password})
	if err != nil {
		return "", err
	}

	post := func(path string) (*http.Response, error) {
		u := *base
		u.Path = path
		u.RawQuery = ""
		req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}

	// Register first; a second run of the harness reuses the account.
	for attempt := 0; attempt < 6; attempt++ {
		resp, err := post("/auth/register")
		if err != nil {
			return "", err
		}
		if c := sessionCookie(resp); c != "" {
			_ = resp.Body.Close()
			return c, nil
		}
		code := resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if code == http.StatusConflict {
			break // already exists: log in below
		}
		if code == http.StatusTooManyRequests {
			time.Sleep(time.Duration(400*(attempt+1)) * time.Millisecond)
			continue
		}
		return "", fmt.Errorf("register %s: status %d", username, code)
	}

	for attempt := 0; attempt < 6; attempt++ {
		resp, err := post("/auth/login")
		if err != nil {
			return "", err
		}
		if c := sessionCookie(resp); c != "" {
			_ = resp.Body.Close()
			return c, nil
		}
		code := resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if code == http.StatusTooManyRequests {
			time.Sleep(time.Duration(400*(attempt+1)) * time.Millisecond)
			continue
		}
		return "", fmt.Errorf("login %s: status %d", username, code)
	}
	return "", fmt.Errorf("login %s: throttled out", username)
}

func botLooks(statsURL, cookie string, i int) error {
	base, err := url.Parse(statsURL)
	if err != nil {
		return err
	}
	cat := protocol.AppearanceCatalog()
	pick := func(opts []protocol.LooksOption) string {
		if len(opts) == 0 {
			return ""
		}
		return opts[i%len(opts)].ID
	}
	body, err := json.Marshal(protocol.Looks{
		Skin: pick(cat.Skin), Hair: pick(cat.Hair),
		HairColor: pick(cat.HairColor), Top: pick(cat.Top),
	})
	if err != nil {
		return err
	}
	u := *base
	u.Path = "/auth/looks"
	u.RawQuery = ""
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("looks: status %d", resp.StatusCode)
	}
	return nil
}

func sessionCookie(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == protocol.SessionCookie && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	return ""
}

// The bots roam the hamlet only. mapMaxX/mapMaxY stop short of the
// southern woods on purpose: this harness measures a crowd on the gather
// loop, and wandering into combat would make the tick numbers mean two
// different things at once.
const (
	mapMaxX = 26
	mapMaxY = 18
)

func bushHotspot(nodes []protocol.NodeView) (x, y int, id string) {
	for _, n := range nodes {
		if n.Kind == protocol.KindBush && n.Ready {
			return n.X, n.Y, n.ID
		}
	}
	for _, n := range nodes {
		if n.Kind == protocol.KindBush {
			return n.X, n.Y, n.ID
		}
	}
	return 0, 0, ""
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
