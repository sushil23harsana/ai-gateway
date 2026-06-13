// Package ratelimit implements a per-virtual-key Redis token-bucket limiter and
// the HTTP middleware that enforces it. The bucket holds up to rpm tokens and
// refills at rpm/60 tokens per second; each request costs one token. The refill
// + check is done atomically in a single Lua script.
package ratelimit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
)

// tokenBucketScript refills the bucket based on elapsed time, then tries to take
// one token. Returns {allowed(0|1), retry_after_seconds}.
var tokenBucketScript = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local data = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then
  tokens = capacity
  ts = now
end

local delta = now - ts
if delta < 0 then delta = 0 end
tokens = math.min(capacity, tokens + delta * rate)

local allowed = 0
local retry_after = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
else
  retry_after = math.ceil((requested - tokens) / rate)
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'ts', now)
local ttl = math.ceil(capacity / rate) + 1
redis.call('EXPIRE', KEYS[1], ttl)

return {allowed, retry_after}
`)

// Decision is the outcome of a limiter check.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration // populated only when Allowed is false
}

// Allower is the limiter behavior the middleware depends on (real impl: *Limiter).
type Allower interface {
	Allow(ctx context.Context, id string, rpm int) (Decision, error)
}

// Limiter is the Redis-backed token-bucket limiter.
type Limiter struct {
	rdb *redis.Client
}

// NewLimiter wraps a Redis client.
func NewLimiter(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb} }

// Allow takes one token from the key's bucket. rpm <= 0 means unlimited.
func (l *Limiter) Allow(ctx context.Context, id string, rpm int) (Decision, error) {
	if rpm <= 0 {
		return Decision{Allowed: true}, nil
	}
	capacity := rpm
	rate := float64(rpm) / 60.0
	now := float64(time.Now().UnixNano()) / 1e9

	res, err := tokenBucketScript.Run(ctx, l.rdb, []string{"ratelimit:" + id},
		capacity,
		strconv.FormatFloat(rate, 'f', 9, 64),
		strconv.FormatFloat(now, 'f', 6, 64),
		1,
	).Result()
	if err != nil {
		return Decision{}, err
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) < 2 {
		return Decision{Allowed: true}, nil
	}
	allowed, _ := arr[0].(int64)
	retry, _ := arr[1].(int64)

	d := Decision{Allowed: allowed == 1}
	if !d.Allowed {
		if retry < 1 {
			retry = 1
		}
		d.RetryAfter = time.Duration(retry) * time.Second
	}
	return d, nil
}

// Middleware enforces the limiter using the authenticated Identity from context.
// It must be chained after the auth middleware. On a Redis error it fails open
// (allows the request) so an infra blip doesn't take down the proxy.
func Middleware(a Allower, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := keys.IdentityFrom(r.Context())
			if !ok {
				// No identity (not behind auth) — nothing to limit on.
				next.ServeHTTP(w, r)
				return
			}
			dec, err := a.Allow(r.Context(), id.ID, id.RateLimitRPM)
			if err != nil {
				log.Error("rate limiter error; failing open", "err", err, "key_id", id.ID)
				next.ServeHTTP(w, r)
				return
			}
			if !dec.Allowed {
				secs := int(dec.RetryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
