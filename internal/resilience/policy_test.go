package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

// noSleepPolicy builds a policy whose backoff never actually sleeps, so retry
// behavior can be tested without wall-clock delays.
func noSleepPolicy(retry RetryConfig, bc BreakerConfig) *Policy {
	p := NewPolicy(retry, bc, discardLog())
	p.sleep = func(context.Context, time.Duration) error { return nil }
	p.jitter = func(int64) int64 { return 0 }
	return p
}

func TestRetriesTransientThenSucceeds(t *testing.T) {
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: false},
	)
	var calls int
	status, _, err := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		calls++
		if calls < 3 {
			return 500, []byte("err"), nil
		}
		return 200, []byte("ok"), nil
	})
	if err != nil || status != 200 {
		t.Fatalf("got status=%d err=%v, want 200/nil", status, err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetriesExhaustReturnsLast(t *testing.T) {
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: false},
	)
	var calls int
	status, _, _ := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		calls++
		return 503, []byte("down"), nil
	})
	if status != 503 {
		t.Errorf("status = %d, want 503", status)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (MaxAttempts)", calls)
	}
}

func TestNonRetryableNotRetried(t *testing.T) {
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: false},
	)
	var calls int
	status, _, _ := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		calls++
		return 400, []byte("bad request"), nil
	})
	if status != 400 || calls != 1 {
		t.Errorf("status=%d calls=%d, want 400/1 (a 4xx must not be retried)", status, calls)
	}
}

func TestTransportErrorIsRetried(t *testing.T) {
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: false},
	)
	var calls int
	boom := errors.New("dial tcp: connection refused")
	_, _, err := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		calls++
		return 0, nil, boom
	})
	if err == nil {
		t.Error("expected the transport error to surface after retries are exhausted")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestOpenBreakerShortCircuits(t *testing.T) {
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: true, Threshold: 1, Cooldown: time.Minute, HalfOpenMax: 1},
	)
	// First call fails and trips the breaker (threshold 1).
	if _, _, err := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		return 500, nil, nil
	}); err != nil {
		t.Fatalf("first call returned unexpected err: %v", err)
	}
	// The next call must short-circuit without invoking fn.
	var called bool
	_, _, err := p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
		called = true
		return 200, nil, nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err = %v, want ErrCircuitOpen", err)
	}
	if called {
		t.Error("fn must not be called while the breaker is open")
	}
}

func TestRecoveredRetryDoesNotTripBreaker(t *testing.T) {
	// A request that fails once then succeeds on retry reports a single SUCCESS
	// to the breaker, so a flaky-but-recovering provider stays closed.
	p := noSleepPolicy(
		RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		BreakerConfig{Enabled: true, Threshold: 2, Cooldown: time.Minute, HalfOpenMax: 1},
	)
	for i := 0; i < 5; i++ {
		var calls int
		p.Do(context.Background(), "openai", func(context.Context) (int, []byte, error) {
			calls++
			if calls == 1 {
				return 500, nil, nil
			}
			return 200, nil, nil
		})
	}
	if p.State("openai") != StateClosed {
		t.Errorf("state = %v, want closed (retries recovered every request)", p.State("openai"))
	}
}

func TestContextCancelStopsRetry(t *testing.T) {
	// Real sleepCtx with a long backoff; a cancelled context aborts the retry.
	p := NewPolicy(
		RetryConfig{MaxAttempts: 5, BaseDelay: time.Hour, MaxDelay: time.Hour},
		BreakerConfig{Enabled: false},
		discardLog(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	p.Do(ctx, "openai", func(context.Context) (int, []byte, error) {
		calls++
		return 500, nil, nil
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (context cancelled before the retry backoff)", calls)
	}
}
