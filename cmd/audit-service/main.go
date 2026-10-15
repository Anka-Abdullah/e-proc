// Command audit-service records every domain event as an append-only audit
// entry and serves read-only queries over them.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/internal/audit"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
)

// auditQueue is the durable queue this service consumes from.
const auditQueue = "audit.events"

func main() {
	cfg, err := config.Load("audit-service")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log := logger.New(cfg.Service, cfg.Env, cfg.LogLevel)

	ctx := context.Background()
	// This service only consumes; it never publishes.
	deps, cleanup, err := app.Connect(ctx, cfg, log, true)
	if err != nil {
		log.Error("dependency connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	repo := audit.NewRepository(deps.DB)
	svc := audit.NewService(repo, deps.Cache, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	go runConsumer(consumerCtx, cfg, svc, log)

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

// runConsumer subscribes to the domain events and writes audit entries. A
// handler error nacks the message, so a poison message cannot loop forever.
func runConsumer(ctx context.Context, cfg config.Config, svc *audit.Service, log *slog.Logger) {
	consumer, err := broker.NewConsumer(cfg.RabbitURL, auditQueue, audit.RoutingKeys(), log)
	if err != nil {
		log.Error("audit consumer setup failed", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = consumer.Close() }()

	log.Info("audit consumer started", slog.String("queue", auditQueue), slog.Int("routing_keys", len(audit.RoutingKeys())))
	if err := consumer.Run(ctx, svc.Handler()); err != nil {
		log.Error("audit consumer stopped", slog.String("error", err.Error()))
	}
}
