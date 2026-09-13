package store

import (
	"context"
	"sync"

	"github.com/thegliffy/2007mmo/internal/world"
)

// Memory is an in-process Store used by unit tests and as a last-resort
// fallback. It still applies mutations only inside a lock so tests can
// assert commit-before-memory semantics.
type Memory struct {
	mu      sync.Mutex
	Players map[string]world.PlayerRec
	Nodes   map[string]world.NodeRec
	Fail    bool
}

func NewMemory() *Memory {
	return &Memory{
		Players: make(map[string]world.PlayerRec),
		Nodes:   make(map[string]world.NodeRec),
	}
}

func (m *Memory) LoadPlayer(_ context.Context, id string) (*world.PlayerRec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.Players[id]
	if !ok {
		return nil, nil
	}
	cp := copyRec(&p)
	return &cp, nil
}

func (m *Memory) SavePlayer(_ context.Context, p *world.PlayerRec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail {
		return errFailed
	}
	m.Players[p.ID] = copyRec(p)
	return nil
}

func (m *Memory) CommitAction(_ context.Context, p *world.PlayerRec, n *world.NodeRec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail {
		return errFailed
	}
	m.Players[p.ID] = copyRec(p)
	if n != nil {
		m.Nodes[n.ID] = *n
	}
	return nil
}

func (m *Memory) LoadNodes(_ context.Context) ([]world.NodeRec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]world.NodeRec, 0, len(m.Nodes))
	for _, n := range m.Nodes {
		out = append(out, n)
	}
	return out, nil
}

func (m *Memory) UpsertNode(_ context.Context, n world.NodeRec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail {
		return errFailed
	}
	m.Nodes[n.ID] = n
	return nil
}

// copyRec deep-copies everything a PlayerRec points at, so a stored row
// can never be mutated through the caller's copy.
func copyRec(p *world.PlayerRec) world.PlayerRec {
	cp := *p
	cp.Inv = append([]world.ItemStack(nil), p.Inv...)
	cp.Bank = append([]world.ItemStack(nil), p.Bank...)
	cp.Skills = copySkills(p.Skills)
	if p.HP != nil {
		hp := *p.HP
		cp.HP = &hp
	}
	if p.Equipped != nil {
		cp.Equipped = make(map[string]string, len(p.Equipped))
		for k, v := range p.Equipped {
			cp.Equipped[k] = v
		}
	}
	return cp
}

func copySkills(in map[string]world.SkillState) map[string]world.SkillState {
	if in == nil {
		return map[string]world.SkillState{}
	}
	out := make(map[string]world.SkillState, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type failErr struct{}

func (failErr) Error() string { return "store failed" }

var errFailed failErr
