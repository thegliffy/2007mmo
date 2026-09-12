// Headless WebSocket load harness. Not real browsers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func main() {
	n := flag.Int("n", 50, "number of bot clients")
	addr := flag.String("addr", "ws://127.0.0.1:8080/ws", "world websocket URL")
	statsURL := flag.String("stats", "http://127.0.0.1:8080/stats", "world /stats URL")
	hotspot := flag.Bool("hotspot", false, "crowd bots onto the berry thicket")
	duration := flag.Duration("duration", 45*time.Second, "how long to run")
	flag.Parse()

	log.SetPrefix("bots ")
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	ctxDone := make(chan os.Signal, 1)
	signal.Notify(ctxDone, os.Interrupt, syscall.SIGTERM)

	var connected, drops, msgs atomic.Int64
	var rttNanos atomic.Int64
	var rttN atomic.Int64

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runBot(i, *addr, *hotspot, stop, &connected, &drops, &msgs, &rttNanos, &rttN)
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
		fmt.Printf("%s bots=%d/%d drops=%d msgs=%d rtt=%s | world tick=%d p50=%.2fms p99=%.2fms ws=%d\n",
			tag, connected.Load(), *n, drops.Load(), msgs.Load(), avgRTT,
			st.Tick, st.TickP50Ms, st.TickP99Ms, st.WS)
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
	if st := lastStats(*statsURL); st.TickP99Ms > 0 {
		if st.TickP99Ms < 50 {
			fmt.Println("gate T1-ish: tick p99 < 50ms  PASS (check empty-world separately)")
		}
		if *n >= 200 && st.TickP99Ms < 150 && drops.Load() == 0 {
			fmt.Println("gate T2: 200 bots hotspot p99 < 150ms, no WS collapse  PASS")
		} else if *n >= 200 {
			fmt.Printf("gate T2: p99=%.2fms drops=%d  (target p99<150, drops=0)\n", st.TickP99Ms, drops.Load())
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

func runBot(i int, addr string, hotspot bool, stop <-chan struct{}, connected, drops, msgs, rttNanos, rttN *atomic.Int64) {
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := dialer.Dial(addr, nil)
	if err != nil {
		drops.Add(1)
		return
	}
	defer conn.Close()
	connected.Add(1)
	defer connected.Add(-1)

	id := uuid.NewString()
	hello, _ := json.Marshal(protocol.In{T: protocol.MsgHello, PlayerID: id, Name: fmt.Sprintf("Bot%03d", i)})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		drops.Add(1)
		return
	}

	var youX, youY int
	nodes := []protocol.NodeView{}
	inWorld := make(chan struct{}, 1)

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
					youX, youY = st.You.X, st.You.Y
					nodes = st.Nodes
				}
				select {
				case inWorld <- struct{}{}:
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

	select {
	case <-inWorld:
	case <-time.After(8 * time.Second):
		drops.Add(1)
		return
	case <-stop:
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
				tx, ty, nid := bushHotspot(nodes)
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
				b, _ := json.Marshal(protocol.In{T: protocol.MsgMove, X: clamp(tx+rng.Intn(3)-1, 1, 22), Y: clamp(ty+rng.Intn(3)-1, 1, 14)})
				if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
					drops.Add(1)
					return
				}
				continue
			}
			x := clamp(youX+rng.Intn(7)-3, 1, 22)
			y := clamp(youY+rng.Intn(7)-3, 1, 14)
			b, _ := json.Marshal(protocol.In{T: protocol.MsgMove, X: x, Y: y})
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				drops.Add(1)
				return
			}
		}
	}
}

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
