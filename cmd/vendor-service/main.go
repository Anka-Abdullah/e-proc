// Command vendor-service runs the supplier master data service: REST for the
// gateway, gRPC for internal callers.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"

	"github.com/ank/e-proc/internal/vendor"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
	vendorv1 "github.com/ank/e-proc/proto/gen/vendors/v1"
)

func main() {
	// Runs as its own container probe (`vendor-service -healthcheck`) and exits
	// immediately when that flag is absent.
	app.MaybeHealthcheck("vendor-service")

	cfg, err := config.Load("vendor-service")
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

	repo := vendor.NewRepository(deps.DB)
	svc := vendor.NewService(repo, deps.Cache, deps.Publisher, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	grpcServer := vendor.NewGRPCServer(svc)

	err = app.Run(ctx, *deps, app.Options{
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
		RegisterGRPC: func(s *grpc.Server, _ app.Deps) {
			vendorv1.RegisterVendorServiceServer(s, grpcServer)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
