package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thegliffy/2007mmo/internal/world"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 16
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	pg := &Postgres{pool: pool}
	if err := pg.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pg, nil
}

func (p *Postgres) Close() {
	if p != nil && p.pool != nil {
		p.pool.Close()
	}
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// writeTimeout bounds every store write. These calls run on the world's
// single goroutine, so an unbounded one stops the whole hamlet: the tick
// is 600ms, and this is the ceiling on how far behind one slow write can
// push it. Exceeding it drops that write, which is always the safe
// direction — items are granted only after a commit succeeds.
const writeTimeout = 2 * time.Second

// marshalEquipped always produces a JSON object, never null, so the
// NOT NULL column stays satisfied for a player wearing nothing.
func marshalEquipped(m map[string]string) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// A migration is one numbered, named step. Every statement must be
// idempotent: this scheme was adopted over a database that had already
// been built by an unversioned CREATE IF NOT EXISTS block, so migration 1
// has to be a no-op against the live schema rather than a fresh build.
type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{1, "core tables", `
CREATE TABLE IF NOT EXISTS players (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    x          INTEGER NOT NULL,
    y          INTEGER NOT NULL,
    inventory  JSONB NOT NULL DEFAULT '[]',
    skills     JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS nodes (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    x               INTEGER NOT NULL,
    y               INTEGER NOT NULL,
    remaining       INTEGER NOT NULL,
    cooldown_ticks  INTEGER NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS players_updated_at_idx ON players (updated_at DESC);`},

	{2, "accounts and the player link", `
CREATE TABLE IF NOT EXISTS accounts (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL,
    username_key  TEXT NOT NULL UNIQUE,
    pw_hash       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);
ALTER TABLE players ADD COLUMN IF NOT EXISTS account_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS players_account_id_idx ON players (account_id);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'players_account_id_fkey') THEN
        ALTER TABLE players
            ADD CONSTRAINT players_account_id_fkey
            FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE;
    END IF;
END $$;`},

	{3, "persisted health", `
ALTER TABLE players ADD COLUMN IF NOT EXISTS hp INTEGER;`},

	{4, "admin roles and audit log", `
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS banned_until TIMESTAMPTZ;

-- Append-only. actor_name and target_name are denormalised on purpose so a
-- row stays readable after the account it names is deleted.
CREATE TABLE IF NOT EXISTS admin_actions (
    id          BIGSERIAL PRIMARY KEY,
    actor       TEXT,
    actor_name  TEXT NOT NULL,
    action      TEXT NOT NULL,
    target      TEXT,
    target_name TEXT,
    detail      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_actions_created_at_idx ON admin_actions (created_at DESC);`},

	{5, "coin purse", `
ALTER TABLE players ADD COLUMN IF NOT EXISTS coins INTEGER NOT NULL DEFAULT 0;`},

	{6, "appearance looks", `
ALTER TABLE players ADD COLUMN IF NOT EXISTS looks JSONB;`},

	{7, "personal bank", `
ALTER TABLE players ADD COLUMN IF NOT EXISTS bank JSONB NOT NULL DEFAULT '[]';
ALTER TABLE players ADD COLUMN IF NOT EXISTS bank_coins INTEGER NOT NULL DEFAULT 0;`},

	{8, "worn equipment", `
ALTER TABLE players ADD COLUMN IF NOT EXISTS equipped JSONB NOT NULL DEFAULT '{}';`},
}

// migrate applies whatever has not been recorded yet, each in its own
// transaction, newest schema last.
func (p *Postgres) migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := p.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		tx, err := p.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1,$2)`,
			m.version, m.name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
		log.Printf("applied migration %d: %s", m.version, m.name)
	}
	return nil
}

// SchemaVersion reports the highest applied migration.
func (p *Postgres) SchemaVersion(ctx context.Context) (int, error) {
	var v *int
	if err := p.pool.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, err
	}
	if v == nil {
		return 0, nil
	}
	return *v, nil
}

func (p *Postgres) LoadPlayer(ctx context.Context, id string) (*world.PlayerRec, error) {
	row := p.pool.QueryRow(ctx, `
SELECT id, name, x, y, inventory, skills, hp, coins, bank, bank_coins, looks, equipped FROM players WHERE id=$1`, id)
	var rec world.PlayerRec
	var inv, skills, bank, looks, equipped []byte
	var hp *int
	err := row.Scan(&rec.ID, &rec.Name, &rec.X, &rec.Y, &inv, &skills, &hp, &rec.Coins, &bank, &rec.BankCoins, &looks, &equipped)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Refuse rather than repair. Returning an empty pack here would look
	// like a successful load and hand the player a wiped inventory, which
	// the next save would then make permanent.
	if err := json.Unmarshal(inv, &rec.Inv); err != nil {
		return nil, fmt.Errorf("player %s: unreadable inventory json: %w", id, err)
	}
	if err := json.Unmarshal(skills, &rec.Skills); err != nil {
		return nil, fmt.Errorf("player %s: unreadable skills json: %w", id, err)
	}
	if rec.Skills == nil {
		rec.Skills = map[string]world.SkillState{}
	}
	if len(bank) == 0 || string(bank) == "null" {
		rec.Bank = nil
	} else if err := json.Unmarshal(bank, &rec.Bank); err != nil {
		return nil, fmt.Errorf("player %s: unreadable bank json: %w", id, err)
	}
	if len(equipped) > 0 && string(equipped) != "null" {
		if err := json.Unmarshal(equipped, &rec.Equipped); err != nil {
			return nil, fmt.Errorf("player %s: unreadable equipment json: %w", id, err)
		}
	}
	if err := unmarshalLooks(looks, &rec.Looks); err != nil {
		return nil, fmt.Errorf("player %s: unreadable looks json: %w", id, err)
	}
	rec.HP = hp
	return &rec, nil
}

func (p *Postgres) SavePlayer(ctx context.Context, rec *world.PlayerRec) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	inv, err := json.Marshal(rec.Inv)
	if err != nil {
		return err
	}
	if rec.Inv == nil {
		inv = []byte("[]")
	}
	sk, err := json.Marshal(rec.Skills)
	if err != nil {
		return err
	}
	looks, err := marshalLooks(rec.Looks)
	if err != nil {
		return err
	}
	eq, err := marshalEquipped(rec.Equipped)
	if err != nil {
		return err
	}
	bank, err := marshalBank(rec.Bank)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, hp, coins, bank, bank_coins, looks, equipped, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
ON CONFLICT (id) DO UPDATE SET
    name=EXCLUDED.name,
    x=EXCLUDED.x,
    y=EXCLUDED.y,
    inventory=EXCLUDED.inventory,
    skills=EXCLUDED.skills,
    hp=EXCLUDED.hp,
    coins=EXCLUDED.coins,
    bank=EXCLUDED.bank,
    bank_coins=EXCLUDED.bank_coins,
    looks=EXCLUDED.looks,
    equipped=EXCLUDED.equipped,
    updated_at=now()`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP, rec.Coins, bank, rec.BankCoins, looks, eq)
	return err
}

func (p *Postgres) CommitAction(ctx context.Context, rec *world.PlayerRec, n *world.NodeRec) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inv, err := json.Marshal(rec.Inv)
	if err != nil {
		return err
	}
	if rec.Inv == nil {
		inv = []byte("[]")
	}
	sk, err := json.Marshal(rec.Skills)
	if err != nil {
		return err
	}
	looks, err := marshalLooks(rec.Looks)
	if err != nil {
		return err
	}
	eq, err := marshalEquipped(rec.Equipped)
	if err != nil {
		return err
	}
	bank, err := marshalBank(rec.Bank)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, hp, coins, bank, bank_coins, looks, equipped, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
ON CONFLICT (id) DO UPDATE SET
    name=EXCLUDED.name,
    x=EXCLUDED.x,
    y=EXCLUDED.y,
    inventory=EXCLUDED.inventory,
    skills=EXCLUDED.skills,
    hp=EXCLUDED.hp,
    coins=EXCLUDED.coins,
    bank=EXCLUDED.bank,
    bank_coins=EXCLUDED.bank_coins,
    looks=EXCLUDED.looks,
    equipped=EXCLUDED.equipped,
    updated_at=now()`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP, rec.Coins, bank, rec.BankCoins, looks, eq); err != nil {
		return err
	}
	if n != nil {
		if _, err := tx.Exec(ctx, `
INSERT INTO nodes (id, kind, x, y, remaining, cooldown_ticks, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,now())
ON CONFLICT (id) DO UPDATE SET
    remaining=EXCLUDED.remaining,
    cooldown_ticks=EXCLUDED.cooldown_ticks,
    updated_at=now()`,
			n.ID, n.Kind, n.X, n.Y, n.Remaining, n.Cooldown); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) LoadNodes(ctx context.Context) ([]world.NodeRec, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, kind, x, y, remaining, cooldown_ticks FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []world.NodeRec
	for rows.Next() {
		var n world.NodeRec
		if err := rows.Scan(&n.ID, &n.Kind, &n.X, &n.Y, &n.Remaining, &n.Cooldown); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (p *Postgres) UpsertNode(ctx context.Context, n world.NodeRec) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
INSERT INTO nodes (id, kind, x, y, remaining, cooldown_ticks, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,now())
ON CONFLICT (id) DO UPDATE SET
    remaining=EXCLUDED.remaining,
    cooldown_ticks=EXCLUDED.cooldown_ticks,
    updated_at=now()`,
		n.ID, n.Kind, n.X, n.Y, n.Remaining, n.Cooldown)
	return err
}

func (p *Postgres) SeedNodes(ctx context.Context, nodes []world.NodeRec) error {
	existing, err := p.LoadNodes(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, n := range existing {
		have[n.ID] = true
	}
	for _, n := range nodes {
		if have[n.ID] {
			continue
		}
		if err := p.UpsertNode(ctx, n); err != nil {
			return err
		}
	}
	return nil
}
