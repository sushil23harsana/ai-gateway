package keys

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/store"
)

func TestGenerateAndHash(t *testing.T) {
	raw, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.HasPrefix(raw, KeyPrefix) {
		t.Errorf("raw key %q missing prefix %q", raw, KeyPrefix)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 (hex sha256)", len(hash))
	}
	if Hash(raw) != hash {
		t.Errorf("Hash(raw) != returned hash")
	}
	if raw == hash {
		t.Error("raw key must not equal its hash")
	}

	raw2, _, _ := Generate()
	if raw2 == raw {
		t.Error("generated keys must be unique")
	}
}

type fakeLookup struct {
	rec   store.APIKey
	found bool
	err   error
}

func (f fakeLookup) GetAPIKeyByHash(_ context.Context, _ string) (store.APIKey, bool, error) {
	return f.rec, f.found, f.err
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestAuthMiddleware(t *testing.T) {
	identityHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := IdentityFrom(r.Context()); !ok {
			t.Error("expected Identity in request context")
		}
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name       string
		lookup     fakeLookup
		authHeader string
		wantStatus int
	}{
		{"missing header", fakeLookup{}, "", http.StatusUnauthorized},
		{"unknown key", fakeLookup{found: false}, "Bearer sk-gw-unknown", http.StatusUnauthorized},
		{"disabled key", fakeLookup{found: true, rec: store.APIKey{ID: "k1", Disabled: true}}, "Bearer sk-gw-x", http.StatusForbidden},
		{"valid key", fakeLookup{found: true, rec: store.APIKey{ID: "k1", Name: "team", RateLimitRPM: 60}}, "Bearer sk-gw-x", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAuthenticator(tc.lookup, discardLogger())
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			a.Middleware(identityHandler).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
