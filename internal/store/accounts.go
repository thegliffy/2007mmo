package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thegliffy/2007mmo/internal/auth"
	"github.com/thegliffy/2007mmo/internal/world"
)

// pgUniqueViolation is SQLSTATE 23505.
const pgUniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}

// CreateAccount writes the credential row and the player row it owns in
// one transaction, so a failed signup never leaves a nameless player or
// an account with nothing to play.
func (p *Postgres) CreateAccount(ctx context.Context, a auth.Account, playerID, playerName string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
INSERT INTO accounts (id, username, username_key, pw_hash, created_at)
VALUES ($1,$2,$3,$4,now())`,
		a.ID, a.Username, a.UsernameKey, a.PWHash); err != nil {
		if isUniqueViolation(err) {
			return auth.ErrUsernameTaken
		}
		return err
	}

	rec := world.NewPlayerRec(playerID, playerName)
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
INSERT INTO players (id, name, x, y, inventory, skills, hp, account_id, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP, a.ID); err != nil {
		if isUniqueViolation(err) {
			return auth.ErrUsernameTaken
		}
		return err
	}

	return tx.Commit(ctx)
}

func (p *Postgres) AccountByUsernameKey(ctx context.Context, key string) (*auth.Account, error) {
	return p.accountBy(ctx, `SELECT id, username, username_key, pw_hash FROM accounts WHERE username_key=$1`, key)
}

func (p *Postgres) AccountByID(ctx context.Context, id string) (*auth.Account, error) {
	return p.accountBy(ctx, `SELECT id, username, username_key, pw_hash FROM accounts WHERE id=$1`, id)
}

func (p *Postgres) accountBy(ctx context.Context, q, arg string) (*auth.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var a auth.Account
	err := p.pool.QueryRow(ctx, q, arg).Scan(&a.ID, &a.Username, &a.UsernameKey, &a.PWHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (p *Postgres) UpdatePasswordHash(ctx context.Context, accountID, hash string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET pw_hash=$2 WHERE id=$1`, accountID, hash)
	return err
}

func (p *Postgres) PlayerIDForAccount(ctx context.Context, accountID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var id string
	err := p.pool.QueryRow(ctx, `SELECT id FROM players WHERE account_id=$1`, accountID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func (p *Postgres) TouchLogin(ctx context.Context, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET last_login_at=now() WHERE id=$1`, accountID)
	return err
}
