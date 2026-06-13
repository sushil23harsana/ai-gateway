package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

type fakeKeyStore struct {
	inserted store.APIKey
	list     []store.APIKey
}

func (f *fakeKeyStore) InsertAPIKey(_ context.Context, name, keyHash string, rpm int, budget *float64, cacheEnabled bool) (store.APIKey, error) {
	f.inserted = store.APIKey{ID: "k1", Name: name, KeyHash: keyHash, RateLimitRPM: rpm, MonthlyBudgetUSD: budget, CacheEnabled: cacheEnabled}
	return f.inserted, nil
}

func (f *fakeKeyStore) ListAPIKeys(context.Context) ([]store.APIKey, error) { return f.list, nil }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCreateKeyReturnsRawOnceAndStoresHash(t *testing.T) {
	fs := &fakeKeyStore{}
	a := NewKeyAdmin(fs, discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/admin/keys",
		strings.NewReader(`{"name":"team-frontend","rate_limit_rpm":120}`))
	rec := httptest.NewRecorder()
	a.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Key          string `json:"key"`
		RateLimitRPM int    `json:"rate_limit_rpm"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Key, keys.KeyPrefix) {
		t.Errorf("response key %q should be the raw key", resp.Key)
	}
	if resp.RateLimitRPM != 120 {
		t.Errorf("rate_limit_rpm = %d, want 120", resp.RateLimitRPM)
	}
	// The store must receive the HASH, never the raw key.
	if fs.inserted.KeyHash == resp.Key {
		t.Error("stored the raw key instead of its hash")
	}
	if fs.inserted.KeyHash != keys.Hash(resp.Key) {
		t.Error("stored hash does not match sha256(raw key)")
	}
}

func TestCreateKeyRequiresName(t *testing.T) {
	a := NewKeyAdmin(&fakeKeyStore{}, discardLogger())
	rec := httptest.NewRecorder()
	a.Create(rec, httptest.NewRequest(http.MethodPost, "/admin/keys", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestListHidesHash(t *testing.T) {
	fs := &fakeKeyStore{list: []store.APIKey{
		{ID: "k1", Name: "team", KeyHash: "deadbeefsecret", RateLimitRPM: 60},
	}}
	a := NewKeyAdmin(fs, discardLogger())
	rec := httptest.NewRecorder()
	a.List(rec, httptest.NewRequest(http.MethodGet, "/admin/keys", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "deadbeefsecret") {
		t.Errorf("list response leaked key_hash: %s", rec.Body.String())
	}
}

func TestAdminAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	t.Run("no token configured", func(t *testing.T) {
		rec := httptest.NewRecorder()
		AdminAuth("", discardLogger())(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/keys", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})
	t.Run("wrong token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
		req.Header.Set("Authorization", "Bearer nope")
		rec := httptest.NewRecorder()
		AdminAuth("secret", discardLogger())(next).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
	t.Run("correct token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		AdminAuth("secret", discardLogger())(next).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}
