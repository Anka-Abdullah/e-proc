package approval

import (
	"encoding/json"
	"testing"
)

func TestHandleEventPayloadHelpers(t *testing.T) {
	raw := json.RawMessage(`{
		"po_id": "po-1",
		"requires_approval": true,
		"total": {"amount": 248750000, "currency": "IDR"},
		"number": "PO-2026-0007"
	}`)

	payload := decodePayload(raw)

	if got := stringField(payload, "po_id"); got != "po-1" {
		t.Errorf("po_id = %q, want po-1", got)
	}
	if got := stringField(payload, "number"); got != "PO-2026-0007" {
		t.Errorf("number = %q", got)
	}
	if !boolField(payload, "requires_approval") {
		t.Error("requires_approval should be true")
	}

	amount, currency := moneyField(payload, "total")
	if amount != 248_750_000 {
		t.Errorf("amount = %d, want 248750000", amount)
	}
	if currency != "IDR" {
		t.Errorf("currency = %q, want IDR", currency)
	}
}

func TestRequiresApprovalFlagIsRead(t *testing.T) {
	// A purchase order below the threshold must not open an approval.
	raw := json.RawMessage(`{"po_id":"po-2","requires_approval":false,"total":{"amount":5000000}}`)
	if boolField(decodePayload(raw), "requires_approval") {
		t.Error("requires_approval=false must not be treated as true")
	}

	// An event without the field at all must default to "no approval".
	if boolField(decodePayload(json.RawMessage(`{"po_id":"po-3"}`)), "requires_approval") {
		t.Error("missing requires_approval must default to false")
	}
}

func TestPayloadHelpersTolerateMissingData(t *testing.T) {
	if got := decodePayload(nil); got != nil {
		t.Errorf("empty payload should decode to nil, got %v", got)
	}
	if got := stringField(nil, "po_id"); got != "" {
		t.Errorf("missing payload should give empty string, got %q", got)
	}
	if amount, currency := moneyField(nil, "total"); amount != 0 || currency != "" {
		t.Errorf("missing money should be zero value, got %d %q", amount, currency)
	}
	if boolField(map[string]any{"requires_approval": "yes"}, "requires_approval") {
		t.Error("a non-boolean value must not be coerced to true")
	}
}
