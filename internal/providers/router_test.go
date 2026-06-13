package providers

import "testing"

func TestRouterProviderFor(t *testing.T) {
	oa := NewOpenAI("", "k1")
	an := NewAnthropic("", "k2", "", 100)
	r := NewRouter(
		[]Provider{oa, an},
		map[string]string{"gpt-4o-mini": "openai", "claude-haiku-4-5": "anthropic"},
		"openai",
	)

	cases := map[string]string{
		"gpt-4o-mini":      "openai",    // explicit pricing map
		"claude-haiku-4-5": "anthropic", // explicit pricing map
		"claude-3-opus":    "anthropic", // prefix heuristic
		"gpt-5":            "openai",    // prefix heuristic
		"o3-mini":          "openai",    // prefix heuristic
		"mystery-model":    "openai",    // default provider
	}
	for model, want := range cases {
		p, ok := r.ProviderFor(model)
		if !ok {
			t.Fatalf("no provider for %q", model)
		}
		if p.Name() != want {
			t.Errorf("ProviderFor(%q) = %q, want %q", model, p.Name(), want)
		}
	}
}

func TestRouterByName(t *testing.T) {
	oa := NewOpenAI("", "k1")
	r := NewRouter([]Provider{oa}, nil, "openai")
	if _, ok := r.ByName("openai"); !ok {
		t.Error("expected openai to be registered")
	}
	if _, ok := r.ByName("anthropic"); ok {
		t.Error("anthropic should not be registered")
	}
}
