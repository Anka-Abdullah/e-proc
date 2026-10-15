// Package gateway is the single entry point for clients. It terminates
// authentication at the edge, applies rate limiting and CORS, and routes each
// API prefix to the service that owns it.
//
// The gateway talks to services over their REST API and forwards the caller's
// token, so each service still authorises independently (defence in depth).
// gRPC is the transport for service-to-service calls, not for this hop.
package gateway

import (
	"os"
	"strings"
)

// Upstream is one service the gateway can reach.
type Upstream struct {
	Name    string
	BaseURL string
}

// Route binds an API prefix to an upstream. Prefix is stripped before the
// request is forwarded, so /api/v1/vendors/123 becomes /vendors/123.
type Route struct {
	Prefix   string
	Upstream Upstream
}

// Config holds the gateway's own settings plus every upstream address.
type Config struct {
	FrontendOrigin string
	Upstreams      map[string]Upstream
	Routes         []Route
}

// LoadConfig reads upstream URLs from the environment. Defaults point at the
// local development ports documented in the README.
func LoadConfig() Config {
	upstreams := map[string]Upstream{
		"auth":      {Name: "auth-service", BaseURL: env("AUTH_SERVICE_URL", "http://localhost:8008")},
		"vendor":    {Name: "vendor-service", BaseURL: env("VENDOR_SERVICE_URL", "http://localhost:8001")},
		"tender":    {Name: "tender-service", BaseURL: env("TENDER_SERVICE_URL", "http://localhost:8002")},
		"bid":       {Name: "bid-service", BaseURL: env("BID_SERVICE_URL", "http://localhost:8003")},
		"po":        {Name: "po-service", BaseURL: env("PO_SERVICE_URL", "http://localhost:8004")},
		"approval":  {Name: "approval-service", BaseURL: env("APPROVAL_SERVICE_URL", "http://localhost:8005")},
		"audit":     {Name: "audit-service", BaseURL: env("AUDIT_SERVICE_URL", "http://localhost:8006")},
		"reporting": {Name: "reporting-service", BaseURL: env("REPORTING_SERVICE_URL", "http://localhost:8007")},
	}

	routes := []Route{
		{Prefix: "/api/v1/auth", Upstream: upstreams["auth"]},
		{Prefix: "/api/v1/vendors", Upstream: upstreams["vendor"]},
		{Prefix: "/api/v1/tenders", Upstream: upstreams["tender"]},
		{Prefix: "/api/v1/bids", Upstream: upstreams["bid"]},
		{Prefix: "/api/v1/purchase-orders", Upstream: upstreams["po"]},
		{Prefix: "/api/v1/approvals", Upstream: upstreams["approval"]},
		{Prefix: "/api/v1/audit-logs", Upstream: upstreams["audit"]},
		{Prefix: "/api/v1/reports", Upstream: upstreams["reporting"]},
	}

	return Config{
		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:3000"),
		Upstreams:      upstreams,
		Routes:         routes,
	}
}

// PublicPaths are the routes served without a token: login plus the health
// endpoints. Everything else requires a valid JWT.
func PublicPaths() []string {
	return []string{"/api/v1/auth/login", "/healthz", "/readyz"}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
