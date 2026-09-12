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

func (r *Redis) SetSession(ctx context.Context, token, playerID string, ttl time.Duration) error {
	return r.c.Set(ctx, "session:"+token, playerID, ttl).Err()
}

func (r *Redis) GetSession(ctx context.Context, token string) (string, error) {
	s, err := r.c.Get(ctx, "session:"+token).Result()
	if err == redis.Nil {
		return "", nil
	}
	return s, err
}

func (r *Redis) SetPresence(ctx context.Context, playerID string) error {
	return r.c.Set(ctx, "presence:"+playerID, "1", 30*time.Second).Err()
}

func (r *Redis) ClearPresence(ctx context.Context, playerID string) error {
	return r.c.Del(ctx, "presence:"+playerID).Err()
}
