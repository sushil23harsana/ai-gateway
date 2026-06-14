// Package proxy implements the gateway's request lifecycle. It routes a request
// to a provider by model, checks the response cache, and on a miss forwards to
// the provider (translating to/from the provider's native API), relays the
// unified response, records an async request_logs entry, and stores the response
// in the cache. On a primary 5xx/timeout it fails over to a configured fallback
// provider. The response path never blocks on the log write.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
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

// LogSink receives request logs without blocking the caller.
type LogSink interface {
	Enqueue(store.RequestLog)
}

// ResponseCache is the exact-match response-cache behavior the handler needs.
type ResponseCache interface {
	Enabled() bool
	Key(apiKeyID, provider string, body []byte) (string, bool)
	Get(ctx context.Context, key string) (*cache.Entry, bool, error)
	Set(ctx context.Context, key string, e cache.Entry) error
}

// SemanticCache serves near-duplicate prompts from a vector store. Lookup also
// returns the computed embedding so it can be reused by Store on a miss.
type SemanticCache interface {
	Enabled() bool
	Lookup(ctx context.Context, apiKeyID, provider, model, prompt string) (*cache.Entry, []float32, bool, error)
	Store(ctx context.Context, apiKeyID, provider, model string, embedding []float32, body string, tokensIn, tokensOut int) error
}

// Router selects a provider for a model.
type Router interface {
	ProviderFor(model string) (providers.Provider, bool)
	ByName(name string) (providers.Provider, bool)
}

// FailoverConfig controls cross-provider failover on primary 5xx/timeout.
type FailoverConfig struct {
	Enabled  bool
	Provider string // fallback provider name ("" disables failover)
	Model    string // model to use on the fallback provider
}

// Handler routes chat-completions traffic, translates per provider, and logs.
type Handler struct {
	client   *http.Client
	router   Router
	pricing  config.Pricing
	sink     LogSink
	cache    ResponseCache
	semantic SemanticCache
	failover FailoverConfig
	log      *slog.Logger
}

// NewHandler wires the proxy dependencies.
func NewHandler(client *http.Client, router Router, pricing config.Pricing, sink LogSink, c ResponseCache, semantic SemanticCache, failover FailoverConfig, log *slog.Logger) *Handler {
	return &Handler{client: client, router: router, pricing: pricing, sink: sink, cache: c, semantic: semantic, failover: failover, log: log}
}

// ChatCompletions handles POST /v1/chat/completions.
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var apiKeyID *string
	var keyCacheEnabled bool
	if id, ok := keys.IdentityFrom(r.Context()); ok {
		apiKeyID = &id.ID
		keyCacheEnabled = id.CacheEnabled
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &meta)

	primary, ok := h.router.ProviderFor(meta.Model)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "no provider configured to serve this model")
		return
	}
	if primary.APIKey() == "" {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("%s provider not configured on gateway", primary.Name()))
		return
	}

	// Streaming: supported only for providers that can passthrough (OpenAI) in this phase.
	if meta.Stream {
		if !primary.SupportsStreaming() {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("streaming is not supported for the %q provider yet", primary.Name()))
			return
		}
		h.stream(w, r, primary, body, apiKeyID, meta.Model, start)
		return
	}

	// Exact-match cache lookup (scoped by the primary provider).
	var cacheKey string
	if h.cache.Enabled() && keyCacheEnabled {
		if k, ok := h.cache.Key(deref(apiKeyID), primary.Name(), body); ok {
			cacheKey = k
			if entry, hit, gerr := h.cache.Get(r.Context(), cacheKey); gerr != nil {
				h.log.Warn("cache get failed; treating as miss", "err", gerr)
			} else if hit {
				h.serveCached(w, start, apiKeyID, primary.Name(), entry, "HIT")
				return
			}
		}
	}

	// Semantic cache lookup (near-duplicate prompts) — after an exact miss. The
	// embedding is kept so a miss can reuse it for the store below.
	var semEmbedding []float32
	if h.semantic.Enabled() && keyCacheEnabled {
		if prompt := cache.PromptText(body); prompt != "" {
			entry, emb, hit, serr := h.semantic.Lookup(r.Context(), deref(apiKeyID), primary.Name(), meta.Model, prompt)
			if serr != nil {
				h.log.Warn("semantic lookup failed; treating as miss", "err", serr)
			} else if hit {
				h.serveCached(w, start, apiKeyID, primary.Name(), entry, "SEMANTIC")
				return
			} else {
				semEmbedding = emb
			}
		}
	}

	// Primary attempt.
	used := primary
	sentModel := meta.Model
	status, raw, terr := h.doUpstream(r.Context(), primary, body)

	// Failover on transport error or 5xx.
	if (terr != nil || status >= 500) && h.failover.Enabled && h.failover.Provider != "" && h.failover.Provider != primary.Name() {
		if fb, ok := h.router.ByName(h.failover.Provider); ok && fb.APIKey() != "" {
			h.enqueueFailure(apiKeyID, primary.Name(), meta.Model, status, terr, raw, start)
			h.log.Warn("primary provider failed; failing over",
				"primary", primary.Name(), "fallback", fb.Name(), "status", status, "err", terr)

			fbModel := h.failover.Model
			if fbModel == "" {
				fbModel = meta.Model
			}
			used = fb
			sentModel = fbModel
			status, raw, terr = h.doUpstream(r.Context(), fb, rewriteModel(body, fbModel))
		}
	}

	if terr != nil {
		h.fail(w, start, apiKeyID, used.Name(), sentModel, http.StatusBadGateway, terr)
		return
	}

	unified, usage, model, perr := used.TranslateResponse(status, raw)
	if perr != nil {
		h.log.Warn("response translation failed; relaying raw upstream body", "provider", used.Name(), "err", perr)
		unified = raw
	}
	latency := time.Since(start)

	// Relay the unified response.
	w.Header().Set("Content-Type", "application/json")
	if cacheKey != "" {
		w.Header().Set("X-Cache", "MISS")
	}
	w.WriteHeader(status)
	_, _ = w.Write(unified)

	// Cost: price by the resolved model, then the model we sent to the provider.
	resolved := model
	if resolved == "" {
		resolved = sentModel
	}
	cost, priced := h.pricing.Cost(resolved, usage.PromptTokens, usage.CompletionTokens)
	if !priced && sentModel != "" && sentModel != resolved {
		cost, priced = h.pricing.Cost(sentModel, usage.PromptTokens, usage.CompletionTokens)
	}
	if !priced && resolved != "" {
		h.log.Warn("model not found in pricing.yaml; cost recorded as 0", "model", resolved, "sent", sentModel)
	}

	rl := store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  used.Name(),
		Model:     resolved,
		Status:    status,
		TokensIn:  usage.PromptTokens,
		TokensOut: usage.CompletionTokens,
		CostUSD:   cost,
		LatencyMs: int(latency.Milliseconds()),
	}
	if status >= 400 {
		e := truncate(string(raw), maxErrorBytes)
		rl.Error = &e
	}
	h.sink.Enqueue(rl)

	// Store successful responses (under the primary-scoped cache key).
	if cacheKey != "" && status >= 200 && status < 300 {
		if serr := h.cache.Set(r.Context(), cacheKey, cache.Entry{
			Status:      status,
			ContentType: "application/json",
			Body:        string(unified),
			Model:       resolved,
			TokensIn:    usage.PromptTokens,
			TokensOut:   usage.CompletionTokens,
		}); serr != nil {
			h.log.Warn("cache set failed", "err", serr)
		}
	}

	// Store semantically too (reusing the lookup embedding) — primary path only.
	if len(semEmbedding) > 0 && used.Name() == primary.Name() && status >= 200 && status < 300 {
		if serr := h.semantic.Store(r.Context(), deref(apiKeyID), primary.Name(), meta.Model, semEmbedding, string(unified), usage.PromptTokens, usage.CompletionTokens); serr != nil {
			h.log.Warn("semantic store failed", "err", serr)
		}
	}
}

// doUpstream builds and executes a provider request, returning status, raw body,
// and a transport error (status 0 on transport failure).
func (h *Handler) doUpstream(ctx context.Context, p providers.Provider, body []byte) (int, []byte, error) {
	req, err := p.BuildUpstreamRequest(ctx, body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// stream does an OpenAI-style streaming passthrough (no failover, no caching).
func (h *Handler) stream(w http.ResponseWriter, r *http.Request, p providers.Provider, body []byte, apiKeyID *string, model string, start time.Time) {
	req, err := p.BuildUpstreamRequest(r.Context(), body)
	if err != nil {
		h.fail(w, start, apiKeyID, p.Name(), model, http.StatusInternalServerError, err)
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.fail(w, start, apiKeyID, p.Name(), model, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

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
		Provider:  p.Name(),
		Model:     model,
		Status:    resp.StatusCode,
		LatencyMs: int(time.Since(start).Milliseconds()),
	})
}

// serveCached returns a cached response and logs a cache hit (cost 0). xcache is
// the X-Cache header value: "HIT" (exact) or "SEMANTIC" (near-duplicate).
func (h *Handler) serveCached(w http.ResponseWriter, start time.Time, apiKeyID *string, provider string, e *cache.Entry, xcache string) {
	ct := e.ContentType
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Cache", xcache)
	w.WriteHeader(e.Status)
	_, _ = io.WriteString(w, e.Body)

	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  provider,
		Model:     e.Model,
		Status:    e.Status,
		CacheHit:  true,
		TokensIn:  e.TokensIn,
		TokensOut: e.TokensOut,
		CostUSD:   0,
		LatencyMs: int(time.Since(start).Milliseconds()),
	})
}

// enqueueFailure logs a failed (pre-failover) upstream attempt.
func (h *Handler) enqueueFailure(apiKeyID *string, provider, model string, status int, terr error, raw []byte, start time.Time) {
	logStatus := status
	if logStatus == 0 {
		logStatus = http.StatusBadGateway
	}
	var msg string
	if terr != nil {
		msg = terr.Error()
	} else {
		msg = truncate(string(raw), maxErrorBytes)
	}
	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  provider,
		Model:     model,
		Status:    logStatus,
		LatencyMs: int(time.Since(start).Milliseconds()),
		Error:     &msg,
	})
}

// fail records a failed request and returns a JSON error to the client.
func (h *Handler) fail(w http.ResponseWriter, start time.Time, apiKeyID *string, provider, model string, status int, cause error) {
	h.log.Error("proxy error", "provider", provider, "status", status, "err", cause)
	msg := cause.Error()
	h.sink.Enqueue(store.RequestLog{
		APIKeyID:  apiKeyID,
		Provider:  provider,
		Model:     model,
		Status:    status,
		LatencyMs: int(time.Since(start).Milliseconds()),
		Error:     &msg,
	})
	writeJSONError(w, status, msg)
}

// rewriteModel returns body with its "model" field set to model (for failover).
func rewriteModel(body []byte, model string) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m["model"] = model
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
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
