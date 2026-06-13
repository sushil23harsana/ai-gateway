package proxy

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/cache"
	"github.com/sushil23harsana/ai-gateway/internal/config"
	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/providers"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

type fakeSink struct{ logs []store.RequestLog }

func (f *fakeSink) Enqueue(rl store.RequestLog) { f.logs = append(f.logs, rl) }

// fakeCache implements ResponseCache for tests.
type fakeCache struct {
	enabled bool
	entry   *cache.Entry
	hit     bool
	sets    []cache.Entry
}

func (f *fakeCache) Enabled() bool                                 { return f.enabled }
func (f *fakeCache) Key(_, _ string, _ []byte) (string, bool)      { return "cache:test", true }
func (f *fakeCache) Get(context.Context, string) (*cache.Entry, bool, error) {
	return f.entry, f.hit, nil
}
func (f *fakeCache) Set(_ context.Context, _ string, e cache.Entry) error {
	f.sets = append(f.sets, e)
	return nil
}

func testPricing() config.Pricing {
	return config.Pricing{Models: map[string]config.ModelPricing{
		"gpt-4o-mini": {Provider: "openai", InPer1K: 0.00015, OutPer1K: 0.0006},
	}}
}

func newTestHandler(client *http.Client, baseURL string, sink LogSink, c ResponseCache) *Handler {
	oa := providers.NewOpenAI(baseURL, "test-key")
	return NewHandler(client, oa, testPricing(), sink, c, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func disabledCache() *fakeCache { return &fakeCache{enabled: false} }

func withKey(req *http.Request) *http.Request {
	return req.WithContext(keys.WithIdentity(req.Context(), keys.Identity{ID: "k1", CacheEnabled: true}))
}

func TestChatCompletionsProxiesRelaysAndLogs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("upstream Authorization = %q, want Bearer test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"gpt-4o-mini","usage":{"prompt_tokens":100,"completion_tokens":200}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	h := newTestHandler(upstream.Client(), upstream.URL, sink, disabledCache())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"prompt_tokens":100`) {
		t.Errorf("response body not relayed verbatim: %s", rec.Body.String())
	}

	if len(sink.logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(sink.logs))
	}
	got := sink.logs[0]
	if got.Provider != "openai" || got.Model != "gpt-4o-mini" {
		t.Errorf("log provider/model = %q/%q", got.Provider, got.Model)
	}
	if got.Status != 200 || got.TokensIn != 100 || got.TokensOut != 200 {
		t.Errorf("log status/tokens = %d / %d / %d", got.Status, got.TokensIn, got.TokensOut)
	}
	wantCost := 0.000135
	if math.Abs(got.CostUSD-wantCost) > 1e-9 {
		t.Errorf("cost = %v, want %v", got.CostUSD, wantCost)
	}
	if got.Error != nil {
		t.Errorf("expected nil error, got %q", *got.Error)
	}
}

func TestChatCompletionsPricesByRequestedAliasWhenSnapshotUnpriced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"gpt-4o-mini-2024-07-18","usage":{"prompt_tokens":100,"completion_tokens":200}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	h := newTestHandler(upstream.Client(), upstream.URL, sink, disabledCache())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[]}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if len(sink.logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(sink.logs))
	}
	got := sink.logs[0]
	if got.Model != "gpt-4o-mini-2024-07-18" {
		t.Errorf("logged model = %q, want the resolved snapshot", got.Model)
	}
	wantCost := 0.000135
	if math.Abs(got.CostUSD-wantCost) > 1e-9 {
		t.Errorf("cost = %v, want %v (alias fallback failed)", got.CostUSD, wantCost)
	}
}

func TestCacheHitSkipsUpstream(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	fc := &fakeCache{enabled: true, hit: true, entry: &cache.Entry{
		Status: 200, ContentType: "application/json",
		Body: `{"cached":true}`, Model: "gpt-4o-mini", TokensIn: 5, TokensOut: 7,
	}}
	h := newTestHandler(upstream.Client(), upstream.URL, sink, fc)

	req := withKey(httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[]}`)))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if upstreamCalled {
		t.Error("cache hit must NOT call the upstream provider")
	}
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Errorf("X-Cache = %q, want HIT", rec.Header().Get("X-Cache"))
	}
	if rec.Body.String() != `{"cached":true}` {
		t.Errorf("body = %q, want cached body", rec.Body.String())
	}
	if len(sink.logs) != 1 || !sink.logs[0].CacheHit {
		t.Fatalf("expected 1 log with CacheHit=true, got %+v", sink.logs)
	}
	if sink.logs[0].CostUSD != 0 {
		t.Errorf("cache-hit cost = %v, want 0", sink.logs[0].CostUSD)
	}
}

func TestCacheMissStoresResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"gpt-4o-mini","usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	fc := &fakeCache{enabled: true, hit: false}
	h := newTestHandler(upstream.Client(), upstream.URL, sink, fc)

	req := withKey(httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[]}`)))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Header().Get("X-Cache") != "MISS" {
		t.Errorf("X-Cache = %q, want MISS", rec.Header().Get("X-Cache"))
	}
	if len(fc.sets) != 1 {
		t.Fatalf("expected 1 cache Set on miss, got %d", len(fc.sets))
	}
	if fc.sets[0].TokensIn != 3 || fc.sets[0].TokensOut != 4 {
		t.Errorf("stored tokens = %d/%d, want 3/4", fc.sets[0].TokensIn, fc.sets[0].TokensOut)
	}
	if len(sink.logs) != 1 || sink.logs[0].CacheHit {
		t.Errorf("expected 1 log with CacheHit=false")
	}
}

func TestChatCompletionsLogsUpstreamErrorBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limit"}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	h := newTestHandler(upstream.Client(), upstream.URL, sink, disabledCache())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini"}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if len(sink.logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(sink.logs))
	}
	got := sink.logs[0]
	if got.Status != 429 {
		t.Errorf("log status = %d, want 429", got.Status)
	}
	if got.Error == nil || !strings.Contains(*got.Error, "rate limit") {
		t.Errorf("expected error body recorded, got %v", got.Error)
	}
}

func TestChatCompletionsMissingKeyReturns500(t *testing.T) {
	sink := &fakeSink{}
	oa := providers.NewOpenAI("http://unused", "")
	h := NewHandler(http.DefaultClient, oa, testPricing(), sink, disabledCache(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
