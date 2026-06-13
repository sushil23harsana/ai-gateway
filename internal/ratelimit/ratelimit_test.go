package ratelimit

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
)

type fakeAllower struct {
	dec Decision
	err error
}

func (f fakeAllower) Allow(_ context.Context, _ string, _ int) (Decision, error) {
	return f.dec, f.err
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func withIdentity(req *http.Request) *http.Request {
	return req.WithContext(keys.WithIdentity(req.Context(), keys.Identity{ID: "k1", RateLimitRPM: 60}))
}

func TestMiddlewareAllows(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := Middleware(fakeAllower{dec: Decision{Allowed: true}}, discardLogger())

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, withIdentity(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))

	if !called {
		t.Error("expected next handler to be called when allowed")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareRejectsWithRetryAfter(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not run when rate-limited")
	})
	mw := Middleware(fakeAllower{dec: Decision{Allowed: false, RetryAfter: 3 * time.Second}}, discardLogger())

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, withIdentity(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "3" {
		t.Errorf("Retry-After = %q, want 3", ra)
	}
}

func TestMiddlewareFailsOpenOnError(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw := Middleware(fakeAllower{err: context.DeadlineExceeded}, discardLogger())

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, withIdentity(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))

	if !called {
		t.Error("expected fail-open (next called) on limiter error")
	}
}
