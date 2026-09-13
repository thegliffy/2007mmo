package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// SessionTTL is how long a login cookie stays good. Refreshed whenever
// the holder proves it is still around (WS connect, /auth/me).
const SessionTTL = 7 * 24 * time.Hour

// maxConcurrentHashes bounds scrypt memory. Each hash is ~16 MiB, so a
// login flood costs at most this many times that, instead of one
// allocation per request.
const maxConcurrentHashes = 4

// Account is one set of login credentials. Exactly one player row hangs
// off each account.
// Roles. Grants are host-only on purpose: an admin compromise must not be
// able to mint more admins. See docs/adr/0001-admin-role.md.
const (
	RolePlayer    = "player"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// ValidRole reports whether r is a role we recognise.
func ValidRole(r string) bool {
	switch r {
	case RolePlayer, RoleModerator, RoleAdmin:
		return true
	}
	return false
}

type Account struct {
	ID          string
	Username    string
	UsernameKey string
	PWHash      string
	Role        string
	BannedUntil *time.Time
}

// Banned reports whether the account is currently shut out.
func (a *Account) Banned() bool {
	return a != nil && a.BannedUntil != nil && a.BannedUntil.After(time.Now())
}

// Accounts is the canonical credential store.
type Accounts interface {
	// CreateAccount writes the account and its player row in one
	// transaction. It returns ErrUsernameTaken if the key is present.
	CreateAccount(ctx context.Context, a Account, playerID, playerName string) error
	AccountByUsernameKey(ctx context.Context, key string) (*Account, error)
	AccountByID(ctx context.Context, id string) (*Account, error)
	UpdatePasswordHash(ctx context.Context, accountID, hash string) error
	SetRole(ctx context.Context, accountID, role string) error
	SetBannedUntil(ctx context.Context, accountID string, until *time.Time) error
	PlayerIDForAccount(ctx context.Context, accountID string) (string, error)
	TouchLogin(ctx context.Context, accountID string) error
}

// Sessions holds live login tokens. Redis in production: losing it logs
// everyone out, it never holds a password.
type Sessions interface {
	CreateSession(ctx context.Context, token, accountID string, ttl time.Duration) error
	SessionAccount(ctx context.Context, token string) (string, error)
	TouchSession(ctx context.Context, token string, ttl time.Duration) error
	DeleteSession(ctx context.Context, token string) error
	// DeleteAccountSessions drops every session for an account except
	// keep (pass "" to drop all of them).
	DeleteAccountSessions(ctx context.Context, accountID, keep string) error
}

type Service struct {
	accounts Accounts
	sessions Sessions
	sem      chan struct{}
}

func NewService(a Accounts, s Sessions) *Service {
	return &Service{
		accounts: a,
		sessions: s,
		sem:      make(chan struct{}, maxConcurrentHashes),
	}
}

// acquire bounds concurrent scrypt work. It respects ctx so a client that
// hangs up while queued does not still pay for a hash.
func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() { <-s.sem }

// Register creates an account, its player row, and a first session.
func (s *Service) Register(ctx context.Context, rawName, password string) (accountID, playerID, username, token string, err error) {
	display, key, err := ValidateUsername(rawName)
	if err != nil {
		return "", "", "", "", err
	}
	if err := ValidatePassword(display, password); err != nil {
		return "", "", "", "", err
	}

	// Cheap pre-check for a friendly error. CreateAccount still relies on
	// the unique index, which is what actually decides the race.
	if existing, err := s.accounts.AccountByUsernameKey(ctx, key); err == nil && existing != nil {
		return "", "", "", "", ErrUsernameTaken
	}

	if err := s.acquire(ctx); err != nil {
		return "", "", "", "", err
	}
	hash, err := hashPassword(password)
	s.release()
	if err != nil {
		return "", "", "", "", err
	}

	acct := Account{
		ID:          newID(),
		Username:    display,
		UsernameKey: key,
		PWHash:      hash,
	}
	pid := newID()
	if err := s.accounts.CreateAccount(ctx, acct, pid, display); err != nil {
		return "", "", "", "", err
	}

	token, err = s.issue(ctx, acct.ID)
	if err != nil {
		return "", "", "", "", err
	}
	return acct.ID, pid, display, token, nil
}

// Login verifies credentials and issues a session.
func (s *Service) Login(ctx context.Context, rawName, password string) (accountID, playerID, username, token string, err error) {
	key := normalizeKey(rawName)

	var acct *Account
	if key != "" {
		acct, _ = s.accounts.AccountByUsernameKey(ctx, key)
	}

	stored := timingEqualizerHash()
	if acct != nil {
		stored = acct.PWHash
	}

	if err := s.acquire(ctx); err != nil {
		return "", "", "", "", err
	}
	ok, stale, verr := verifyPassword(stored, password)
	s.release()

	if acct == nil || verr != nil || !ok {
		return "", "", "", "", ErrBadCredentials
	}

	if stale {
		if err := s.acquire(ctx); err == nil {
			if fresh, herr := hashPassword(password); herr == nil {
				_ = s.accounts.UpdatePasswordHash(ctx, acct.ID, fresh)
			}
			s.release()
		}
	}

	if acct.Banned() {
		return "", "", "", "", ErrBanned
	}

	pid, err := s.accounts.PlayerIDForAccount(ctx, acct.ID)
	if err != nil {
		return "", "", "", "", err
	}
	token, err = s.issue(ctx, acct.ID)
	if err != nil {
		return "", "", "", "", err
	}
	// One live session per account. A login from another device must
	// kill the cookie already sitting in the first browser, or "signed
	// in elsewhere" is only a socket kick the old tab can walk back in
	// from.
	_ = s.sessions.DeleteAccountSessions(ctx, acct.ID, token)
	_ = s.accounts.TouchLogin(ctx, acct.ID)
	return acct.ID, pid, acct.Username, token, nil
}

// Resolve turns a session token into the account and player it belongs
// to, and slides the session's expiry forward.
func (s *Service) Resolve(ctx context.Context, token string) (accountID, playerID, username string, err error) {
	if token == "" {
		return "", "", "", ErrNoSession
	}
	accountID, err = s.sessions.SessionAccount(ctx, token)
	if err != nil {
		return "", "", "", err
	}
	if accountID == "" {
		return "", "", "", ErrNoSession
	}
	acct, err := s.accounts.AccountByID(ctx, accountID)
	if err != nil {
		return "", "", "", err
	}
	if acct == nil {
		// Account deleted under a live cookie.
		_ = s.sessions.DeleteSession(ctx, token)
		return "", "", "", ErrNoSession
	}
	if acct.Banned() {
		// Drop the session so the ban survives a reconnect, and let the
		// heartbeat loop evict whatever socket is still open.
		_ = s.sessions.DeleteAccountSessions(ctx, acct.ID, "")
		return "", "", "", ErrBanned
	}
	playerID, err = s.accounts.PlayerIDForAccount(ctx, accountID)
	if err != nil {
		return "", "", "", err
	}
	_ = s.sessions.TouchSession(ctx, token, SessionTTL)
	return accountID, playerID, acct.Username, nil
}

// ChangePassword rotates the password and returns a fresh session token.
// Every other session for the account is dropped, so a stolen cookie
// dies when the owner changes their password.
func (s *Service) ChangePassword(ctx context.Context, accountID, current, next string) (newToken string, err error) {
	acct, err := s.accounts.AccountByID(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acct == nil {
		return "", ErrNoSession
	}
	if err := s.acquire(ctx); err != nil {
		return "", err
	}
	ok, _, verr := verifyPassword(acct.PWHash, current)
	s.release()
	if verr != nil || !ok {
		return "", ErrBadCredentials
	}
	if err := ValidatePassword(acct.Username, next); err != nil {
		return "", err
	}
	if err := s.acquire(ctx); err != nil {
		return "", err
	}
	hash, herr := hashPassword(next)
	s.release()
	if herr != nil {
		return "", herr
	}
	if err := s.accounts.UpdatePasswordHash(ctx, accountID, hash); err != nil {
		return "", err
	}
	newToken, err = s.issue(ctx, accountID)
	if err != nil {
		return "", err
	}
	if err := s.sessions.DeleteAccountSessions(ctx, accountID, newToken); err != nil {
		return newToken, nil // password did change; stale sessions are the lesser problem
	}
	return newToken, nil
}

// FindAccount resolves a login name to its account id and display name.
// Returns an empty id when no such account exists.
func (s *Service) FindAccount(ctx context.Context, rawName string) (accountID, username string, err error) {
	key := normalizeKey(rawName)
	if key == "" {
		return "", "", fmt.Errorf("%w: %q", ErrBadUsername, rawName)
	}
	acct, err := s.accounts.AccountByUsernameKey(ctx, key)
	if err != nil {
		return "", "", err
	}
	if acct == nil {
		return "", "", nil
	}
	return acct.ID, acct.Username, nil
}

// ResetPassword sets a new password without knowing the old one, for an
// operator with access to the host. It revokes every session for the
// account: a reset that left live sessions running would be no use for
// the case it exists to handle.
//
// The password is changed first and sessions dropped second, so a Redis
// failure leaves the account reachable with the new password rather than
// locked out with the old one still working. The caller is told which
// half succeeded.
func (s *Service) ResetPassword(ctx context.Context, accountID, next string) error {
	acct, err := s.accounts.AccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acct == nil {
		return ErrNoSession
	}
	if err := ValidatePassword(acct.Username, next); err != nil {
		return err
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	hash, herr := hashPassword(next)
	s.release()
	if herr != nil {
		return herr
	}
	if err := s.accounts.UpdatePasswordHash(ctx, accountID, hash); err != nil {
		return err
	}
	if err := s.sessions.DeleteAccountSessions(ctx, accountID, ""); err != nil {
		return fmt.Errorf("password changed, but sessions were not revoked: %w", err)
	}
	return nil
}

// RevokeSessions signs an account out everywhere without touching the
// password, for when a cookie leaks but the password is still good.
func (s *Service) RevokeSessions(ctx context.Context, accountID string) error {
	return s.sessions.DeleteAccountSessions(ctx, accountID, "")
}

// RevokeOtherSessions drops every session for an account except keep.
// Used when a new login or an authenticated join becomes the sole owner.
func (s *Service) RevokeOtherSessions(ctx context.Context, accountID, keep string) error {
	if accountID == "" {
		return nil
	}
	return s.sessions.DeleteAccountSessions(ctx, accountID, keep)
}

// GeneratePassword returns a strong temporary password from an alphabet
// with no visually ambiguous characters, so it survives being read aloud
// or copied by hand. It is regenerated on the vanishing chance that it
// trips ValidatePassword.
func GeneratePassword(username string) (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789" // no l, o, 0, 1
	for attempt := 0; attempt < 8; attempt++ {
		b := make([]byte, 20)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		out := make([]byte, 0, 23)
		for i, v := range b {
			if i > 0 && i%5 == 0 {
				out = append(out, '-')
			}
			out = append(out, alphabet[int(v)%len(alphabet)])
		}
		pw := string(out)
		if ValidatePassword(username, pw) == nil {
			return pw, nil
		}
	}
	return "", errors.New("auth: could not generate an acceptable password")
}

// SetRole changes an account's role. Host-only by design; there is no
// HTTP path to this.
func (s *Service) SetRole(ctx context.Context, accountID, role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("unknown role %q", role)
	}
	return s.accounts.SetRole(ctx, accountID, role)
}

// Ban shuts an account out until the given time and signs it out
// everywhere. Pass nil to lift it.
func (s *Service) Ban(ctx context.Context, accountID string, until *time.Time) error {
	if err := s.accounts.SetBannedUntil(ctx, accountID, until); err != nil {
		return err
	}
	if until == nil {
		return nil
	}
	return s.sessions.DeleteAccountSessions(ctx, accountID, "")
}

// Account reports an account by id, for operator tooling.
func (s *Service) Account(ctx context.Context, accountID string) (*Account, error) {
	return s.accounts.AccountByID(ctx, accountID)
}

// Logout drops one session.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.DeleteSession(ctx, token)
}

func (s *Service) issue(ctx context.Context, accountID string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if err := s.sessions.CreateSession(ctx, token, accountID, SessionTTL); err != nil {
		return "", fmt.Errorf("auth: cannot store session: %w", err)
	}
	return token, nil
}

func normalizeKey(raw string) string {
	_, key, err := ValidateUsername(raw)
	if err != nil {
		return ""
	}
	return key
}

// newToken returns 256 bits of base64url randomness.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail in practice; a panic here is louder
		// and safer than a predictable id.
		panic("auth: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// IsAuthError reports whether err is one a client caused and should see.
func IsAuthError(err error) bool {
	return errors.Is(err, ErrBadCredentials) ||
		errors.Is(err, ErrBanned) ||
		errors.Is(err, ErrUsernameTaken) ||
		errors.Is(err, ErrBadUsername) ||
		errors.Is(err, ErrWeakPassword) ||
		errors.Is(err, ErrNoSession)
}
