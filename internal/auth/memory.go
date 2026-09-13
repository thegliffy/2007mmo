package auth

import (
	"context"
	"sync"
	"time"
)

// Memory is an in-process Accounts + Sessions implementation for tests
// and for running the world without Postgres/Redis. It is deliberately
// not a fallback the server picks on its own: losing real accounts to a
// silent in-memory store would be worse than refusing to start.
type Memory struct {
	mu       sync.Mutex
	accounts map[string]Account // by id
	byKey    map[string]string  // username_key -> id
	players  map[string]string  // accountID -> playerID
	names    map[string]string  // playerID -> name
	sessions map[string]memSession
	// FailCreate forces CreateAccount to error, for testing the signup
	// path when the store is unavailable.
	FailCreate error
}

type memSession struct {
	accountID string
	expires   time.Time
}

func NewMemory() *Memory {
	return &Memory{
		accounts: map[string]Account{},
		byKey:    map[string]string{},
		players:  map[string]string{},
		names:    map[string]string{},
		sessions: map[string]memSession{},
	}
}

func (m *Memory) CreateAccount(_ context.Context, a Account, playerID, playerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailCreate != nil {
		return m.FailCreate
	}
	if _, taken := m.byKey[a.UsernameKey]; taken {
		return ErrUsernameTaken
	}
	m.accounts[a.ID] = a
	m.byKey[a.UsernameKey] = a.ID
	m.players[a.ID] = playerID
	m.names[playerID] = playerName
	return nil
}

func (m *Memory) AccountByUsernameKey(_ context.Context, key string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byKey[key]
	if !ok {
		return nil, nil
	}
	a := m.accounts[id]
	return &a, nil
}

func (m *Memory) AccountByID(_ context.Context, id string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	if !ok {
		return nil, nil
	}
	return &a, nil
}

func (m *Memory) UpdatePasswordHash(_ context.Context, accountID, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[accountID]
	if !ok {
		return ErrNoSession
	}
	a.PWHash = hash
	m.accounts[accountID] = a
	return nil
}

func (m *Memory) PlayerIDForAccount(_ context.Context, accountID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.players[accountID], nil
}

func (m *Memory) TouchLogin(_ context.Context, _ string) error { return nil }

// PlayerName reports the name recorded at signup. Test helper.
func (m *Memory) PlayerName(playerID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.names[playerID]
}

func (m *Memory) CreateSession(_ context.Context, token, accountID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = memSession{accountID: accountID, expires: time.Now().Add(ttl)}
	return nil
}

func (m *Memory) SessionAccount(_ context.Context, token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return "", nil
	}
	if time.Now().After(s.expires) {
		delete(m.sessions, token)
		return "", nil
	}
	return s.accountID, nil
}

func (m *Memory) TouchSession(_ context.Context, token string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[token]; ok {
		s.expires = time.Now().Add(ttl)
		m.sessions[token] = s
	}
	return nil
}

func (m *Memory) DeleteSession(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *Memory) DeleteAccountSessions(_ context.Context, accountID, keep string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, s := range m.sessions {
		if s.accountID == accountID && token != keep {
			delete(m.sessions, token)
		}
	}
	return nil
}

// SessionCount reports live sessions for an account. Test helper.
func (m *Memory) SessionCount(accountID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sessions {
		if s.accountID == accountID {
			n++
		}
	}
	return n
}
