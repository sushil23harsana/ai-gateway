package resilience

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

// ErrCircuitOpen is returned by Policy.Do when the provider's breaker is open,
// so the caller can fail over (or fail fast) without touching the upstream.
var ErrCircuitOpen = errors.New("circuit breaker open")

// RetryConfig tunes bounded retry with exponential backoff + full jitter.
type RetryConfig struct {
	MaxAttempts int           // total attempts including the first (1 = no retry)
	BaseDelay   time.Duration // backoff before the first retry
	MaxDelay    time.Duration // cap on the backoff
}

// Policy bundles retry + circuit breaking for upstream provider calls.
type Policy struct {
	retry    RetryConfig
	breakers *Registry
	log      *slog.Logger

	// Injectable for tests.
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func(max int64) int64
}

// NewPolicy builds a resilience policy from a retry and breaker config.
func NewPolicy(retry RetryConfig, breaker BreakerConfig, log *slog.Logger) *Policy {
	if retry.MaxAttempts < 1 {
		retry.MaxAttempts = 1
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Policy{
		retry:    retry,
		breakers: NewRegistry(breaker, log),
		log:      log,
		sleep:    sleepCtx,
		jitter: func(max int64) int64 {
			if max <= 0 {
				return 0
			}
			return rand.Int63n(max)
		},
	}
}

// Disabled returns a no-op policy: one attempt, no breaker. Used in tests and
// when resilience is turned off.
func Disabled() *Policy {
	return NewPolicy(RetryConfig{MaxAttempts: 1}, BreakerConfig{Enabled: false}, nil)
}

// State exposes a provider's current breaker state (for observability).
func (p *Policy) State(provider string) State { return p.breakers.State(provider) }

// Do runs fn under the provider's circuit breaker, retrying transient failures
// (transport errors, 5xx, and 429) per the retry policy. It returns the final
// status/body/error. If the breaker is open it returns ErrCircuitOpen without
// calling fn, so the caller can fail over.
//
// Exactly one outcome (the final result, after any retries) is reported to the
// breaker, so a provider that recovers on retry is not counted as unhealthy.
func (p *Policy) Do(ctx context.Context, provider string, fn func(context.Context) (int, []byte, error)) (int, []byte, error) {
	if !p.breakers.Allow(provider) {
		return 0, nil, ErrCircuitOpen
	}

	var (
		status int
		body   []byte
		err    error
	)
	for attempt := 1; attempt <= p.retry.MaxAttempts; attempt++ {
		status, body, err = fn(ctx)
		retryable := isRetryable(status, err)
		if !retryable || attempt == p.retry.MaxAttempts {
			p.breakers.Report(provider, !retryable)
			return status, body, err
		}
		delay := p.backoff(attempt)
		p.log.Warn("upstream call failed; retrying",
			"provider", provider, "attempt", attempt, "max", p.retry.MaxAttempts,
			"status", status, "err", err, "backoff_ms", delay.Milliseconds())
		if serr := p.sleep(ctx, delay); serr != nil {
			// Context cancelled/expired during backoff — record the failure and
			// return the last upstream result so the caller can fail over.
			p.breakers.Report(provider, false)
			return status, body, err
		}
	}
	return status, body, err
}

// backoff returns BaseDelay * 2^(attempt-1), capped at MaxDelay, then full
// jitter applied (a random duration in [0, capped)).
func (p *Policy) backoff(attempt int) time.Duration {
	d := p.retry.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= p.retry.MaxDelay {
			d = p.retry.MaxDelay
			break
		}
	}
	if d > p.retry.MaxDelay {
		d = p.retry.MaxDelay
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(p.jitter(int64(d)))
}

// isRetryable reports whether a result is a transient failure worth retrying:
// a transport error, a 5xx, or a 429 (provider rate-limit). A 4xx is the
// caller's fault and is never retried.
func isRetryable(status int, err error) bool {
	if err != nil {
		return true
	}
	if status >= 500 {
		return true
	}
	return status == http.StatusTooManyRequests
}

// sleepCtx sleeps for d, returning early if the context is done. It checks the
// context first so a cancelled context is honored even for a zero delay.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
