package hub

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/world"
)

func readWelcome(t *testing.T, conn *websocket.Conn) protocol.Welcome {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read welcome: %v", err)
		}
		if !strings.Contains(string(data), `"t":"welcome"`) {
			continue
		}
		var welcome protocol.Welcome
		if err := json.Unmarshal(data, &welcome); err != nil {
			t.Fatalf("decode welcome: %v", err)
		}
		return welcome
	}
}

func readStateMatching(t *testing.T, conn *websocket.Conn, ok func(protocol.State) bool) protocol.State {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read state: %v", err)
		}
		var st protocol.State
		if json.Unmarshal(data, &st) != nil || st.T != protocol.MsgState {
			continue
		}
		if ok(st) {
			return st
		}
	}
}

func playerIDFor(t *testing.T, hr *harness, usernameKey string) string {
	t.Helper()
	acct, err := hr.mem.AccountByUsernameKey(context.Background(), usernameKey)
	if err != nil || acct == nil {
		t.Fatalf("account %s: %v %v", usernameKey, acct, err)
	}
	pid, err := hr.mem.PlayerIDForAccount(context.Background(), acct.ID)
	if err != nil || pid == "" {
		t.Fatalf("player id: %q %v", pid, err)
	}
	return pid
}

func seedPlayer(t *testing.T, hr *harness, usernameKey, name string, rec *world.PlayerRec) string {
	t.Helper()
	pid := playerIDFor(t, hr, usernameKey)
	rec.ID = pid
	rec.Name = name
	if rec.X == 0 && rec.Y == 0 {
		rec.X, rec.Y = 8, 8 // stile, beside the oak chest
	}
	if err := hr.hub.World.Store.SavePlayer(context.Background(), rec); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return pid
}

func countYou(inv []protocol.Item, id string) int {
	n := 0
	for _, it := range inv {
		if it.ID == id {
			n += it.N
		}
	}
	return n
}

func TestDepositRelogWithdrawAndPeerIsolation(t *testing.T) {
	hr := newHarness(t)
	kyleCookie := hr.register(t, "Kyle")
	briarCookie := hr.register(t, "Briar")

	hp := 10
	kyleID := seedPlayer(t, hr, "kyle", "Kyle", &world.PlayerRec{
		Inv:   []world.ItemStack{{ID: protocol.ItemBerry, N: 2}},
		Coins: 8,
		HP:    &hp,
	})
	briarID := playerIDFor(t, hr, "briar")

	conn := dialAuthed(t, hr, kyleCookie)
	if err := conn.WriteJSON(protocol.In{T: protocol.MsgHello}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	welcome := readWelcome(t, conn)
	if welcome.BankSlots != protocol.BankSlots {
		t.Fatalf("welcome bankSlots=%d, want %d", welcome.BankSlots, protocol.BankSlots)
	}
	if countYou(welcome.You.Inv, protocol.ItemBerry) != 2 {
		t.Fatalf("seed pack missing: %v", welcome.You.Inv)
	}

	if err := conn.WriteJSON(protocol.In{T: protocol.MsgDeposit, ID: protocol.ItemBerry, N: 1}); err != nil {
		t.Fatalf("deposit berry: %v", err)
	}
	if err := conn.WriteJSON(protocol.In{T: protocol.MsgDeposit, ID: protocol.TokenCoins, N: 8}); err != nil {
		t.Fatalf("deposit coins: %v", err)
	}
	st := readStateMatching(t, conn, func(s protocol.State) bool {
		return countYou(s.You.Bank, protocol.ItemBerry) == 1 && s.You.BankCoins == 8
	})
	if countYou(st.You.Inv, protocol.ItemBerry) != 1 || st.You.Coins != 0 {
		t.Fatalf("after deposit pack=%v coins=%d", st.You.Inv, st.You.Coins)
	}

	_ = conn.Close()

	conn2 := dialAuthed(t, hr, kyleCookie)
	if err := conn2.WriteJSON(protocol.In{T: protocol.MsgHello}); err != nil {
		t.Fatalf("relog hello: %v", err)
	}
	again := readWelcome(t, conn2)
	if countYou(again.You.Bank, protocol.ItemBerry) != 1 || again.You.BankCoins != 8 {
		t.Fatalf("relog chest bank=%v coins=%d", again.You.Bank, again.You.BankCoins)
	}

	if err := conn2.WriteJSON(protocol.In{T: protocol.MsgWithdraw, ID: protocol.ItemBerry, N: 1}); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	out := readStateMatching(t, conn2, func(s protocol.State) bool {
		return countYou(s.You.Inv, protocol.ItemBerry) == 2 && countYou(s.You.Bank, protocol.ItemBerry) == 0
	})
	if out.You.BankCoins != 8 {
		t.Fatalf("withdrawing a berry spent the chest purse: %d", out.You.BankCoins)
	}

	bConn := dialAuthed(t, hr, briarCookie)
	if err := bConn.WriteJSON(protocol.In{T: protocol.MsgHello}); err != nil {
		t.Fatalf("briar hello: %v", err)
	}
	bWelcome := readWelcome(t, bConn)
	if countYou(bWelcome.You.Bank, protocol.ItemBerry) != 0 || bWelcome.You.BankCoins != 0 {
		t.Fatalf("briar saw kyle's chest: bank=%v coins=%d", bWelcome.You.Bank, bWelcome.You.BankCoins)
	}
	if err := bConn.WriteJSON(protocol.In{T: protocol.MsgWithdraw, ID: protocol.TokenCoins, N: 1}); err != nil {
		t.Fatalf("briar withdraw: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	kyleRec, err := hr.hub.World.Store.LoadPlayer(context.Background(), kyleID)
	if err != nil || kyleRec == nil {
		t.Fatalf("load kyle: %v %v", kyleRec, err)
	}
	if kyleRec.BankCoins != 8 {
		t.Fatalf("kyle's chest coins vanished: %+v", kyleRec)
	}
	briarRec, err := hr.hub.World.Store.LoadPlayer(context.Background(), briarID)
	if err != nil {
		t.Fatalf("load briar: %v", err)
	}
	if briarRec != nil && (briarRec.Coins != 0 || briarRec.BankCoins != 0) {
		t.Fatalf("briar store grew coins: purse=%d chest=%d", briarRec.Coins, briarRec.BankCoins)
	}
}
