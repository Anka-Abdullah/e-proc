// Package middleware holds the HTTP middleware every service installs:
// request ids, structured logging, panic recovery, JWT auth, RBAC and rate
// limiting.
package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/cache"
	"github.com/ank/e-proc/pkg/httpx"
)

type contextKey string

const (
	ctxKeyRequestID contextKey = "request_id"
	ctxKeyClaims    contextKey = "claims"
)

// RequestID attaches a request id, honouring an inbound X-Request-Id.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID, id)))
	})
}

// RequestIDFrom returns the request id stored in ctx.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// statusRecorder captures the status code for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Logger writes one structured access log line per request.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			log.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recoverer turns a panic into a 500 instead of killing the process.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(ctx, "panic recovered",
						slog.Any("panic", rec),
						slog.String("request_id", RequestIDFrom(ctx)),
					)
					httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Authenticator validates bearer tokens and injects claims into the context.
type Authenticator struct {
	secret   string
	sessions *cache.Client
	log      *slog.Logger
}

// NewAuthenticator builds an Authenticator. sessions may be nil, in which case
// token revocation is not checked.
func NewAuthenticator(secret string, sessions *cache.Client, log *slog.Logger) *Authenticator {
	return &Authenticator{secret: secret, sessions: sessions, log: log}
}

// Authenticate rejects requests without a valid, non-revoked token.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := httpx.BearerToken(r)
		if !ok {
			httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "missing bearer token")
			return
		}
		claims, err := auth.Parse(a.secret, token)
		if err != nil {
			httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "invalid or expired token")
			return
		}
		// The JWT is self-contained: a missing session record is normal (tokens
		// minted by CLI tools, service tokens). Only an explicit revocation
		// marker in Redis invalidates it.
		if a.sessions != nil {
			revoked, err := a.sessions.IsRevoked(r.Context(), token)
			if err != nil {
				httpx.Fail(w, http.StatusServiceUnavailable, httpx.CodeInternal, "authorisation store unavailable")
				return
			}
			if revoked {
				httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "token revoked")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// WithClaims stores claims in a context.
func WithClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, claims)
}

// ClaimsFrom returns the authenticated claims, or nil for anonymous callers.
func ClaimsFrom(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(ctxKeyClaims).(*auth.Claims)
	return claims
}

// RequirePermission denies the request unless the caller's role holds perm.
func RequirePermission(perm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r.Context())
			if claims == nil {
				httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
				return
			}
			if !claims.Can(perm) {
				httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "role "+string(claims.Role)+" may not "+string(perm))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole denies the request unless the caller holds one of roles.
func RequireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	allowed := make(map[auth.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r.Context())
			if claims == nil {
				httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
				return
			}
			if _, ok := allowed[claims.Role]; !ok {
				httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "role not permitted")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// probePaths are never rate limited: liveness/readiness endpoints are polled by
// orchestration and monitoring, and counting those probes against the client
// budget makes a healthy service look throttled.
var probePaths = map[string]struct{}{
	"/healthz": {},
	"/readyz":  {},
}

// RateLimit caps requests per client IP per minute using Redis counters.
func RateLimit(store *cache.Client, limitPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if store == nil || limitPerMinute <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			if _, isProbe := probePaths[r.URL.Path]; isProbe {
				next.ServeHTTP(w, r)
				return
			}
			key := "ratelimit:" + clientIP(r)
			count, err := store.IncrWithTTL(r.Context(), key, time.Minute)
			if err == nil && count > int64(limitPerMinute) {
				w.Header().Set("Retry-After", "60")
				httpx.Fail(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
