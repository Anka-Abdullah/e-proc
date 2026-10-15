// Command api-gateway is the single entry point for clients: it terminates
// authentication, applies CORS and rate limiting, and routes each API prefix to
// the service that owns it.
//
// The gateway has no database. It needs Redis (for rate limiting) and its
// upstreams; readiness therefore reports upstream health, not datastore health.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/internal/gateway"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/cache"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
)

func main() {
	// Runs as its own container probe (`api-gateway -healthcheck`) and exits
	// immediately when that flag is absent.
	app.MaybeHealthcheck("api-gateway")

	cfg, err := config.Load("api-gateway")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log := logger.New(cfg.Service, cfg.Env, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Redis backs the edge rate limiter. Failing loudly beats silently running
	// an edge gateway with no protection at all.
	cacheClient, err := cache.New(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Error("redis connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = cacheClient.Close() }()

	gwCfg := gateway.LoadConfig()
	gw, err := gateway.New(gwCfg, log)
	if err != nil {
		log.Error("gateway setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger(log))
	router.Use(middleware.Recoverer(log))
	router.Use(gateway.CORS(gwCfg.FrontendOrigin))
	router.Use(middleware.RateLimit(cacheClient, cfg.RateLimitRPS))

	gw.MountHealth(router)

	authenticator := middleware.NewAuthenticator(cfg.JWTSecret, cacheClient, log)
	gw.Mount(router, authenticator)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "route not found")
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("api-gateway listening",
			slog.String("addr", cfg.HTTPAddr),
			slog.Any("routes", routeSummary(gwCfg)))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Error("gateway stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	case <-ctx.Done():
		log.Info("shutdown signal received", slog.String("service", cfg.Service))
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown error", slog.String("error", err.Error()))
	}
	log.Info("shutdown complete")
}

func routeSummary(cfg gateway.Config) []string {
	routes := make([]string, 0, len(cfg.Routes))
	for _, route := range cfg.Routes {
		routes = append(routes, route.Prefix+" -> "+route.Upstream.BaseURL)
	}
	return routes
}
