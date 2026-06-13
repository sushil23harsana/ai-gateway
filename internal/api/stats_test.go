package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/metrics"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

type fakeStatsStore struct {
	overview   store.Overview
	timeseries []store.TimeBucket
	byModel    []store.ModelStat
	byProvider []store.ProviderStat
	byKey      []store.KeyStat
	gotRange   string
}

func (f *fakeStatsStore) StatsOverview(context.Context) (store.Overview, error) {
	return f.overview, nil
}
func (f *fakeStatsStore) StatsTimeseries(_ context.Context, rng string) ([]store.TimeBucket, error) {
	f.gotRange = rng
	return f.timeseries, nil
}
func (f *fakeStatsStore) StatsByModel(context.Context) ([]store.ModelStat, error) {
	return f.byModel, nil
}
func (f *fakeStatsStore) StatsByProvider(context.Context) ([]store.ProviderStat, error) {
	return f.byProvider, nil
}
func (f *fakeStatsStore) StatsByKey(context.Context) ([]store.KeyStat, error) { return f.byKey, nil }

type fakeLive struct{ recent []metrics.MinuteCount }

func (f *fakeLive) Recent(context.Context, int) ([]metrics.MinuteCount, error) { return f.recent, nil }

func TestStatsOverview(t *testing.T) {
	fs := &fakeStatsStore{overview: store.Overview{SpendMonth: 1.25, TotalRequests: 42, CacheHitRate: 0.5, LatencyP95Ms: 1800}}
	h := NewStats(fs, &fakeLive{}, discardLogger())

	rec := httptest.NewRecorder()
	h.Overview(rec, httptest.NewRequest(http.MethodGet, "/admin/stats/overview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got store.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalRequests != 42 || got.SpendMonth != 1.25 || got.LatencyP95Ms != 1800 {
		t.Errorf("overview round-trip mismatch: %+v", got)
	}
}

func TestStatsTimeseriesDefaultsTo24h(t *testing.T) {
	fs := &fakeStatsStore{}
	h := NewStats(fs, &fakeLive{}, discardLogger())

	rec := httptest.NewRecorder()
	h.Timeseries(rec, httptest.NewRequest(http.MethodGet, "/admin/stats/timeseries", nil))
	if fs.gotRange != "24h" {
		t.Errorf("default range = %q, want 24h", fs.gotRange)
	}

	rec = httptest.NewRecorder()
	h.Timeseries(rec, httptest.NewRequest(http.MethodGet, "/admin/stats/timeseries?range=7d", nil))
	if fs.gotRange != "7d" {
		t.Errorf("range = %q, want 7d", fs.gotRange)
	}
}

func TestStatsLive(t *testing.T) {
	fl := &fakeLive{recent: []metrics.MinuteCount{{Timestamp: 60, Count: 2}, {Timestamp: 120, Count: 5}}}
	h := NewStats(&fakeStatsStore{}, fl, discardLogger())

	rec := httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodGet, "/admin/stats/live", nil))

	var got struct {
		CurrentPerMinute int `json:"current_per_minute"`
		Recent           []metrics.MinuteCount `json:"recent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CurrentPerMinute != 5 {
		t.Errorf("current_per_minute = %d, want 5 (last bucket)", got.CurrentPerMinute)
	}
	if len(got.Recent) != 2 {
		t.Errorf("recent len = %d, want 2", len(got.Recent))
	}
}
