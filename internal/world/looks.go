package world

import (
	"context"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// ApplyLooksFirst writes appearance if the player row has none. A missing
// row is created from rec. Spam-clicks return the looks that already
// stuck — one character, one face, no second row.
func ApplyLooksFirst(ctx context.Context, st Store, rec *PlayerRec, looks protocol.Looks) (protocol.Looks, bool, error) {
	if st == nil || rec == nil {
		return protocol.Looks{}, false, nil
	}
	existing, err := st.LoadPlayer(ctx, rec.ID)
	if err != nil {
		return protocol.Looks{}, false, err
	}
	if existing != nil && existing.Looks.Set() {
		return existing.Looks, false, nil
	}
	if existing == nil {
		existing = rec
	}
	existing.Looks = looks
	if existing.Name == "" {
		existing.Name = rec.Name
	}
	if err := st.SavePlayer(ctx, existing); err != nil {
		return protocol.Looks{}, false, err
	}
	return looks, true, nil
}

// SetLooks paints a live figure. Called on the world goroutine after the
// store has accepted the creator pass, so the next snapshot already
// matches what Postgres will hand back on relog.
func (w *World) SetLooks(id string, looks protocol.Looks) {
	if p := w.Players[id]; p != nil {
		p.Looks = looks
	}
}
