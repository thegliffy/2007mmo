package world

import (
	"strings"
	"testing"
)

// A peer-visible handle must never be the real player id, and must not be
// derived from it in any recoverable way.
func TestSnapshotUsesHandlesNotPlayerIDs(t *testing.T) {
	w := testWorld(t, newMem())
	a := w.UpsertPlayer(NewPlayerRec("11111111-1111-4111-8111-111111111111", "Ash"), true)
	b := w.UpsertPlayer(NewPlayerRec("22222222-2222-4222-8222-222222222222", "Briar"), true)
	w.RotateHandle(a.ID)
	w.RotateHandle(b.ID)

	snap := w.Snapshot(a.ID)
	if snap.You.ID == a.ID {
		t.Fatal("your own view still carries the real player id")
	}
	if snap.You.ID != w.HandleFor(a.ID) {
		t.Fatalf("you.id = %q, want the handle %q", snap.You.ID, w.HandleFor(a.ID))
	}
	if len(snap.Players) != 1 {
		t.Fatalf("expected one peer, got %d", len(snap.Players))
	}
	peer := snap.Players[0]
	if peer.ID == b.ID {
		t.Fatal("peer view carries the real player id")
	}
	if peer.ID != w.HandleFor(b.ID) {
		t.Fatalf("peer id = %q, want handle %q", peer.ID, w.HandleFor(b.ID))
	}
	if peer.Name != "Briar" {
		t.Fatalf("peer name = %q", peer.Name)
	}
	// The handle must not contain the id it stands for.
	if strings.Contains(peer.ID, b.ID) || strings.Contains(b.ID, peer.ID) {
		t.Fatal("handle and player id share material")
	}
}

// Rejoining must mint a new handle, so nobody can follow a player across
// sessions by remembering the old one.
func TestHandleRotatesOnRejoin(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("33333333-3333-4333-8333-333333333333", "Kyle"), true)

	first := w.RotateHandle(p.ID)
	second := w.RotateHandle(p.ID)
	if first == second {
		t.Fatal("handle did not change on rejoin")
	}
	if w.HandleFor(p.ID) != second {
		t.Fatal("HandleFor should report the newest handle")
	}
}

// Going offline forgets the handle rather than leaving it mapped forever.
func TestHandleDroppedOnLogout(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("44444444-4444-4444-8444-444444444444", "Kyle"), true)
	h := w.RotateHandle(p.ID)
	w.SetOnline(p.ID, false)
	if got := w.handles[p.ID]; got != "" {
		t.Fatalf("handle %q survived logout (was %q)", got, h)
	}
}

// Two players must never be handed the same handle.
func TestHandlesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		h := newHandle()
		if seen[h] {
			t.Fatalf("duplicate handle after %d draws", i)
		}
		seen[h] = true
	}
}
