package approval

import "testing"

func TestRequestInputValidate(t *testing.T) {
	valid := RequestInput{
		TenantID:   "t1",
		EntityType: "purchase_order",
		EntityID:   "po-1",
		Amount:     60_000_000,
	}

	if errs := valid.Validate(); errs.Any() {
		t.Fatalf("valid payload rejected: %v", map[string]string(errs))
	}

	tests := []struct {
		name      string
		mutate    func(*RequestInput)
		wantField string
	}{
		{"missing tenant", func(in *RequestInput) { in.TenantID = "" }, "tenant_id"},
		{"missing entity id", func(in *RequestInput) { in.EntityID = "" }, "entity_id"},
		{"unknown entity type", func(in *RequestInput) { in.EntityType = "invoice" }, "entity_type"},
		{"zero amount", func(in *RequestInput) { in.Amount = 0 }, "amount"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			errs := in.Validate()
			if _, ok := errs[tc.wantField]; !ok {
				t.Fatalf("expected error on %q, got %v", tc.wantField, map[string]string(errs))
			}
		})
	}
}

func TestDecideInputRequiresReasonToReject(t *testing.T) {
	approve := DecideInput{Approve: true}
	if errs := approve.Validate(); errs.Any() {
		t.Errorf("approval without comment should be valid, got %v", map[string]string(errs))
	}

	rejectNoReason := DecideInput{Approve: false, Comment: ""}
	if errs := rejectNoReason.Validate(); errs["comment"] == "" {
		t.Error("rejection without a comment should be rejected")
	}

	rejectWithReason := DecideInput{Approve: false, Comment: "harga di atas anggaran"}
	if errs := rejectWithReason.Validate(); errs.Any() {
		t.Errorf("rejection with a reason should be valid, got %v", map[string]string(errs))
	}
}

func TestCanBeDecidedBy(t *testing.T) {
	tests := []struct {
		name      string
		assigned  string
		actor     string
		isAdmin   bool
		wantAllow bool
	}{
		{"assignee decides", "u-approver", "u-approver", false, true},
		{"other approver blocked", "u-approver", "u-other", false, false},
		{"unassigned anyone decides", "", "u-other", false, true},
		{"admin decides regardless", "u-approver", "u-admin", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Approval{AssignedTo: tc.assigned, Status: StatusPending}
			if got := a.CanBeDecidedBy(tc.actor, tc.isAdmin); got != tc.wantAllow {
				t.Fatalf("got %v, want %v", got, tc.wantAllow)
			}
		})
	}
}

func TestIsPending(t *testing.T) {
	if (&Approval{Status: StatusPending}).IsPending() != true {
		t.Error("pending approval should report pending")
	}
	if (&Approval{Status: StatusApproved}).IsPending() != false {
		t.Error("approved approval should not report pending")
	}
	var nilApproval *Approval
	if nilApproval.IsPending() {
		t.Error("nil approval must not report pending")
	}
}
