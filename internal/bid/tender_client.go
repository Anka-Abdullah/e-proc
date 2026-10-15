package bid

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/ank/e-proc/pkg/apperr"
	tenderv1 "github.com/ank/e-proc/proto/gen/tender/v1"
)

// TenderValidator is the slice of tender-service this service needs. Keeping it
// an interface means the rule can be exercised in tests without a live service.
type TenderValidator interface {
	// EnsureOpen returns nil when the tender exists, belongs to the tenant and
	// is still accepting bids.
	EnsureOpen(ctx context.Context, tenantID, tenderID string) error
}

// TenderClient talks to tender-service over gRPC. This is the cross-service hop:
// bid-service must not read tender-service's collection directly.
type TenderClient struct {
	conn   *grpc.ClientConn
	client tenderv1.TenderServiceClient
}

// NewTenderClient dials tender-service. gRPC connections are lazy, so a service
// that is not up yet does not block startup; the first call fails instead.
func NewTenderClient(addr string) (*TenderClient, error) {
	if addr == "" {
		return nil, errors.New("tender gRPC address is empty")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial tender service %s: %w", addr, err)
	}
	return &TenderClient{conn: conn, client: tenderv1.NewTenderServiceClient(conn)}, nil
}

// Close releases the connection.
func (c *TenderClient) Close() error { return c.conn.Close() }

// EnsureOpen fetches the tender and checks its state.
func (c *TenderClient) EnsureOpen(ctx context.Context, tenantID, tenderID string) error {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := c.client.GetTender(callCtx, &tenderv1.GetTenderRequest{
		TenantId: tenantID,
		Id:       tenderID,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return apperr.ErrNotFound
		}
		// A transport failure is not the caller's fault; surface it as such.
		return fmt.Errorf("tender lookup failed: %w", err)
	}

	tender := response.GetTender()
	return validateTenderState(tender.GetStatus(), tender.GetCode(), closeAtOf(tender), time.Now().UTC())
}

// validateTenderState is the rule that decides whether a quotation may be
// accepted. It is separated from the RPC so it can be tested directly.
func validateTenderState(state tenderv1.TenderStatus, code string, closeAt, now time.Time) error {
	if state != tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED {
		return apperr.NewConflict("tender %s is %s and is not accepting bids", code, statusName(state))
	}
	if !closeAt.IsZero() && closeAt.Before(now) {
		return apperr.NewConflict("tender %s closed at %s", code, closeAt.Format(time.RFC3339))
	}
	return nil
}

func closeAtOf(tender *tenderv1.Tender) time.Time {
	if tender.GetCloseAt() == nil {
		return time.Time{}
	}
	return tender.GetCloseAt().AsTime().UTC()
}

func statusName(s tenderv1.TenderStatus) string {
	name, ok := tenderv1.TenderStatus_name[int32(s)]
	if !ok {
		return "unknown"
	}
	return name
}
