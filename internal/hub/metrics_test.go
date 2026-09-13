package hub

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/world"
)

func TestSessionDeadIsOnlyDefinitiveAuthLoss(t *testing.T) {
	if !sessionDead(auth.ErrNoSession) || !sessionDead(auth.ErrBanned) {
		t.Fatal("a missing or banned session must evict")
	}
	if sessionDead(errors.New("redis: i/o timeout")) {
		t.Fatal("a store blip must not look like a dead session")
	}
	if sessionDead(nil) {
		t.Fatal("nil is not a dead session")
	}
}

func TestMetricsExposeOpsNumbers(t *testing.T) {
	h := New(world.New(nil), nil, nil, nil, 600*time.Millisecond)
	h.metrics.Observe(2 * time.Millisecond)
	h.metrics.ObserveLoop(5 * time.Millisecond)
	h.metrics.ObserveLag(-3 * time.Millisecond) // late-by-negative is idle; clamp to 0
	h.metrics.ObserveLag(1 * time.Millisecond)
	h.metrics.AddJoin()
	h.metrics.AddReconnect()
	h.metrics.AddAction()
	h.metrics.AddLimited("ws")
	h.metrics.AddLimited("login")
	h.metrics.SetOnline(3, 4)
	h.metrics.SetWS(2)
	h.metrics.SetTick(9)

	rec := httptest.NewRecorder()
	h.ServeMetrics(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		"hollowmere_tick_p50_ms",
		"hollowmere_tick_p99_ms",
		"hollowmere_loop_p50_ms",
		"hollowmere_loop_p99_ms",
		"hollowmere_tick_lag_p50_ms",
		"hollowmere_tick_lag_p99_ms",
		"hollowmere_online 3",
		"hollowmere_ws 2",
		"hollowmere_joins_total 1",
		"hollowmere_reconnects_total 1",
		"hollowmere_actions_total 1",
		`hollowmere_rate_limited_total{kind="ws"} 1`,
		`hollowmere_rate_limited_total{kind="login"} 1`,
		`hollowmere_rate_limited_total{kind="auth"}`,
		"hollowmere_tick_samples",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q\n%s", want, body)
		}
	}

	stRec := httptest.NewRecorder()
	h.ServeStats(stRec, httptest.NewRequest("GET", "/stats", nil))
	var st protocol.Stats
	if err := json.Unmarshal(stRec.Body.Bytes(), &st); err != nil {
		t.Fatalf("stats json: %v", err)
	}
	if st.Online != 3 || st.WS != 2 || st.Joins != 1 || st.Reconnects != 1 || st.Actions != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if st.LagP50Ms < 0 {
		t.Fatalf("lag p50 must not be negative: %v", st.LagP50Ms)
	}
}

func TestLagClampsNegativeToZero(t *testing.T) {
	m := NewMetrics()
	m.ObserveLag(-50 * time.Millisecond)
	_, _, _, lagP50, _, _, _ := m.LoopPercentiles()
	if lagP50 != 0 {
		t.Fatalf("negative lag should clamp to 0, got %v", lagP50)
	}
}
