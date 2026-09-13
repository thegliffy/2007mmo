package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/store"
	"github.com/thegliffy/2007mmo/internal/world"
)

const testPW = "bramble-hollow-9"

type harness struct {
	hub    *Hub
	srv    *httptest.Server
	mem    *auth.Memory
	cancel context.CancelFunc
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	mem := auth.NewMemory()
	svc := auth.NewService(mem, mem)
	w := world.New(store.NewMemory())
	h := New(w, nil, nil, svc, 40*time.Millisecond)

	mux := http.NewServeMux()
	h.Routes(mux)
	srv := httptest.NewServer(mux)

	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)

	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	return &harness{hub: h, srv: srv, mem: mem, cancel: cancel}
}

// post sends a same-origin JSON request the way the browser client does.
func (hr *harness) post(t *testing.T, path string, body any, cookie *http.Cookie) *http.Response {
	t.Helper()
	var buf []byte
	if body != nil {
		var err error
		buf, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	} else {
		buf = []byte("{}")
	}
	req, err := http.NewRequest(http.MethodPost, hr.srv.URL+path, bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", hr.srv.URL)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func sessionCookieFrom(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == protocol.SessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

func (hr *harness) register(t *testing.T, name string) *http.Cookie {
	t.Helper()
	resp := hr.post(t, "/auth/register", protocol.AuthRequest{Username: name, Password: testPW}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: status %d", name, resp.StatusCode)
	}
	c := sessionCookieFrom(resp)
	if c == nil {
		t.Fatal("register set no session cookie")
	}
	return c
}

func TestRegisterSetsHttpOnlyCookie(t *testing.T) {
	hr := newHarness(t)
	resp := hr.post(t, "/auth/register", protocol.AuthRequest{Username: "Kyle", Password: testPW}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	c := sessionCookieFrom(resp)
	if c == nil {
		t.Fatal("no session cookie")
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly so script cannot read it")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want /", c.Path)
	}

	// The response body must not echo the token back into script reach.
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	for k, v := range body {
		if s, ok := v.(string); ok && s == c.Value {
			t.Fatalf("field %q leaks the session token into the page", k)
		}
	}
}

func TestMeRequiresSession(t *testing.T) {
	hr := newHarness(t)

	resp, err := http.Get(hr.srv.URL + "/auth/me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous /auth/me = %d, want 401", resp.StatusCode)
	}

	cookie := hr.register(t, "Kyle")
	req, _ := http.NewRequest(http.MethodGet, hr.srv.URL+"/auth/me", nil)
	req.AddCookie(cookie)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("authenticated /auth/me = %d", resp2.StatusCode)
	}
	var out protocol.AuthResponse
	_ = json.NewDecoder(resp2.Body).Decode(&out)
	if out.Username != "Kyle" {
		t.Fatalf("username = %q", out.Username)
	}
}

func TestLoginWrongPasswordIs401(t *testing.T) {
	hr := newHarness(t)
	hr.register(t, "Kyle")
	resp := hr.post(t, "/auth/login", protocol.AuthRequest{Username: "Kyle", Password: "not-the-password"}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
	if sessionCookieFrom(resp) != nil {
		t.Fatal("a failed login must not set a session cookie")
	}
}

func TestLogoutClearsCookieAndSession(t *testing.T) {
	hr := newHarness(t)
	cookie := hr.register(t, "Kyle")

	resp := hr.post(t, "/auth/logout", nil, cookie)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status %d", resp.StatusCode)
	}
	cleared := sessionCookieFrom(resp)
	if cleared != nil {
		t.Fatal("logout should clear, not reissue, the cookie")
	}

	// The old token must be dead server-side, not just dropped by the client.
	if _, _, _, err := hr.hub.Auth.Resolve(context.Background(), cookie.Value); err == nil {
		t.Fatal("session survived logout")
	}
}

// Cookie auth means these endpoints need CSRF protection. An HTML form on
// a hostile page can only send urlencoded/multipart/text-plain, so
// demanding JSON blocks it outright.
func TestNonJSONPostIsRejected(t *testing.T) {
	hr := newHarness(t)
	body := strings.NewReader(`{"username":"Kyle","password":"` + testPW + `"}`)
	req, _ := http.NewRequest(http.MethodPost, hr.srv.URL+"/auth/register", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestCrossOriginPostIsRejected(t *testing.T) {
	hr := newHarness(t)
	buf, _ := json.Marshal(protocol.AuthRequest{Username: "Kyle", Password: testPW})
	req, _ := http.NewRequest(http.MethodPost, hr.srv.URL+"/auth/register", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestUnknownJSONFieldsRejected(t *testing.T) {
	hr := newHarness(t)
	body := strings.NewReader(`{"username":"Kyle","password":"` + testPW + `","playerId":"someone-else"}`)
	req, _ := http.NewRequest(http.MethodPost, hr.srv.URL+"/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", hr.srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 for an unexpected field", resp.StatusCode)
	}
}

func wsURL(srvURL string) string {
	return "ws" + strings.TrimPrefix(srvURL, "http") + "/ws"
}

func TestWebSocketRefusedWithoutSession(t *testing.T) {
	hr := newHarness(t)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(hr.srv.URL), nil)
	if err == nil {
		t.Fatal("an unauthenticated upgrade succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		got := 0
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("status %d, want 401", got)
	}
}

func TestWebSocketRefusedFromForeignOrigin(t *testing.T) {
	hr := newHarness(t)
	cookie := hr.register(t, "Kyle")
	hdr := http.Header{}
	hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
	hdr.Set("Origin", "https://evil.example")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(hr.srv.URL), hdr)
	if err == nil {
		t.Fatal("a cross-site upgrade succeeded; a hostile page could ride the cookie")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		got := 0
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("status %d, want 403", got)
	}
}

// The full path: register, upgrade with the cookie, receive a welcome
// addressed to the right account — and make sure that welcome carries a
// handle rather than the real player id.
func TestWebSocketJoinWithSessionNeverLeaksPlayerID(t *testing.T) {
	hr := newHarness(t)
	cookie := hr.register(t, "Kyle")

	acct, err := hr.mem.AccountByUsernameKey(context.Background(), "kyle")
	if err != nil || acct == nil {
		t.Fatalf("account lookup: %v", err)
	}
	realPlayerID, err := hr.mem.PlayerIDForAccount(context.Background(), acct.ID)
	if err != nil || realPlayerID == "" {
		t.Fatalf("player id lookup: %v", err)
	}

	hdr := http.Header{}
	hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(hr.srv.URL), hdr)
	if err != nil {
		got := 0
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, got)
	}
	defer conn.Close()

	if err := conn.WriteJSON(protocol.In{T: protocol.MsgHello}); err != nil {
		t.Fatalf("hello: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var welcome protocol.Welcome
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(data), `"t":"welcome"`) {
			if err := json.Unmarshal(data, &welcome); err != nil {
				t.Fatalf("decode welcome: %v", err)
			}
			if strings.Contains(string(data), realPlayerID) {
				t.Fatal("the welcome frame contains the real player id")
			}
			if strings.Contains(string(data), cookie.Value) {
				t.Fatal("the welcome frame contains the session token")
			}
			break
		}
	}
	if welcome.Username != "Kyle" {
		t.Fatalf("welcome username = %q", welcome.Username)
	}
	if welcome.Handle == "" {
		t.Fatal("welcome carried no handle")
	}
	if welcome.Handle == realPlayerID {
		t.Fatal("the handle is the real player id")
	}
	if welcome.You.ID != welcome.Handle {
		t.Fatalf("you.id %q should be the handle %q", welcome.You.ID, welcome.Handle)
	}
}

// A second player must never learn the first player's real id.
func TestPeerFramesCarryHandlesOnly(t *testing.T) {
	hr := newHarness(t)
	ctx := context.Background()

	join := func(name string) (*websocket.Conn, string) {
		cookie := hr.register(t, name)
		acct, _ := hr.mem.AccountByUsernameKey(ctx, strings.ToLower(name))
		pid, _ := hr.mem.PlayerIDForAccount(ctx, acct.ID)
		hdr := http.Header{}
		hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
		conn, _, err := websocket.DefaultDialer.Dial(wsURL(hr.srv.URL), hdr)
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		if err := conn.WriteJSON(protocol.In{T: protocol.MsgHello}); err != nil {
			t.Fatalf("hello %s: %v", name, err)
		}
		return conn, pid
	}

	aConn, aID := join("Ash")
	defer aConn.Close()
	bConn, bID := join("Briar")
	defer bConn.Close()

	// Read Ash's frames until Briar shows up in the roster.
	_ = aConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 60; i++ {
		_, data, err := aConn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		raw := string(data)
		if strings.Contains(raw, aID) || strings.Contains(raw, bID) {
			t.Fatalf("a frame sent to Ash contains a real player id: %s", raw)
		}
		var st protocol.State
		if json.Unmarshal(data, &st) != nil || st.T != protocol.MsgState {
			continue
		}
		for _, p := range st.Players {
			if p.Name == "Briar" {
				if p.ID == bID {
					t.Fatal("peer view exposed Briar's real player id")
				}
				return // saw the peer, carried by handle
			}
		}
	}
	t.Fatal("Briar never appeared in Ash's snapshot")
}

// A name collision is a normal signup outcome, not a credential failure.
// Counting it would make a busy registration day look like a break-in.
func TestNameCollisionIsNotCountedAsLoginFailure(t *testing.T) {
	hr := newHarness(t)
	hr.register(t, "Kyle")
	before := hr.hub.stats().LoginFails

	resp := hr.post(t, "/auth/register", protocol.AuthRequest{Username: "Kyle", Password: testPW}, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status %d, want 409", resp.StatusCode)
	}
	if got := hr.hub.stats().LoginFails; got != before {
		t.Fatalf("loginFails moved %d -> %d on a name collision", before, got)
	}

	// A wrong password, on the other hand, must be counted.
	resp2 := hr.post(t, "/auth/login", protocol.AuthRequest{Username: "Kyle", Password: "not-the-password"}, nil)
	resp2.Body.Close()
	if got := hr.hub.stats().LoginFails; got != before+1 {
		t.Fatalf("loginFails = %d, want %d after one wrong password", got, before+1)
	}
}
