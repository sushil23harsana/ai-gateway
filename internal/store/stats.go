package store

import (
	"context"
	"fmt"
	"time"
)

// Overview is the headline dashboard summary.
type Overview struct {
	SpendToday     float64 `json:"spend_today_usd"`
	SpendMonth     float64 `json:"spend_month_usd"`
	TotalRequests  int64   `json:"total_requests"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	LatencyP50Ms   int     `json:"latency_p50_ms"`
	LatencyP95Ms   int     `json:"latency_p95_ms"`
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
}

// TimeBucket is one point in the spend/requests timeseries.
type TimeBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Requests  int64     `json:"requests"`
	CostUSD   float64   `json:"cost_usd"`
}

// ModelStat aggregates usage for one model.
type ModelStat struct {
	Model     string  `json:"model"`
	Provider  string  `json:"provider"`
	Requests  int64   `json:"requests"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
}

// ProviderStat aggregates usage for one provider.
type ProviderStat struct {
	Provider string  `json:"provider"`
	Requests int64   `json:"requests"`
	CostUSD  float64 `json:"cost_usd"`
}

// KeyStat is per-virtual-key usage vs budget.
type KeyStat struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	RateLimitRPM     int      `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd,omitempty"`
	Disabled         bool     `json:"disabled"`
	Requests         int64    `json:"requests"`
	MonthCostUSD     float64  `json:"month_cost_usd"`
	TotalCostUSD     float64  `json:"total_cost_usd"`
}

// StatsOverview returns the headline summary. Latency percentiles are computed
// over non-cache-hit rows (cache hits have ~0 latency and would skew them).
func (s *Store) StatsOverview(ctx context.Context) (Overview, error) {
	const q = `
		SELECT
			COALESCE(SUM(cost_usd) FILTER (WHERE created_at >= date_trunc('day', now())), 0),
			COALESCE(SUM(cost_usd) FILTER (WHERE created_at >= date_trunc('month', now())), 0),
			COUNT(*),
			COALESCE(AVG(CASE WHEN cache_hit THEN 1.0 ELSE 0.0 END), 0),
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE NOT cache_hit), 0),
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE NOT cache_hit), 0),
			COALESCE(SUM(tokens_in), 0),
			COALESCE(SUM(tokens_out), 0)
		FROM request_logs`
	var o Overview
	var p50, p95 float64
	err := s.pool.QueryRow(ctx, q).Scan(
		&o.SpendToday, &o.SpendMonth, &o.TotalRequests, &o.CacheHitRate,
		&p50, &p95, &o.TotalTokensIn, &o.TotalTokensOut,
	)
	if err != nil {
		return Overview{}, fmt.Errorf("stats overview: %w", err)
	}
	o.LatencyP50Ms = int(p50)
	o.LatencyP95Ms = int(p95)
	return o, nil
}

// StatsTimeseries buckets requests + spend over a range ("24h", "7d", "30d").
func (s *Store) StatsTimeseries(ctx context.Context, rng string) ([]TimeBucket, error) {
	unit, interval := timeseriesWindow(rng)
	const q = `
		SELECT date_trunc($1, created_at) AS ts, COUNT(*), COALESCE(SUM(cost_usd), 0)
		FROM request_logs
		WHERE created_at >= now() - $2::interval
		GROUP BY 1 ORDER BY 1`
	rows, err := s.pool.Query(ctx, q, unit, interval)
	if err != nil {
		return nil, fmt.Errorf("stats timeseries: %w", err)
	}
	defer rows.Close()

	out := []TimeBucket{}
	for rows.Next() {
		var b TimeBucket
		if err := rows.Scan(&b.Timestamp, &b.Requests, &b.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func timeseriesWindow(rng string) (unit, interval string) {
	switch rng {
	case "7d":
		return "day", "7 days"
	case "30d":
		return "day", "30 days"
	default: // "24h"
		return "hour", "24 hours"
	}
}

// StatsByModel aggregates usage grouped by model + provider, costliest first.
func (s *Store) StatsByModel(ctx context.Context) ([]ModelStat, error) {
	const q = `
		SELECT model, provider, COUNT(*),
			COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COALESCE(SUM(cost_usd), 0)
		FROM request_logs
		GROUP BY model, provider
		ORDER BY SUM(cost_usd) DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("stats by-model: %w", err)
	}
	defer rows.Close()

	out := []ModelStat{}
	for rows.Next() {
		var m ModelStat
		if err := rows.Scan(&m.Model, &m.Provider, &m.Requests, &m.TokensIn, &m.TokensOut, &m.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// StatsByProvider aggregates requests + spend grouped by provider.
func (s *Store) StatsByProvider(ctx context.Context) ([]ProviderStat, error) {
	const q = `
		SELECT provider, COUNT(*), COALESCE(SUM(cost_usd), 0)
		FROM request_logs
		GROUP BY provider
		ORDER BY COUNT(*) DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("stats by-provider: %w", err)
	}
	defer rows.Close()

	out := []ProviderStat{}
	for rows.Next() {
		var p ProviderStat
		if err := rows.Scan(&p.Provider, &p.Requests, &p.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecentRequest is one row for the live-logs view (joined to the key name).
type RecentRequest struct {
	CreatedAt time.Time `json:"created_at"`
	KeyName   *string   `json:"key_name,omitempty"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Status    int       `json:"status"`
	CacheHit  bool      `json:"cache_hit"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
	CostUSD   float64   `json:"cost_usd"`
	LatencyMs int       `json:"latency_ms"`
	Error     *string   `json:"error,omitempty"`
}

// RecentRequests returns the most recent request_logs rows (newest first).
func (s *Store) RecentRequests(ctx context.Context, limit int) ([]RecentRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
		SELECT r.created_at, k.name, r.provider, r.model, r.status, r.cache_hit,
		       r.tokens_in, r.tokens_out, r.cost_usd, r.latency_ms, r.error
		FROM request_logs r
		LEFT JOIN api_keys k ON k.id = r.api_key_id
		ORDER BY r.created_at DESC
		LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("recent requests: %w", err)
	}
	defer rows.Close()

	out := []RecentRequest{}
	for rows.Next() {
		var r RecentRequest
		if err := rows.Scan(&r.CreatedAt, &r.KeyName, &r.Provider, &r.Model, &r.Status, &r.CacheHit,
			&r.TokensIn, &r.TokensOut, &r.CostUSD, &r.LatencyMs, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SemanticEntry summarizes one stored semantic-cache row.
type SemanticEntry struct {
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
}

// CacheStats is the data for the Caches view.
type CacheStats struct {
	SemanticEntries int             `json:"semantic_entries"`
	RecentSemantic  []SemanticEntry `json:"recent_semantic"`
}

// CacheStats returns semantic-cache size + a recent sample.
func (s *Store) CacheStats(ctx context.Context) (CacheStats, error) {
	var cs CacheStats
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM semantic_cache`).Scan(&cs.SemanticEntries); err != nil {
		return CacheStats{}, fmt.Errorf("cache stats count: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT created_at, provider, model, tokens_in, tokens_out
		FROM semantic_cache ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		return CacheStats{}, fmt.Errorf("cache stats recent: %w", err)
	}
	defer rows.Close()
	cs.RecentSemantic = []SemanticEntry{}
	for rows.Next() {
		var e SemanticEntry
		if err := rows.Scan(&e.CreatedAt, &e.Provider, &e.Model, &e.TokensIn, &e.TokensOut); err != nil {
			return CacheStats{}, err
		}
		cs.RecentSemantic = append(cs.RecentSemantic, e)
	}
	return cs, rows.Err()
}

// StatsByKey returns per-key usage vs budget (all keys, even with no traffic).
func (s *Store) StatsByKey(ctx context.Context) ([]KeyStat, error) {
	const q = `
		SELECT k.id::text, k.name, k.rate_limit_rpm, k.monthly_budget_usd, k.disabled,
			COUNT(r.id),
			COALESCE(SUM(r.cost_usd) FILTER (WHERE r.created_at >= date_trunc('month', now())), 0),
			COALESCE(SUM(r.cost_usd), 0)
		FROM api_keys k
		LEFT JOIN request_logs r ON r.api_key_id = k.id
		GROUP BY k.id, k.name, k.rate_limit_rpm, k.monthly_budget_usd, k.disabled
		ORDER BY 7 DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("stats by-key: %w", err)
	}
	defer rows.Close()

	out := []KeyStat{}
	for rows.Next() {
		var k KeyStat
		if err := rows.Scan(&k.ID, &k.Name, &k.RateLimitRPM, &k.MonthlyBudgetUSD, &k.Disabled,
			&k.Requests, &k.MonthCostUSD, &k.TotalCostUSD); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
