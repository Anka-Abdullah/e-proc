package approval

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the approval routes. Reading is open to approvers
// and procurement; deciding needs approval:act.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/approvals", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermApprovalRead)).Get("/", s.handleList)
		r.With(middleware.RequirePermission(auth.PermApprovalRead)).Get("/{id}", s.handleGet)
		r.With(middleware.RequirePermission(auth.PermPOWrite)).Post("/", s.handleRequest)
		r.With(middleware.RequirePermission(auth.PermApprovalAct)).Post("/{id}/decide", s.handleDecide)
	})
}

func (s *Service) handleRequest(w http.ResponseWriter, r *http.Request) {
	var in RequestInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())
	if in.TenantID == "" {
		in.TenantID = claims.TenantID
	}

	a, err := s.Request(r.Context(), in, claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.Created(w, a)
}

func (s *Service) handleDecide(w http.ResponseWriter, r *http.Request) {
	var in DecideInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())

	a, err := s.Decide(r.Context(), claims.TenantID, chi.URLParam(r, "id"), in, claims)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, a)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	a, err := s.Get(r.Context(), claims.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, a)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := httpx.PageParams(r)
	claims := middleware.ClaimsFrom(r.Context())

	approvals, total, err := s.List(r.Context(), ListFilter{
		TenantID:   claims.TenantID,
		Status:     StatusFromString(r.URL.Query().Get("status")),
		AssignedTo: r.URL.Query().Get("assigned_to"),
		EntityID:   r.URL.Query().Get("entity_id"),
		Page:       page,
		PerPage:    perPage,
	})
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.List(w, approvals, httpx.Meta{Page: page, PerPage: perPage, Total: total})
}
