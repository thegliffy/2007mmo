package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (p *Postgres) migrate(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `
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
CREATE INDEX IF NOT EXISTS players_updated_at_idx ON players (updated_at DESC);

-- Accounts: login credentials. One account owns exactly one player row.
CREATE TABLE IF NOT EXISTS accounts (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL,
    username_key  TEXT NOT NULL UNIQUE,
    pw_hash       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

-- Nullable so player rows created before accounts existed survive the
-- upgrade. A unique index still permits many NULLs, so those legacy rows
-- simply become unreachable rather than blocking the migration.
ALTER TABLE players ADD COLUMN IF NOT EXISTS account_id TEXT;
-- Nullable: rows written before health was persisted read back as "full".
ALTER TABLE players ADD COLUMN IF NOT EXISTS hp INTEGER;
CREATE UNIQUE INDEX IF NOT EXISTS players_account_id_idx ON players (account_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'players_account_id_fkey'
    ) THEN
        ALTER TABLE players
            ADD CONSTRAINT players_account_id_fkey
            FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE;
    END IF;
END $$;`)
	return err
}

// writeTimeout bounds every store write. These calls run on the world's
// single goroutine, so an unbounded one stops the whole hamlet: the tick
// is 600ms, and this is the ceiling on how far behind one slow write can
// push it. Exceeding it drops that write, which is always the safe
// direction — items are granted only after a commit succeeds.
const writeTimeout = 2 * time.Second

func (p *Postgres) LoadPlayer(ctx context.Context, id string) (*world.PlayerRec, error) {
	row := p.pool.QueryRow(ctx, `
SELECT id, name, x, y, inventory, skills, hp FROM players WHERE id=$1`, id)
	var rec world.PlayerRec
	var inv, skills []byte
	var hp *int
	err := row.Scan(&rec.ID, &rec.Name, &rec.X, &rec.Y, &inv, &skills, &hp)
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
	_, err = p.pool.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, hp, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,now())
ON CONFLICT (id) DO UPDATE SET
    name=EXCLUDED.name,
    x=EXCLUDED.x,
    y=EXCLUDED.y,
    inventory=EXCLUDED.inventory,
    skills=EXCLUDED.skills,
    hp=EXCLUDED.hp,
    updated_at=now()`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP)
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
	if _, err := tx.Exec(ctx, `
INSERT INTO players (id, name, x, y, inventory, skills, hp, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,now())
ON CONFLICT (id) DO UPDATE SET
    name=EXCLUDED.name,
    x=EXCLUDED.x,
    y=EXCLUDED.y,
    inventory=EXCLUDED.inventory,
    skills=EXCLUDED.skills,
    hp=EXCLUDED.hp,
    updated_at=now()`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP); err != nil {
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
