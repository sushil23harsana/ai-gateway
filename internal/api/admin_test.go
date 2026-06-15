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

	updated  store.APIKey
	updFound bool
	updErr   error
	delFound bool
	delErr   error

	gotID  string
	gotUpd store.KeyUpdate
}

func (f *fakeKeyStore) InsertAPIKey(_ context.Context, name, keyHash string, rpm int, budget *float64, cacheEnabled bool) (store.APIKey, error) {
	f.inserted = store.APIKey{ID: "k1", Name: name, KeyHash: keyHash, RateLimitRPM: rpm, MonthlyBudgetUSD: budget, CacheEnabled: cacheEnabled}
	return f.inserted, nil
}

func (f *fakeKeyStore) ListAPIKeys(context.Context) ([]store.APIKey, error) { return f.list, nil }

func (f *fakeKeyStore) UpdateAPIKey(_ context.Context, id string, upd store.KeyUpdate) (store.APIKey, bool, error) {
	f.gotID, f.gotUpd = id, upd
	return f.updated, f.updFound, f.updErr
}

func (f *fakeKeyStore) DeleteAPIKey(_ context.Context, id string) (bool, error) {
	f.gotID = id
	return f.delFound, f.delErr
}

const validID = "11111111-1111-1111-1111-111111111111"

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

func TestUpdateKeyDisable(t *testing.T) {
	fs := &fakeKeyStore{updated: store.APIKey{ID: validID, Name: "app", Disabled: true}, updFound: true}
	a := NewKeyAdmin(fs, discardLogger())

	req := httptest.NewRequest(http.MethodPatch, "/admin/keys/"+validID, strings.NewReader(`{"disabled":true}`))
	req.SetPathValue("id", validID)
	rec := httptest.NewRecorder()
	a.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if fs.gotUpd.Disabled == nil || !*fs.gotUpd.Disabled {
		t.Errorf("store did not receive disabled=true: %+v", fs.gotUpd)
	}
	if !strings.Contains(rec.Body.String(), `"disabled":true`) {
		t.Errorf("response missing disabled=true: %s", rec.Body.String())
	}
}

func TestUpdateKeyNotFound(t *testing.T) {
	a := NewKeyAdmin(&fakeKeyStore{updFound: false}, discardLogger())
	req := httptest.NewRequest(http.MethodPatch, "/admin/keys/"+validID, strings.NewReader(`{"name":"x"}`))
	req.SetPathValue("id", validID)
	rec := httptest.NewRecorder()
	a.Update(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateKeyValidation(t *testing.T) {
	for _, tc := range []struct {
		name, id, body string
		want           int
	}{
		{"invalid uuid", "not-a-uuid", `{"name":"x"}`, http.StatusBadRequest},
		{"zero rate limit", validID, `{"rate_limit_rpm":0}`, http.StatusBadRequest},
		{"negative budget", validID, `{"monthly_budget_usd":-5}`, http.StatusBadRequest},
		{"blank name", validID, `{"name":"  "}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeKeyStore{updated: store.APIKey{ID: validID}, updFound: true}
			a := NewKeyAdmin(fs, discardLogger())
			req := httptest.NewRequest(http.MethodPatch, "/admin/keys/"+tc.id, strings.NewReader(tc.body))
			req.SetPathValue("id", tc.id)
			rec := httptest.NewRecorder()
			a.Update(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestDeleteKey(t *testing.T) {
	for _, tc := range []struct {
		name  string
		found bool
		want  int
	}{
		{"found", true, http.StatusNoContent},
		{"missing", false, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewKeyAdmin(&fakeKeyStore{delFound: tc.found}, discardLogger())
			req := httptest.NewRequest(http.MethodDelete, "/admin/keys/"+validID, nil)
			req.SetPathValue("id", validID)
			rec := httptest.NewRecorder()
			a.Delete(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestWriteGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"empty blocks", "", http.StatusForbidden},
		{"default blocks", "change-me", http.StatusForbidden},
		{"strong passes", "s3cret-admin-token", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteGuard(tc.token, discardLogger())(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/keys", nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
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
