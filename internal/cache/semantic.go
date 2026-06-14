package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/sushil23harsana/ai-gateway/internal/store"
)

// VectorStore is the pgvector-backed persistence the semantic cache needs.
type VectorStore interface {
	SemanticNearest(ctx context.Context, apiKeyID *string, provider, model string, embedding []float32, maxDistance float64) (*store.SemanticResult, error)
	SemanticInsert(ctx context.Context, apiKeyID *string, provider, model string, embedding []float32, body, contentType string, tokensIn, tokensOut int) error
}

// SemanticCache serves near-duplicate prompts from cache: it embeds the prompt,
// finds the nearest stored response within a cosine-distance threshold, and (on a
// miss) stores the fresh response for next time. Off by default — a too-loose
// threshold can serve a similar-but-different prompt's answer.
type SemanticCache struct {
	store     VectorStore
	embedder  Embedder
	threshold float64
	scope     Scope
	enabled   bool
	log       *slog.Logger
}

// NewSemantic builds the semantic cache. threshold is max cosine distance.
func NewSemantic(vs VectorStore, embedder Embedder, threshold float64, scope string, enabled bool, log *slog.Logger) *SemanticCache {
	s := ScopeKey
	if Scope(scope) == ScopeGlobal {
		s = ScopeGlobal
	}
	if threshold <= 0 {
		threshold = 0.05
	}
	return &SemanticCache{store: vs, embedder: embedder, threshold: threshold, scope: s, enabled: enabled, log: log}
}

// Enabled reports whether semantic caching is on and wired.
func (c *SemanticCache) Enabled() bool {
	return c != nil && c.enabled && c.embedder != nil && c.store != nil
}

func (c *SemanticCache) scopedKey(apiKeyID string) *string {
	if c.scope == ScopeKey && apiKeyID != "" {
		return &apiKeyID
	}
	return nil
}

// Lookup embeds the prompt and returns a near-duplicate cached response if one is
// within the threshold. It also returns the computed embedding so the caller can
// reuse it for Store on a miss (avoiding a second embedding call).
func (c *SemanticCache) Lookup(ctx context.Context, apiKeyID, provider, model, prompt string) (*Entry, []float32, bool, error) {
	emb, err := c.embedder.Embed(ctx, prompt)
	if err != nil {
		return nil, nil, false, err
	}
	res, err := c.store.SemanticNearest(ctx, c.scopedKey(apiKeyID), provider, model, emb, c.threshold)
	if err != nil {
		return nil, emb, false, err
	}
	if res == nil {
		return nil, emb, false, nil
	}
	return &Entry{
		Status:      200,
		ContentType: res.ContentType,
		Body:        res.Body,
		Model:       res.Model,
		TokensIn:    res.TokensIn,
		TokensOut:   res.TokensOut,
	}, emb, true, nil
}

// Store saves a response under a precomputed embedding (from Lookup).
func (c *SemanticCache) Store(ctx context.Context, apiKeyID, provider, model string, embedding []float32, body string, tokensIn, tokensOut int) error {
	return c.store.SemanticInsert(ctx, c.scopedKey(apiKeyID), provider, model, embedding, body, "application/json", tokensIn, tokensOut)
}

// PromptText flattens an OpenAI chat request's messages into a single string to
// embed (role-prefixed, in order).
func PromptText(body []byte) string {
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(messageText(m.Content))
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func messageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}
