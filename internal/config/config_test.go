package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env that could leak in from the host.
	for _, k := range []string{"PORT", "CACHE_TTL_SECONDS", "REDIS_URL", "DATABASE_URL", "PRICING_PATH"} {
		t.Setenv(k, "")
	}
	// Point at a path that does not exist so pricing is empty (not fatal).
	t.Setenv("PRICING_PATH", filepath.Join(t.TempDir(), "nope.yaml"))

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if c.Port != "8080" {
		t.Errorf("Port = %q, want 8080", c.Port)
	}
	if c.CacheTTLSeconds != 3600 {
		t.Errorf("CacheTTLSeconds = %d, want 3600", c.CacheTTLSeconds)
	}
	if c.RedisURL == "" || c.DatabaseURL == "" {
		t.Errorf("expected infra URLs to have defaults, got redis=%q db=%q", c.RedisURL, c.DatabaseURL)
	}
}

func TestLoadPricingAndCost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.yaml")
	yaml := `
models:
  gpt-4o-mini:      { provider: openai,    in_per_1k: 0.00015, out_per_1k: 0.0006 }
  claude-haiku-4-5: { provider: anthropic, in_per_1k: 0.0008,  out_per_1k: 0.004 }
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPricing(path)
	if err != nil {
		t.Fatalf("LoadPricing() error: %v", err)
	}
	if len(p.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(p.Models))
	}

	// 1000 in + 1000 out on gpt-4o-mini = 0.00015 + 0.0006 = 0.00075
	got, ok := p.Cost("gpt-4o-mini", 1000, 1000)
	if !ok {
		t.Fatal("expected gpt-4o-mini to be priced")
	}
	want := 0.00075
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Cost = %v, want %v", got, want)
	}

	if _, ok := p.Cost("unknown-model", 100, 100); ok {
		t.Error("expected unknown model to report ok=false")
	}
}

func TestLoadPricingMissingFileIsNotFatal(t *testing.T) {
	p, err := LoadPricing(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("missing pricing file should not error, got: %v", err)
	}
	if p.Models == nil {
		t.Error("expected non-nil (empty) Models map")
	}
}
