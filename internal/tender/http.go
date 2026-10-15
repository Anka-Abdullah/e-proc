package tender

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the tender routes. Reading is open to every role
// holding tender:read (vendors included, so they can see published tenders),
// while publishing needs its own permission.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/tenders", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermTenderRead)).Get("/", s.handleList)
		r.With(middleware.RequirePermission(auth.PermTenderRead)).Get("/{id}", s.handleGet)
		r.With(middleware.RequirePermission(auth.PermTenderWrite)).Post("/", s.handleCreate)
		r.With(middleware.RequirePermission(auth.PermTenderPublish)).Post("/{id}/publish", s.handlePublish)
		r.With(middleware.RequirePermission(auth.PermTenderPublish)).Post("/{id}/close", s.handleClose)
	})
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())
	if in.TenantID == "" {
		in.TenantID = claims.TenantID
	}

	t, err := s.Create(r.Context(), in, claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.Created(w, t)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	t, err := s.Get(r.Context(), claims.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, t)
}

func (s *Service) handlePublish(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	t, err := s.Publish(r.Context(), claims.TenantID, chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, t)
}

func (s *Service) handleClose(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	t, err := s.Close(r.Context(), claims.TenantID, chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, t)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := httpx.PageParams(r)
	claims := middleware.ClaimsFrom(r.Context())

	tenders, total, err := s.List(r.Context(), ListFilter{
		TenantID: claims.TenantID,
		Search:   r.URL.Query().Get("search"),
		Status:   StatusFromString(r.URL.Query().Get("status")),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.List(w, tenders, httpx.Meta{Page: page, PerPage: perPage, Total: total})
}
