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
INSERT INTO players (id, name, x, y, inventory, skills, hp, coins, account_id, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`,
		rec.ID, rec.Name, rec.X, rec.Y, inv, sk, rec.HP, rec.Coins, a.ID); err != nil {
		if isUniqueViolation(err) {
			return auth.ErrUsernameTaken
		}
		return err
	}

	return tx.Commit(ctx)
}

const accountCols = `SELECT id, username, username_key, pw_hash, role, banned_until FROM accounts`

func (p *Postgres) AccountByUsernameKey(ctx context.Context, key string) (*auth.Account, error) {
	return p.accountBy(ctx, accountCols+` WHERE username_key=$1`, key)
}

func (p *Postgres) AccountByID(ctx context.Context, id string) (*auth.Account, error) {
	return p.accountBy(ctx, accountCols+` WHERE id=$1`, id)
}

func (p *Postgres) accountBy(ctx context.Context, q, arg string) (*auth.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var a auth.Account
	err := p.pool.QueryRow(ctx, q, arg).Scan(
		&a.ID, &a.Username, &a.UsernameKey, &a.PWHash, &a.Role, &a.BannedUntil)
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

func (p *Postgres) SetRole(ctx context.Context, accountID, role string) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET role=$2 WHERE id=$1`, accountID, role)
	return err
}

func (p *Postgres) SetBannedUntil(ctx context.Context, accountID string, until *time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET banned_until=$2 WHERE id=$1`, accountID, until)
	return err
}

// AdminAction is one row of the audit log.
type AdminAction struct {
	Actor      *string // nil when the host CLI acted with no account behind it
	ActorName  string
	Action     string
	Target     *string
	TargetName string
	Detail     string
	CreatedAt  time.Time
}

// RecordAdminAction appends to the audit log. Append-only: nothing in the
// codebase updates or deletes these rows.
func (p *Postgres) RecordAdminAction(ctx context.Context, a AdminAction) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `
INSERT INTO admin_actions (actor, actor_name, action, target, target_name, detail)
VALUES ($1,$2,$3,$4,$5,$6)`,
		a.Actor, a.ActorName, a.Action, a.Target, a.TargetName, a.Detail)
	return err
}

// RecentAdminActions reports the audit log, newest first.
func (p *Postgres) RecentAdminActions(ctx context.Context, limit int) ([]AdminAction, error) {
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := p.pool.Query(ctx, `
SELECT actor, actor_name, action, target, target_name, coalesce(detail,''), created_at
FROM admin_actions ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminAction
	for rows.Next() {
		var a AdminAction
		if err := rows.Scan(&a.Actor, &a.ActorName, &a.Action, &a.Target, &a.TargetName, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
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

// AccountSummary is one row of the admin account listing.
type AccountSummary struct {
	ID          string
	Username    string
	Role        string
	BannedUntil *time.Time
	CreatedAt   time.Time
	LastLogin   *time.Time
	PlayerID    string
}

// ListAccounts reports every account, newest first. Reporting only — it
// deliberately does not carry password hashes.
func (p *Postgres) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := p.pool.Query(ctx, `
SELECT a.id, a.username, a.role, a.banned_until, a.created_at, a.last_login_at, coalesce(pl.id,'')
FROM accounts a
LEFT JOIN players pl ON pl.account_id = a.id
ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountSummary
	for rows.Next() {
		var s AccountSummary
		if err := rows.Scan(&s.ID, &s.Username, &s.Role, &s.BannedUntil, &s.CreatedAt, &s.LastLogin, &s.PlayerID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) TouchLogin(ctx context.Context, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET last_login_at=now() WHERE id=$1`, accountID)
	return err
}
