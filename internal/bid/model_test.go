package bid

import "testing"

func TestPriceItemsComputesLineAndGrandTotals(t *testing.T) {
	in := SubmitInput{
		TenantID: "t1",
		TenderID: "tnd-1",
		VendorID: "v-1",
		Currency: "IDR",
		Items: []Item{
			{Name: "Laptop", Quantity: 10, UnitPrice: Money{Amount: 11_500_000}},
			{Name: "Docking", Quantity: 10, UnitPrice: Money{Amount: 850_000}},
		},
	}

	items, total := in.PriceItems()

	if items[0].TotalPrice.Amount != 115_000_000 {
		t.Errorf("line 1 = %d, want 115000000", items[0].TotalPrice.Amount)
	}
	if items[1].TotalPrice.Amount != 8_500_000 {
		t.Errorf("line 2 = %d, want 8500000", items[1].TotalPrice.Amount)
	}
	if total.Amount != 123_500_000 {
		t.Errorf("grand total = %d, want 123500000", total.Amount)
	}
	if items[0].UnitPrice.Currency != "IDR" {
		t.Errorf("unit price currency = %q, want IDR", items[0].UnitPrice.Currency)
	}
}

func TestPriceItemsUsesLineCurrencyWhenGiven(t *testing.T) {
	in := SubmitInput{
		Currency: "IDR",
		Items: []Item{
			{Name: "Import", Quantity: 1, UnitPrice: Money{Amount: 100, Currency: "USD"}},
		},
	}

	items, total := in.PriceItems()

	if items[0].TotalPrice.Currency != "USD" {
		t.Errorf("line currency = %q, want USD", items[0].TotalPrice.Currency)
	}
	if total.Currency != "IDR" {
		t.Errorf("total currency = %q, want IDR", total.Currency)
	}
}

func TestSubmitInputValidate(t *testing.T) {
	valid := SubmitInput{
		TenantID: "t1", TenderID: "tnd-1", VendorID: "v-1",
		Items: []Item{{Name: "Item", Quantity: 1, UnitPrice: Money{Amount: 1000}}},
	}

	if errs := valid.Validate(); errs.Any() {
		t.Fatalf("valid payload rejected: %v", map[string]string(errs))
	}

	tests := []struct {
		name      string
		mutate    func(*SubmitInput)
		wantField string
	}{
		{"missing tenant", func(in *SubmitInput) { in.TenantID = "" }, "tenant_id"},
		{"missing tender", func(in *SubmitInput) { in.TenderID = "" }, "tender_id"},
		{"missing vendor", func(in *SubmitInput) { in.VendorID = "" }, "vendor_id"},
		{"empty items", func(in *SubmitInput) { in.Items = nil }, "items"},
		{"zero quantity", func(in *SubmitInput) { in.Items[0].Quantity = 0 }, "items"},
		{"zero unit price", func(in *SubmitInput) { in.Items[0].UnitPrice.Amount = 0 }, "items"},
		{"negative delivery", func(in *SubmitInput) { in.DeliveryDays = -1 }, "delivery_days"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			in.Items = append([]Item(nil), valid.Items...)
			tc.mutate(&in)
			errs := in.Validate()
			if _, ok := errs[tc.wantField]; !ok {
				t.Fatalf("expected error on %q, got %v", tc.wantField, map[string]string(errs))
			}
		})
	}
}

func TestNormalizeDefaultsCurrency(t *testing.T) {
	in := SubmitInput{Items: []Item{{Name: "  Item  "}}}
	in.Normalize()

	if in.Currency != "IDR" {
		t.Errorf("currency = %q, want IDR", in.Currency)
	}
	if in.Items[0].Name != "Item" {
		t.Errorf("item name = %q, want trimmed", in.Items[0].Name)
	}
	if in.Items[0].UnitPrice.Currency != "IDR" {
		t.Errorf("unit price currency = %q, want IDR", in.Items[0].UnitPrice.Currency)
	}
}

func TestIsValidStatus(t *testing.T) {
	for _, s := range []Status{StatusSubmitted, StatusShortlisted, StatusSelected, StatusRejected} {
		if !IsValidStatus(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Status{"", "draft", "SUBMITTED"} {
		if IsValidStatus(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}
