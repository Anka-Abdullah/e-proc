package po

import "testing"

func TestPriceLinesComputesTotals(t *testing.T) {
	in := CreateInput{
		TenantID: "t1",
		TenderID: "tnd-1",
		BidID:    "bid-1",
		VendorID: "v-1",
		Currency: "IDR",
		Lines: []Line{
			{Name: "Laptop", Quantity: 3, UnitPrice: Money{Amount: 12_000_000}},
			{Name: "Mouse", Quantity: 5, UnitPrice: Money{Amount: 250_000}},
		},
	}

	lines, total := in.PriceLines()

	if lines[0].TotalPrice.Amount != 36_000_000 {
		t.Errorf("line 1 total = %d, want 36000000", lines[0].TotalPrice.Amount)
	}
	if lines[1].TotalPrice.Amount != 1_250_000 {
		t.Errorf("line 2 total = %d, want 1250000", lines[1].TotalPrice.Amount)
	}
	if total.Amount != 37_250_000 {
		t.Errorf("order total = %d, want 37250000", total.Amount)
	}
	if total.Currency != "IDR" {
		t.Errorf("currency = %q, want IDR", total.Currency)
	}
}

func TestPriceLinesIgnoresClientSuppliedTotals(t *testing.T) {
	// A client sending a bogus total price must not influence the result.
	in := CreateInput{
		Currency: "IDR",
		Lines: []Line{
			{Name: "Item", Quantity: 2, UnitPrice: Money{Amount: 100_000}, TotalPrice: Money{Amount: 1}},
		},
	}

	lines, total := in.PriceLines()

	if lines[0].TotalPrice.Amount != 200_000 {
		t.Errorf("line total was taken from the client: %d", lines[0].TotalPrice.Amount)
	}
	if total.Amount != 200_000 {
		t.Errorf("order total was taken from the client: %d", total.Amount)
	}
}

func TestRequiresApprovalThreshold(t *testing.T) {
	const threshold = 50_000_000

	tests := []struct {
		total int64
		want  bool
	}{
		{49_999_999, false},
		{50_000_000, true},
		{75_000_000, true},
	}
	for _, tc := range tests {
		if got := RequiresApproval(tc.total, threshold); got != tc.want {
			t.Errorf("total %d: got %v, want %v", tc.total, got, tc.want)
		}
	}
}

func TestStatusMachine(t *testing.T) {
	legal := []struct{ from, to Status }{
		{StatusDraft, StatusPendingApproval},
		{StatusDraft, StatusIssued},
		{StatusPendingApproval, StatusApproved},
		{StatusPendingApproval, StatusRejected},
		{StatusApproved, StatusIssued},
		{StatusApproved, StatusClosed},
		{StatusIssued, StatusClosed},
		{StatusApproved, StatusApproved},
	}
	for _, tc := range legal {
		if !CanTransition(tc.from, tc.to) {
			t.Errorf("%s -> %s should be allowed", tc.from, tc.to)
		}
	}

	illegal := []struct{ from, to Status }{
		{StatusDraft, StatusApproved},
		{StatusDraft, StatusClosed},
		{StatusPendingApproval, StatusIssued},
		{StatusRejected, StatusIssued},
		{StatusRejected, StatusApproved},
		{StatusClosed, StatusIssued},
		{StatusClosed, StatusDraft},
	}
	for _, tc := range illegal {
		if CanTransition(tc.from, tc.to) {
			t.Errorf("%s -> %s should be rejected", tc.from, tc.to)
		}
	}
}

func TestCreateInputValidate(t *testing.T) {
	valid := CreateInput{
		TenantID: "t1", TenderID: "tnd-1", BidID: "bid-1", VendorID: "v-1",
		Lines: []Line{{Name: "Item", Quantity: 1, UnitPrice: Money{Amount: 1000}}},
	}

	if errs := valid.Validate(); errs.Any() {
		t.Fatalf("valid payload rejected: %v", map[string]string(errs))
	}

	tests := []struct {
		name      string
		mutate    func(*CreateInput)
		wantField string
	}{
		{"missing vendor", func(in *CreateInput) { in.VendorID = "" }, "vendor_id"},
		{"missing bid", func(in *CreateInput) { in.BidID = "" }, "bid_id"},
		{"missing tender", func(in *CreateInput) { in.TenderID = "" }, "tender_id"},
		{"no lines", func(in *CreateInput) { in.Lines = nil }, "lines"},
		{"zero quantity", func(in *CreateInput) { in.Lines[0].Quantity = 0 }, "lines"},
		{"zero price", func(in *CreateInput) { in.Lines[0].UnitPrice.Amount = 0 }, "lines"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			in.Lines = append([]Line(nil), valid.Lines...)
			tc.mutate(&in)
			errs := in.Validate()
			if _, ok := errs[tc.wantField]; !ok {
				t.Fatalf("expected error on %q, got %v", tc.wantField, map[string]string(errs))
			}
		})
	}
}
