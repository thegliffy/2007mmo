package world

import (
	"context"
	"testing"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

func firstHostile(w *World, name string) *NPC {
	for _, n := range w.NPCs {
		if n.Hostile && (name == "" || n.Name == name) {
			return n
		}
	}
	return nil
}

func TestMapBiggerThanPoCWithSouthPath(t *testing.T) {
	w := testWorld(t, newMem())
	if w.W < 28 || w.H < 32 {
		t.Fatalf("map should be larger than the 24x16 PoC, got %dx%d", w.W, w.H)
	}
	if !w.Walkable(spawnX, spawnY) {
		t.Fatal("stile spawn must stay walkable")
	}
	path := w.FindPath(spawnX, spawnY, 16, 26)
	if len(path) == 0 {
		t.Fatal("no path from the stile south into the clearing")
	}
	if w.Tiles[1][1] != 'T' {
		t.Fatalf("hamlet tree at 1,1 moved; woodland loop coords drifted")
	}
}

func TestHostilesLiveInTheSouth(t *testing.T) {
	w := testWorld(t, newMem())
	n := 0
	for _, npc := range w.NPCs {
		if !npc.Hostile {
			continue
		}
		n++
		if npc.Y < 18 {
			t.Fatalf("%s seeded in the hamlet at %d,%d", npc.Name, npc.X, npc.Y)
		}
		if !w.Walkable(npc.X, npc.Y) {
			t.Fatalf("%s on blocked tile %d,%d", npc.Name, npc.X, npc.Y)
		}
	}
	if n < 2 {
		t.Fatalf("want at least two hostiles in the south, got %d", n)
	}
}

func TestClickToAttackKillsThornkin(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	if npc == nil {
		t.Fatal("no thornkin")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	if p.Target != npc.ID {
		t.Fatalf("expected target %s, got %q", npc.ID, p.Target)
	}
	for i := 0; i < 12; i++ {
		w.Tick(context.Background())
		if !npc.Living() {
			break
		}
	}
	if npc.Living() {
		t.Fatalf("thornkin should fall, hp=%d", npc.HP)
	}
	if p.HP <= 0 || (p.X == spawnX && p.Y == spawnY && p.Target == "") {
		t.Fatalf("player should win vs thornkin, pos=%d,%d hp=%d", p.X, p.Y, p.HP)
	}
	if npc.RespawnIn <= 0 {
		t.Fatal("fallen thornkin should be on a respawn timer")
	}
	note := w.TakeNote("p1")
	if note == "" {
		t.Fatal("expected a kill note")
	}
	snap := w.Snapshot("p1")
	for _, n := range snap.NPCs {
		if n.ID == npc.ID {
			t.Fatal("dead thornkin should not appear in the snapshot")
		}
	}
}

func TestDefeatRespawnsAtStileKeepsPack(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	npc := firstHostile(w, "Brambleback")
	if npc == nil {
		t.Fatal("no brambleback")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemBerry, N: 2}}
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	died := false
	for i := 0; i < 20; i++ {
		w.Tick(context.Background())
		if p.X == spawnX && p.Y == spawnY && p.HP == p.MaxHP && p.Target == "" {
			died = true
			break
		}
	}
	if !died {
		t.Fatalf("expected soft defeat vs brambleback, pos=%d,%d hp=%d npc=%d", p.X, p.Y, p.HP, npc.HP)
	}
	if countItem(p.Inv, protocol.ItemBerry) != 2 {
		t.Fatalf("soft defeat must keep the pack: %v", p.Inv)
	}
	if p.Target != "" {
		t.Fatal("threat should be empty after respawn")
	}
	if npc.Target == p.ID {
		t.Fatal("brambleback should drop the fallen player")
	}
	note := w.TakeNote("p1")
	if note == "" {
		t.Fatal("expected a stile wake note")
	}
}

func TestFriendlyVillagerNotAttackable(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	w.SetAttack("p1", "npc-marta")
	if p.Target != "" {
		t.Fatal("Marta should not become a combat target")
	}
	if w.TakeNote("p1") == "" {
		t.Fatal("expected a refusal note")
	}
}

func TestOnePlayerOneNPCLock(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	a := w.UpsertPlayer(NewPlayerRec("a", "Ann"), true)
	b := w.UpsertPlayer(NewPlayerRec("b", "Bob"), true)
	a.X, a.Y = npc.X, npc.Y
	b.X, b.Y = npc.X, npc.Y
	w.SetAttack("a", npc.ID)
	w.Tick(context.Background())
	w.SetAttack("b", npc.ID)
	if b.Target == npc.ID {
		t.Fatal("second player should not steal a locked 1vNPC fight")
	}
}

func TestFoodHealsWithoutDuping(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Inv = []ItemStack{{ID: protocol.ItemTart, N: 1}}
	p.HP = 4
	msg, ok := w.UseItem(context.Background(), "p1", protocol.ItemTart)
	if !ok || countItem(p.Inv, protocol.ItemTart) != 0 {
		t.Fatalf("eat failed ok=%v inv=%v", ok, p.Inv)
	}
	if p.HP != 4+tartHeal {
		t.Fatalf("expected heal to %d, hp=%d", 4+tartHeal, p.HP)
	}
	if countItem(st.players["p1"].Inv, protocol.ItemTart) != 0 {
		t.Fatalf("store still has tart: %+v", st.players["p1"].Inv)
	}
	if msg == "" {
		t.Fatal("expected eat flavor")
	}
}

func TestCombatTickDoesNotMintItems(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	for i := 0; i < 8; i++ {
		w.Tick(context.Background())
	}
	if len(p.Inv) != 0 {
		t.Fatalf("combat must not mint pack items: %v", p.Inv)
	}
	if _, ok := st.players["p1"]; ok && len(st.players["p1"].Inv) != 0 {
		t.Fatalf("store gained items from combat: %+v", st.players["p1"].Inv)
	}
}

func TestSetAttackNoStandTileNotes(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	if npc == nil {
		t.Fatal("no thornkin")
	}
	// Park the beast on the NW wall corner: the tile and every cardinal
	// neighbor are blocked, so nearestAdjacent must fail.
	npc.X, npc.Y = 0, 0
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = spawnX, spawnY
	w.SetAttack("p1", npc.ID)
	if p.Target != "" {
		t.Fatalf("should not arm a fight with no stand tile, target=%q", p.Target)
	}
	note := w.TakeNote("p1")
	if note == "" {
		t.Fatal("failed SetAttack must surface a note, not silence")
	}
}

func TestInteractOnNPCStartsAttack(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	w.SetInteract("p1", npc.ID)
	if p.Target != npc.ID {
		t.Fatalf("interact on a hostile should arm a fight, target=%q", p.Target)
	}
}

func TestSnapshotReportsVitals(t *testing.T) {
	w := testWorld(t, newMem())
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.HP = 7
	npc := firstHostile(w, "Thornkin")
	p.Target = npc.ID
	you := w.Snapshot("p1").You
	if you.HP != 7 || you.MaxHP != playerMaxHP || you.Target != npc.ID {
		t.Fatalf("vitals missing from snapshot: %+v", you)
	}
	found := false
	for _, n := range w.Snapshot("p1").NPCs {
		if n.ID == npc.ID && n.Hostile && n.MaxHP == thornkinHP {
			found = true
		}
	}
	if !found {
		t.Fatal("hostile vitals missing from NPC snapshot")
	}
}
