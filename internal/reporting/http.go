package reporting

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountProtected registers the report routes. Every report is tenant-scoped and
// requires report:read.
func (s *Service) MountProtected(r chi.Router) {
	r.Route("/reports", func(r chi.Router) {
		r.With(middleware.RequirePermission(auth.PermReportRead)).Get("/po-by-vendor", s.handlePOByVendor)
		r.With(middleware.RequirePermission(auth.PermReportRead)).Get("/tender-summary", s.handleTenderSummary)
		r.With(middleware.RequirePermission(auth.PermReportRead)).Get("/approval-summary", s.handleApprovalSummary)
	})
}

func (s *Service) handlePOByVendor(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())

	rows, err := s.PurchaseOrdersByVendor(r.Context(), claims.TenantID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}

	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="po-by-vendor.csv"`)

		writer := csv.NewWriter(w)
		defer writer.Flush()
		_ = writer.Write([]string{"vendor_id", "order_count", "total", "currency"})
		for _, row := range rows {
			_ = writer.Write([]string{
				row.VendorID,
				strconv.FormatInt(row.OrderCount, 10),
				strconv.FormatInt(row.Total, 10),
				row.Currency,
			})
		}
		return
	}

	httpx.OK(w, rows)
}

func (s *Service) handleTenderSummary(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	summary, err := s.TenderSummary(r.Context(), claims.TenantID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, summary)
}

func (s *Service) handleApprovalSummary(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r.Context())
	summary, err := s.ApprovalSummary(r.Context(), claims.TenantID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, summary)
}
