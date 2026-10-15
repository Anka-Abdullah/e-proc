// Command tender-service runs the tender/RFQ service. Besides the HTTP and
// gRPC transports it runs a scheduler that auto-closes tenders whose deadline
// has passed.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"

	"github.com/ank/e-proc/internal/tender"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
	tenderv1 "github.com/ank/e-proc/proto/gen/tender/v1"
)

// autoCloseInterval is how often the scheduler looks for expired tenders. It
// comes from config so a deployment (or a test) can shorten it.

func main() {
	// Runs as its own container probe (`tender-service -healthcheck`) and exits
	// immediately when that flag is absent.
	app.MaybeHealthcheck("tender-service")

	cfg, err := config.Load("tender-service")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log := logger.New(cfg.Service, cfg.Env, cfg.LogLevel)

	ctx := context.Background()
	deps, cleanup, err := app.Connect(ctx, cfg, log, false)
	if err != nil {
		log.Error("dependency connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	repo := tender.NewRepository(deps.DB)
	svc := tender.NewService(repo, deps.Cache, deps.Publisher, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Scheduler: runs beside the API, stops when the process stops.
	schedCtx, stopScheduler := context.WithCancel(ctx)
	defer stopScheduler()
	go runAutoClose(schedCtx, svc, log, cfg.AutoCloseInterval)

	grpcServer := tender.NewGRPCServer(svc)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
		RegisterGRPC: func(s *grpc.Server, _ app.Deps) {
			tenderv1.RegisterTenderServiceServer(s, grpcServer)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func runAutoClose(ctx context.Context, svc *tender.Service, log *slog.Logger, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			closed, err := svc.AutoClose(ctx)
			if err != nil {
				log.ErrorContext(ctx, "auto-close sweep failed", slog.String("error", err.Error()))
				continue
			}
			if closed > 0 {
				log.InfoContext(ctx, "auto-close sweep finished", slog.Int("closed", closed))
			}
		}
	}
}
