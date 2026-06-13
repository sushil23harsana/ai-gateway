package proxy

import (
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/config"
	"github.com/sushil23harsana/ai-gateway/internal/providers"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

type fakeSink struct{ logs []store.RequestLog }

func (f *fakeSink) Enqueue(rl store.RequestLog) { f.logs = append(f.logs, rl) }

func testPricing() config.Pricing {
	return config.Pricing{Models: map[string]config.ModelPricing{
		"gpt-4o-mini": {Provider: "openai", InPer1K: 0.00015, OutPer1K: 0.0006},
	}}
}

func newTestHandler(client *http.Client, baseURL string, sink LogSink) *Handler {
	oa := providers.NewOpenAI(baseURL, "test-key")
	return NewHandler(client, oa, testPricing(), sink, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	h := newTestHandler(upstream.Client(), upstream.URL, sink)

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
	// 100/1000*0.00015 + 200/1000*0.0006 = 0.000015 + 0.00012 = 0.000135
	wantCost := 0.000135
	if math.Abs(got.CostUSD-wantCost) > 1e-9 {
		t.Errorf("cost = %v, want %v", got.CostUSD, wantCost)
	}
	if got.Error != nil {
		t.Errorf("expected nil error, got %q", *got.Error)
	}
}

func TestChatCompletionsPricesByRequestedAliasWhenSnapshotUnpriced(t *testing.T) {
	// OpenAI commonly resolves an alias ("gpt-4o-mini") to a dated snapshot
	// ("gpt-4o-mini-2024-07-18") that isn't a key in pricing.yaml. Cost must
	// still be computed by falling back to the requested alias.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model":"gpt-4o-mini-2024-07-18","usage":{"prompt_tokens":100,"completion_tokens":200}}`)
	}))
	defer upstream.Close()

	sink := &fakeSink{}
	h := newTestHandler(upstream.Client(), upstream.URL, sink)

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
	wantCost := 0.000135 // priced via the requested alias
	if math.Abs(got.CostUSD-wantCost) > 1e-9 {
		t.Errorf("cost = %v, want %v (alias fallback failed)", got.CostUSD, wantCost)
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
	h := newTestHandler(upstream.Client(), upstream.URL, sink)

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
	// Empty API key.
	oa := providers.NewOpenAI("http://unused", "")
	h := NewHandler(http.DefaultClient, oa, testPricing(), sink, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
