package dbquery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestInvoicesQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	db, err := New("testdata/test.db", accountCodes)
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
		{
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "Example", // a regex
			noRecords:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-001",
				InvoiceNumber: "INV-2025-101",
				Date:          time.Date(2025, time.April, 10, 10, 0, 0, 0, time.UTC),
				ContactName:   "Example Corp Ltd",
				Total:         500,
				DonationTotal: 500,
				CRMSTotal:     550,
				IsReconciled:  false,
			},
		},
		{
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "INV-2025.*Ex.*Corp", // a regex
			noRecords:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-001",
				InvoiceNumber: "INV-2025-101",
				Date:          time.Date(2025, time.April, 10, 10, 0, 0, 0, time.UTC),
				ContactName:   "Example Corp Ltd",
				Total:         500,
				DonationTotal: 500,
				CRMSTotal:     550,
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

func TestBankTransactionsQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	db, err := New("testdata/test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		reconciliationStatus string
		dateFrom             time.Time
		dateTo               time.Time
		searchString         string

		noRecords       int
		lastTransaction BankTransaction
	}{

		{
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            7,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-06",
				Reference:     "STRIPE-PAYOUT-2025-05-04",
				Date:          time.Date(2025, time.May, 4, 9, 0, 0, 0, time.UTC),
				ContactName:   "Stripe",
				Total:         332.5,
				DonationTotal: 340,
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
			lastTransaction: BankTransaction{
				ID:            "bt-001",
				Reference:     "JG-PAYOUT-2025-04-15",
				Date:          time.Date(2025, time.April, 15, 14, 0, 0, 0, time.UTC),
				ContactName:   "JustGiving",
				Total:         337.25,
				DonationTotal: 355.0,
				CRMSTotal:     355.0,
				IsReconciled:  true,
			},
		},
		{
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			noRecords:            8,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-06",
				Reference:     "STRIPE-PAYOUT-2025-05-04",
				Date:          time.Date(2025, time.May, 4, 9, 0, 0, 0, time.UTC),
				ContactName:   "Stripe",
				Total:         332.5,
				DonationTotal: 340,
				CRMSTotal:     0,
				IsReconciled:  false,
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
			searchString:         "ENTH.*04-28",
			noRecords:            1,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-03",
				Reference:     "ENTHUSE-PAYOUT-2025-04-28",
				Date:          time.Date(2025, time.April, 28, 10, 0, 0, 0, time.UTC),
				ContactName:   "Enthuse",
				Total:         112,
				DonationTotal: 115,
				CRMSTotal:     0,
				IsReconciled:  false,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

			transactions, err := db.GetBankTransactions(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString)
			if err != nil {
				t.Fatalf("get bank transactions error: %v", err)
			}
			if got, want := len(transactions), tt.noRecords; got != want {
				t.Fatalf("got %d records want %d records", got, want)
			}
			if len(transactions) == 0 {
				return
			}
			if diff := cmp.Diff(tt.lastTransaction, transactions[len(transactions)-1]); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func ptrTime(ti time.Time) *time.Time {
	return &ti
}

func ptrStr(s string) *string {
	return &s
}

func ptrFloat64(f float64) *float64 {
	return &f
}

func TestDonationsQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	db, err := New("testdata/test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		dateFrom        time.Time
		dateTo          time.Time
		linkageStatus   string
		payoutReference string
		searchString    string

		noRecords  int
		lastRecord Donation
	}{
		{
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "All",
			payoutReference: "",
			searchString:    "",
			noRecords:       21,
			lastRecord: Donation{
				ID:              "sf-opp-odd-02",
				Name:            "Unlinked Donation",
				Amount:          75,
				CloseDate:       ptrTime(time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)),
				PayoutReference: nil,
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
			},
		},
		{
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "",
			searchString:    "",
			noRecords:       17,
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
			},
		},
		{
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "INV-2025-101",
			searchString:    "Data Entry",
			noRecords:       1,
			lastRecord: Donation{
				ID:              "sf-opp-odd-01",
				Name:            "Data Entry Error Donation",
				Amount:          50,
				CloseDate:       ptrTime(time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC)),
				PayoutReference: ptrStr("INV-2025-101"),
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
			},
		},
		{
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "",
			noRecords:       4,
			lastRecord: Donation{
				ID:              "sf-opp-odd-02",
				Name:            "Unlinked Donation",
				Amount:          75,
				CloseDate:       ptrTime(time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)),
				PayoutReference: nil,
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
			},
		},
		{
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "Unlinked Donation",
			noRecords:       1,
			lastRecord: Donation{
				ID:              "sf-opp-odd-02",
				Name:            "Unlinked Donation",
				Amount:          75,
				CloseDate:       ptrTime(time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)),
				PayoutReference: nil,
				CreatedDate:     nil,
				CreatedName:     nil,
				ModifiedDate:    nil,
				ModifiedName:    nil,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

			donations, err := db.GetDonations(ctx, tt.dateFrom, tt.dateTo, tt.linkageStatus, tt.payoutReference, tt.searchString)
			if err != nil {
				t.Fatalf("get donations error: %v", err)
			}
			if got, want := len(donations), tt.noRecords; got != want {
				t.Fatalf("got %d records want %d records", got, want)
			}
			if len(donations) == 0 {
				return
			}
			if diff := cmp.Diff(tt.lastRecord, donations[len(donations)-1]); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// printInvoiceLineItems is a helper print function.
func printInvoiceLineItems(t *testing.T, invoice IWLInvoice, lineItems []IWLLineItem) {
	t.Helper()
	tpl := `template output:
Invoice: {{ .Invoice.ID }} No {{ .Invoice.InvoiceNumber }} {{ .Invoice.Total }}
	{{- range .LineItems}}
	Line item: {{ .LiAccountName }} {{ .LiLineAmount }}
	{{- end }}
`
	t1 := template.New("t1")
	parsedTemplate, err := t1.Parse(tpl)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]any{
		"Invoice":   invoice,
		"LineItems": lineItems,
	}
	parsedTemplate.Execute(os.Stdout, data)
}

func TestInvoiceWithLineItemsQuery(t *testing.T) {
	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	db, err := New("testdata/test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		invoiceID string
		err       error
		invoice   IWLInvoice
		lineItems []IWLLineItem
	}{
		{
			invoiceID: "inv-002",
			err:       nil,
			invoice: IWLInvoice{
				ID:            "inv-002",
				InvoiceNumber: "INV-2025-102",
				Date:          time.Date(2025, 4, 12, 11, 0, 0, 0, time.UTC),
				Type:          nil,
				Status:        "PAID",
				Reference:     nil,
				ContactName:   "Generous Individual",
				Total:         196.5,
				DonationTotal: 200,
				CRMSTotal:     200,
				IsReconciled:  true,
			},
			lineItems: []IWLLineItem{
				IWLLineItem{
					LiAccountCode:    ptrStr("5301"),
					LiAccountName:    ptrStr("Fundraising Dinners"),
					LiDescription:    ptrStr("Pledged donation via Stripe"),
					LiLineAmount:     ptrFloat64(200),
					LiDonationAmount: ptrFloat64(200),
				},
				IWLLineItem{
					LiAccountCode:    ptrStr("429"),
					LiAccountName:    ptrStr("Platform Fees"),
					LiDescription:    ptrStr("Stripe processing fee"),
					LiLineAmount:     ptrFloat64(-3.5),
					LiDonationAmount: ptrFloat64(0),
				},
			},
		},
		{
			invoiceID: "xxxxxxxxxxxx",
			err:       sql.ErrNoRows,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			// Run query
			invoice, lineItems, err := db.GetInvoiceWLI(ctx, tt.invoiceID)
			if err != nil && !errors.Is(tt.err, err) {
				t.Fatalf("query execute error: %v", err)
				return
			}
			if err == nil && tt.err != nil {
				t.Fatalf("expected rows error: (rows %d) %v", len(lineItems), tt.err)
			}
			if diff := cmp.Diff(invoice, tt.invoice); diff != "" {
				t.Errorf("invoice diff error: %v", diff)
			}
			if diff := cmp.Diff(lineItems, tt.lineItems); diff != "" {
				t.Errorf("invoice diff error: %v", diff)
			}

		})
	}
}
