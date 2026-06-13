package providers

import (
	"encoding/json"
	"testing"
)

func TestAnthropicTranslateRequest(t *testing.T) {
	a := NewAnthropic("", "k", "", 555)
	body := []byte(`{
		"model":"claude-haiku-4-5",
		"messages":[
			{"role":"system","content":"be brief"},
			{"role":"user","content":"hi"}
		],
		"temperature":0.4
	}`)

	raw, err := a.translateRequest(body)
	if err != nil {
		t.Fatalf("translateRequest error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["system"] != "be brief" {
		t.Errorf("system = %v, want 'be brief' (lifted to top level)", m["system"])
	}
	if mt, _ := m["max_tokens"].(float64); mt != 555 {
		t.Errorf("max_tokens = %v, want 555 (default backfilled)", m["max_tokens"])
	}
	if temp, _ := m["temperature"].(float64); temp != 0.4 {
		t.Errorf("temperature = %v, want 0.4", m["temperature"])
	}
	msgs, _ := m["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 non-system message, got %d", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "hi" {
		t.Errorf("message = %v", first)
	}
}

func TestAnthropicStripsSamplingForNewModels(t *testing.T) {
	a := NewAnthropic("", "k", "", 100)
	body := []byte(`{"model":"claude-opus-4-8","messages":[{"role":"user","content":"hi"}],"temperature":0.9,"top_p":0.5}`)
	raw, _ := a.translateRequest(body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["temperature"]; ok {
		t.Error("temperature must be stripped for opus-4-8")
	}
	if _, ok := m["top_p"]; ok {
		t.Error("top_p must be stripped for opus-4-8")
	}
}

func TestAnthropicTranslateResponse(t *testing.T) {
	a := NewAnthropic("", "k", "", 100)
	resp := []byte(`{"id":"msg_1","model":"claude-haiku-4-5","stop_reason":"end_turn","content":[{"type":"text","text":"Hello"}],"usage":{"input_tokens":10,"output_tokens":4}}`)

	unified, usage, model, err := a.TranslateResponse(200, resp)
	if err != nil {
		t.Fatalf("TranslateResponse error: %v", err)
	}
	if model != "claude-haiku-4-5" {
		t.Errorf("model = %q", model)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 4 {
		t.Errorf("usage = %+v, want 10/4", usage)
	}
	var oai map[string]any
	if err := json.Unmarshal(unified, &oai); err != nil {
		t.Fatal(err)
	}
	if oai["object"] != "chat.completion" {
		t.Errorf("object = %v, want chat.completion", oai["object"])
	}
	choices := oai["choices"].([]any)
	choice := choices[0].(map[string]any)
	if choice["finish_reason"] != "stop" {
		t.Errorf("finish_reason = %v, want stop", choice["finish_reason"])
	}
	if choice["message"].(map[string]any)["content"] != "Hello" {
		t.Errorf("content = %v, want Hello", choice["message"])
	}
}

func TestAnthropicPassesThroughErrorBody(t *testing.T) {
	a := NewAnthropic("", "k", "", 100)
	errBody := []byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
	out, usage, _, err := a.TranslateResponse(529, errBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(errBody) {
		t.Errorf("error body should pass through unchanged, got %s", out)
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
		t.Errorf("usage should be zero on error, got %+v", usage)
	}
}
