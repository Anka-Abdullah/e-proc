package gateway

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/httpx"
)

// readinessTimeout bounds how long a readiness probe waits per upstream.
const readinessTimeout = 2 * time.Second

// CORS answers preflight requests and echoes the configured frontend origin.
// Only one origin is allowed: the platform's own web client.
func CORS(frontendOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && strings.EqualFold(origin, frontendOrigin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After")
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ReadinessHandler reports gateway readiness plus a per-upstream summary.
func (g *Gateway) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstreams := g.CheckUpstreams(r.Context(), readinessTimeout)
		ready := true
		for _, status := range upstreams {
			if status != "ok" {
				ready = false
				break
			}
		}

		payload := map[string]any{"gateway": "ok", "upstreams": upstreams}
		if !ready {
			httpx.JSON(w, http.StatusServiceUnavailable, httpx.Envelope{
				Data:  payload,
				Error: &httpx.APIError{Code: "not_ready", Message: "one or more upstreams are not ready"},
			})
			return
		}
		httpx.OK(w, payload)
	}
}

// HealthHandler is the liveness probe: it answers without touching upstreams.
func (g *Gateway) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.OK(w, map[string]any{"status": "ok", "service": "api-gateway"})
	}
}

// MountHealth registers the probe endpoints on the router.
func (g *Gateway) MountHealth(router chi.Router) {
	router.Get("/healthz", g.HealthHandler())
	router.Get("/readyz", g.ReadinessHandler())
}
