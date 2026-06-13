// Package keys handles virtual API keys: generating them, hashing them, and the
// authentication middleware that validates the Authorization: Bearer header
// against the stored hash. Raw keys are shown once at creation and never stored.
package keys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sushil23harsana/ai-gateway/internal/store"
)

// KeyPrefix marks gateway-issued virtual keys (distinct from provider keys).
const KeyPrefix = "sk-gw-"

// Generate returns a new random virtual key and its SHA-256 hash. Store the hash;
// return the raw key to the caller exactly once.
func Generate() (raw, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = KeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, Hash(raw), nil
}

// Hash returns the hex SHA-256 of a raw key.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Identity is the authenticated virtual key, carried on the request context.
type Identity struct {
	ID           string
	Name         string
	RateLimitRPM int
}

type ctxKey struct{}

// WithIdentity attaches an Identity to a context (set by the auth middleware).
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFrom retrieves the authenticated Identity, if any.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Lookup is the slice of the store the authenticator needs.
type Lookup interface {
	GetAPIKeyByHash(ctx context.Context, keyHash string) (store.APIKey, bool, error)
}

// Authenticator validates virtual keys against the store.
type Authenticator struct {
	store Lookup
	log   *slog.Logger
}

// NewAuthenticator wires the authenticator.
func NewAuthenticator(s Lookup, log *slog.Logger) *Authenticator {
	return &Authenticator{store: s, log: log}
}

// Middleware authenticates the Bearer virtual key, rejecting unknown (401) and
// disabled (403) keys, and attaches the Identity to the request context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		rec, found, err := a.store.GetAPIKeyByHash(r.Context(), Hash(raw))
		if err != nil {
			a.log.Error("auth lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !found {
			writeError(w, http.StatusUnauthorized, "invalid api key")
			return
		}
		if rec.Disabled {
			writeError(w, http.StatusForbidden, "api key disabled")
			return
		}
		ctx := WithIdentity(r.Context(), Identity{ID: rec.ID, Name: rec.Name, RateLimitRPM: rec.RateLimitRPM})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) (string, bool) {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(p):])
	return tok, tok != ""
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
