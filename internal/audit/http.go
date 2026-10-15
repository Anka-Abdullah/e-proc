package audit

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the audit read routes. There is no write route: the
// log is filled exclusively from domain events.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/audit-logs", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermAuditRead)).Get("/", s.handleList)
		r.With(middleware.RequirePermission(auth.PermAuditRead)).Get("/{id}", s.handleGet)
	})
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := httpx.PageParams(r)
	claims := middleware.ClaimsFrom(r.Context())
	query := r.URL.Query()

	filter := Filter{
		TenantID:   claims.TenantID,
		ActorID:    query.Get("actor_id"),
		EntityType: query.Get("entity_type"),
		EntityID:   query.Get("entity_id"),
		Action:     query.Get("action"),
		Page:       page,
		PerPage:    perPage,
	}

	if from, ok := parseTime(query.Get("from")); ok {
		filter.From = &from
	}
	if to, ok := parseTime(query.Get("to")); ok {
		filter.To = &to
	}

	entries, total, err := s.List(r.Context(), filter)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.List(w, entries, httpx.Meta{Page: page, PerPage: perPage, Total: total})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	entry, err := s.Get(r.Context(), claims.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, entry)
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
