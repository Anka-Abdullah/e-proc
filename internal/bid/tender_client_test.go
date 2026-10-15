package bid

import (
	"testing"
	"time"

	"github.com/ank/e-proc/pkg/apperr"
	tenderv1 "github.com/ank/e-proc/proto/gen/tender/v1"
)

func TestValidateTenderState(t *testing.T) {
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name    string
		state   tenderv1.TenderStatus
		closeAt time.Time
		wantErr bool
	}{
		{"published with a future deadline is open", tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED, future, false},
		{"published but past deadline is closed", tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED, past, true},
		{"published with no deadline is open", tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED, time.Time{}, false},
		{"draft cannot receive bids", tenderv1.TenderStatus_TENDER_STATUS_DRAFT, future, true},
		{"closed cannot receive bids", tenderv1.TenderStatus_TENDER_STATUS_CLOSED, future, true},
		{"cancelled cannot receive bids", tenderv1.TenderStatus_TENDER_STATUS_CANCELLED, future, true},
		{"awarded cannot receive bids", tenderv1.TenderStatus_TENDER_STATUS_AWARDED, future, true},
		{"unspecified cannot receive bids", tenderv1.TenderStatus_TENDER_STATUS_UNSPECIFIED, future, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTenderState(tc.state, "TND-001", tc.closeAt, now)
			if tc.wantErr && err == nil {
				t.Fatal("expected the tender to be rejected")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected the tender to be accepted, got %v", err)
			}
			if tc.wantErr {
				// The caller must see a 409, not a 500: this is a state conflict.
				if _, ok := apperr.AsConflict(err); !ok {
					t.Fatalf("expected a conflict error, got %T: %v", err, err)
				}
			}
		})
	}
}

func TestStatusName(t *testing.T) {
	if got := statusName(tenderv1.TenderStatus_TENDER_STATUS_PUBLISHED); got != "TENDER_STATUS_PUBLISHED" {
		t.Errorf("unexpected status name %q", got)
	}
}
