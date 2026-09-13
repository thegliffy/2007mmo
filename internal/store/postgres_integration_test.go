package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/protocol"
	"github.com/thegliffy/2007mmo/internal/world"
)

// These tests exercise the real SQL: the migration, the account/player
// transaction, and the unique constraints. They are skipped unless a
// throwaway database is provided:
//
//	HOLLOWMERE_TEST_DATABASE_URL=postgres://user:pw@127.0.0.1:5432/hollowmere_test?sslmode=disable go test ./internal/store/...
//
// Never point this at live data: the tests truncate accounts and players.
func testPG(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("HOLLOWMERE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set HOLLOWMERE_TEST_DATABASE_URL to run the Postgres integration tests")
	}
	pg, err := NewPostgres(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pg.Close)
	// account_id is ON DELETE CASCADE, so clearing accounts clears the
	// player rows that belong to them.
	if _, err := pg.pool.Exec(context.Background(), `DELETE FROM accounts; DELETE FROM players;`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	return pg
}

func TestPGMigrationIsIdempotent(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	// NewPostgres already migrated once; a second pass must be a no-op.
	for i := 0; i < 2; i++ {
		if err := pg.migrate(ctx); err != nil {
			t.Fatalf("migrate pass %d: %v", i+2, err)
		}
	}
	var n int
	if err := pg.pool.QueryRow(ctx, `
SELECT count(*) FROM pg_constraint WHERE conname='players_account_id_fkey'`).Scan(&n); err != nil {
		t.Fatalf("constraint check: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly one account_id foreign key, found %d", n)
	}
}

func TestPGCreateAccountWritesBothRows(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()

	acct := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "scrypt$16384$8$1$aaaa$bbbb"}
	if err := pg.CreateAccount(ctx, acct, "player-1", "Kyle"); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := pg.AccountByUsernameKey(ctx, "kyle")
	if err != nil || got == nil {
		t.Fatalf("lookup by key: %v %v", got, err)
	}
	if got.ID != "acct-1" || got.Username != "Kyle" {
		t.Fatalf("account = %+v", got)
	}

	byID, err := pg.AccountByID(ctx, "acct-1")
	if err != nil || byID == nil || byID.UsernameKey != "kyle" {
		t.Fatalf("lookup by id: %+v %v", byID, err)
	}

	pid, err := pg.PlayerIDForAccount(ctx, "acct-1")
	if err != nil || pid != "player-1" {
		t.Fatalf("player id = %q err=%v", pid, err)
	}

	// The player row must be a real, loadable player.
	rec, err := pg.LoadPlayer(ctx, "player-1")
	if err != nil || rec == nil {
		t.Fatalf("load player: %v %v", rec, err)
	}
	if rec.Name != "Kyle" {
		t.Fatalf("player name = %q", rec.Name)
	}
	if rec.Skills["forage"].Lv != 1 || rec.Skills["cook"].Lv != 1 {
		t.Fatalf("skills not seeded: %+v", rec.Skills)
	}
}

func TestPGDuplicateUsernameRejected(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	a := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "h"}
	if err := pg.CreateAccount(ctx, a, "player-1", "Kyle"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	b := auth.Account{ID: "acct-2", Username: "KYLE", UsernameKey: "kyle", PWHash: "h"}
	err := pg.CreateAccount(ctx, b, "player-2", "KYLE")
	if !errors.Is(err, auth.ErrUsernameTaken) {
		t.Fatalf("second create gave %v, want ErrUsernameTaken", err)
	}
	// The losing signup must leave no orphan player row behind.
	rec, _ := pg.LoadPlayer(ctx, "player-2")
	if rec != nil {
		t.Fatal("a rejected signup left a player row")
	}
}

// One account owns exactly one character.
func TestPGOneAccountOnePlayer(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	a := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "h"}
	if err := pg.CreateAccount(ctx, a, "player-1", "Kyle"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := pg.pool.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, account_id, updated_at)
VALUES ('player-2','Sneak',8,8,'[]','{}','acct-1',now())`)
	if err == nil {
		t.Fatal("a second player row for one account was allowed")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("want a unique violation, got %v", err)
	}
}

func TestPGPasswordHashUpdate(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	a := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "old"}
	if err := pg.CreateAccount(ctx, a, "player-1", "Kyle"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := pg.UpdatePasswordHash(ctx, "acct-1", "new"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := pg.AccountByID(ctx, "acct-1")
	if got == nil || got.PWHash != "new" {
		t.Fatalf("hash not updated: %+v", got)
	}
	if err := pg.TouchLogin(ctx, "acct-1"); err != nil {
		t.Fatalf("touch login: %v", err)
	}
	var n int
	if err := pg.pool.QueryRow(ctx,
		`SELECT count(*) FROM accounts WHERE id='acct-1' AND last_login_at IS NOT NULL`).Scan(&n); err != nil {
		t.Fatalf("last_login check: %v", err)
	}
	if n != 1 {
		t.Fatal("last_login_at was not recorded")
	}
}

func TestPGMissingLookupsAreNotErrors(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	if a, err := pg.AccountByUsernameKey(ctx, "nobody"); err != nil || a != nil {
		t.Fatalf("missing key: %v %v", a, err)
	}
	if a, err := pg.AccountByID(ctx, "nope"); err != nil || a != nil {
		t.Fatalf("missing id: %v %v", a, err)
	}
	if pid, err := pg.PlayerIDForAccount(ctx, "nope"); err != nil || pid != "" {
		t.Fatalf("missing account: %q %v", pid, err)
	}
}

// Legacy player rows predate accounts; the migration must leave them in
// place with a NULL account_id rather than failing or deleting them.
func TestPGLegacyPlayersSurviveMigration(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	if _, err := pg.pool.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, updated_at)
VALUES ('legacy-1','OldHand',8,8,'[{"id":"berry","n":4}]','{}',now()),
       ('legacy-2','OldFriend',9,9,'[]','{}',now())`); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}
	// Many NULL account_ids must coexist under the unique index.
	if err := pg.migrate(ctx); err != nil {
		t.Fatalf("migrate over legacy rows: %v", err)
	}
	rec, err := pg.LoadPlayer(ctx, "legacy-1")
	if err != nil || rec == nil {
		t.Fatalf("legacy player lost: %v %v", rec, err)
	}
	if len(rec.Inv) != 1 || rec.Inv[0].N != 4 {
		t.Fatalf("legacy pack changed: %+v", rec.Inv)
	}
}

func TestPGLooksPersistAndFirstWriteWins(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	a := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "h"}
	if err := pg.CreateAccount(ctx, a, "player-1", "Kyle"); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := pg.LoadPlayer(ctx, "player-1")
	if err != nil || rec == nil {
		t.Fatalf("load: %v %v", rec, err)
	}
	if rec.Looks.Set() {
		t.Fatalf("a fresh row must have no face: %+v", rec.Looks)
	}

	first := protocol.Looks{
		Skin: protocol.SkinOlive, Hair: protocol.HairTied,
		HairColor: protocol.HairRusset, Top: protocol.TopInk,
	}
	got, wrote, err := world.ApplyLooksFirst(ctx, pg, rec, first)
	if err != nil || !wrote || got != first {
		t.Fatalf("first looks: %+v wrote=%v err=%v", got, wrote, err)
	}

	// Relog: a fresh load must still see the same face.
	reloaded, err := pg.LoadPlayer(ctx, "player-1")
	if err != nil || reloaded == nil {
		t.Fatalf("relog load: %v %v", reloaded, err)
	}
	if reloaded.Looks != first {
		t.Fatalf("relog looks = %+v, want %+v", reloaded.Looks, first)
	}

	other := protocol.Looks{
		Skin: protocol.SkinFair, Hair: protocol.HairLong,
		HairColor: protocol.HairSnow, Top: protocol.TopBerry,
	}
	again, wrote, err := world.ApplyLooksFirst(ctx, pg, rec, other)
	if err != nil || wrote || again != first {
		t.Fatalf("spam create: looks=%+v wrote=%v err=%v", again, wrote, err)
	}

	var n int
	if err := pg.pool.QueryRow(ctx, `SELECT count(*) FROM players WHERE account_id='acct-1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("player rows for the account = %d, want 1", n)
	}

	// A pack commit must keep the face.
	reloaded.Inv = append(reloaded.Inv, world.ItemStack{ID: "berry", N: 3})
	if err := pg.SavePlayer(ctx, reloaded); err != nil {
		t.Fatalf("save pack: %v", err)
	}
	kept, _ := pg.LoadPlayer(ctx, "player-1")
	if kept.Looks != first {
		t.Fatalf("pack write wiped looks: %+v", kept.Looks)
	}
}

func TestPGBankPersistsAndPackWriteKeepsIt(t *testing.T) {
	pg := testPG(t)
	ctx := context.Background()
	a := auth.Account{ID: "acct-1", Username: "Kyle", UsernameKey: "kyle", PWHash: "h"}
	if err := pg.CreateAccount(ctx, a, "player-1", "Kyle"); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := pg.LoadPlayer(ctx, "player-1")
	if err != nil || rec == nil {
		t.Fatalf("load: %v %v", rec, err)
	}
	if len(rec.Bank) != 0 || rec.BankCoins != 0 {
		t.Fatalf("a fresh row must have an empty chest: %+v coins=%d", rec.Bank, rec.BankCoins)
	}

	rec.Bank = []world.ItemStack{{ID: protocol.ItemBerry, N: 3}, {ID: protocol.ItemAxe, N: 1}}
	rec.BankCoins = 14
	rec.Inv = []world.ItemStack{{ID: protocol.ItemNut, N: 2}}
	rec.Coins = 5
	if err := pg.SavePlayer(ctx, rec); err != nil {
		t.Fatalf("save bank: %v", err)
	}

	relog, err := pg.LoadPlayer(ctx, "player-1")
	if err != nil || relog == nil {
		t.Fatalf("relog: %v %v", relog, err)
	}
	if len(relog.Bank) != 2 || relog.Bank[0].ID != protocol.ItemBerry || relog.Bank[0].N != 3 {
		t.Fatalf("relog bank = %+v", relog.Bank)
	}
	if relog.BankCoins != 14 || relog.Coins != 5 {
		t.Fatalf("relog coins purse=%d chest=%d", relog.Coins, relog.BankCoins)
	}

	// A later pack write must not wipe the chest, and a chest write must
	// not wipe the face if one has been carved.
	first := protocol.Looks{
		Skin: protocol.SkinOlive, Hair: protocol.HairTied,
		HairColor: protocol.HairRusset, Top: protocol.TopInk,
	}
	if _, wrote, err := world.ApplyLooksFirst(ctx, pg, relog, first); err != nil || !wrote {
		t.Fatalf("looks: wrote=%v err=%v", wrote, err)
	}
	relog, err = pg.LoadPlayer(ctx, "player-1")
	if err != nil || relog == nil {
		t.Fatalf("load after looks: %v %v", relog, err)
	}
	relog.Inv = append(relog.Inv, world.ItemStack{ID: protocol.ItemLog, N: 1})
	if err := pg.SavePlayer(ctx, relog); err != nil {
		t.Fatalf("pack write: %v", err)
	}
	kept, _ := pg.LoadPlayer(ctx, "player-1")
	if kept.Looks != first {
		t.Fatalf("pack write wiped looks: %+v", kept.Looks)
	}
	if len(kept.Bank) != 2 || kept.BankCoins != 14 {
		t.Fatalf("pack write wiped the chest: bank=%+v coins=%d", kept.Bank, kept.BankCoins)
	}

	// Unreadable bank JSON must refuse rather than hand back an empty chest.
	if _, err := pg.pool.Exec(ctx, `UPDATE players SET bank='not-json' WHERE id='player-1'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := pg.LoadPlayer(ctx, "player-1"); err == nil {
		t.Fatal("corrupt bank json loaded as an empty chest")
	}
}
