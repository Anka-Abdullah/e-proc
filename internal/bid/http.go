package bid

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the bid routes. Vendors hold bid:submit, internal
// roles hold bid:read for the comparison report.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/bids", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermBidRead)).Get("/", s.handleList)
		r.With(middleware.RequirePermission(auth.PermBidRead)).Get("/{id}", s.handleGet)
		r.With(middleware.RequirePermission(auth.PermBidSubmit)).Post("/", s.handleSubmit)
		r.With(middleware.RequirePermission(auth.PermBidRead)).Get("/compare/{tenderID}", s.handleCompare)
		r.With(middleware.RequirePermission(auth.PermTenderWrite)).Post("/{id}/select", s.handleSelect)
	})
}

func (s *Service) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var in SubmitInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())
	if in.TenantID == "" {
		in.TenantID = claims.TenantID
	}
	// A vendor may only quote as itself; internal roles submit on behalf of a vendor.
	if claims.Role == auth.RoleVendor && in.VendorID == "" {
		in.VendorID = claims.UserID
	}

	b, err := s.Submit(r.Context(), in, claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.Created(w, b)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	b, err := s.Get(r.Context(), claims.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, b)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := httpx.PageParams(r)
	claims := middleware.ClaimsFrom(r.Context())

	bids, total, err := s.List(r.Context(), ListFilter{
		TenantID: claims.TenantID,
		TenderID: r.URL.Query().Get("tender_id"),
		VendorID: r.URL.Query().Get("vendor_id"),
		Status:   StatusFromString(r.URL.Query().Get("status")),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.List(w, bids, httpx.Meta{Page: page, PerPage: perPage, Total: total})
}

func (s *Service) handleCompare(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	comparisons, lowest, err := s.Compare(r.Context(), claims.TenantID, chi.URLParam(r, "tenderID"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, map[string]any{"comparisons": comparisons, "lowest_total": lowest})
}

func (s *Service) handleSelect(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	b, err := s.Select(r.Context(), claims.TenantID, chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, b)
}
