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
	"strings"
	"time"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

const defaultRateLimitRPM = 60

// KeyStore is the slice of the store the admin key endpoints need.
type KeyStore interface {
	InsertAPIKey(ctx context.Context, name, keyHash string, rateLimitRPM int, monthlyBudgetUSD *float64) (store.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]store.APIKey, error)
}

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
}

type createKeyResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Key              string   `json:"key"` // raw key — returned once, never again
	RateLimitRPM     int      `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd,omitempty"`
	Warning          string   `json:"warning"`
}

// keyView is the safe representation for listing (no hash, no raw key).
type keyView struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	RateLimitRPM     int       `json:"rate_limit_rpm"`
	MonthlyBudgetUSD *float64  `json:"monthly_budget_usd,omitempty"`
	Disabled         bool      `json:"disabled"`
	CreatedAt        time.Time `json:"created_at"`
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

	raw, hash, err := keys.Generate()
	if err != nil {
		a.log.Error("failed to generate key", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}

	rec, err := a.store.InsertAPIKey(r.Context(), req.Name, hash, req.RateLimitRPM, req.MonthlyBudgetUSD)
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
		views = append(views, keyView{
			ID:               k.ID,
			Name:             k.Name,
			RateLimitRPM:     k.RateLimitRPM,
			MonthlyBudgetUSD: k.MonthlyBudgetUSD,
			Disabled:         k.Disabled,
			CreatedAt:        k.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": views})
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
