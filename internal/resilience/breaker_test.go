package resilience

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func testRegistry(cfg BreakerConfig) (*Registry, *fakeClock) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	r := NewRegistry(cfg, discardLog())
	r.now = clk.Now
	return r, clk
}

func TestBreakerOpensAfterThreshold(t *testing.T) {
	cfg := BreakerConfig{Enabled: true, Threshold: 3, Cooldown: 10 * time.Second, HalfOpenMax: 1}
	r, _ := testRegistry(cfg)

	for i := 0; i < 3; i++ {
		if !r.Allow("openai") {
			t.Fatalf("allow should be true before the breaker opens (i=%d)", i)
		}
		r.Report("openai", false)
	}
	if r.Allow("openai") {
		t.Error("breaker should reject requests after the failure threshold")
	}
	if r.State("openai") != StateOpen {
		t.Errorf("state = %v, want open", r.State("openai"))
	}
}

func TestBreakerSuccessResetsCount(t *testing.T) {
	cfg := BreakerConfig{Enabled: true, Threshold: 3, Cooldown: time.Second, HalfOpenMax: 1}
	r, _ := testRegistry(cfg)

	report := func(success bool) { r.Allow("openai"); r.Report("openai", success) }
	report(false)
	report(false)
	report(true) // resets the consecutive-failure count
	report(false)
	report(false)

	if r.State("openai") != StateClosed {
		t.Errorf("state = %v, want closed (a success should reset the count)", r.State("openai"))
	}
}

func TestBreakerHalfOpenRecovers(t *testing.T) {
	cfg := BreakerConfig{Enabled: true, Threshold: 2, Cooldown: 10 * time.Second, HalfOpenMax: 1}
	r, clk := testRegistry(cfg)

	r.Allow("openai")
	r.Report("openai", false)
	r.Allow("openai")
	r.Report("openai", false)
	if r.Allow("openai") {
		t.Fatal("breaker should be open")
	}

	clk.advance(11 * time.Second)
	if !r.Allow("openai") {
		t.Fatal("should admit a half-open trial after the cooldown")
	}
	if r.Allow("openai") {
		t.Error("half-open should admit only HalfOpenMax trials")
	}
	r.Report("openai", true)
	if r.State("openai") != StateClosed {
		t.Errorf("state = %v, want closed after a successful probe", r.State("openai"))
	}
}

func TestBreakerHalfOpenReopensOnFailedProbe(t *testing.T) {
	cfg := BreakerConfig{Enabled: true, Threshold: 2, Cooldown: 10 * time.Second, HalfOpenMax: 1}
	r, clk := testRegistry(cfg)

	r.Allow("openai")
	r.Report("openai", false)
	r.Allow("openai")
	r.Report("openai", false)

	clk.advance(11 * time.Second)
	if !r.Allow("openai") {
		t.Fatal("should admit a trial after the cooldown")
	}
	r.Report("openai", false) // probe fails

	if r.State("openai") != StateOpen {
		t.Errorf("state = %v, want open after a failed probe", r.State("openai"))
	}
	if r.Allow("openai") {
		t.Error("should be open again (no immediate re-probe before another cooldown)")
	}
}

func TestBreakerIsolatesProviders(t *testing.T) {
	cfg := BreakerConfig{Enabled: true, Threshold: 1, Cooldown: time.Minute, HalfOpenMax: 1}
	r, _ := testRegistry(cfg)

	r.Allow("openai")
	r.Report("openai", false) // trips openai only

	if r.Allow("openai") {
		t.Error("openai breaker should be open")
	}
	if !r.Allow("anthropic") {
		t.Error("anthropic breaker must be unaffected by openai's failures")
	}
}

func TestBreakerDisabledAlwaysAllows(t *testing.T) {
	r := NewRegistry(BreakerConfig{Enabled: false}, discardLog())
	for i := 0; i < 100; i++ {
		if !r.Allow("x") {
			t.Fatal("a disabled breaker must always allow")
		}
		r.Report("x", false)
	}
	if r.State("x") != StateClosed {
		t.Errorf("disabled breaker state = %v, want closed", r.State("x"))
	}
}
