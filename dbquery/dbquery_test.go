package dbquery

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestInvoicesQuery(t *testing.T) {

	ctx := context.Background()

	accountCodes := "^(53|55|57)"

	db, err := New("test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	var (
		reconciled   = false
		dateFrom     = time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local)
		dateTo       = time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local)
		searchString = ""
	)
	invoices, err := db.GetInvoices(ctx, &reconciled, dateFrom, dateTo, searchString)
	if err != nil {
		t.Fatalf("get invoices error: %v", err)
	}

	if got, want := len(invoices), 8; got != want {
		t.Fatalf("got %d records want %d records", got, want)
	}
	expectedLastInvoice := Invoice{
		InvoiceID:     "inv-unrec-06",
		InvoiceNumber: "INV-2025-108",
		Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
		ContactName:   "Major Donor Pledge",
		Total:         2000,
		DonationTotal: 2000,
		CRMSTotal:     0,
		IsReconciled:  false,
	}
	if diff := cmp.Diff(expectedLastInvoice, invoices[len(invoices)-1]); diff != "" {
		t.Error(diff)
	}
}
