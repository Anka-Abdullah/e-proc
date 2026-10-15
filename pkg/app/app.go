// Package app assembles a service: shared dependencies, HTTP server, gRPC
// server, health checks and graceful shutdown. Each cmd/<service>/main.go only
// wires its own routes and RPC registrations.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/cache"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/db"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// Deps is everything a service shares: configuration, logger and the three
// infrastructure clients.
type Deps struct {
	Cfg       config.Config
	Log       *slog.Logger
	DB        *mongo.Database
	Cache     *cache.Client
	Publisher *broker.Publisher
	Mongo     *mongo.Client
}

// Options describes what is specific to one service.
type Options struct {
	// Routes mounts the service's HTTP handlers.
	Routes func(r chi.Router, deps Deps)
	// RegisterGRPC mounts the service's gRPC servers.
	RegisterGRPC func(s *grpc.Server, deps Deps)
	// HealthChecks are extra readiness probes (name -> check).
	HealthChecks map[string]func(ctx context.Context) error
	// Authenticated is called with the router after the public routes, so each
	// service can mount protected routes without repeating the middleware chain.
	Authenticated func(r chi.Router, deps Deps, auth *middleware.Authenticator)
	// SkipPublisher avoids connecting to RabbitMQ (tests, pure read services).
	SkipPublisher bool
}

// Connect builds Deps from configuration: MongoDB, Redis and RabbitMQ.
func Connect(ctx context.Context, cfg config.Config, log *slog.Logger, skipPublisher bool) (*Deps, func(), error) {
	mongoClient, database, err := db.Connect(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return nil, nil, err
	}

	cacheClient, err := cache.New(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		_ = mongoClient.Disconnect(ctx)
		return nil, nil, err
	}

	deps := &Deps{Cfg: cfg, Log: log, DB: database, Cache: cacheClient, Mongo: mongoClient}

	// Cleanup must survive the caller's context being cancelled (services call
	// it from a deferred cleanup while shutting down), but it should still
	// derive from it so values and cancellation intent are explicit.
	cleanupCtx := context.WithoutCancel(ctx)

	closers := []func(){
		func() { _ = cacheClient.Close() },
		func() { _ = mongoClient.Disconnect(cleanupCtx) },
	}

	if !skipPublisher {
		publisher, err := broker.NewPublisher(cfg.RabbitURL, log)
		if err != nil {
			for _, closeFn := range closers {
				closeFn()
			}
			return nil, nil, err
		}
		deps.Publisher = publisher
		closers = append(closers, func() { _ = publisher.Close() })
	}

	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	return deps, cleanup, nil
}

// Run starts the HTTP and gRPC servers and blocks until the context is
// cancelled or a termination signal arrives, then shuts both down gracefully.
func Run(ctx context.Context, deps Deps, opts Options) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(recoveryInterceptor(deps.Log)),
	)
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	if opts.RegisterGRPC != nil {
		opts.RegisterGRPC(grpcServer, deps)
	}
	reflection.Register(grpcServer)

	listenConfig := net.ListenConfig{}

	grpcListener, err := listenConfig.Listen(ctx, "tcp", deps.Cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc %s: %w", deps.Cfg.GRPCAddr, err)
	}

	httpServer := &http.Server{
		Addr:              deps.Cfg.HTTPAddr,
		Handler:           buildHandler(deps, opts),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	httpListener, err := listenConfig.Listen(ctx, "tcp", deps.Cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen http %s: %w", deps.Cfg.HTTPAddr, err)
	}

	errCh := make(chan error, 2)
	go func() {
		deps.Log.Info("http server listening", slog.String("addr", deps.Cfg.HTTPAddr), slog.String("service", deps.Cfg.Service))
		if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http serve: %w", err)
		}
	}()
	go func() {
		deps.Log.Info("grpc server listening", slog.String("addr", deps.Cfg.GRPCAddr), slog.String("service", deps.Cfg.Service))
		if err := grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("grpc serve: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		stop()
		shutdown(ctx, httpServer, grpcServer, deps.Log)
		return err
	case <-ctx.Done():
		deps.Log.Info("shutdown signal received", slog.String("service", deps.Cfg.Service))
	}

	shutdown(ctx, httpServer, grpcServer, deps.Log)
	return nil
}

func shutdown(ctx context.Context, httpServer *http.Server, grpcServer *grpc.Server, log *slog.Logger) {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown error", slog.String("error", err.Error()))
	}
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Warn("grpc graceful stop timed out, forcing")
		grpcServer.Stop()
	}
	log.Info("shutdown complete")
}

func buildHandler(deps Deps, opts Options) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger(deps.Log))
	router.Use(middleware.Recoverer(deps.Log))
	router.Use(middleware.RateLimit(deps.Cache, deps.Cfg.RateLimitRPS))

	router.Get("/healthz", healthHandler(deps))
	router.Get("/readyz", readinessHandler(deps, opts))

	if opts.Routes != nil {
		opts.Routes(router, deps)
	}

	if opts.Authenticated != nil {
		auth := middleware.NewAuthenticator(deps.Cfg.JWTSecret, deps.Cache, deps.Log)
		router.Group(func(protected chi.Router) {
			protected.Use(auth.Authenticate)
			opts.Authenticated(protected, deps, auth)
		})
	}

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "route not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, http.StatusMethodNotAllowed, httpx.CodeBadRequest, "method not allowed")
	})
	return router
}

func healthHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.OK(w, map[string]any{"status": "ok", "service": deps.Cfg.Service})
	}
}

func readinessHandler(deps Deps, opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := map[string]error{
			"mongo": deps.Mongo.Ping(ctx, nil),
		}
		if deps.Cache != nil {
			checks["redis"] = deps.Cache.Raw().Ping(ctx).Err()
		}
		for name, check := range opts.HealthChecks {
			if check == nil {
				continue
			}
			checks[name] = check(ctx)
		}

		status := http.StatusOK
		report := make(map[string]string, len(checks))
		for name, err := range checks {
			if err != nil {
				status = http.StatusServiceUnavailable
				report[name] = err.Error()
				continue
			}
			report[name] = "ok"
		}
		if status != http.StatusOK {
			httpx.JSON(w, status, httpx.Envelope{Error: &httpx.APIError{Code: "not_ready", Message: "dependency check failed"}, Data: report})
			return
		}
		httpx.OK(w, report)
	}
}

func recoveryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				log.ErrorContext(ctx, "grpc panic recovered", slog.Any("panic", rec), slog.String("method", info.FullMethod))
				err = fmt.Errorf("internal error")
			}
		}()
		return handler(ctx, req)
	}
}
