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

func (f *fakeCache) Enabled() bool                            { return f.enabled }
func (f *fakeCache) Key(_, _ string, _ []byte) (string, bool) { return "cache:test", true }
func (f *fakeCache) Get(context.Context, string) (*cache.Entry, bool, error) {
	return f.entry, f.hit, nil
}
func (f *fakeCache) Set(_ context.Context, _ string, e cache.Entry) error {
	f.sets = append(f.sets, e)
	return nil
}

// fakeSemantic implements SemanticCache for tests.
type fakeSemantic struct {
	enabled bool
	entry   *cache.Entry
	hit     bool
	stores  int
}

func (f *fakeSemantic) Enabled() bool { return f.enabled }
func (f *fakeSemantic) Lookup(_ context.Context, _, _, _, _ string) (*cache.Entry, []float32, bool, error) {
	return f.entry, []float32{0.1, 0.2, 0.3}, f.hit, nil
}
func (f *fakeSemantic) Store(_ context.Context, _, _, _ string, _ []float32, _ string, _, _ int) error {
	f.stores++
	return nil
}

func disabledSemantic() *fakeSemantic { return &fakeSemantic{enabled: false} }

func oneProviderRouter(baseURL string) *providers.Router {
	oa := providers.NewOpenAI(baseURL, "test-key")
	return providers.NewRouter([]providers.Provider{oa}, testPricing().ProviderMap(), "openai")
}

func testPricing() config.Pricing {
	return config.Pricing{Models: map[string]config.ModelPricing{
		"gpt-4o-mini":      {Provider: "openai", InPer1K: 0.00015, OutPer1K: 0.0006},
		"claude-haiku-4-5": {Provider: "anthropic", InPer1K: 0.001, OutPer1K: 0.005},
	}}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestHandler(client *http.Client, baseURL string, sink LogSink, c ResponseCache) *Handler {
	return NewHandler(client, oneProviderRouter(baseURL), testPricing(), sink, c, disabledSemantic(), FailoverConfig{}, discardLogger())
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
	if math.Abs(got.CostUSD-0.000135) > 1e-9 {
		t.Errorf("cost = %v, want 0.000135", got.CostUSD)
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
	if len(sink.logs) != 1 || !sink.logs[0].CacheHit || sink.logs[0].CostUSD != 0 {
		t.Fatalf("expected 1 cache-hit log with cost 0, got %+v", sink.logs)
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
	if len(fc.sets) != 1 || fc.sets[0].TokensIn != 3 || fc.sets[0].TokensOut != 4 {
		t.Fatalf("expected 1 cache Set with tokens 3/4, got %+v", fc.sets)
	}
}

func TestFailoverOnPrimary5xx(t *testing.T) {
	openaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))
	defer openaiSrv.Close()

	anthropicCalled := false
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anthropicCalled = true
		if got := r.Header.Get("x-api-key"); got != "an-key" {
			t.Errorf("anthropic x-api-key = %q, want an-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-4-5","stop_reason":"end_turn","content":[{"type":"text","text":"fallback hi"}],"usage":{"input_tokens":8,"output_tokens":3}}`)
	}))
	defer anthropicSrv.Close()

	oa := providers.NewOpenAI(openaiSrv.URL, "oa-key")
	an := providers.NewAnthropic(anthropicSrv.URL, "an-key", "2023-06-01", 1024)
	router := providers.NewRouter([]providers.Provider{oa, an}, testPricing().ProviderMap(), "openai")
	sink := &fakeSink{}
	h := NewHandler(&http.Client{}, router, testPricing(), sink, disabledCache(), disabledSemantic(),
		FailoverConfig{Enabled: true, Provider: "anthropic", Model: "claude-haiku-4-5"}, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if !anthropicCalled {
		t.Fatal("fallback provider (anthropic) was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	// Response must be translated to the unified OpenAI shape.
	if !strings.Contains(rec.Body.String(), `"choices"`) || !strings.Contains(rec.Body.String(), "fallback hi") {
		t.Errorf("response not translated to OpenAI shape: %s", rec.Body.String())
	}
	if len(sink.logs) != 2 {
		t.Fatalf("expected 2 log rows (failed primary + fallback), got %d", len(sink.logs))
	}
	if sink.logs[0].Provider != "openai" || sink.logs[0].Status < 500 || sink.logs[0].Error == nil {
		t.Errorf("first log should be the failed openai 5xx attempt: %+v", sink.logs[0])
	}
	if sink.logs[1].Provider != "anthropic" || sink.logs[1].Status != 200 ||
		sink.logs[1].TokensIn != 8 || sink.logs[1].TokensOut != 3 {
		t.Errorf("second log should be the successful anthropic attempt: %+v", sink.logs[1])
	}
}

func TestSemanticHitSkipsUpstream(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	sem := &fakeSemantic{enabled: true, hit: true, entry: &cache.Entry{
		Status: 200, ContentType: "application/json",
		Body: `{"semantic":true}`, Model: "gpt-4o-mini", TokensIn: 4, TokensOut: 6,
	}}
	h := NewHandler(upstream.Client(), oneProviderRouter(upstream.URL), testPricing(), sink, disabledCache(), sem, FailoverConfig{}, discardLogger())

	req := withKey(httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi there"}]}`)))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if upstreamCalled {
		t.Error("semantic hit must NOT call the provider")
	}
	if rec.Header().Get("X-Cache") != "SEMANTIC" {
		t.Errorf("X-Cache = %q, want SEMANTIC", rec.Header().Get("X-Cache"))
	}
	if rec.Body.String() != `{"semantic":true}` {
		t.Errorf("body = %q, want cached semantic body", rec.Body.String())
	}
	if len(sink.logs) != 1 || !sink.logs[0].CacheHit || sink.logs[0].CostUSD != 0 {
		t.Fatalf("expected 1 cache-hit log with cost 0, got %+v", sink.logs)
	}
}

func TestSemanticMissStoresAfterProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"gpt-4o-mini","usage":{"prompt_tokens":5,"completion_tokens":6}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	sem := &fakeSemantic{enabled: true, hit: false}
	h := NewHandler(upstream.Client(), oneProviderRouter(upstream.URL), testPricing(), sink, disabledCache(), sem, FailoverConfig{}, discardLogger())

	req := withKey(httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi there"}]}`)))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if sem.stores != 1 {
		t.Errorf("expected 1 semantic Store on miss, got %d", sem.stores)
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
	if len(sink.logs) != 1 || sink.logs[0].Status != 429 {
		t.Fatalf("expected 1 log with status 429, got %+v", sink.logs)
	}
	if sink.logs[0].Error == nil || !strings.Contains(*sink.logs[0].Error, "rate limit") {
		t.Errorf("expected error body recorded, got %v", sink.logs[0].Error)
	}
}

func TestChatCompletionsMissingKeyReturns500(t *testing.T) {
	sink := &fakeSink{}
	oa := providers.NewOpenAI("http://unused", "") // no key
	router := providers.NewRouter([]providers.Provider{oa}, testPricing().ProviderMap(), "openai")
	h := NewHandler(http.DefaultClient, router, testPricing(), sink, disabledCache(), disabledSemantic(), FailoverConfig{}, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini"}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
