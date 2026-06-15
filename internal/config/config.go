// Package config loads gateway configuration from the environment (12-factor)
// and the per-model pricing table from pricing.yaml. No secrets live in code.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the fully-resolved runtime configuration for the gateway.
type Config struct {
	// HTTP
	Port     string
	LogLevel slog.Level

	// Admin API protection (separate from virtual keys).
	AdminToken string

	// Real provider keys — server-side only, never logged or returned.
	OpenAIAPIKey    string
	AnthropicAPIKey string

	// Provider endpoints / upstream behavior.
	OpenAIBaseURL          string
	AnthropicBaseURL       string
	AnthropicVersion       string
	AnthropicMaxTokens     int // default max_tokens for Anthropic (required by its API)
	UpstreamTimeoutSeconds int

	// Routing & failover.
	DefaultProvider  string // provider used when a model can't be routed otherwise
	FailoverEnabled  bool
	FailoverProvider string // fallback provider name on primary 5xx/timeout ("" disables)
	FailoverModel    string // model to use on the fallback provider

	// Infra
	RedisURL    string
	DatabaseURL string

	// Cache
	CacheTTLSeconds int
	CacheEnabled    bool   // global response-cache toggle
	CacheScope      string // "key" (per-virtual-key, isolated) | "global" (shared)
	CacheMaxBytes   int    // responses larger than this are not cached

	// Semantic cache (pgvector + embeddings) — opt-in, off by default.
	SemanticCacheEnabled bool
	SemanticThreshold    float64 // max cosine distance for a near-duplicate hit (smaller = stricter)
	EmbeddingModel       string

	// Budgets — enforce per-key monthly spend caps. A key is only checked when its
	// monthly_budget_usd is set; this flag is the global kill-switch.
	BudgetEnforced bool

	// Pricing table (loaded from PricingPath).
	PricingPath string
	Pricing     Pricing
}

// ModelPricing is the $/1K-token cost for a single model.
type ModelPricing struct {
	Provider string  `yaml:"provider"`
	InPer1K  float64 `yaml:"in_per_1k"`
	OutPer1K float64 `yaml:"out_per_1k"`
}

// Pricing is the parsed pricing.yaml table, keyed by model name.
type Pricing struct {
	Models map[string]ModelPricing `yaml:"models"`
}

// ProviderMap returns a model → provider-name mapping derived from the pricing
// table, used by the router to pick a provider for a requested model.
func (p Pricing) ProviderMap() map[string]string {
	m := make(map[string]string, len(p.Models))
	for model, mp := range p.Models {
		if mp.Provider != "" {
			m[model] = mp.Provider
		}
	}
	return m
}

// Cost returns the USD cost for a request against the named model. The bool is
// false when the model is unknown to the pricing table (caller decides policy).
func (p Pricing) Cost(model string, tokensIn, tokensOut int) (float64, bool) {
	m, ok := p.Models[model]
	if !ok {
		return 0, false
	}
	cost := (float64(tokensIn)/1000.0)*m.InPer1K + (float64(tokensOut)/1000.0)*m.OutPer1K
	return cost, true
}

// Load reads configuration from the environment, applying sane defaults, and
// loads the pricing table. It returns an error only for genuinely fatal
// problems (e.g. a pricing file that exists but cannot be parsed).
func Load() (*Config, error) {
	c := &Config{
		Port:            getenv("PORT", "8080"),
		LogLevel:        parseLevel(getenv("LOG_LEVEL", "info")),
		AdminToken:      os.Getenv("ADMIN_TOKEN"),
		OpenAIAPIKey:           os.Getenv("OPENAI_API_KEY"),
		AnthropicAPIKey:        os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIBaseURL:          getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		AnthropicBaseURL:       getenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		AnthropicVersion:       getenv("ANTHROPIC_VERSION", "2023-06-01"),
		AnthropicMaxTokens:     getenvInt("ANTHROPIC_MAX_TOKENS", 4096),
		UpstreamTimeoutSeconds: getenvInt("UPSTREAM_TIMEOUT_SECONDS", 120),
		DefaultProvider:        getenv("DEFAULT_PROVIDER", "openai"),
		FailoverEnabled:        getenvBool("FAILOVER_ENABLED", true),
		FailoverProvider:       os.Getenv("FAILOVER_PROVIDER"),
		FailoverModel:          os.Getenv("FAILOVER_MODEL"),
		RedisURL:        getenv("REDIS_URL", "redis://localhost:6379"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://gw:gw@localhost:5432/aigateway?sslmode=disable"),
		CacheTTLSeconds: getenvInt("CACHE_TTL_SECONDS", 3600),
		CacheEnabled:    getenvBool("CACHE_ENABLED", true),
		CacheScope:      getenv("CACHE_SCOPE", "key"),
		CacheMaxBytes:   getenvInt("CACHE_MAX_BYTES", 1<<20),

		SemanticCacheEnabled: getenvBool("SEMANTIC_CACHE_ENABLED", false),
		SemanticThreshold:    getenvFloat("SEMANTIC_THRESHOLD", 0.25),
		EmbeddingModel:       getenv("EMBEDDING_MODEL", "text-embedding-3-small"),

		BudgetEnforced: getenvBool("BUDGET_ENFORCED", true),

		PricingPath: getenv("PRICING_PATH", "pricing.yaml"),
	}

	pricing, err := LoadPricing(c.PricingPath)
	if err != nil {
		return nil, err
	}
	c.Pricing = pricing

	return c, nil
}

// LoadPricing parses a pricing.yaml file. A missing file is not fatal — it
// yields an empty table (pricing is first exercised in Phase 1). A file that
// exists but is malformed is an error.
func LoadPricing(path string) (Pricing, error) {
	var p Pricing
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("pricing file not found; cost calculation will be unavailable", "path", path)
			return Pricing{Models: map[string]ModelPricing{}}, nil
		}
		return p, fmt.Errorf("read pricing file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse pricing file %q: %w", path, err)
	}
	if p.Models == nil {
		p.Models = map[string]ModelPricing{}
	}
	return p, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid integer env var; using default", "key", key, "value", v, "default", def)
		return def
	}
	return n
}

func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("invalid bool env var; using default", "key", key, "value", v, "default", def)
		return def
	}
	return b
}

func getenvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("invalid float env var; using default", "key", key, "value", v, "default", def)
		return def
	}
	return f
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN", "warning":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
