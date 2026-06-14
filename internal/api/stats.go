package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sushil23harsana/ai-gateway/internal/metrics"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

// StatsStore is the slice of the store the dashboard endpoints read from.
type StatsStore interface {
	StatsOverview(ctx context.Context) (store.Overview, error)
	StatsTimeseries(ctx context.Context, rng string) ([]store.TimeBucket, error)
	StatsByModel(ctx context.Context) ([]store.ModelStat, error)
	StatsByProvider(ctx context.Context) ([]store.ProviderStat, error)
	StatsByKey(ctx context.Context) ([]store.KeyStat, error)
	RecentRequests(ctx context.Context, limit int) ([]store.RecentRequest, error)
	CacheStats(ctx context.Context) (store.CacheStats, error)
}

// LiveReader exposes recent per-minute request counts.
type LiveReader interface {
	Recent(ctx context.Context, n int) ([]metrics.MinuteCount, error)
}

// Stats serves the GET /admin/stats/* dashboard endpoints.
type Stats struct {
	store StatsStore
	live  LiveReader
	log   *slog.Logger
}

// NewStats wires the stats handler.
func NewStats(s StatsStore, live LiveReader, log *slog.Logger) *Stats {
	return &Stats{store: s, live: live, log: log}
}

// Overview handles GET /admin/stats/overview.
func (h *Stats) Overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.store.StatsOverview(r.Context())
	if err != nil {
		h.fail(w, "overview", err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// Timeseries handles GET /admin/stats/timeseries?range=24h|7d|30d.
func (h *Stats) Timeseries(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}
	buckets, err := h.store.StatsTimeseries(r.Context(), rng)
	if err != nil {
		h.fail(w, "timeseries", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": rng, "buckets": buckets})
}

// ByModel handles GET /admin/stats/by-model.
func (h *Stats) ByModel(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.StatsByModel(r.Context())
	if err != nil {
		h.fail(w, "by-model", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": rows})
}

// ByProvider handles GET /admin/stats/by-provider.
func (h *Stats) ByProvider(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.StatsByProvider(r.Context())
	if err != nil {
		h.fail(w, "by-provider", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": rows})
}

// ByKey handles GET /admin/stats/by-key.
func (h *Stats) ByKey(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.StatsByKey(r.Context())
	if err != nil {
		h.fail(w, "by-key", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": rows})
}

// Recent handles GET /admin/stats/recent?limit=N (live logs).
func (h *Stats) Recent(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := h.store.RecentRequests(r.Context(), limit)
	if err != nil {
		h.fail(w, "recent", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": rows})
}

// Cache handles GET /admin/stats/cache (semantic-cache size + recent).
func (h *Stats) Cache(w http.ResponseWriter, r *http.Request) {
	cs, err := h.store.CacheStats(r.Context())
	if err != nil {
		h.fail(w, "cache", err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

// Live handles GET /admin/stats/live (requests/min from Redis).
func (h *Stats) Live(w http.ResponseWriter, r *http.Request) {
	recent, err := h.live.Recent(r.Context(), 15)
	if err != nil {
		h.fail(w, "live", err)
		return
	}
	current := 0
	if n := len(recent); n > 0 {
		current = recent[n-1].Count
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current_per_minute": current,
		"recent":             recent,
	})
}

func (h *Stats) fail(w http.ResponseWriter, name string, err error) {
	h.log.Error("stats query failed", "endpoint", name, "err", err)
	writeError(w, http.StatusInternalServerError, "failed to compute "+name+" stats")
}
