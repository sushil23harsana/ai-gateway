// Command gateway is the entrypoint for the AI Gateway HTTP server.
//
// Phase 0: config, structured logging, graceful shutdown, GET /healthz.
// Phase 1: core proxy POST /v1/chat/completions → OpenAI + async request_logs.
// Phase 2: virtual-key auth (Authorization: Bearer <virtual-key>), an admin API
// to mint keys (POST/GET /admin/keys, guarded by ADMIN_TOKEN), and a Redis
// token-bucket rate limiter per key (429 + Retry-After when exceeded).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sushil23harsana/ai-gateway/internal/api"
	"github.com/sushil23harsana/ai-gateway/internal/cache"
	"github.com/sushil23harsana/ai-gateway/internal/config"
	"github.com/sushil23harsana/ai-gateway/internal/keys"
	"github.com/sushil23harsana/ai-gateway/internal/metrics"
	"github.com/sushil23harsana/ai-gateway/internal/providers"
	"github.com/sushil23harsana/ai-gateway/internal/proxy"
	"github.com/sushil23harsana/ai-gateway/internal/ratelimit"
	"github.com/sushil23harsana/ai-gateway/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	logger.Info("starting ai-gateway",
		"port", cfg.Port,
		"cache_ttl_seconds", cfg.CacheTTLSeconds,
		"priced_models", len(cfg.Pricing.Models),
	)
	if cfg.OpenAIAPIKey == "" {
		logger.Warn("OPENAI_API_KEY is not set; /v1/chat/completions will return 500 until configured")
	}
	if cfg.AdminToken == "" {
		logger.Warn("ADMIN_TOKEN is not set; /admin endpoints are disabled (503)")
	} else if cfg.AdminToken == "change-me" {
		logger.Warn("ADMIN_TOKEN is the insecure default 'change-me'; set a real value")
	}

	// PostgreSQL — request_logs and api_keys live here. Connectivity is required.
	st, err := store.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "err", err)
		return err
	}
	defer st.Close()

	// Redis — token-bucket rate limiting. Required.
	ropt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("failed to parse REDIS_URL", "err", err)
		return err
	}
	rdb := redis.NewClient(ropt)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		pingCancel()
		logger.Error("failed to connect to redis", "err", err)
		return err
	}
	pingCancel()
	defer rdb.Close()

	// Async request-log writer (channel + worker pool).
	mlogger := metrics.NewLogger(st, 1000, 4, logger)
	mlogger.Start()

	openai := providers.NewOpenAI(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey)
	upstream := &http.Client{Timeout: time.Duration(cfg.UpstreamTimeoutSeconds) * time.Second}
	respCache := cache.New(rdb, cfg.CacheTTLSeconds, cfg.CacheScope, cfg.CacheMaxBytes, cfg.CacheEnabled, logger)
	logger.Info("response cache", "enabled", respCache.Enabled(), "scope", string(respCache.Scope()), "ttl_seconds", cfg.CacheTTLSeconds)
	proxyHandler := proxy.NewHandler(upstream, openai, cfg.Pricing, mlogger, respCache, logger)

	authenticator := keys.NewAuthenticator(st, logger)
	limiter := ratelimit.NewLimiter(rdb)
	keyAdmin := api.NewKeyAdmin(st, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	// Admin control plane (guarded by ADMIN_TOKEN).
	adminAuth := api.AdminAuth(cfg.AdminToken, logger)
	mux.Handle("POST /admin/keys", adminAuth(http.HandlerFunc(keyAdmin.Create)))
	mux.Handle("GET /admin/keys", adminAuth(http.HandlerFunc(keyAdmin.List)))

	// Proxy: authenticate the virtual key, then rate-limit per key, then forward.
	rateLimited := ratelimit.Middleware(limiter, logger)
	mux.Handle("POST /v1/chat/completions",
		authenticator.Middleware(rateLimited(http.HandlerFunc(proxyHandler.ChatCompletions))))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// WriteTimeout is intentionally 0: LLM responses (and SSE streams) can run
		// far longer than a normal HTTP response. The upstream client has its own
		// timeout, and ReadHeaderTimeout still guards against slow-loris.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		// fall through to drain the logger anyway
	}

	// Drain pending request logs before the deferred st.Close() runs.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer drainCancel()
	mlogger.Stop(drainCtx)

	logger.Info("shutdown complete")
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// requestLogger is a minimal access-log middleware. It never blocks the
// response and captures the status code for observability.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush exposes the underlying flusher so SSE streaming works through the
// access-log middleware.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
