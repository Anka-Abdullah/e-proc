// Package config loads every runtime setting from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the union of settings every service understands. Services read only
// the fields they need; keeping one struct avoids a config type per service.
type Config struct {
	Env        string
	LogLevel   string
	Service    string
	HTTPAddr   string
	GRPCAddr   string
	GatewayURL string

	MongoURI      string
	MongoDatabase string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	RabbitURL string

	JWTSecret string
	JWTTTL    time.Duration

	RateLimitRPS      int
	ApprovalThreshold float64
	// AutoCloseInterval controls how often the tender scheduler looks for
	// expired tenders. Configurable so it can be exercised in tests.
	AutoCloseInterval time.Duration
	// ApprovalAssignee receives approvals created automatically from events.
	ApprovalAssignee string
	// ApprovalReminderAfter is how long an approval may sit pending before a
	// reminder is emitted.
	ApprovalReminderAfter time.Duration
	// ReminderInterval is how often the reminder sweep runs.
	ReminderInterval time.Duration
	// ReportInterval is how often the reporting service snapshots its numbers.
	ReportInterval time.Duration
	// TenderGRPCAddr enables cross-service validation over gRPC (bid-service
	// checks the tender before accepting a quotation). Empty disables it.
	TenderGRPCAddr string
}

// Load builds a Config for the named service. Env vars override every default,
// so the same binary runs in dev, staging and prod without code changes.
func Load(service string) (Config, error) {
	cfg := Config{
		Env:                   env("APP_ENV", "development"),
		LogLevel:              env("LOG_LEVEL", "info"),
		Service:               service,
		HTTPAddr:              env("HTTP_ADDR", ":8080"),
		GRPCAddr:              env("GRPC_ADDR", ":9090"),
		GatewayURL:            env("GATEWAY_URL", "http://localhost:8000"),
		MongoURI:              env("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase:         env("MONGO_DATABASE", "eproc"),
		RedisAddr:             env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:         env("REDIS_PASSWORD", ""),
		RabbitURL:             env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		JWTSecret:             env("JWT_SECRET", "dev-secret-change-me"),
		JWTTTL:                envDuration("JWT_TTL", 12*time.Hour),
		RateLimitRPS:          envInt("RATE_LIMIT_RPS", 50),
		ApprovalThreshold:     envFloat("APPROVAL_THRESHOLD", 50_000_000),
		AutoCloseInterval:     envDuration("AUTO_CLOSE_INTERVAL", time.Minute),
		ApprovalAssignee:      env("APPROVAL_ASSIGNEE", "u-approver"),
		ApprovalReminderAfter: envDuration("APPROVAL_REMINDER_AFTER", 24*time.Hour),
		ReminderInterval:      envDuration("REMINDER_INTERVAL", time.Hour),
		ReportInterval:        envDuration("REPORT_INTERVAL", 24*time.Hour),
		TenderGRPCAddr:        env("TENDER_GRPC_ADDR", ""),
	}

	cfg.RedisDB = envInt("REDIS_DB", 0)

	if cfg.JWTSecret == "dev-secret-change-me" && cfg.Env == "production" {
		return cfg, fmt.Errorf("JWT_SECRET must be set when APP_ENV=production")
	}
	if err := validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validate(cfg Config) error {
	required := map[string]string{
		"MONGO_URI":      cfg.MongoURI,
		"MONGO_DATABASE": cfg.MongoDatabase,
		"REDIS_ADDR":     cfg.RedisAddr,
		"RABBITMQ_URL":   cfg.RabbitURL,
		"JWT_SECRET":     cfg.JWTSecret,
	}
	var missing []string
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
