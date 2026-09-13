package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func testLooks() protocol.Looks {
	return protocol.Looks{
		Skin: protocol.SkinOlive, Hair: protocol.HairTied,
		HairColor: protocol.HairRusset, Top: protocol.TopInk,
	}
}

func TestLooksSurviveAStoreRoundTrip(t *testing.T) {
	st := newMem()
	rec := NewPlayerRec("p1", "Kyle")
	rec.Looks = testLooks()
	if err := st.SavePlayer(context.Background(), rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.Looks != testLooks() {
		t.Fatalf("looks = %+v, want %+v", got.Looks, testLooks())
	}

	// A later pack write must not wipe the face.
	got.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 2}}
	if err := st.SavePlayer(context.Background(), got); err != nil {
		t.Fatalf("save pack: %v", err)
	}
	again, _ := st.LoadPlayer(context.Background(), "p1")
	if again.Looks != testLooks() {
		t.Fatalf("pack write wiped looks: %+v", again.Looks)
	}
}

func TestApplyLooksFirstIsIdempotent(t *testing.T) {
	st := newMem()
	rec := NewPlayerRec("p1", "Kyle")
	first := testLooks()
	got, wrote, err := ApplyLooksFirst(context.Background(), st, rec, first)
	if err != nil || !wrote || got != first {
		t.Fatalf("first write: looks=%+v wrote=%v err=%v", got, wrote, err)
	}

	other := protocol.Looks{
		Skin: protocol.SkinFair, Hair: protocol.HairLong,
		HairColor: protocol.HairSnow, Top: protocol.TopBerry,
	}
	again, wrote, err := ApplyLooksFirst(context.Background(), st, rec, other)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if wrote {
		t.Fatal("a second create must not overwrite the first face")
	}
	if again != first {
		t.Fatalf("stuck looks = %+v, want the first pass %+v", again, first)
	}

	// Still one row.
	if len(st.players) != 1 {
		t.Fatalf("player rows = %d, want 1", len(st.players))
	}
}

func TestSnapshotShowsPeerLooks(t *testing.T) {
	w := testWorld(t, newMem())
	ash := NewPlayerRec("a", "Ash")
	ash.Looks = protocol.Looks{
		Skin: protocol.SkinFair, Hair: protocol.HairShort,
		HairColor: protocol.HairStraw, Top: protocol.TopMoss,
	}
	briar := NewPlayerRec("b", "Briar")
	briar.Looks = testLooks()
	w.UpsertPlayer(ash, true)
	w.UpsertPlayer(briar, true)
	w.RotateHandle(ash.ID)
	w.RotateHandle(briar.ID)

	snap := w.Snapshot(ash.ID)
	if snap.You.Looks == nil || *snap.You.Looks != ash.Looks {
		t.Fatalf("your looks = %+v, want %+v", snap.You.Looks, ash.Looks)
	}
	if len(snap.Players) != 1 {
		t.Fatalf("peers = %d, want 1", len(snap.Players))
	}
	peer := snap.Players[0]
	if peer.Name != "Briar" {
		t.Fatalf("peer name = %q", peer.Name)
	}
	if peer.Looks == nil || *peer.Looks != briar.Looks {
		t.Fatalf("peer looks = %+v, want %+v", peer.Looks, briar.Looks)
	}
}

func TestSnapshotOmitsUnsetLooks(t *testing.T) {
	w := testWorld(t, newMem())
	w.UpsertPlayer(NewPlayerRec("a", "Ash"), true)
	w.UpsertPlayer(NewPlayerRec("b", "Briar"), true)
	snap := w.Snapshot("a")
	if snap.You.Looks != nil {
		t.Fatalf("unset self looks leaked: %+v", snap.You.Looks)
	}
	if len(snap.Players) != 1 || snap.Players[0].Looks != nil {
		t.Fatalf("unset peer looks leaked: %+v", snap.Players)
	}
}
