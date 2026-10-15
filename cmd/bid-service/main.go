// Command bid-service runs the vendor quotation service, including the price
// comparison report.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"

	"github.com/ank/e-proc/internal/bid"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
	bidv1 "github.com/ank/e-proc/proto/gen/bid/v1"
)

func main() {
	// Runs as its own container probe (`bid-service -healthcheck`) and exits
	// immediately when that flag is absent.
	app.MaybeHealthcheck("bid-service")

	cfg, err := config.Load("bid-service")
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

	repo := bid.NewRepository(deps.DB)
	// Cross-service check over gRPC. Disabled when the address is unset, and the
	// warning says so instead of leaving the behaviour implicit.
	var validator bid.TenderValidator
	if cfg.TenderGRPCAddr != "" {
		client, err := bid.NewTenderClient(cfg.TenderGRPCAddr)
		if err != nil {
			log.Error("tender client setup failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()
		validator = client
		log.Info("cross-service tender validation enabled", slog.String("addr", cfg.TenderGRPCAddr))
	} else {
		log.Warn("TENDER_GRPC_ADDR is not set: bids are accepted without checking the tender state")
	}

	svc := bid.NewService(repo, deps.Cache, deps.Publisher, validator, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	grpcServer := bid.NewGRPCServer(svc)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
		RegisterGRPC: func(s *grpc.Server, _ app.Deps) {
			bidv1.RegisterBidServiceServer(s, grpcServer)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
