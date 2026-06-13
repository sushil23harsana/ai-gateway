package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// OpenAI is the OpenAI provider. The gateway's unified shape *is* OpenAI's
// chat/completions shape, so its request/response translation is passthrough.
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

func (o *OpenAI) Name() string            { return "openai" }
func (o *OpenAI) APIKey() string          { return o.apiKey }
func (o *OpenAI) SupportsStreaming() bool { return true }

// ChatCompletionsURL is the upstream endpoint the proxy forwards to.
func (o *OpenAI) ChatCompletionsURL() string { return o.baseURL + "/chat/completions" }

// BuildUpstreamRequest forwards the unified body unchanged, injecting the key.
func (o *OpenAI) BuildUpstreamRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.ChatCompletionsURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey) // injected server-side
	return req, nil
}

// TranslateResponse is passthrough for OpenAI; it parses usage on success.
func (o *OpenAI) TranslateResponse(status int, raw []byte) ([]byte, Usage, string, error) {
	if status < 200 || status >= 300 {
		return raw, Usage{}, "", nil
	}
	usage, model, err := o.ParseUsage(raw)
	if err != nil {
		return raw, Usage{}, "", err
	}
	return raw, usage, model, nil
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
