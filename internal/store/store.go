// Package store is the PostgreSQL layer: it owns the connection pool and the
// reads/writes for request_logs and api_keys. It is intentionally thin — callers
// build a value and hand it over; the store does not know about HTTP or providers.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequestLog is one row destined for the request_logs table. Nullable columns
// use pointers (nil → SQL NULL).
type RequestLog struct {
	APIKeyID  *string // virtual key id; nil for unauthenticated/failed-before-auth requests
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

// APIKey is a virtual key record. KeyHash is the SHA-256 of the raw key; the raw
// key is never stored.
type APIKey struct {
	ID               string
	Name             string
	KeyHash          string
	MonthlyBudgetUSD *float64
	RateLimitRPM     int
	CreatedAt        time.Time
	Disabled         bool
	CacheEnabled     bool
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

// Ping verifies database connectivity (used by the readiness probe).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

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

// InsertAPIKey creates a virtual key (storing only the hash) and returns the row.
func (s *Store) InsertAPIKey(ctx context.Context, name, keyHash string, rateLimitRPM int, monthlyBudgetUSD *float64, cacheEnabled bool) (APIKey, error) {
	const q = `
		INSERT INTO api_keys (name, key_hash, rate_limit_rpm, monthly_budget_usd, cache_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, name, key_hash, rate_limit_rpm, monthly_budget_usd, created_at, disabled, cache_enabled`
	var k APIKey
	err := s.pool.QueryRow(ctx, q, name, keyHash, rateLimitRPM, monthlyBudgetUSD, cacheEnabled).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.RateLimitRPM, &k.MonthlyBudgetUSD, &k.CreatedAt, &k.Disabled, &k.CacheEnabled)
	if err != nil {
		return APIKey{}, fmt.Errorf("insert api_key: %w", err)
	}
	return k, nil
}

// GetAPIKeyByHash looks up a key by its hash for authentication. found is false
// (with nil error) when no row matches.
func (s *Store) GetAPIKeyByHash(ctx context.Context, keyHash string) (APIKey, bool, error) {
	const q = `
		SELECT id::text, name, key_hash, rate_limit_rpm, monthly_budget_usd, created_at, disabled, cache_enabled
		FROM api_keys WHERE key_hash = $1`
	var k APIKey
	err := s.pool.QueryRow(ctx, q, keyHash).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.RateLimitRPM, &k.MonthlyBudgetUSD, &k.CreatedAt, &k.Disabled, &k.CacheEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, false, nil
	}
	if err != nil {
		return APIKey{}, false, fmt.Errorf("get api_key by hash: %w", err)
	}
	return k, true, nil
}

// KeyUpdate carries optional field changes for UpdateAPIKey. Nil fields are left
// unchanged. Budget cannot be cleared back to NULL through this path (set a value
// or leave it nil to keep the current one).
type KeyUpdate struct {
	Name             *string
	RateLimitRPM     *int
	MonthlyBudgetUSD *float64
	CacheEnabled     *bool
	Disabled         *bool
}

// UpdateAPIKey applies a partial update to a key and returns the new row. found is
// false (with nil error) when no key has that id.
func (s *Store) UpdateAPIKey(ctx context.Context, id string, upd KeyUpdate) (APIKey, bool, error) {
	const q = `
		UPDATE api_keys SET
			name               = COALESCE($2, name),
			rate_limit_rpm     = COALESCE($3, rate_limit_rpm),
			monthly_budget_usd = COALESCE($4, monthly_budget_usd),
			cache_enabled      = COALESCE($5, cache_enabled),
			disabled           = COALESCE($6, disabled)
		WHERE id = $1::uuid
		RETURNING id::text, name, rate_limit_rpm, monthly_budget_usd, created_at, disabled, cache_enabled`
	var k APIKey
	err := s.pool.QueryRow(ctx, q, id, upd.Name, upd.RateLimitRPM, upd.MonthlyBudgetUSD, upd.CacheEnabled, upd.Disabled).
		Scan(&k.ID, &k.Name, &k.RateLimitRPM, &k.MonthlyBudgetUSD, &k.CreatedAt, &k.Disabled, &k.CacheEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, false, nil
	}
	if err != nil {
		return APIKey{}, false, fmt.Errorf("update api_key: %w", err)
	}
	return k, true, nil
}

// DeleteAPIKey removes a key. found is false (with nil error) when no key has that id.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) (bool, error) {
	ct, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1::uuid`, id)
	if err != nil {
		return false, fmt.Errorf("delete api_key: %w", err)
	}
	return ct.RowsAffected() > 0, nil
}

// ListAPIKeys returns all keys (without the hash) for the admin list endpoint.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	const q = `
		SELECT id::text, name, rate_limit_rpm, monthly_budget_usd, created_at, disabled, cache_enabled
		FROM api_keys ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.RateLimitRPM, &k.MonthlyBudgetUSD, &k.CreatedAt, &k.Disabled, &k.CacheEnabled); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
