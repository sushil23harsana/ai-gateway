package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// LiveCounter tracks per-minute request counts in Redis for the dashboard's live
// tile. Keys are `live:reqs:<unix-minute>` with a short TTL, so idle buckets
// expire on their own.
type LiveCounter struct {
	rdb *redis.Client
	log *slog.Logger
}

// MinuteCount is the request count for one minute bucket.
type MinuteCount struct {
	Timestamp int64 `json:"timestamp"` // unix seconds at the start of the minute
	Count     int   `json:"count"`
}

// NewLiveCounter wraps a Redis client.
func NewLiveCounter(rdb *redis.Client, log *slog.Logger) *LiveCounter {
	return &LiveCounter{rdb: rdb, log: log}
}

func minuteKey(min int64) string { return fmt.Sprintf("live:reqs:%d", min) }

// Incr bumps the current minute's counter. Best-effort: failures are logged, not
// surfaced, so the response path is never affected.
func (lc *LiveCounter) Incr(ctx context.Context) {
	if lc == nil || lc.rdb == nil {
		return
	}
	key := minuteKey(time.Now().Unix() / 60)
	pipe := lc.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		lc.log.Warn("live counter increment failed", "err", err)
	}
}

// Middleware increments the live counter for each passing request.
func (lc *LiveCounter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lc.Incr(r.Context())
		next.ServeHTTP(w, r)
	})
}

// Recent returns the last n minute buckets, oldest first.
func (lc *LiveCounter) Recent(ctx context.Context, n int) ([]MinuteCount, error) {
	if n <= 0 {
		n = 15
	}
	nowMin := time.Now().Unix() / 60
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = minuteKey(nowMin - int64(n-1-i)) // oldest → newest
	}
	vals, err := lc.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("live counter MGET: %w", err)
	}
	out := make([]MinuteCount, n)
	for i, v := range vals {
		count := 0
		if s, ok := v.(string); ok {
			count, _ = strconv.Atoi(s)
		}
		out[i] = MinuteCount{Timestamp: (nowMin - int64(n-1-i)) * 60, Count: count}
	}
	return out, nil
}
