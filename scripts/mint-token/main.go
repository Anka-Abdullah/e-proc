// Command mint-token prints a signed JWT for local testing, so the API can be
// exercised before the auth service exists.
//
// Usage:
//
//	go run ./scripts/mint-token -role admin -tenant demo -user u-admin
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/config"
)

func main() {
	role := flag.String("role", string(auth.RoleAdmin), "role: admin | procurement_officer | approver | vendor")
	tenant := flag.String("tenant", "demo", "tenant id")
	user := flag.String("user", "", "user id (defaults to role name)")
	ttl := flag.Duration("ttl", 12*time.Hour, "token lifetime")
	flag.Parse()

	cfg, err := config.Load("mint-token")
	if err != nil {
		slog.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	parsed := auth.Role(*role)
	switch parsed {
	case auth.RoleAdmin, auth.RoleProcurement, auth.RoleApprover, auth.RoleVendor:
	default:
		fmt.Fprintf(os.Stderr, "unknown role %q\n", *role)
		os.Exit(1)
	}

	userID := *user
	if userID == "" {
		userID = "u-" + *role
	}

	token, err := auth.Issue(cfg.JWTSecret, *ttl, auth.Claims{
		UserID:   userID,
		Email:    userID + "@example.com",
		Role:     parsed,
		TenantID: *tenant,
	})
	if err != nil {
		slog.Error("token issue failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	fmt.Println(token)
}
