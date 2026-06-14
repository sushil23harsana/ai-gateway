package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Embedder turns text into a vector. The semantic cache depends on this small
// interface so it can be faked in tests.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OpenAIEmbedder calls the OpenAI embeddings API (e.g. text-embedding-3-small).
type OpenAIEmbedder struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbedder builds an embedder. baseURL is the OpenAI base (".../v1").
func NewOpenAIEmbedder(baseURL, apiKey, model string, client *http.Client) *OpenAIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAIEmbedder{
		url:    strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/embeddings",
		apiKey: apiKey,
		model:  model,
		client: client,
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": e.model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("embeddings HTTP %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embeddings: empty response")
	}
	return out.Data[0].Embedding, nil
}
