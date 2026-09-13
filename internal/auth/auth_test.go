package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodPW = "bramble-hollow-9"

func newTestService(t *testing.T) (*Service, *Memory) {
	t.Helper()
	m := NewMemory()
	return NewService(m, m), m
}

func TestRegisterThenLogin(t *testing.T) {
	svc, mem := newTestService(t)
	ctx := context.Background()

	acctID, playerID, username, token, err := svc.Register(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if username != "Kyle" {
		t.Fatalf("display name not preserved: %q", username)
	}
	if acctID == "" || playerID == "" || token == "" {
		t.Fatalf("register returned empty ids: %q %q %q", acctID, playerID, token)
	}
	if mem.PlayerName(playerID) != "Kyle" {
		t.Fatalf("player row name = %q", mem.PlayerName(playerID))
	}

	// Password must not be recoverable from the store.
	acct, _ := mem.AccountByID(ctx, acctID)
	if strings.Contains(acct.PWHash, goodPW) {
		t.Fatal("stored hash contains the plaintext password")
	}
	if !strings.HasPrefix(acct.PWHash, "scrypt$") {
		t.Fatalf("hash is not self-describing: %q", acct.PWHash)
	}

	// Login is case-insensitive on the name, exact on the password.
	gotAcct, gotPlayer, gotName, tok2, err := svc.Login(ctx, "kYLe", goodPW)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if gotAcct != acctID || gotPlayer != playerID {
		t.Fatalf("login resolved a different account/player: %q/%q", gotAcct, gotPlayer)
	}
	if gotName != "Kyle" {
		t.Fatalf("login returned name %q, want the registered casing", gotName)
	}
	if tok2 == token {
		t.Fatal("login reused the previous session token")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, _, _, _, err := svc.Register(ctx, "Kyle", goodPW); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, _, _, _, err := svc.Login(ctx, "Kyle", goodPW+"x"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password gave %v, want ErrBadCredentials", err)
	}
}

// An unknown name and a wrong password must be indistinguishable, so a
// probe cannot enumerate who has an account.
func TestUnknownUserLooksLikeWrongPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, _, _, _, err := svc.Register(ctx, "Kyle", goodPW); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, _, _, _, missing := svc.Login(ctx, "Nobody", goodPW)
	_, _, _, _, wrong := svc.Login(ctx, "Kyle", "not-the-password")
	if missing == nil || wrong == nil {
		t.Fatal("both probes should fail")
	}
	if missing.Error() != wrong.Error() {
		t.Fatalf("error text differs: %q vs %q", missing, wrong)
	}
}

func TestDuplicateNameRejected(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, _, _, _, err := svc.Register(ctx, "Kyle", goodPW); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Different casing is still the same name.
	if _, _, _, _, err := svc.Register(ctx, "kyle", goodPW); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate gave %v, want ErrUsernameTaken", err)
	}
}

func TestResolveSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	acctID, playerID, _, token, err := svc.Register(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	gotAcct, gotPlayer, gotName, err := svc.Resolve(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotAcct != acctID || gotPlayer != playerID || gotName != "Kyle" {
		t.Fatalf("resolve mismatch: %q %q %q", gotAcct, gotPlayer, gotName)
	}
	if _, _, _, err := svc.Resolve(ctx, ""); !errors.Is(err, ErrNoSession) {
		t.Fatalf("empty token gave %v", err)
	}
	if _, _, _, err := svc.Resolve(ctx, "not-a-real-token"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("bogus token gave %v", err)
	}
}

func TestLogoutInvalidatesOnlyThatSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	acctID, _, _, first, err := svc.Register(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, _, _, second, err := svc.Login(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := svc.Logout(ctx, first); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, _, err := svc.Resolve(ctx, first); !errors.Is(err, ErrNoSession) {
		t.Fatalf("logged-out token still resolves: %v", err)
	}
	if _, _, _, err := svc.Resolve(ctx, second); err != nil {
		t.Fatalf("the other session should survive: %v", err)
	}
	_ = acctID
}

// Changing a password must kill every other session, so a stolen cookie
// stops working the moment the owner reacts.
func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	svc, mem := newTestService(t)
	ctx := context.Background()
	acctID, _, _, stolen, err := svc.Register(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, _, _, mine, err := svc.Login(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if got := mem.SessionCount(acctID); got != 2 {
		t.Fatalf("expected 2 live sessions, got %d", got)
	}

	newPW := "different-hollow-77"
	fresh, err := svc.ChangePassword(ctx, acctID, goodPW, newPW)
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if fresh == mine || fresh == stolen {
		t.Fatal("change password reused an old token")
	}
	if _, _, _, err := svc.Resolve(ctx, stolen); !errors.Is(err, ErrNoSession) {
		t.Fatalf("stolen session survived the password change: %v", err)
	}
	if _, _, _, err := svc.Resolve(ctx, mine); !errors.Is(err, ErrNoSession) {
		t.Fatalf("old session survived the password change: %v", err)
	}
	if _, _, _, err := svc.Resolve(ctx, fresh); err != nil {
		t.Fatalf("the new session should work: %v", err)
	}

	// The old password must no longer open the door.
	if _, _, _, _, err := svc.Login(ctx, "Kyle", goodPW); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	if _, _, _, _, err := svc.Login(ctx, "Kyle", newPW); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
}

func TestChangePasswordNeedsCurrentPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	acctID, _, _, _, err := svc.Register(ctx, "Kyle", goodPW)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.ChangePassword(ctx, acctID, "wrong-current-pw", "another-good-one"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("got %v, want ErrBadCredentials", err)
	}
	if _, err := svc.ChangePassword(ctx, acctID, goodPW, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak replacement accepted: %v", err)
	}
}

func TestRegisterValidatesInput(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	cases := []struct {
		name, user, pass string
		want             error
	}{
		{"short name", "ky", goodPW, ErrBadUsername},
		{"long name", strings.Repeat("k", 17), goodPW, ErrBadUsername},
		{"space in name", "Kyle Smith", goodPW, ErrBadUsername},
		{"symbol in name", "Kyle!", goodPW, ErrBadUsername},
		{"leading dash", "-kyle", goodPW, ErrBadUsername},
		{"short password", "Kyle", "abc123", ErrWeakPassword},
		{"name in password", "Kyle", "kyle-kyle-kyle", ErrWeakPassword},
		{"common password", "Kyle", "password123", ErrWeakPassword},
		{"one repeated rune", "Kyle", "aaaaaaaaaaaa", ErrWeakPassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := svc.Register(ctx, tc.user, tc.pass)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword(goodPW)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, stale, err := verifyPassword(h, goodPW)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v stale=%v err=%v", ok, stale, err)
	}
	if stale {
		t.Fatal("a freshly made hash should not be stale")
	}
	ok, _, err = verifyPassword(h, goodPW+"!")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
	// Two hashes of the same password must differ (unique salt).
	h2, _ := hashPassword(goodPW)
	if h == h2 {
		t.Fatal("hashes are not salted")
	}
}

// A hash written with weaker parameters must verify and be flagged for
// upgrade, so raising the cost later does not lock anyone out.
func TestWeakerParamsVerifyButAreStale(t *testing.T) {
	legacy, err := hashWithParams(goodPW, 1024, 8, 1)
	if err != nil {
		t.Fatalf("legacy hash: %v", err)
	}
	ok, stale, err := verifyPassword(legacy, goodPW)
	if err != nil || !ok {
		t.Fatalf("legacy verify: ok=%v err=%v", ok, err)
	}
	if !stale {
		t.Fatal("a weaker hash should be reported stale")
	}
}

func TestMalformedHashIsAnError(t *testing.T) {
	for _, bad := range []string{"", "plaintext", "scrypt$x$8$1$a$b", "bcrypt$1$2$3$4$5", "scrypt$16384$8$1$!!!$b"} {
		if _, _, err := verifyPassword(bad, goodPW); err == nil {
			t.Fatalf("malformed hash %q was accepted", bad)
		}
	}
}
