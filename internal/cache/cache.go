// Package cache implements exact-match response caching backed by Redis. On a
// hit the gateway returns the stored response and skips the provider call
// entirely. The cache key is the SHA-256 of the canonicalized request (volatile
// fields stripped, object keys sorted), optionally namespaced per virtual key.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Scope controls cache-key namespacing.
type Scope string

const (
	ScopeKey    Scope = "key"    // per virtual key (tenant-isolated)
	ScopeGlobal Scope = "global" // shared across all keys
)

// volatileFields are request fields that don't affect the model output. They are
// stripped before hashing so they neither fragment nor wrongly share cache keys.
var volatileFields = []string{"stream", "stream_options", "user", "metadata", "store"}

// Entry is a cached response.
type Entry struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
	Model       string `json:"model"`
	TokensIn    int    `json:"tokens_in"`
	TokensOut   int    `json:"tokens_out"`
}

// Cache is the Redis-backed response cache.
type Cache struct {
	rdb      *redis.Client
	ttl      time.Duration
	scope    Scope
	maxBytes int
	enabled  bool
	log      *slog.Logger
}

// New builds a Cache. enabled is the global toggle; scope is "key" or "global".
func New(rdb *redis.Client, ttlSeconds int, scope string, maxBytes int, enabled bool, log *slog.Logger) *Cache {
	s := ScopeKey
	if Scope(scope) == ScopeGlobal {
		s = ScopeGlobal
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &Cache{
		rdb:      rdb,
		ttl:      time.Duration(ttlSeconds) * time.Second,
		scope:    s,
		maxBytes: maxBytes,
		enabled:  enabled,
		log:      log,
	}
}

// Enabled reports whether caching is globally on and backed by Redis.
func (c *Cache) Enabled() bool { return c != nil && c.enabled && c.rdb != nil }

// Scope returns the configured namespacing scope (for logging/introspection).
func (c *Cache) Scope() Scope { return c.scope }

// Key returns the cache key for a request body, or ok=false if the body can't be
// normalized (not JSON). Provider and (for ScopeKey) the api key id are folded in.
func (c *Cache) Key(apiKeyID, provider string, body []byte) (string, bool) {
	canon, ok := normalize(body)
	if !ok {
		return "", false
	}
	h := sha256.New()
	h.Write([]byte(provider))
	h.Write([]byte{0})
	if c.scope == ScopeKey {
		h.Write([]byte(apiKeyID))
		h.Write([]byte{0})
	}
	h.Write(canon)
	return "cache:" + hex.EncodeToString(h.Sum(nil)), true
}

// normalize strips volatile fields and re-marshals with sorted object keys
// (encoding/json sorts map keys recursively), yielding a canonical byte form.
func normalize(body []byte) ([]byte, bool) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false
	}
	for _, f := range volatileFields {
		delete(m, f)
	}
	canon, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return canon, true
}

// Get returns a cached entry. hit is false (with nil error) on a miss.
func (c *Cache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, false, err
	}
	return &e, true, nil
}

// Set stores an entry with the configured TTL. Oversized bodies are skipped.
func (c *Cache) Set(ctx context.Context, key string, e Entry) error {
	if len(e.Body) > c.maxBytes {
		return nil
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, raw, c.ttl).Err()
}
