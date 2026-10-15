package po

import (
	"encoding/json"
	"testing"
)

func TestRoutingKeysCoverBothDecisions(t *testing.T) {
	keys := RoutingKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 routing keys, got %v", keys)
	}

	found := map[string]bool{}
	for _, key := range keys {
		found[key] = true
	}
	if !found["po.approved"] || !found["po.rejected"] {
		t.Fatalf("routing keys must cover approve and reject, got %v", keys)
	}
}

func TestApprovalPayloadHelpers(t *testing.T) {
	raw := json.RawMessage(`{"po_id":"po-9","approval_id":"ap-3","amount":{"amount":1000,"currency":"IDR"}}`)
	payload := decodeApprovalPayload(raw)

	if got := stringFromPayload(payload, "po_id"); got != "po-9" {
		t.Errorf("po_id = %q, want po-9", got)
	}
	if got := stringFromPayload(payload, "approval_id"); got != "ap-3" {
		t.Errorf("approval_id = %q, want ap-3", got)
	}
	if got := stringFromPayload(nil, "po_id"); got != "" {
		t.Errorf("missing payload should give empty string, got %q", got)
	}
	if got := decodeApprovalPayload(nil); got != nil {
		t.Errorf("empty payload should decode to nil, got %v", got)
	}
}
