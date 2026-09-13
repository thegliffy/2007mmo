// Package auth owns Hollowmere account credentials and login sessions.
//
// Passwords are stored as self-describing scrypt strings so the cost
// parameters can be raised later and old hashes upgraded on next login
// without a migration:
//
//	scrypt$<N>$<r>$<p>$<salt-b64>$<key-b64>
//
// scrypt (not argon2id) because argon2 pulls golang.org/x/sys through
// blake2b; scrypt needs only pbkdf2, which keeps the module graph as it
// was. It is still memory-hard, which is the property that matters.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/crypto/scrypt"
)

const (
	// scryptN at r=8 costs 128*N*r bytes ≈ 16 MiB per hash. Every hash and
	// verify goes through Service.sem so a login flood cannot turn that
	// into unbounded server memory.
	scryptN   = 16384
	scryptR   = 8
	scryptP   = 1
	scryptKey = 32
	saltLen   = 16

	// MinPasswordLen follows NIST SP 800-63B: length, not composition rules.
	MinPasswordLen = 10
	MaxPasswordLen = 128

	MinUsernameLen = 3
	MaxUsernameLen = 16
)

var (
	ErrBadCredentials = errors.New("that name and password do not match")
	ErrUsernameTaken  = errors.New("that name is already spoken for")
	ErrBadUsername    = errors.New("bad username")
	ErrWeakPassword   = errors.New("weak password")
	ErrNoSession      = errors.New("no session")
)

// dummyHash is verified against when a username does not exist, so a
// login probe costs the same time whether or not the account is real.
// Built on first use rather than in init() so no binary pays for it at
// startup.
var (
	dummyOnce sync.Once
	dummyHash string
)

func timingEqualizerHash() string {
	dummyOnce.Do(func() {
		h, err := hashPassword("hollowmere-timing-equalizer")
		if err != nil {
			// Only reachable if crypto/rand fails; an empty hash still
			// costs a decode attempt and returns ErrBadCredentials.
			return
		}
		dummyHash = h
	})
	return dummyHash
}

func hashPassword(pw string) (string, error) {
	return hashWithParams(pw, scryptN, scryptR, scryptP)
}

// hashWithParams exists so tests can produce a deliberately weak hash and
// prove the stale-parameter upgrade path works.
func hashWithParams(pw string, n, r, p int) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := scrypt.Key([]byte(pw), salt, n, r, p, scryptKey)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("scrypt$%d$%d$%d$%s$%s",
		n, r, p, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// verifyPassword reports whether pw matches the stored hash, and whether
// the stored hash used weaker parameters than we now want.
func verifyPassword(stored, pw string) (ok bool, stale bool, err error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false, false, errors.New("auth: unrecognized password hash")
	}
	n, err1 := strconv.Atoi(parts[1])
	r, err2 := strconv.Atoi(parts[2])
	p, err3 := strconv.Atoi(parts[3])
	if err1 != nil || err2 != nil || err3 != nil || n < 2 || r < 1 || p < 1 {
		return false, false, errors.New("auth: bad password hash parameters")
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, false, errors.New("auth: bad password hash salt")
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, false, errors.New("auth: bad password hash key")
	}
	got, err := scrypt.Key([]byte(pw), salt, n, r, p, len(want))
	if err != nil {
		return false, false, err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	return true, n < scryptN || r < scryptR || p < scryptP, nil
}

// ValidateUsername normalizes a login name. It returns the display form
// and the unique lookup key.
//
// Deliberately stricter than world.SanitizeName: ASCII letters, digits,
// '_' and '-' only, no spaces. Spaces and non-ASCII letters invite
// lookalike names, and a login handle is the one string where two
// accounts must never be confusable.
func ValidateUsername(raw string) (display, key string, err error) {
	display = strings.TrimSpace(raw)
	if len(display) < MinUsernameLen || len(display) > MaxUsernameLen {
		return "", "", fmt.Errorf("%w: a name is %d to %d characters", ErrBadUsername, MinUsernameLen, MaxUsernameLen)
	}
	for i, r := range display {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-':
			if i == 0 {
				return "", "", fmt.Errorf("%w: start with a letter or digit", ErrBadUsername)
			}
		default:
			return "", "", fmt.Errorf("%w: letters, digits, _ and - only", ErrBadUsername)
		}
	}
	return display, strings.ToLower(display), nil
}

// ValidatePassword enforces length and rejects the handful of passwords
// that are guessed first. No composition rules.
func ValidatePassword(username, pw string) error {
	if len(pw) < MinPasswordLen {
		return fmt.Errorf("%w: at least %d characters", ErrWeakPassword, MinPasswordLen)
	}
	if len(pw) > MaxPasswordLen {
		return fmt.Errorf("%w: at most %d characters", ErrWeakPassword, MaxPasswordLen)
	}
	if strings.TrimSpace(pw) == "" {
		return fmt.Errorf("%w: not only whitespace", ErrWeakPassword)
	}
	low := strings.ToLower(pw)
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" && strings.Contains(low, u) {
		return fmt.Errorf("%w: do not put your name in it", ErrWeakPassword)
	}
	for _, bad := range commonPasswords {
		if low == bad {
			return fmt.Errorf("%w: that one is guessed first", ErrWeakPassword)
		}
	}
	if isSingleRuneRepeat(pw) {
		return fmt.Errorf("%w: one repeated character is not a password", ErrWeakPassword)
	}
	return nil
}

func isSingleRuneRepeat(s string) bool {
	var first rune = -1
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if first == -1 {
			first = r
			continue
		}
		if r != first {
			return false
		}
	}
	return first != -1
}

// commonPasswords is a short stop-list, not a substitute for a breach
// corpus. It catches the passwords a bored visitor tries by hand.
var commonPasswords = []string{
	"password", "password1", "password12", "password123", "password1234",
	"1234567890", "12345678901", "123456789012", "qwertyuiop", "qwerty1234",
	"letmein123", "iloveyou123", "hollowmere", "hollowmere1", "hollowmere123",
	"brambleberry", "administrator", "changeme123", "welcome123", "passw0rd123",
}
