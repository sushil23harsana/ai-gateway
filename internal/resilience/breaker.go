// Package resilience adds production-grade resilience to upstream provider
// calls: bounded retry with exponential backoff + jitter for transient
// failures, and a per-provider circuit breaker that stops hammering a provider
// that is persistently failing (so the handler can fail over or fail fast
// instead of piling requests onto a dead upstream).
//
// The two work together: retry smooths over brief blips within a single
// request; the breaker tracks health across requests. A request only reports
// one outcome to the breaker — the final result after its retries — so a flaky
// provider that recovers on retry does not trip the breaker, but one that fails
// every attempt repeatedly does.
package resilience

import (
	"log/slog"
	"sync"
	"time"
)

// State is a circuit-breaker state.
type State int

const (
	StateClosed   State = iota // requests flow; failures are counted
	StateOpen                  // requests are rejected until the cooldown elapses
	StateHalfOpen              // a limited number of trial requests are allowed
)

func (s State) String() string {
	switch s {
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// BreakerConfig tunes a circuit breaker.
type BreakerConfig struct {
	Enabled     bool
	Threshold   int           // consecutive failures that trip a closed breaker
	Cooldown    time.Duration // how long a breaker stays open before a trial
	HalfOpenMax int           // trial requests allowed in half-open (and successes needed to close)
}

// breaker is a single provider's circuit breaker. It is safe for concurrent use.
type breaker struct {
	name string
	cfg  BreakerConfig
	log  *slog.Logger
	now  func() time.Time

	mu                sync.Mutex
	state             State
	consecutiveFails  int
	openedAt          time.Time
	halfOpenInFlight  int
	halfOpenSuccesses int
}

// allow reports whether a request may proceed, transitioning Open→HalfOpen once
// the cooldown elapses. In half-open it admits up to HalfOpenMax trial requests.
func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if b.now().Sub(b.openedAt) < b.cfg.Cooldown {
			return false
		}
		// Cooldown elapsed — probe the provider with half-open trials.
		b.state = StateHalfOpen
		b.halfOpenInFlight = 0
		b.halfOpenSuccesses = 0
		b.log.Info("circuit breaker half-open (probing)", "provider", b.name)
		fallthrough
	case StateHalfOpen:
		if b.halfOpenInFlight < b.cfg.HalfOpenMax {
			b.halfOpenInFlight++
			return true
		}
		return false
	}
	return true
}

// report records the outcome of a request that allow() admitted.
func (b *breaker) report(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		if success {
			b.consecutiveFails = 0
			return
		}
		b.consecutiveFails++
		if b.consecutiveFails >= b.cfg.Threshold {
			b.state = StateOpen
			b.openedAt = b.now()
			b.log.Warn("circuit breaker opened", "provider", b.name, "consecutive_failures", b.consecutiveFails)
		}
	case StateHalfOpen:
		if b.halfOpenInFlight > 0 {
			b.halfOpenInFlight--
		}
		if !success {
			// A failed probe re-opens the breaker for another cooldown.
			b.state = StateOpen
			b.openedAt = b.now()
			b.log.Warn("circuit breaker re-opened after failed probe", "provider", b.name)
			return
		}
		b.halfOpenSuccesses++
		if b.halfOpenSuccesses >= b.cfg.HalfOpenMax {
			b.state = StateClosed
			b.consecutiveFails = 0
			b.log.Info("circuit breaker closed (recovered)", "provider", b.name)
		}
	case StateOpen:
		// Shouldn't happen: allow() gates requests while open. Ignore.
	}
}

func (b *breaker) currentState() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Registry holds one breaker per provider name, all sharing one config.
type Registry struct {
	cfg BreakerConfig
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	breakers map[string]*breaker
}

// NewRegistry builds a breaker registry. When cfg.Enabled is false, Allow always
// returns true and Report is a no-op.
func NewRegistry(cfg BreakerConfig, log *slog.Logger) *Registry {
	return &Registry{cfg: cfg, log: log, now: time.Now, breakers: map[string]*breaker{}}
}

func (r *Registry) get(provider string) *breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.breakers[provider]
	if !ok {
		b = &breaker{name: provider, cfg: r.cfg, log: r.log, now: r.now}
		r.breakers[provider] = b
	}
	return b
}

// Allow reports whether a request to provider may proceed.
func (r *Registry) Allow(provider string) bool {
	if !r.cfg.Enabled {
		return true
	}
	return r.get(provider).allow()
}

// Report records the outcome of a request that Allow admitted.
func (r *Registry) Report(provider string, success bool) {
	if !r.cfg.Enabled {
		return
	}
	r.get(provider).report(success)
}

// State returns the current breaker state for a provider (StateClosed when
// breaking is disabled). Useful for observability.
func (r *Registry) State(provider string) State {
	if !r.cfg.Enabled {
		return StateClosed
	}
	return r.get(provider).currentState()
}
