// Package logger provides the shared structured logger for every service.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog logger. Production emits JSON for log collectors; other
// environments emit text so the terminal stays readable.
func New(service, env, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler).With(slog.String("service", service), slog.String("env", env))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
