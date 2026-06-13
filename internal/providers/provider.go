// Package providers abstracts upstream LLM APIs. The gateway speaks a single
// unified shape — OpenAI's chat/completions — on both the inbound and outbound
// edge; each Provider translates that to and from its own native API. This is
// what lets one gateway request be served by either OpenAI or Anthropic and
// fail over between them.
package providers

import (
	"context"
	"net/http"
)

// Usage is normalized token accounting parsed from a provider response.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
}

// Provider is one upstream LLM API.
type Provider interface {
	// Name is the identifier stored in request_logs (e.g. "openai", "anthropic").
	Name() string
	// APIKey returns the configured key; empty means this provider isn't usable.
	APIKey() string
	// SupportsStreaming reports whether streaming passthrough works in this phase.
	SupportsStreaming() bool
	// BuildUpstreamRequest builds the native HTTP request for this provider from
	// a unified (OpenAI-shaped) request body — endpoint, auth headers, and a
	// translated body.
	BuildUpstreamRequest(ctx context.Context, unifiedBody []byte) (*http.Request, error)
	// TranslateResponse converts the provider's raw response into a unified
	// (OpenAI chat.completion) body, plus normalized usage and the resolved model.
	// For non-2xx responses it returns the raw body and zero usage.
	TranslateResponse(status int, raw []byte) (unified []byte, usage Usage, model string, err error)
}
