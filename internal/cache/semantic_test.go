package cache

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/sushil23harsana/ai-gateway/internal/store"
)

type fakeEmbedder struct {
	vec []float32
	err error
}

func (f fakeEmbedder) Embed(context.Context, string) ([]float32, error) { return f.vec, f.err }

type fakeVS struct {
	nearest *store.SemanticResult
	inserts int
}

func (f *fakeVS) SemanticNearest(_ context.Context, _ *string, _, _ string, _ []float32, maxDist float64) (*store.SemanticResult, error) {
	if f.nearest != nil && f.nearest.Distance <= maxDist {
		return f.nearest, nil
	}
	return nil, nil
}

func (f *fakeVS) SemanticInsert(_ context.Context, _ *string, _, _ string, _ []float32, _, _ string, _, _ int) error {
	f.inserts++
	return nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSemanticLookupHitWithinThreshold(t *testing.T) {
	vs := &fakeVS{nearest: &store.SemanticResult{Body: `{"x":1}`, Model: "m", Distance: 0.02}}
	c := NewSemantic(vs, fakeEmbedder{vec: []float32{1, 2, 3}}, 0.05, "key", true, discardLog())

	entry, emb, hit, err := c.Lookup(context.Background(), "k1", "openai", "m", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !hit || entry == nil || entry.Body != `{"x":1}` {
		t.Errorf("expected hit with cached body, got hit=%v entry=%+v", hit, entry)
	}
	if len(emb) != 3 {
		t.Errorf("expected embedding returned for reuse, got %v", emb)
	}
}

func TestSemanticLookupMissBeyondThreshold(t *testing.T) {
	vs := &fakeVS{nearest: &store.SemanticResult{Body: `{"x":1}`, Distance: 0.2}} // beyond 0.05
	c := NewSemantic(vs, fakeEmbedder{vec: []float32{1}}, 0.05, "key", true, discardLog())

	entry, emb, hit, err := c.Lookup(context.Background(), "k1", "openai", "m", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if hit || entry != nil {
		t.Error("expected miss beyond threshold")
	}
	if len(emb) != 1 {
		t.Error("embedding should still be returned on a miss (for reuse by Store)")
	}
}

func TestSemanticStoreInserts(t *testing.T) {
	vs := &fakeVS{}
	c := NewSemantic(vs, fakeEmbedder{vec: []float32{1, 2}}, 0.05, "key", true, discardLog())
	if err := c.Store(context.Background(), "k1", "openai", "m", []float32{1, 2}, `{"y":2}`, 3, 4); err != nil {
		t.Fatal(err)
	}
	if vs.inserts != 1 {
		t.Errorf("expected 1 insert, got %d", vs.inserts)
	}
}

func TestPromptText(t *testing.T) {
	got := PromptText([]byte(`{"messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`))
	want := "system: be brief\nuser: hi"
	if got != want {
		t.Errorf("PromptText = %q, want %q", got, want)
	}
}
