package providers

import "testing"

func TestParseUsage(t *testing.T) {
	oa := NewOpenAI("", "")
	body := []byte(`{
		"model": "gpt-4o-mini",
		"choices": [{"message": {"role": "assistant", "content": "hi"}}],
		"usage": {
			"prompt_tokens": 11,
			"completion_tokens": 22,
			"prompt_tokens_details": {"cached_tokens": 8}
		}
	}`)

	usage, model, err := oa.ParseUsage(body)
	if err != nil {
		t.Fatalf("ParseUsage() error: %v", err)
	}
	if model != "gpt-4o-mini" {
		t.Errorf("model = %q, want gpt-4o-mini", model)
	}
	if usage.PromptTokens != 11 || usage.CompletionTokens != 22 {
		t.Errorf("tokens = %d/%d, want 11/22", usage.PromptTokens, usage.CompletionTokens)
	}
	if usage.CachedTokens != 8 {
		t.Errorf("cached = %d, want 8", usage.CachedTokens)
	}
}

func TestChatCompletionsURL(t *testing.T) {
	cases := map[string]string{
		"":                          "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1/": "https://api.openai.com/v1/chat/completions",
		"http://localhost:1234/v1":   "http://localhost:1234/v1/chat/completions",
	}
	for in, want := range cases {
		if got := NewOpenAI(in, "k").ChatCompletionsURL(); got != want {
			t.Errorf("NewOpenAI(%q).ChatCompletionsURL() = %q, want %q", in, got, want)
		}
	}
}
