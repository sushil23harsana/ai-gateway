// Package providers holds per-provider specifics: endpoints, auth, and how to
// read token usage out of a response. Phase 1 ships only OpenAI; Phase 4 will
// extract a shared Provider interface and add Anthropic.
package providers

import (
	"encoding/json"
	"strings"
)

// OpenAI describes how to reach the OpenAI chat/completions endpoint and parse
// its responses. The API key is held here but never logged or returned.
type OpenAI struct {
	baseURL string
	apiKey  string
}

// NewOpenAI builds an OpenAI provider. baseURL falls back to the public API.
func NewOpenAI(baseURL, apiKey string) *OpenAI {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAI{baseURL: baseURL, apiKey: apiKey}
}

// Name is the provider identifier stored in request_logs.
func (o *OpenAI) Name() string { return "openai" }

// APIKey returns the configured key (used to set the upstream Authorization
// header server-side). Empty means the gateway is not configured for OpenAI.
func (o *OpenAI) APIKey() string { return o.apiKey }

// ChatCompletionsURL is the upstream endpoint the proxy forwards to.
func (o *OpenAI) ChatCompletionsURL() string { return o.baseURL + "/chat/completions" }

// Usage is the token accounting parsed from a chat/completions response.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int // OpenAI prompt-cache hits (usage.prompt_tokens_details.cached_tokens)
}

// ParseUsage extracts token usage and the resolved model from an OpenAI
// chat/completions JSON response body.
func (o *OpenAI) ParseUsage(body []byte) (usage Usage, model string, err error) {
	var resp struct {
		Model string `json:"model"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err = json.Unmarshal(body, &resp); err != nil {
		return Usage{}, "", err
	}
	return Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		CachedTokens:     resp.Usage.PromptTokensDetails.CachedTokens,
	}, resp.Model, nil
}
