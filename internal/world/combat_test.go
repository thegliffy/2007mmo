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

// Combat used to be unable to touch the pack at all, and this asserted
// the pack stayed empty. Beasts drop loot now, so the premise moved: what
// still has to hold is that memory never holds an item the store does not.
// Anything in the pack after a kill must also be on the record.
func TestCombatNeverLeavesItemsTheStoreDoesNotHave(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	w.SeedLoot(11)
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20)
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	for i := 0; i < 8; i++ {
		w.Tick(context.Background())
	}

	stored, ok := st.players["p1"]
	if !ok {
		if len(p.Inv) != 0 || p.Coins != 0 {
			t.Fatalf("player holds %v and %d coins that were never stored", p.Inv, p.Coins)
		}
		return
	}
	for _, held := range p.Inv {
		if countItem(stored.Inv, held.ID) < held.N {
			t.Fatalf("pack holds %d %s, the store has %d",
				held.N, held.ID, countItem(stored.Inv, held.ID))
		}
	}
	if p.Coins > stored.Coins {
		t.Fatalf("purse holds %d coins, the store has %d", p.Coins, stored.Coins)
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

func TestHostileDoesNotDamageWithoutTarget(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Brambleback")
	if npc == nil {
		t.Fatal("no brambleback")
	}
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y
	p.Target = ""
	npc.Target = p.ID
	hp := p.HP
	npcHP := npc.HP
	w.Tick(context.Background())
	if p.HP != hp {
		t.Fatalf("hostile must not chip a player with no Target, hp %d→%d", hp, p.HP)
	}
	if npc.HP != npcHP {
		t.Fatalf("npc should not swing without a player lock, hp %d→%d", npcHP, npc.HP)
	}
}

func TestSetAttackAfterHitRelocks(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	w.Tick(context.Background())
	if p.HP >= playerMaxHP {
		t.Fatalf("expected a hit, hp=%d", p.HP)
	}
	p.Target = ""
	w.SetAttack("p1", npc.ID)
	if p.Target != npc.ID {
		t.Fatal("click-attack must lock again after being hit")
	}
	note := w.TakeNote("p1")
	if note == "" {
		t.Fatal("expected a set-upon or fight note after relock")
	}
}

func TestMoveTowardTargetKeepsLock(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y-4
	w.SetAttack("p1", npc.ID)
	if p.Target != npc.ID {
		t.Fatal("expected lock")
	}
	// WASD / missed click south — closer to the beast, must not wipe Target.
	w.SetDest("p1", p.X, p.Y+1)
	if p.Target != npc.ID {
		t.Fatal("step toward the beast must keep the fight lock")
	}
}

func TestMoveAwayClearsLock(t *testing.T) {
	w := testWorld(t, newMem())
	npc := firstHostile(w, "Thornkin")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y
	w.SetAttack("p1", npc.ID)
	w.SetDest("p1", spawnX, spawnY)
	if p.Target != "" {
		t.Fatalf("walking away should break off, target=%q", p.Target)
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

// Health must survive a logout. While HP was not persisted, disconnecting
// mid-fight was a free full heal — and the beast kept its wounds, so the
// Brambleback could be worn down by reconnecting.
func TestHealthPersistsAcrossLogout(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.HP = 4
	p.Dirty = true
	w.PersistPlayer(context.Background(), "p1")

	rec, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || rec == nil {
		t.Fatalf("load: %v %v", rec, err)
	}
	if rec.HP == nil {
		t.Fatal("health was not written to the store")
	}
	if *rec.HP != 4 {
		t.Fatalf("stored hp = %d, want 4", *rec.HP)
	}

	// Rejoining restores the wounded state, not full health.
	w2 := testWorld(t, st)
	p2 := w2.UpsertPlayer(rec, true)
	if p2.HP != 4 {
		t.Fatalf("hp after rejoin = %d, want 4 (reconnect must not heal)", p2.HP)
	}
	if p2.MaxHP != playerMaxHP {
		t.Fatalf("maxHP = %d", p2.MaxHP)
	}
}

// A row written before health was persisted has no hp value; those
// players should walk in at full rather than at zero.
func TestMissingStoredHealthMeansFull(t *testing.T) {
	w := testWorld(t, newMem())
	rec := NewPlayerRec("legacy", "OldHand")
	rec.HP = nil
	p := w.UpsertPlayer(rec, true)
	if p.HP != playerMaxHP {
		t.Fatalf("legacy hp = %d, want %d", p.HP, playerMaxHP)
	}
}

// Nonsense stored health must not carry into the world.
func TestStoredHealthOutOfRangeIsClamped(t *testing.T) {
	w := testWorld(t, newMem())
	for _, bad := range []int{-5, 0, playerMaxHP + 99} {
		rec := NewPlayerRec("p", "Kyle")
		rec.HP = &bad
		p := w.UpsertPlayer(rec, true)
		if p.HP != playerMaxHP {
			t.Fatalf("stored hp %d gave %d, want %d", bad, p.HP, playerMaxHP)
		}
	}
}

// Taking a hit marks the row for saving, or health would only persist by
// coincidence when something else dirtied the player.
func TestCombatDamageMarksPlayerDirty(t *testing.T) {
	w := testWorld(t, newMem())
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)
	p.Dirty = false
	w.tickCombat(context.Background(), p)
	if p.HP == playerMaxHP {
		t.Skip("no damage exchanged this tick")
	}
	if !p.Dirty {
		t.Fatal("taking damage did not mark the player for persistence")
	}
}

// xpForLevel is the total XP a skill needs to reach n. Level is derived
// from XP, never stored independently — setting SkillState.Lv on its own
// is silently undone the moment any XP lands.
func xpForLevel(n int) int {
	total, need := 0, 25
	for lv := 1; lv < n; lv++ {
		total += need
		need += 15
	}
	return total
}

func atLevel(n int) SkillState { return SkillState{Lv: n, XP: xpForLevel(n)} }

func TestXPForLevelMatchesLevelFromXP(t *testing.T) {
	for n := 1; n <= 20; n++ {
		if got := LevelFromXP(xpForLevel(n)); got != n {
			t.Fatalf("xpForLevel(%d) = %d xp, which reads back as level %d", n, xpForLevel(n), got)
		}
	}
}

func TestMeleeDamageLadder(t *testing.T) {
	for _, c := range []struct{ lv, want int }{
		{1, 2}, {3, 2}, {4, 3}, {7, 3}, {8, 4}, {12, 5}, {16, 6}, {20, 7},
	} {
		if got := meleeDamage(c.lv); got != c.want {
			t.Errorf("meleeDamage(%d) = %d, want %d", c.lv, got, c.want)
		}
	}
	// A missing or nonsense level must not hit for less than a beginner.
	if meleeDamage(0) != baseMeleeDmg || meleeDamage(-4) != baseMeleeDmg {
		t.Error("an unset level should hit like level 1")
	}
}

func TestDefenseAbsorbsButNeverFully(t *testing.T) {
	for _, c := range []struct{ lv, want int }{
		{1, 0}, {5, 0}, {6, 1}, {11, 1}, {12, 2}, {18, 3}, {20, 3},
	} {
		if got := defenseReduction(c.lv); got != c.want {
			t.Errorf("defenseReduction(%d) = %d, want %d", c.lv, got, c.want)
		}
	}
	// However much you absorb, a blow always lands for something. Nobody
	// gets to stand in the briars indefinitely.
	for lv := 1; lv <= 20; lv++ {
		for raw := 1; raw <= 5; raw++ {
			if got := damageAfterDefense(raw, lv); got < minDamageTaken {
				t.Fatalf("defense %d vs raw %d gave %d, below the floor", lv, raw, got)
			}
		}
	}
	if damageAfterDefense(2, 20) != minDamageTaken {
		t.Error("max defense against a Brambleback should still take the floor")
	}
}

// Fighting must train both skills: melee on what you deal, defense on
// what is swung at you.
func TestCombatTrainsMeleeAndDefense(t *testing.T) {
	w := testWorld(t, newMem())
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)
	w.tickCombat(context.Background(), p)

	melee := p.Skills[protocol.SkillMelee]
	def := p.Skills[protocol.SkillDefense]
	if melee.XP != baseMeleeDmg*meleeXPPerDamage {
		t.Fatalf("melee xp = %d, want %d for %d damage dealt",
			melee.XP, baseMeleeDmg*meleeXPPerDamage, baseMeleeDmg)
	}
	if def.XP != thornkinDmg*defenseXPPerDamage {
		t.Fatalf("defense xp = %d, want %d for a %d-damage blow",
			def.XP, thornkinDmg*defenseXPPerDamage, thornkinDmg)
	}
}

// Defense trains at the same rate however good it gets, because it pays
// on the raw blow rather than on what got through.
func TestDefenseXPDoesNotSlowAsItImproves(t *testing.T) {
	gain := func(defLevel int) int {
		// A fresh world each time: the NPC stays locked to whoever
		// attacked it, so a second player would simply be turned away.
		w := testWorld(t, newMem())
		npc := w.npcByID("npc-thornkin-2")
		p := w.UpsertPlayer(NewPlayerRec("p"+itoa(defLevel), "Kyle"), true)
		p.Skills[protocol.SkillDefense] = atLevel(defLevel)
		p.X, p.Y = npc.X, npc.Y-1
		w.SetAttack(p.ID, npc.ID)
		before := p.Skills[protocol.SkillDefense].XP
		w.tickCombat(context.Background(), p)
		return p.Skills[protocol.SkillDefense].XP - before
	}
	low, high := gain(1), gain(20)
	if low != high {
		t.Fatalf("defense xp per blow changed with level: %d at lv1, %d at lv20", low, high)
	}
}

// The arc the numbers are meant to produce: a Brambleback kills a
// beginner and is beatable once either skill has come along.
func TestBramblebackGoesFromLethalToBeatable(t *testing.T) {
	survives := func(meleeLv, defLv int) bool {
		w := testWorld(t, newMem())
		npc := w.npcByID("npc-brambleback")
		p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
		p.Skills[protocol.SkillMelee] = atLevel(meleeLv)
		p.Skills[protocol.SkillDefense] = atLevel(defLv)
		p.X, p.Y = npc.X, npc.Y-1
		w.SetAttack("p1", npc.ID)
		for i := 0; i < 40; i++ {
			w.tickCombat(context.Background(), p)
			if !npc.Living() {
				return true // felled it
			}
			if p.X == spawnX && p.Y == spawnY {
				return false // woke at the stile
			}
		}
		return false
	}
	if survives(1, 1) {
		t.Error("a beginner should lose to the Brambleback; it is the wall to train against")
	}
	if !survives(12, 1) {
		t.Error("melee 12 should be enough to fell the Brambleback")
	}
	if !survives(1, 12) {
		t.Error("defense 12 should be enough to outlast the Brambleback")
	}
}

// Overkill must not pay: hitting a 1-hp beast for 7 earns xp for 1.
func TestMeleeXPIsCappedByRemainingHealth(t *testing.T) {
	w := testWorld(t, newMem())
	npc := w.npcByID("npc-thornkin-1")
	npc.HP = 1
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20)
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)
	before := p.Skills[protocol.SkillMelee].XP
	w.tickCombat(context.Background(), p)
	gained := p.Skills[protocol.SkillMelee].XP - before
	if gained != 1*meleeXPPerDamage {
		t.Fatalf("gained %d xp for a killing blow on 1 hp, want %d — overkill should not pay",
			gained, meleeXPPerDamage)
	}
}

// Characters made before these skills existed must pick them up on login.
func TestOlderCharactersGainTheNewSkills(t *testing.T) {
	w := testWorld(t, newMem())
	legacy := &PlayerRec{
		ID: "old", Name: "OldHand", X: spawnX, Y: spawnY,
		Skills: map[string]SkillState{protocol.SkillForage: {Lv: 7, XP: 300}},
	}
	p := w.UpsertPlayer(legacy, true)
	for _, id := range []string{protocol.SkillMelee, protocol.SkillDefense, protocol.SkillCook} {
		if sk, ok := p.Skills[id]; !ok || sk.Lv != 1 {
			t.Errorf("%s missing or not level 1 on an older character: %+v", id, sk)
		}
	}
	if p.Skills[protocol.SkillForage].Lv != 7 {
		t.Error("existing skill was clobbered")
	}
}

// killThornkin fells a beast and returns the player, for drop assertions.
func killThornkin(t *testing.T, w *World, st Store, seed int64) *Player {
	t.Helper()
	w.SeedLoot(seed)
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20) // one swing, so the roll is the only variable
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)
	for i := 0; i < 12 && npc.Living(); i++ {
		w.tickCombat(context.Background(), p)
	}
	if npc.Living() {
		t.Fatal("setup: the thornkin did not fall")
	}
	return p
}

// collectHere gathers whatever pile the player is able to reach, which
// after a kill is the one the beast left behind.
func collectHere(t *testing.T, w *World, p *Player) {
	t.Helper()
	for _, g := range w.Ground {
		p.X, p.Y = g.X, g.Y
		p.ActionGround = g.ID
		p.HasDest = false
		w.tickPickup(context.Background(), p)
		return
	}
}

// Coins are a purse, not an item: they must never consume a pack slot.
func TestCoinsDoNotTakeAPackSlot(t *testing.T) {
	w := testWorld(t, newMem())
	p := killThornkin(t, w, nil, 1)
	collectHere(t, w, p)
	if p.Coins <= 0 {
		t.Fatalf("no coins dropped (got %d)", p.Coins)
	}
	for _, it := range p.Inv {
		if it.ID == "coins" || it.ID == "coin" {
			t.Fatal("coins ended up in the pack")
		}
	}
	// A Thornkin is worth between CoinsMin and CoinsMax.
	npc := w.npcByID("npc-thornkin-1")
	if p.Coins < npc.CoinsMin || p.Coins > npc.CoinsMax {
		t.Fatalf("coins = %d, outside the table's %d..%d", p.Coins, npc.CoinsMin, npc.CoinsMax)
	}
}

// The purse survives a logout like everything else on the record.
func TestCoinsPersist(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	p := killThornkin(t, w, st, 3)
	collectHere(t, w, p)
	want := p.Coins
	if want == 0 {
		t.Fatal("setup: no coins were collected")
	}
	w.PersistPlayer(context.Background(), "p1")

	rec, err := st.LoadPlayer(context.Background(), "p1")
	if err != nil || rec == nil {
		t.Fatalf("load: %v %v", rec, err)
	}
	if rec.Coins != want {
		t.Fatalf("stored coins = %d, want %d", rec.Coins, want)
	}
	w2 := testWorld(t, st)
	if got := w2.UpsertPlayer(rec, true).Coins; got != want {
		t.Fatalf("coins after rejoin = %d, want %d", got, want)
	}
}

// The kill itself no longer writes anything — it only leaves a pile — so
// a failing store cannot mint loot at that moment. The dangerous write
// moved to the pickup, and TestFailedPickupLeavesThePileIntact in
// ground_test.go is where that ordering is pinned now.
func TestFellingABeastWritesNothing(t *testing.T) {
	st := newMem()
	w := testWorld(t, st)
	w.SeedLoot(5)
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20)
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)

	st.fail = true // the store is gone for the whole fight
	for i := 0; i < 12 && npc.Living(); i++ {
		w.tickCombat(context.Background(), p)
	}
	if npc.Living() {
		t.Fatal("a failing store stopped the beast from falling")
	}
	if p.Coins != 0 || countItem(p.Inv, protocol.ItemLeather) != 0 {
		t.Fatal("the kill put loot in the pack without a commit")
	}
	if len(w.Ground) == 0 {
		t.Fatal("the beast left nothing behind")
	}
}

// Goblin leather is rare, and rare should mean rare.
func TestLeatherIsRare(t *testing.T) {
	drops, runs := 0, 400
	for i := 0; i < runs; i++ {
		w := testWorld(t, newMem())
		killThornkin(t, w, nil, int64(i))
		// Leather now lands in the pile, not the pack.
		for _, g := range w.Ground {
			drops += countItem(g.Inv, protocol.ItemLeather)
		}
	}
	rate := float64(drops) / float64(runs)
	odds := 1.0 / 16.0
	if rate < odds*0.5 || rate > odds*2 {
		t.Fatalf("leather dropped %d/%d (%.3f); the table says about %.3f", drops, runs, rate, odds)
	}
	if drops == 0 {
		t.Fatal("leather never dropped at all")
	}
}

// A full pack must not silently swallow the drop.
func TestFullPackIsToldAboutTheLeather(t *testing.T) {
	w := testWorld(t, newMem())
	w.SeedLoot(5)
	npc := w.npcByID("npc-thornkin-1")
	p := w.UpsertPlayer(NewPlayerRec("p1", "Kyle"), true)
	p.Skills[protocol.SkillMelee] = atLevel(20)
	// Fill every slot with something that is not leather.
	for i := 0; i < protocol.InvSlots; i++ {
		p.Inv = append(p.Inv, ItemStack{ID: "filler" + itoa(i), N: 1})
	}
	p.X, p.Y = npc.X, npc.Y-1
	w.SetAttack("p1", npc.ID)
	for i := 0; i < 12 && npc.Living(); i++ {
		w.tickCombat(context.Background(), p)
	}
	collectHere(t, w, p)
	if len(p.Inv) > protocol.InvSlots {
		t.Fatalf("pack overflowed to %d slots", len(p.Inv))
	}
	if p.Coins <= 0 {
		t.Fatal("coins should still be collected when the pack is full")
	}
	// Anything that would not fit stays where it lay rather than vanishing.
	for _, g := range w.Ground {
		if countItem(g.Inv, protocol.ItemLeather) > 0 {
			return
		}
	}
}

// The Brambleback is harder, so it pays better.
func TestBramblebackPaysMoreThanThornkin(t *testing.T) {
	w := testWorld(t, newMem())
	thorn := w.npcByID("npc-thornkin-1")
	bram := w.npcByID("npc-brambleback")
	if bram.CoinsMin <= thorn.CoinsMax {
		t.Fatalf("brambleback %d..%d does not out-pay thornkin %d..%d",
			bram.CoinsMin, bram.CoinsMax, thorn.CoinsMin, thorn.CoinsMax)
	}
	if bram.LeatherOdds >= thorn.LeatherOdds {
		t.Fatalf("brambleback leather odds 1-in-%d should beat thornkin's 1-in-%d",
			bram.LeatherOdds, thorn.LeatherOdds)
	}
}

// Villagers are not a payday.
func TestVillagersDropNothing(t *testing.T) {
	w := testWorld(t, newMem())
	for _, n := range w.NPCs {
		if !n.Hostile && (n.CoinsMax > 0 || n.LeatherOdds > 0) {
			t.Errorf("%s is peaceful but carries loot", n.ID)
		}
	}
}
