package budget

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/keys"
)

type fakeReader struct {
	spent float64
	err   error
}

func (f fakeReader) Spend(_ context.Context, _ string) (float64, error) { return f.spent, f.err }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// next is a handler that records whether it ran and returns 200.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

// reqWith builds a request carrying an authenticated identity with the given budget.
func reqWith(budget float64) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx := keys.WithIdentity(r.Context(), keys.Identity{ID: "k1", Name: "test", MonthlyBudgetUSD: budget})
	return r.WithContext(ctx)
}

func TestUnderBudgetPasses(t *testing.T) {
	var ran bool
	mw := Middleware(fakeReader{spent: 4.0}, true, discardLogger())
	rec := httptest.NewRecorder()
	mw(okHandler(&ran)).ServeHTTP(rec, reqWith(10.0))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("under budget should pass: ran=%v code=%d", ran, rec.Code)
	}
}

func TestAtOrOverBudgetBlocks(t *testing.T) {
	for _, spent := range []float64{10.0, 12.5} {
		var ran bool
		mw := Middleware(fakeReader{spent: spent}, true, discardLogger())
		rec := httptest.NewRecorder()
		mw(okHandler(&ran)).ServeHTTP(rec, reqWith(10.0))
		if ran {
			t.Errorf("spent=%.1f: over budget must NOT reach the proxy", spent)
		}
		if rec.Code != http.StatusPaymentRequired {
			t.Errorf("spent=%.1f: want 402, got %d", spent, rec.Code)
		}
	}
}

func TestNoBudgetPasses(t *testing.T) {
	var ran bool
	// Budget 0 = no cap; even huge spend passes.
	mw := Middleware(fakeReader{spent: 999.0}, true, discardLogger())
	rec := httptest.NewRecorder()
	mw(okHandler(&ran)).ServeHTTP(rec, reqWith(0))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("no budget should pass: ran=%v code=%d", ran, rec.Code)
	}
}

func TestDisabledPasses(t *testing.T) {
	var ran bool
	// Enforcement off: over budget still passes.
	mw := Middleware(fakeReader{spent: 999.0}, false, discardLogger())
	rec := httptest.NewRecorder()
	mw(okHandler(&ran)).ServeHTTP(rec, reqWith(10.0))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("disabled enforcement should pass: ran=%v code=%d", ran, rec.Code)
	}
}

func TestReaderErrorFailsOpen(t *testing.T) {
	var ran bool
	mw := Middleware(fakeReader{err: errors.New("redis down")}, true, discardLogger())
	rec := httptest.NewRecorder()
	mw(okHandler(&ran)).ServeHTTP(rec, reqWith(10.0))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("a reader error must fail open: ran=%v code=%d", ran, rec.Code)
	}
}

func TestNoIdentityPasses(t *testing.T) {
	var ran bool
	mw := Middleware(fakeReader{spent: 999.0}, true, discardLogger())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil) // no identity
	mw(okHandler(&ran)).ServeHTTP(rec, r)
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("no identity should pass: ran=%v code=%d", ran, rec.Code)
	}
}
