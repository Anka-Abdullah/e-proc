// Command po-service runs the purchase order service: order creation from a
// selected bid, the status machine and the approval threshold rule. It also
// consumes po.approved / po.rejected so an approval decision moves the order
// automatically, with no UI patching.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"

	"github.com/ank/e-proc/internal/po"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/broker"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
	pov1 "github.com/ank/e-proc/proto/gen/po/v1"
)

// poQueue is the durable queue this service consumes from.
const poQueue = "po.events"

func main() {
	cfg, err := config.Load("po-service")
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

	repo := po.NewRepository(deps.DB)
	svc := po.NewService(repo, deps.Cache, deps.Publisher, cfg.ApprovalThreshold, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	go runConsumer(consumerCtx, cfg, svc, log)

	grpcServer := po.NewGRPCServer(svc)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
		RegisterGRPC: func(s *grpc.Server, _ app.Deps) {
			pov1.RegisterPurchaseOrderServiceServer(s, grpcServer)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// runConsumer applies approval decisions to their purchase orders.
func runConsumer(ctx context.Context, cfg config.Config, svc *po.Service, log *slog.Logger) {
	consumer, err := broker.NewConsumer(cfg.RabbitURL, poQueue, po.RoutingKeys(), log)
	if err != nil {
		log.Error("po consumer setup failed", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = consumer.Close() }()

	log.Info("po consumer started",
		slog.String("queue", poQueue),
		slog.Any("routing_keys", po.RoutingKeys()))
	if err := consumer.Run(ctx, svc.Handler()); err != nil {
		log.Error("po consumer stopped", slog.String("error", err.Error()))
	}
}
