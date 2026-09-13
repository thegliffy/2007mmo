package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	c *redis.Client
}

func NewRedis(url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	c := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Redis{c: c}, nil
}

func (r *Redis) Close() {
	if r != nil && r.c != nil {
		_ = r.c.Close()
	}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.c.Ping(ctx).Err()
}

// Login sessions. A token maps to an account id; each account also keeps
// a set of its live tokens so a password change can revoke the others.
//
// Redis is authoritative for sessions and holds no password material.
// Losing Redis logs everyone out; it never grants access.

func sessionKey(token string) string { return "session:" + token }

func accountSessionsKey(accountID string) string { return "acct-sessions:" + accountID }

func (r *Redis) CreateSession(ctx context.Context, token, accountID string, ttl time.Duration) error {
	pipe := r.c.TxPipeline()
	pipe.Set(ctx, sessionKey(token), accountID, ttl)
	pipe.SAdd(ctx, accountSessionsKey(accountID), token)
	// The index outlives any single token so it can still be cleaned up.
	pipe.Expire(ctx, accountSessionsKey(accountID), ttl*2)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Redis) SessionAccount(ctx context.Context, token string) (string, error) {
	s, err := r.c.Get(ctx, sessionKey(token)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return s, err
}

func (r *Redis) TouchSession(ctx context.Context, token string, ttl time.Duration) error {
	return r.c.Expire(ctx, sessionKey(token), ttl).Err()
}

func (r *Redis) DeleteSession(ctx context.Context, token string) error {
	accountID, err := r.SessionAccount(ctx, token)
	if err == nil && accountID != "" {
		_ = r.c.SRem(ctx, accountSessionsKey(accountID), token).Err()
	}
	return r.c.Del(ctx, sessionKey(token)).Err()
}

// DeleteAccountSessions drops every session for an account except keep.
func (r *Redis) DeleteAccountSessions(ctx context.Context, accountID, keep string) error {
	tokens, err := r.c.SMembers(ctx, accountSessionsKey(accountID)).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	pipe := r.c.TxPipeline()
	for _, t := range tokens {
		if t == keep {
			continue
		}
		pipe.Del(ctx, sessionKey(t))
		pipe.SRem(ctx, accountSessionsKey(accountID), t)
	}
	if keep == "" {
		pipe.Del(ctx, accountSessionsKey(accountID))
	}
	_, err = pipe.Exec(ctx)
	if err == redis.Nil {
		return nil
	}
	return err
}

func muteKey(accountID string) string { return "mute:" + accountID }

// SetMute quiets an account for ttl. Redis expiry does the un-muting, so
// nothing has to remember to.
func (r *Redis) SetMute(ctx context.Context, accountID string, ttl time.Duration) error {
	if ttl <= 0 {
		return r.c.Del(ctx, muteKey(accountID)).Err()
	}
	return r.c.Set(ctx, muteKey(accountID), "1", ttl).Err()
}

// MuteRemaining reports how much longer an account is quiet; zero when it
// is not muted.
func (r *Redis) MuteRemaining(ctx context.Context, accountID string) (time.Duration, error) {
	d, err := r.c.TTL(ctx, muteKey(accountID)).Result()
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, nil
	}
	return d, nil
}

func (r *Redis) SetPresence(ctx context.Context, playerID string) error {
	return r.c.Set(ctx, "presence:"+playerID, "1", 30*time.Second).Err()
}

func (r *Redis) ClearPresence(ctx context.Context, playerID string) error {
	return r.c.Del(ctx, "presence:"+playerID).Err()
}
