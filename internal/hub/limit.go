package hub

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// tokenBucket is a simple refill limiter. Safe for one goroutine at a time
// via its mutex; keyedLimiter holds the map lock only while looking up.
type tokenBucket struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	rate   float64
	burst  float64
}

func newBucket(rate, burst float64) *tokenBucket {
	if rate <= 0 {
		rate = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &tokenBucket{tokens: burst, last: time.Now(), rate: rate, burst: burst}
}

func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type keyedLimiter struct {
	mu    sync.Mutex
	m     map[string]*tokenBucket
	rate  float64
	burst float64
}

func newKeyedLimiter(rate, burst float64) *keyedLimiter {
	return &keyedLimiter{m: make(map[string]*tokenBucket), rate: rate, burst: burst}
}

func (k *keyedLimiter) allow(key string) bool {
	if key == "" {
		key = "unknown"
	}
	k.mu.Lock()
	b := k.m[key]
	if b == nil {
		if len(k.m) > 8000 {
			k.m = make(map[string]*tokenBucket)
		}
		b = newBucket(k.rate, k.burst)
		k.m[key] = b
	}
	k.mu.Unlock()
	return b.allow()
}

type limits struct {
	hello *keyedLimiter
	conn  *keyedLimiter
	ws    *keyedLimiter
	chat  *keyedLimiter
	// login is per source address and loginUser is per account name, so
	// neither one IP nor one distributed spray at a single account gets
	// unlimited password guesses.
	login     *keyedLimiter
	loginUser *keyedLimiter
}

func limitsFromEnv() *limits {
	return &limits{
		hello: newKeyedLimiter(envFloat("HOLLOWMERE_LIMIT_HELLO_RATE", 12), envFloat("HOLLOWMERE_LIMIT_HELLO_BURST", 40)),
		conn:  newKeyedLimiter(envFloat("HOLLOWMERE_LIMIT_CONN_RATE", 20), envFloat("HOLLOWMERE_LIMIT_CONN_BURST", 80)),
		ws:    newKeyedLimiter(envFloat("HOLLOWMERE_LIMIT_WS_RATE", 20), envFloat("HOLLOWMERE_LIMIT_WS_BURST", 32)),
		chat:  newKeyedLimiter(envFloat("HOLLOWMERE_LIMIT_CHAT_RATE", 0.8), envFloat("HOLLOWMERE_LIMIT_CHAT_BURST", 4)),
		login: newKeyedLimiter(envFloat("HOLLOWMERE_LIMIT_LOGIN_RATE", 0.2), envFloat("HOLLOWMERE_LIMIT_LOGIN_BURST", 8)),
		loginUser: newKeyedLimiter(
			envFloat("HOLLOWMERE_LIMIT_LOGIN_USER_RATE", 0.1),
			envFloat("HOLLOWMERE_LIMIT_LOGIN_USER_BURST", 6),
		),
	}
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil && n > 0 {
			return n
		}
	}
	return def
}
