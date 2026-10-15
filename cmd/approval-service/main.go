// Command approval-service runs the approval workflow service. Besides the API
// it consumes po.created to open approvals automatically and runs a reminder
// sweep for approvals that sit too long.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"

	"github.com/ank/e-proc/internal/approval"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
	approvalv1 "github.com/ank/e-proc/proto/gen/approval/v1"
)

// approvalQueue is the durable queue this service consumes from.
const approvalQueue = "approval.events"

func main() {
	cfg, err := config.Load("approval-service")
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

	repo := approval.NewRepository(deps.DB)
	svc := approval.NewService(repo, deps.Cache, deps.Publisher, cfg.ApprovalAssignee, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	workerCtx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	go runConsumer(workerCtx, cfg, svc, log)
	go runReminderSweep(workerCtx, cfg, svc, log)

	grpcServer := approval.NewGRPCServer(svc)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
		RegisterGRPC: func(s *grpc.Server, _ app.Deps) {
			approvalv1.RegisterApprovalServiceServer(s, grpcServer)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// runConsumer opens approvals automatically when po-service asks for one.
func runConsumer(ctx context.Context, cfg config.Config, svc *approval.Service, log *slog.Logger) {
	consumer, err := broker.NewConsumer(cfg.RabbitURL, approvalQueue, approval.RoutingKeys(), log)
	if err != nil {
		log.Error("approval consumer setup failed", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = consumer.Close() }()

	log.Info("approval consumer started",
		slog.String("queue", approvalQueue),
		slog.Any("routing_keys", approval.RoutingKeys()),
		slog.String("assignee", cfg.ApprovalAssignee))
	if err := consumer.Run(ctx, svc.Handler()); err != nil {
		log.Error("approval consumer stopped", slog.String("error", err.Error()))
	}
}

// runReminderSweep announces approvals that have been pending too long.
func runReminderSweep(ctx context.Context, cfg config.Config, svc *approval.Service, log *slog.Logger) {
	interval := cfg.ReminderInterval
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := svc.RemindersDue(ctx, cfg.ApprovalReminderAfter, 100); err != nil {
				log.ErrorContext(ctx, "reminder sweep failed", slog.String("error", err.Error()))
			}
		}
	}
}
