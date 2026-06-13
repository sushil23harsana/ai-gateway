// Package store is the PostgreSQL layer: it owns the connection pool and the
// append-only request_logs writes. It is intentionally thin — callers build a
// RequestLog and hand it over; the store does not know about HTTP or providers.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RequestLog is one row destined for the request_logs table. Nullable columns
// use pointers (nil → SQL NULL).
type RequestLog struct {
	APIKeyID  *string // virtual key id; nil until Phase 2 wires up auth
	Provider  string  // openai | anthropic
	Model     string
	Status    int  // HTTP status returned to the client
	CacheHit  bool // always false until Phase 3
	TokensIn  int
	TokensOut int
	CostUSD   float64
	LatencyMs int
	Error     *string // nil on success
}

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a connection pool and verifies connectivity with a ping.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *Store) Close() { s.pool.Close() }

// InsertRequestLog appends one row to request_logs.
func (s *Store) InsertRequestLog(ctx context.Context, rl RequestLog) error {
	const q = `
		INSERT INTO request_logs
			(api_key_id, provider, model, status, cache_hit, tokens_in, tokens_out, cost_usd, latency_ms, error)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var apiKeyID any
	if rl.APIKeyID != nil {
		apiKeyID = *rl.APIKeyID
	}

	_, err := s.pool.Exec(ctx, q,
		apiKeyID, rl.Provider, rl.Model, rl.Status, rl.CacheHit,
		rl.TokensIn, rl.TokensOut, rl.CostUSD, rl.LatencyMs, rl.Error,
	)
	if err != nil {
		return fmt.Errorf("insert request_log: %w", err)
	}
	return nil
}
