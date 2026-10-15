package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// Gateway routes requests to upstream services.
type Gateway struct {
	cfg     Config
	log     *slog.Logger
	proxies map[string]*httputil.ReverseProxy
}

// New builds a gateway, creating one reverse proxy per upstream.
func New(cfg Config, log *slog.Logger) (*Gateway, error) {
	proxies := make(map[string]*httputil.ReverseProxy, len(cfg.Upstreams))

	for key, upstream := range cfg.Upstreams {
		target, err := url.Parse(upstream.BaseURL)
		if err != nil {
			return nil, err
		}
		name := upstream.Name
		proxy := httputil.NewSingleHostReverseProxy(target)

		// NewSingleHostReverseProxy installs a Director; using Rewrite requires
		// clearing it, or the proxy refuses to run ("exactly one of Director or
		// Rewrite set"). Rewrite is the supported API since Go 1.20.
		proxy.Director = nil
		proxy.Rewrite = func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Service", "api-gateway")
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "upstream unreachable",
				slog.String("service", name),
				slog.String("path", r.URL.Path),
				slog.String("error", err.Error()))
			httpx.Fail(w, http.StatusBadGateway, "bad_gateway", "upstream service "+name+" is unavailable")
		}
		proxies[key] = proxy
	}

	return &Gateway{cfg: cfg, log: log, proxies: proxies}, nil
}

// Mount wires the gateway onto the router: login stays public, every other
// route sits behind the authenticator, and each prefix goes to its owner.
func (g *Gateway) Mount(router chi.Router, authenticator *middleware.Authenticator) {
	router.Group(func(public chi.Router) {
		public.Post("/api/v1/auth/login", g.handler("auth"))
	})

	router.Group(func(protected chi.Router) {
		protected.Use(authenticator.Authenticate)

		for _, route := range g.cfg.Routes {
			key := upstreamKey(route.Upstream.Name)
			if route.Prefix == "/api/v1/auth" {
				continue // handled below so /login is not shadowed
			}
			protected.HandleFunc(route.Prefix, g.handler(key))
			protected.HandleFunc(route.Prefix+"/*", g.handler(key))
		}

		// Authenticated auth endpoints: /auth/me, /auth/logout, /auth/register.
		protected.HandleFunc("/api/v1/auth/me", g.handler("auth"))
		protected.HandleFunc("/api/v1/auth/logout", g.handler("auth"))
		protected.HandleFunc("/api/v1/auth/register", g.handler("auth"))
	})
}

// handler builds the http.HandlerFunc for one upstream, stripping the API
// prefix so services keep their own clean paths.
func (g *Gateway) handler(key string) http.HandlerFunc {
	proxy, ok := g.proxies[key]
	return func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "unknown route")
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		if path == "" {
			path = "/"
		}
		r.URL.Path = path

		// Carry the authenticated identity downstream for logs and auditing.
		if claims := middleware.ClaimsFrom(r.Context()); claims != nil {
			r.Header.Set("X-User-Id", claims.UserID)
			r.Header.Set("X-Tenant-Id", claims.TenantID)
			r.Header.Set("X-User-Role", string(claims.Role))
		}
		if requestID := middleware.RequestIDFrom(r.Context()); requestID != "" {
			r.Header.Set("X-Request-Id", requestID)
		}

		proxy.ServeHTTP(w, r)
	}
}

// CheckUpstreams probes every upstream's /readyz so the gateway can report
// readiness honestly instead of claiming to be healthy while its dependencies
// are down.
func (g *Gateway) CheckUpstreams(ctx context.Context, timeout time.Duration) map[string]string {
	client := &http.Client{Timeout: timeout}
	results := make(map[string]string, len(g.cfg.Upstreams))

	for key, upstream := range g.cfg.Upstreams {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.BaseURL+"/readyz", nil)
		if err != nil {
			results[key] = "unreachable"
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			results[key] = "unreachable"
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			results[key] = "ok"
			continue
		}
		results[key] = "degraded"
	}
	return results
}

// upstreamKey maps a service name to its key in the proxy map. It is the single
// place that knows the naming, so a new service only has to be added once.
func upstreamKey(serviceName string) string {
	switch serviceName {
	case "auth-service":
		return "auth"
	case "vendor-service":
		return "vendor"
	case "tender-service":
		return "tender"
	case "bid-service":
		return "bid"
	case "po-service":
		return "po"
	case "approval-service":
		return "approval"
	case "audit-service":
		return "audit"
	case "reporting-service":
		return "reporting"
	default:
		return serviceName
	}
}
