// Package api holds the admin control-plane endpoints. Phase 2 adds virtual-key
// management (create/list); the stats endpoints arrive in Phase 5. All admin
// routes sit behind AdminAuth (a separate ADMIN_TOKEN, not a virtual key).
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

const defaultRateLimitRPM = 60

// KeyStore is the slice of the store the admin key endpoints need.
type KeyStore interface {
	InsertAPIKey(ctx context.Context, name, keyHash string, rateLimitRPM int, monthlyBudgetUSD *float64, cacheEnabled bool) (store.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]store.APIKey, error)
	UpdateAPIKey(ctx context.Context, id string, upd store.KeyUpdate) (store.APIKey, bool, error)
	DeleteAPIKey(ctx context.Context, id string) (bool, error)
}

// uuidRe validates a path id before it reaches a ::uuid cast, so a malformed id
// returns 400 instead of a 500 from the database driver.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(s string) bool { return uuidRe.MatchString(s) }

// KeyAdmin serves POST/GET /admin/keys.
type KeyAdmin struct {
	store KeyStore
	log   *slog.Logger
}

// NewKeyAdmin wires the handler.
func NewKeyAdmin(s KeyStore, log *slog.Logger) *KeyAdmin {
	return &KeyAdmin{store: s, log: log}
}

type createKeyRequest struct {
	Name             string   `json:"name"`
	RateLimitRPM     int      `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd"`
	CacheEnabled     *bool    `json:"cache_enabled"` // optional; defaults to true
}

type createKeyResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Key              string   `json:"key"` // raw key — returned once, never again
	RateLimitRPM     int      `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd,omitempty"`
	CacheEnabled     bool     `json:"cache_enabled"`
	Warning          string   `json:"warning"`
}

// keyView is the safe representation for listing (no hash, no raw key).
type keyView struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	RateLimitRPM     int       `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64  `json:"monthly_budget_usd,omitempty"`
	CacheEnabled     bool      `json:"cache_enabled"`
	Disabled         bool      `json:"disabled"`
	CreatedAt        time.Time `json:"created_at"`
}

func toKeyView(k store.APIKey) keyView {
	return keyView{
		ID:               k.ID,
		Name:             k.Name,
		RateLimitRPM:     k.RateLimitRPM,
		MonthlyBudgetUSD: k.MonthlyBudgetUSD,
		CacheEnabled:     k.CacheEnabled,
		Disabled:         k.Disabled,
		CreatedAt:        k.CreatedAt,
	}
}

// updateKeyRequest is the PATCH body. Every field is optional; omitted fields are
// left unchanged. Pointers distinguish "absent" from a zero value.
type updateKeyRequest struct {
	Name             *string  `json:"name"`
	RateLimitRPM     *int     `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd"`
	CacheEnabled     *bool    `json:"cache_enabled"`
	Disabled         *bool    `json:"disabled"`
}

// Create handles POST /admin/keys: mints a virtual key, stores only its hash,
// and returns the raw key once.
func (a *KeyAdmin) Create(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.RateLimitRPM <= 0 {
		req.RateLimitRPM = defaultRateLimitRPM
	}
	cacheEnabled := true
	if req.CacheEnabled != nil {
		cacheEnabled = *req.CacheEnabled
	}

	raw, hash, err := keys.Generate()
	if err != nil {
		a.log.Error("failed to generate key", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}

	rec, err := a.store.InsertAPIKey(r.Context(), req.Name, hash, req.RateLimitRPM, req.MonthlyBudgetUSD, cacheEnabled)
	if err != nil {
		a.log.Error("failed to insert api key", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	writeJSON(w, http.StatusCreated, createKeyResponse{
		ID:               rec.ID,
		Name:             rec.Name,
		Key:              raw,
		RateLimitRPM:     rec.RateLimitRPM,
		MonthlyBudgetUSD: rec.MonthlyBudgetUSD,
		CacheEnabled:     rec.CacheEnabled,
		Warning:          "store this key now — it will not be shown again",
	})
}

// List handles GET /admin/keys: returns keys without any secret material.
func (a *KeyAdmin) List(w http.ResponseWriter, r *http.Request) {
	recs, err := a.store.ListAPIKeys(r.Context())
	if err != nil {
		a.log.Error("failed to list api keys", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	views := make([]keyView, 0, len(recs))
	for _, k := range recs {
		views = append(views, toKeyView(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": views})
}

// Update handles PATCH /admin/keys/{id}: a partial update of a key's settings
// (disable/enable, rename, change rate limit or budget, toggle caching). It never
// touches the key hash, so the secret is unaffected.
func (a *KeyAdmin) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	var req updateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}
	if req.RateLimitRPM != nil && *req.RateLimitRPM <= 0 {
		writeError(w, http.StatusBadRequest, "rate_limit_rpm must be positive")
		return
	}
	if req.MonthlyBudgetUSD != nil && *req.MonthlyBudgetUSD < 0 {
		writeError(w, http.StatusBadRequest, "monthly_budget_usd cannot be negative")
		return
	}

	rec, found, err := a.store.UpdateAPIKey(r.Context(), id, store.KeyUpdate{
		Name:             req.Name,
		RateLimitRPM:     req.RateLimitRPM,
		MonthlyBudgetUSD: req.MonthlyBudgetUSD,
		CacheEnabled:     req.CacheEnabled,
		Disabled:         req.Disabled,
	})
	if err != nil {
		a.log.Error("failed to update api key", "err", err, "key_id", id)
		writeError(w, http.StatusInternalServerError, "failed to update key")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}

	// Audit: record which fields changed and the source — never the values.
	a.log.Info("admin: key updated",
		"key_id", id, "remote", r.RemoteAddr,
		"set_disabled", req.Disabled != nil,
		"set_rate_limit", req.RateLimitRPM != nil,
		"set_budget", req.MonthlyBudgetUSD != nil,
		"set_cache", req.CacheEnabled != nil,
		"set_name", req.Name != nil,
	)
	writeJSON(w, http.StatusOK, toKeyView(rec))
}

// Delete handles DELETE /admin/keys/{id}: permanently removes a key. Existing
// request_logs rows are retained (their api_key_id is kept for history).
func (a *KeyAdmin) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	found, err := a.store.DeleteAPIKey(r.Context(), id)
	if err != nil {
		a.log.Error("failed to delete api key", "err", err, "key_id", id)
		writeError(w, http.StatusInternalServerError, "failed to delete key")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	a.log.Info("admin: key deleted", "key_id", id, "remote", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// AdminAuth guards admin routes with a constant-time compare against ADMIN_TOKEN.
// If the token isn't configured, admin endpoints are disabled (503).
func AdminAuth(token string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				writeError(w, http.StatusServiceUnavailable, "admin API disabled: ADMIN_TOKEN not set")
				return
			}
			presented := bearer(r)
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid admin token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WriteGuard blocks control-plane writes (create/update/delete keys) unless a
// real ADMIN_TOKEN is configured. Reads still work with any token, so a fresh
// install is observable but not mutable until the operator sets a strong secret.
// This is layered on top of AdminAuth, not a replacement for it.
func WriteGuard(token string, log *slog.Logger) func(http.Handler) http.Handler {
	weak := token == "" || token == "change-me"
	if weak {
		log.Warn("control-plane writes are DISABLED: set a strong ADMIN_TOKEN (not 'change-me') to enable key management")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if weak {
				writeError(w, http.StatusForbidden,
					"control-plane writes are disabled until a strong ADMIN_TOKEN is set (the default 'change-me' is not allowed)")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearer(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
