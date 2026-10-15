// Command reporting-service maintains reporting projections from domain events
// and serves the platform's reports, including CSV export.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/internal/reporting"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
)

// reportingQueue is the durable queue this service consumes from.
const reportingQueue = "reporting.events"

func main() {
	cfg, err := config.Load("reporting-service")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log := logger.New(cfg.Service, cfg.Env, cfg.LogLevel)

	ctx := context.Background()
	deps, cleanup, err := app.Connect(ctx, cfg, log, true)
	if err != nil {
		log.Error("dependency connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	repo := reporting.NewRepository(deps.DB)
	projector := reporting.NewProjector(repo, log)
	svc := reporting.NewService(repo, projector)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	go runProjector(consumerCtx, cfg, projector, log)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func runProjector(ctx context.Context, cfg config.Config, projector *reporting.Projector, log *slog.Logger) {
	consumer, err := broker.NewConsumer(cfg.RabbitURL, reportingQueue, reporting.RoutingKeys(), log)
	if err != nil {
		log.Error("reporting consumer setup failed", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = consumer.Close() }()

	log.Info("reporting projector started",
		slog.String("queue", reportingQueue),
		slog.Int("routing_keys", len(reporting.RoutingKeys())))
	if err := consumer.Run(ctx, projector.Handler()); err != nil {
		log.Error("reporting projector stopped", slog.String("error", err.Error()))
	}
}
