// Package proxy implements the gateway's request lifecycle. It checks the
// response cache, and on a miss forwards POST /v1/chat/completions to OpenAI
// (injecting the real key server-side), relays the response, records an async
// request_logs entry, and stores the response in the cache. The response path
// never blocks on the log write.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sushil23harsana/ai-gateway/internal/cache"
	"github.com/sushil23harsana/ai-gateway/internal/config"
	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/providers"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

const (
	maxRequestBytes = 10 << 20 // 10 MiB cap on inbound request bodies
	maxErrorBytes   = 500      // how much of an upstream error body to record
)

// LogSink receives request logs without blocking the caller. *metrics.Logger
// satisfies it; tests use a fake.
type LogSink interface {
	Enqueue(store.RequestLog)
}

// ResponseCache is the response-cache behavior the handler needs. *cache.Cache
// satisfies it; tests use a fake.
type ResponseCache interface {
	Enabled() bool
	Key(apiKeyID, provider string, body []byte) (string, bool)
	Get(ctx context.Context, key string) (*cache.Entry, bool, error)
	Set(ctx context.Context, key string, e cache.Entry) error
}

// Handler forwards chat-completions traffic to OpenAI and logs the outcome.
type Handler struct {
	client  *http.Client
	openai  *providers.OpenAI
	pricing config.Pricing
	sink    LogSink
	cache   ResponseCache
	log     *slog.Logger
}

// NewHandler wires the proxy dependencies.
func NewHandler(client *http.Client, openai *providers.OpenAI, pricing config.Pricing, sink LogSink, c ResponseCache, log *slog.Logger) *Handler {
	return &Handler{client: client, openai: openai, pricing: pricing, sink: sink, cache: c, log: log}
}

// ChatCompletions handles POST /v1/chat/completions.
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Authenticated virtual key (set by the auth middleware) → request_logs.api_key_id.
	var apiKeyID *string
	var keyCacheEnabled bool
	if id, ok := keys.IdentityFrom(r.Context()); ok {
		apiKeyID = &id.ID
		keyCacheEnabled = id.CacheEnabled
	}

	if h.openai.APIKey() == "" {
		writeJSONError(w, http.StatusInternalServerError, "OPENAI_API_KEY not configured on gateway")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Peek at model + stream flag; keep the raw body for verbatim forwarding.
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &meta)

	// Cache lookup (non-streaming, caching enabled globally and for this key).
	var cacheKey string
	if h.cache.Enabled() && keyCacheEnabled && !meta.Stream {
		if k, ok := h.cache.Key(deref(apiKeyID), h.openai.Name(), body); ok {
			cacheKey = k
			if entry, hit, gerr := h.cache.Get(r.Context(), cacheKey); gerr != nil {
				h.log.Warn("cache get failed; treating as miss", "err", gerr)
			} else if hit {
				h.serveFromCache(w, start, apiKeyID, entry)
				return
			}
		}
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.openai.ChatCompletionsURL(), bytes.NewReader(body))
	if err != nil {
		h.fail(w, start, apiKeyID, meta.Model, http.StatusInternalServerError, err)
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+h.openai.APIKey()) // injected server-side
	if meta.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		h.fail(w, start, apiKeyID, meta.Model, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

	// Streaming (SSE): pass through; mid-stream token accounting + caching is Phase 6.
	if meta.Stream {
		h.relayStream(w, resp, start, apiKeyID, meta.Model)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.fail(w, start, apiKeyID, meta.Model, http.StatusBadGateway, err)
		return
	}
	latency := time.Since(start)

	// Relay the upstream response verbatim.
	ct := contentTypeOr(resp, "application/json")
	w.Header().Set("Content-Type", ct)
	if cacheKey != "" {
		w.Header().Set("X-Cache", "MISS")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)

	// Parse usage, compute cost, and log — all off the response path.
	model := meta.Model
	var tokensIn, tokensOut int
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if usage, m, perr := h.openai.ParseUsage(respBody); perr == nil {
			tokensIn, tokensOut = usage.PromptTokens, usage.CompletionTokens
			if m != "" {
				model = m
			}
		} else {
			h.log.Warn("could not parse usage from openai response", "err", perr)
		}
	}

	// Price by the resolved model (e.g. "gpt-4o-mini-2024-07-18"); if that exact
	// snapshot isn't priced, fall back to the requested alias ("gpt-4o-mini").
	cost, priced := h.pricing.Cost(model, tokensIn, tokensOut)
	if !priced && meta.Model != "" && meta.Model != model {
		cost, priced = h.pricing.Cost(meta.Model, tokensIn, tokensOut)
	}
	if !priced && model != "" {
		h.log.Warn("model not found in pricing.yaml; cost recorded as 0", "model", model, "requested", meta.Model)
	}

	rl := store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  h.openai.Name(),
		Model:     model,
		Status:    resp.StatusCode,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		CostUSD:   cost,
		LatencyMs: int(latency.Milliseconds()),
	}
	if resp.StatusCode >= 400 {
		e := truncate(string(respBody), maxErrorBytes)
		rl.Error = &e
	}
	h.sink.Enqueue(rl)

	// Store successful responses in the cache (after the client already has the body).
	if cacheKey != "" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if serr := h.cache.Set(r.Context(), cacheKey, cache.Entry{
			Status:      resp.StatusCode,
			ContentType: ct,
			Body:        string(respBody),
			Model:       model,
			TokensIn:    tokensIn,
			TokensOut:   tokensOut,
		}); serr != nil {
			h.log.Warn("cache set failed", "err", serr)
		}
	}
}

// serveFromCache returns a cached response and logs a cache hit (cost 0 — no
// provider call was made).
func (h *Handler) serveFromCache(w http.ResponseWriter, start time.Time, apiKeyID *string, e *cache.Entry) {
	ct := e.ContentType
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(e.Status)
	_, _ = io.WriteString(w, e.Body)

	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  h.openai.Name(),
		Model:     e.Model,
		Status:    e.Status,
		CacheHit:  true,
		TokensIn:  e.TokensIn,
		TokensOut: e.TokensOut,
		CostUSD:   0, // served from cache — no provider charge
		LatencyMs: int(time.Since(start).Milliseconds()),
	})
}

// relayStream copies an SSE response straight through to the client, flushing as
// it goes, and logs status + latency. Token accounting for streams is deferred.
func (h *Handler) relayStream(w http.ResponseWriter, resp *http.Response, start time.Time, apiKeyID *string, model string) {
	w.Header().Set("Content-Type", contentTypeOr(resp, "text/event-stream"))
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			break
		}
	}

	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  h.openai.Name(),
		Model:     model,
		Status:    resp.StatusCode,
		LatencyMs: int(time.Since(start).Milliseconds()),
	})
}

// fail records a failed request and returns a JSON error to the client.
func (h *Handler) fail(w http.ResponseWriter, start time.Time, apiKeyID *string, model string, status int, cause error) {
	h.log.Error("proxy error", "status", status, "err", cause)
	msg := cause.Error()
	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  h.openai.Name(),
		Model:     model,
		Status:    status,
		LatencyMs: int(time.Since(start).Milliseconds()),
		Error:     &msg,
	})
	writeJSONError(w, status, msg)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func contentTypeOr(resp *http.Response, fallback string) string {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return fallback
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
