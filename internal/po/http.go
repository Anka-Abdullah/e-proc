package po

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the purchase order routes.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/purchase-orders", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermPORead)).Get("/", s.handleList)
		r.With(middleware.RequirePermission(auth.PermPORead)).Get("/{id}", s.handleGet)
		r.With(middleware.RequirePermission(auth.PermPOWrite)).Post("/", s.handleCreate)
		r.With(middleware.RequirePermission(auth.PermPOWrite)).Patch("/{id}/status", s.handleUpdateStatus)
		r.With(middleware.RequirePermission(auth.PermPOWrite)).Post("/{id}/close", s.handleClose)
		r.With(middleware.RequirePermission(auth.PermReportRead)).Get("/reports/by-vendor", s.handleByVendor)
	})
}

type statusBody struct {
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id"`
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

	order, requiresApproval, err := s.Create(r.Context(), in, claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.Created(w, map[string]any{"purchase_order": order, "requires_approval": requiresApproval})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	order, err := s.Get(r.Context(), claims.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, order)
}

func (s *Service) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	var body statusBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())
	if claims.Role == auth.RoleApprover && StatusFromString(body.Status) != StatusApproved && StatusFromString(body.Status) != StatusRejected {
		httpx.Fail(w, http.StatusForbidden, httpx.CodeForbidden, "approver may only approve or reject")
		return
	}

	order, err := s.UpdateStatus(r.Context(), claims.TenantID, chi.URLParam(r, "id"), StatusFromString(body.Status), claims.UserID, body.ApprovalID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, order)
}

func (s *Service) handleClose(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	order, err := s.Close(r.Context(), claims.TenantID, chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, order)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := httpx.PageParams(r)
	claims := middleware.ClaimsFrom(r.Context())

	orders, total, err := s.List(r.Context(), ListFilter{
		TenantID: claims.TenantID,
		VendorID: r.URL.Query().Get("vendor_id"),
		Status:   StatusFromString(r.URL.Query().Get("status")),
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.List(w, orders, httpx.Meta{Page: page, PerPage: perPage, Total: total})
}

func (s *Service) handleByVendor(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	rows, err := s.ByVendor(r.Context(), claims.TenantID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, rows)
}
