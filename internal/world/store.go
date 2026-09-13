package world

import (
	"context"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// Store is the canonical persistence boundary. Item mutations must
// commit here before in-memory inventory changes (T3: no dupe on crash).
type Store interface {
	LoadPlayer(ctx context.Context, id string) (*PlayerRec, error)
	SavePlayer(ctx context.Context, p *PlayerRec) error
	CommitAction(ctx context.Context, p *PlayerRec, n *NodeRec) error
	LoadNodes(ctx context.Context) ([]NodeRec, error)
	UpsertNode(ctx context.Context, n NodeRec) error
}

type PlayerRec struct {
	ID     string
	Name   string
	X, Y   int
	Inv    []ItemStack
	Skills map[string]SkillState
	// HP is nil for rows written before health was persisted, and for a
	// freshly minted character. Nil means "start at full".
	HP *int
	// Coins live outside Inv on purpose: a purse costs no pack slot.
	Coins int
	// Looks is empty until the creator writes it. Relog must keep it.
	Looks protocol.Looks
}

type NodeRec struct {
	ID        string
	Kind      string
	X, Y      int
	Remaining int
	Cooldown  int
}

func recFromPlayer(p *Player) *PlayerRec {
	inv := append([]ItemStack(nil), p.Inv...)
	sk := make(map[string]SkillState, len(p.Skills))
	for k, v := range p.Skills {
		sk[k] = v
	}
	hp := p.HP
	return &PlayerRec{ID: p.ID, Name: p.Name, X: p.X, Y: p.Y,
		Inv: inv, Skills: sk, HP: &hp, Coins: p.Coins, Looks: p.Looks}
}

func recFromNode(n *Node) *NodeRec {
	return &NodeRec{ID: n.ID, Kind: n.Kind, X: n.X, Y: n.Y, Remaining: n.Remaining, Cooldown: n.Cooldown}
}
