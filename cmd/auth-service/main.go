// Command auth-service handles accounts: login, logout, registration and the
// current-user endpoint.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/internal/account"
	"github.com/ank/e-proc/pkg/app"
	"github.com/ank/e-proc/pkg/config"
	"github.com/ank/e-proc/pkg/logger"
	"github.com/ank/e-proc/pkg/middleware"
)

func main() {
	cfg, err := config.Load("auth-service")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log := logger.New(cfg.Service, cfg.Env, cfg.LogLevel)

	ctx := context.Background()
	// This service only needs Mongo and Redis; publishing is not part of login.
	deps, cleanup, err := app.Connect(ctx, cfg, log, true)
	if err != nil {
		log.Error("dependency connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	repo := account.NewRepository(deps.DB)
	svc := account.NewService(repo, deps.Cache, cfg.JWTSecret, cfg.JWTTTL, log)
	if err := svc.EnsureIndexes(ctx); err != nil {
		log.Error("index setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	err = app.Run(ctx, *deps, app.Options{
		Routes: func(r chi.Router, _ app.Deps) {
			svc.MountPublic(r)
		},
		Authenticated: func(r chi.Router, _ app.Deps, _ *middleware.Authenticator) {
			svc.MountProtected(r)
		},
	})
	if err != nil {
		log.Error("service stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
