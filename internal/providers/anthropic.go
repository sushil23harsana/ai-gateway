package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Anthropic is the native Anthropic Messages-API provider. It translates the
// unified OpenAI chat/completions shape to /v1/messages and back.
type Anthropic struct {
	baseURL          string
	apiKey           string
	version          string
	defaultMaxTokens int
}

// NewAnthropic builds an Anthropic provider. version is the anthropic-version
// header; defaultMaxTokens backfills max_tokens (which Anthropic requires).
func NewAnthropic(baseURL, apiKey, version string, defaultMaxTokens int) *Anthropic {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	if version == "" {
		version = "2023-06-01"
	}
	if defaultMaxTokens <= 0 {
		defaultMaxTokens = 4096
	}
	return &Anthropic{baseURL: baseURL, apiKey: apiKey, version: version, defaultMaxTokens: defaultMaxTokens}
}

func (a *Anthropic) Name() string            { return "anthropic" }
func (a *Anthropic) APIKey() string          { return a.apiKey }
func (a *Anthropic) SupportsStreaming() bool { return false } // SSE translation is Phase 6

// MessagesURL is the Anthropic Messages endpoint.
func (a *Anthropic) MessagesURL() string { return a.baseURL + "/v1/messages" }

// BuildUpstreamRequest translates the unified body to the Messages API shape and
// sets Anthropic's auth headers (x-api-key + anthropic-version).
func (a *Anthropic) BuildUpstreamRequest(ctx context.Context, unifiedBody []byte) (*http.Request, error) {
	body, err := a.translateRequest(unifiedBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.MessagesURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey) // injected server-side
	req.Header.Set("anthropic-version", a.version)
	return req, nil
}

type oaiRequest struct {
	Model               string `json:"model"`
	Messages            []oaiMessage `json:"messages"`
	MaxTokens           *int     `json:"max_tokens"`
	MaxCompletionTokens *int     `json:"max_completion_tokens"`
	Temperature         *float64 `json:"temperature"`
	TopP                *float64 `json:"top_p"`
	Stop                json.RawMessage `json:"stop"`
}

type oaiMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// translateRequest maps OpenAI chat/completions → Anthropic Messages.
//
// Handles the common text-chat path: system messages are lifted to the top-level
// `system` field, max_tokens is backfilled (Anthropic requires it), and sampling
// params are forwarded except to models that reject them. Tools and multimodal
// content are out of scope for this phase.
func (a *Anthropic) translateRequest(raw []byte) ([]byte, error) {
	var in oaiRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse unified request: %w", err)
	}

	var systemParts []string
	msgs := make([]map[string]any, 0, len(in.Messages))
	for _, m := range in.Messages {
		text := extractText(m.Content)
		if m.Role == "system" {
			if text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, map[string]any{"role": role, "content": text})
	}

	maxTokens := a.defaultMaxTokens
	if in.MaxTokens != nil {
		maxTokens = *in.MaxTokens
	} else if in.MaxCompletionTokens != nil {
		maxTokens = *in.MaxCompletionTokens
	}

	out := map[string]any{
		"model":      in.Model,
		"max_tokens": maxTokens,
		"messages":   msgs,
	}
	if len(systemParts) > 0 {
		out["system"] = strings.Join(systemParts, "\n\n")
	}
	if !rejectsSampling(in.Model) {
		if in.Temperature != nil {
			out["temperature"] = *in.Temperature
		}
		if in.TopP != nil {
			out["top_p"] = *in.TopP
		}
	}
	if seqs := normalizeStop(in.Stop); len(seqs) > 0 {
		out["stop_sequences"] = seqs
	}
	return json.Marshal(out)
}

// TranslateResponse maps an Anthropic Messages response → OpenAI chat.completion.
func (a *Anthropic) TranslateResponse(status int, raw []byte) ([]byte, Usage, string, error) {
	if status < 200 || status >= 300 {
		return raw, Usage{}, "", nil // pass the provider's error body through
	}

	var ar struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &ar); err != nil {
		return raw, Usage{}, "", fmt.Errorf("parse anthropic response: %w", err)
	}

	var text strings.Builder
	for _, c := range ar.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}

	// Anthropic's input_tokens excludes cached tokens; sum for total prompt tokens.
	usage := Usage{
		PromptTokens:     ar.Usage.InputTokens + ar.Usage.CacheReadInputTokens + ar.Usage.CacheCreationInputTokens,
		CompletionTokens: ar.Usage.OutputTokens,
		CachedTokens:     ar.Usage.CacheReadInputTokens,
	}

	unified := map[string]any{
		"id":      ar.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ar.Model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": text.String()},
				"finish_reason": mapStopReason(ar.StopReason),
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.PromptTokens + usage.CompletionTokens,
		},
	}
	out, err := json.Marshal(unified)
	if err != nil {
		return nil, Usage{}, "", err
	}
	return out, usage, ar.Model, nil
}

// extractText pulls plain text out of an OpenAI message content value, which may
// be a string or an array of typed parts.
func extractText(raw json.RawMessage) string {
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

// normalizeStop maps OpenAI `stop` (string or []string) to Anthropic stop_sequences.
func normalizeStop(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

// rejectsSampling reports models that 400 on temperature/top_p (Opus 4.7/4.8, Fable).
func rejectsSampling(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "opus-4-7") || strings.Contains(m, "opus-4-8") || strings.Contains(m, "fable")
}

// mapStopReason maps Anthropic stop_reason → OpenAI finish_reason.
func mapStopReason(r string) string {
	switch r {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	default:
		return "stop"
	}
}
