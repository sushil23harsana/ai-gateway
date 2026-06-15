// Package metrics keeps observability writes off the hot path. The proxy
// enqueues a RequestLog onto a buffered channel; a small worker pool drains it
// to Postgres. The response is never blocked on a metrics/DB write — if the
// buffer is full, the entry is dropped with a warning rather than back-pressuring
// the request.
package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sushil23harsana/ai-gateway/internal/store"
)

// SpendAdder records per-key spend (satisfied by *budget.Tracker). Optional; when
// set, the worker accumulates each billed request's cost for budget enforcement.
type SpendAdder interface {
	Add(ctx context.Context, keyID string, costUSD float64)
}

// Logger is the async request-log writer.
type Logger struct {
	store   *store.Store
	ch      chan store.RequestLog
	wg      sync.WaitGroup
	log     *slog.Logger
	workers int
	spend   SpendAdder
}

// SetSpendTracker attaches a spend tracker. Call before Start.
func (l *Logger) SetSpendTracker(s SpendAdder) { l.spend = s }

// NewLogger constructs a Logger. buffer is the channel depth; workers is the
// number of draining goroutines. Call Start before Enqueue.
func NewLogger(s *store.Store, buffer, workers int, log *slog.Logger) *Logger {
	if buffer <= 0 {
		buffer = 1000
	}
	if workers <= 0 {
		workers = 4
	}
	return &Logger{
		store:   s,
		ch:      make(chan store.RequestLog, buffer),
		log:     log,
		workers: workers,
	}
}

// Start spins up the worker pool.
func (l *Logger) Start() {
	for i := 0; i < l.workers; i++ {
		l.wg.Add(1)
		go l.worker()
	}
}

func (l *Logger) worker() {
	defer l.wg.Done()
	for rl := range l.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := l.store.InsertRequestLog(ctx, rl); err != nil {
			l.log.Error("failed to write request log", "err", err, "provider", rl.Provider, "model", rl.Model)
		}
		// Accumulate spend for budget enforcement (best-effort, same async path).
		if l.spend != nil && rl.APIKeyID != nil && rl.CostUSD > 0 {
			l.spend.Add(ctx, *rl.APIKeyID, rl.CostUSD)
		}
		cancel()
	}
}

// Enqueue hands a log entry to the worker pool. Non-blocking: if the buffer is
// full it drops the entry and warns, so the response path never stalls.
func (l *Logger) Enqueue(rl store.RequestLog) {
	select {
	case l.ch <- rl:
	default:
		l.log.Warn("request-log buffer full; dropping entry", "provider", rl.Provider, "model", rl.Model)
	}
}

// Stop closes the channel and waits for in-flight writes to drain, bounded by ctx.
func (l *Logger) Stop(ctx context.Context) {
	close(l.ch)
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		l.log.Warn("timed out waiting for log workers to drain")
	}
}
