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
	UpstreamTimeoutSeconds int

	// Infra
	RedisURL    string
	DatabaseURL string

	// Cache
	CacheTTLSeconds int

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
		UpstreamTimeoutSeconds: getenvInt("UPSTREAM_TIMEOUT_SECONDS", 120),
		RedisURL:        getenv("REDIS_URL", "redis://localhost:6379"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://gw:gw@localhost:5432/aigateway?sslmode=disable"),
		CacheTTLSeconds: getenvInt("CACHE_TTL_SECONDS", 3600),
		PricingPath:     getenv("PRICING_PATH", "pricing.yaml"),
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
