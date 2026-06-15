// Package budget enforces per-key monthly spend caps. Spend is accumulated in a
// Redis counter — incremented off the hot path by the async metrics logger — and
// checked by a middleware before each request. A key is only ever blocked when it
// has a monthly_budget_usd set; keys without a budget pass through untouched.
//
// This is a soft cap: the request that crosses the threshold is served (spend is
// recorded asynchronously just after), and subsequent requests are blocked. The
// Redis counter is for fast enforcement only; the dashboard reports spend from
// Postgres. Both are driven by the same logger worker, so they stay in step.
package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
)

// monthKey is the Redis key holding a virtual key's spend for the current UTC month.
func monthKey(keyID string, now time.Time) string {
	return fmt.Sprintf("spend:%s:%s", keyID, now.UTC().Format("200601"))
}

// SpendReader reads a key's current-month spend (real impl: *Tracker). The
// middleware depends only on this, so it can be unit-tested without Redis.
type SpendReader interface {
	Spend(ctx context.Context, keyID string) (float64, error)
}

// Tracker records and reads per-key monthly spend in Redis.
type Tracker struct {
	rdb *redis.Client
	log *slog.Logger
}

// NewTracker wraps a Redis client.
func NewTracker(rdb *redis.Client, log *slog.Logger) *Tracker {
	return &Tracker{rdb: rdb, log: log}
}

// Add increments the key's current-month spend by costUSD. Best-effort and off
// the hot path (called by the async logger); failures are logged, not surfaced.
func (t *Tracker) Add(ctx context.Context, keyID string, costUSD float64) {
	if t == nil || t.rdb == nil || keyID == "" || costUSD <= 0 {
		return
	}
	key := monthKey(keyID, time.Now())
	pipe := t.rdb.Pipeline()
	pipe.IncrByFloat(ctx, key, costUSD)
	pipe.Expire(ctx, key, 40*24*time.Hour) // outlive the month, then self-clean
	if _, err := pipe.Exec(ctx); err != nil {
		t.log.Warn("budget spend increment failed", "err", err, "key_id", keyID)
	}
}

// Spend returns the key's current-month spend in USD (0 when unset).
func (t *Tracker) Spend(ctx context.Context, keyID string) (float64, error) {
	v, err := t.rdb.Get(ctx, monthKey(keyID, time.Now())).Float64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v, nil
}

// Middleware blocks (HTTP 402) a request whose key has a monthly budget it has
// already met or exceeded. Keys with no budget pass through. When enabled is
// false it is a no-op. On a Redis error it fails OPEN (allows the request) so an
// infra blip never takes down the proxy. Must be chained after the auth middleware.
func Middleware(r SpendReader, enabled bool, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !enabled {
				next.ServeHTTP(w, req)
				return
			}
			id, ok := keys.IdentityFrom(req.Context())
			if !ok || id.MonthlyBudgetUSD <= 0 {
				next.ServeHTTP(w, req)
				return
			}
			spent, err := r.Spend(req.Context(), id.ID)
			if err != nil {
				log.Error("budget check failed; failing open", "err", err, "key_id", id.ID)
				next.ServeHTTP(w, req)
				return
			}
			if spent >= id.MonthlyBudgetUSD {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":              "monthly budget exceeded",
					"monthly_budget_usd": id.MonthlyBudgetUSD,
					"spent_usd":          spent,
				})
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
