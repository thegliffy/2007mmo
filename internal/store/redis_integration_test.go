package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// Session tests against a real Redis. Skipped unless a throwaway instance
// is provided:
//
//	HOLLOWMERE_TEST_REDIS_URL=redis://127.0.0.1:6379 go test ./internal/store/...
//
// `docker compose up redis` is enough. The tests use their own key names
// but still call FLUSHDB, so do not point this at anything you care about.
func testRedis(t *testing.T) *Redis {
	t.Helper()
	url := os.Getenv("HOLLOWMERE_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set HOLLOWMERE_TEST_REDIS_URL to run the Redis integration tests")
	}
	rd, err := NewRedis(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(rd.Close)
	if err := rd.c.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return rd
}

func TestRedisSessionRoundTrip(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()

	if err := rd.CreateSession(ctx, "tok-1", "acct-1", time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := rd.SessionAccount(ctx, "tok-1")
	if err != nil || got != "acct-1" {
		t.Fatalf("resolve = %q err=%v", got, err)
	}

	// An unknown token is empty, not an error: the caller turns that into
	// "log in first" rather than a 500.
	missing, err := rd.SessionAccount(ctx, "tok-nope")
	if err != nil || missing != "" {
		t.Fatalf("unknown token = %q err=%v", missing, err)
	}

	if err := rd.DeleteSession(ctx, "tok-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := rd.SessionAccount(ctx, "tok-1"); got != "" {
		t.Fatalf("deleted token still resolves to %q", got)
	}
}

func TestRedisSessionExpires(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()
	if err := rd.CreateSession(ctx, "tok-short", "acct-1", 40*time.Millisecond); err != nil {
		t.Fatalf("create: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	if got, _ := rd.SessionAccount(ctx, "tok-short"); got != "" {
		t.Fatalf("expired session still resolves to %q", got)
	}
}

func TestRedisTouchSessionExtends(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()
	if err := rd.CreateSession(ctx, "tok-1", "acct-1", 80*time.Millisecond); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Slide the window twice across what would have been the original expiry.
	for i := 0; i < 2; i++ {
		time.Sleep(50 * time.Millisecond)
		if err := rd.TouchSession(ctx, "tok-1", 200*time.Millisecond); err != nil {
			t.Fatalf("touch: %v", err)
		}
	}
	if got, _ := rd.SessionAccount(ctx, "tok-1"); got != "acct-1" {
		t.Fatalf("touched session expired anyway (got %q)", got)
	}
}

// A password change must be able to kill every other session for the
// account while keeping the one that did the changing.
func TestRedisDeleteAccountSessionsKeepsOne(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()
	for _, tok := range []string{"a", "b", "c"} {
		if err := rd.CreateSession(ctx, tok, "acct-1", time.Minute); err != nil {
			t.Fatalf("create %s: %v", tok, err)
		}
	}
	// A session on a different account must be untouched.
	if err := rd.CreateSession(ctx, "other", "acct-2", time.Minute); err != nil {
		t.Fatalf("create other: %v", err)
	}

	if err := rd.DeleteAccountSessions(ctx, "acct-1", "b"); err != nil {
		t.Fatalf("delete account sessions: %v", err)
	}
	for _, dead := range []string{"a", "c"} {
		if got, _ := rd.SessionAccount(ctx, dead); got != "" {
			t.Fatalf("session %q survived (got %q)", dead, got)
		}
	}
	if got, _ := rd.SessionAccount(ctx, "b"); got != "acct-1" {
		t.Fatalf("kept session died (got %q)", got)
	}
	if got, _ := rd.SessionAccount(ctx, "other"); got != "acct-2" {
		t.Fatalf("another account's session was collateral damage (got %q)", got)
	}
}

func TestRedisDeleteAllAccountSessions(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()
	for _, tok := range []string{"a", "b"} {
		if err := rd.CreateSession(ctx, tok, "acct-1", time.Minute); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := rd.DeleteAccountSessions(ctx, "acct-1", ""); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	for _, tok := range []string{"a", "b"} {
		if got, _ := rd.SessionAccount(ctx, tok); got != "" {
			t.Fatalf("session %q survived a full revoke", tok)
		}
	}
}

// Revoking an account with no sessions must not error.
func TestRedisDeleteAccountSessionsEmpty(t *testing.T) {
	rd := testRedis(t)
	if err := rd.DeleteAccountSessions(context.Background(), "acct-none", ""); err != nil {
		t.Fatalf("empty revoke: %v", err)
	}
}

func TestRedisPresence(t *testing.T) {
	rd := testRedis(t)
	ctx := context.Background()
	if err := rd.SetPresence(ctx, "player-1"); err != nil {
		t.Fatalf("set presence: %v", err)
	}
	if err := rd.ClearPresence(ctx, "player-1"); err != nil {
		t.Fatalf("clear presence: %v", err)
	}
}
