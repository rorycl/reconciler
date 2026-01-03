package dbquery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestInvoicesQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	db, err := New("test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		reconciliationStatus string
		dateFrom             time.Time
		dateTo               time.Time
		searchString         string

		noRecords   int
		lastInvoice Invoice
	}{

		{
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            7,
			lastInvoice: Invoice{
				InvoiceID:     "inv-unrec-06",
				InvoiceNumber: "INV-2025-108",
				Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
				ContactName:   "Major Donor Pledge",
				Total:         2000,
				DonationTotal: 2000,
				CRMSTotal:     0,
				IsReconciled:  false,
			},
		},
		{
			reconciliationStatus: "Reconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-002",
				InvoiceNumber: "INV-2025-102",
				Date:          time.Date(2025, time.April, 12, 11, 0, 0, 0, time.UTC),
				ContactName:   "Generous Individual",
				Total:         196.5,
				DonationTotal: 200,
				CRMSTotal:     200,
				IsReconciled:  true,
			},
		},
		{
			reconciliationStatus: "All",
			dateFrom:             time.Date(2023, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2024, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            0,
		},

		{
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            8,
			lastInvoice: Invoice{
				InvoiceID:     "inv-unrec-06",
				InvoiceNumber: "INV-2025-108",
				Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
				ContactName:   "Major Donor Pledge",
				Total:         2000,
				DonationTotal: 2000,
				CRMSTotal:     0,
				IsReconciled:  false,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

			invoices, err := db.GetInvoices(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString)
			if err != nil {
				t.Fatalf("get invoices error: %v", err)
			}
			if got, want := len(invoices), tt.noRecords; got != want {
				t.Fatalf("got %d records want %d records", got, want)
			}
			if len(invoices) == 0 {
				return
			}
			if diff := cmp.Diff(tt.lastInvoice, invoices[len(invoices)-1]); diff != "" {
				t.Error(diff)
			}
		})
	}
}
