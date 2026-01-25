package db

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

func ptrTime(ti time.Time) *time.Time { return &ti }

func ptrStr(s string) *string { return &s }

func ptrBool(b bool) *bool { return &b }

func ptrFloat64(f float64) *float64 { return &f }

// TestInvoicesQuery test the database invoice records.
func TestInvoicesQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	sqlDir := os.DirFS("sql")

	db, err := New("testdata/test.db", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		name                 string
		reconciliationStatus string
		dateFrom             time.Time
		dateTo               time.Time
		searchString         string
		limit, offset        int

		err error

		RecordsNo   int
		lastInvoice Invoice
	}{

		{
			name:                 "7 unreconciled records",
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                10,
			offset:               -1,
			RecordsNo:            7,
			lastInvoice: Invoice{
				InvoiceID:     "inv-unrec-06",
				InvoiceNumber: "INV-2025-108",
				Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
				ContactName:   "Major Donor Pledge",
				Status:        "PAID",
				Total:         2000,
				DonationTotal: 2000,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      7,
			},
		},
		{
			name:                 "1 reconciled records",
			reconciliationStatus: "Reconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                10,
			offset:               0,
			RecordsNo:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-002",
				InvoiceNumber: "INV-2025-102",
				Date:          time.Date(2025, time.April, 12, 11, 0, 0, 0, time.UTC),
				ContactName:   "Generous Individual",
				Status:        "PAID",
				Total:         196.5,
				DonationTotal: 200,
				CRMSTotal:     200,
				IsReconciled:  true,
				RowCount:      1,
			},
		},
		{
			name:                 "out of date not found records",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2023, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2024, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                10,
			offset:               0,
			RecordsNo:            0,
			err:                  sql.ErrNoRows,
		},

		{
			name:                 "all 8 records",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                10,
			offset:               0,
			RecordsNo:            8,
			lastInvoice: Invoice{
				InvoiceID:     "inv-unrec-06",
				InvoiceNumber: "INV-2025-108",
				Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
				ContactName:   "Major Donor Pledge",
				Status:        "PAID",
				Total:         2000,
				DonationTotal: 2000,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      8,
			},
		},
		{
			name:                 "8 records offset 4",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                4,
			offset:               4,
			RecordsNo:            4, // number of records
			lastInvoice: Invoice{
				InvoiceID:     "inv-unrec-06",
				InvoiceNumber: "INV-2025-108",
				Date:          time.Date(2025, time.May, 5, 15, 0, 0, 0, time.UTC),
				ContactName:   "Major Donor Pledge",
				Status:        "PAID",
				Total:         2000,
				DonationTotal: 2000,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      8, // the full row count for pagination
			},
		},
		{
			name:                 "example search record",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "example", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:                10,
			offset:               0,
			RecordsNo:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-001",
				InvoiceNumber: "INV-2025-101",
				Date:          time.Date(2025, time.April, 10, 10, 0, 0, 0, time.UTC),
				ContactName:   "Example Corp Ltd",
				Status:        "PAID",
				Total:         500,
				DonationTotal: 500,
				CRMSTotal:     550,
				IsReconciled:  false,
				RowCount:      1,
			},
		},
		{
			name:                 "example search record notreconciled",
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "inv-2025.*ex.*corp", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:                10,
			offset:               0,
			RecordsNo:            1,
			lastInvoice: Invoice{
				InvoiceID:     "inv-001",
				InvoiceNumber: "INV-2025-101",
				Date:          time.Date(2025, time.April, 10, 10, 0, 0, 0, time.UTC),
				ContactName:   "Example Corp Ltd",
				Status:        "PAID",
				Total:         500,
				DonationTotal: 500,
				CRMSTotal:     550,
				IsReconciled:  false,
				RowCount:      1,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			invoices, err := db.GetInvoices(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString, tt.limit, tt.offset)
			if err != nil {
				if err != tt.err {
					t.Fatalf("got invoices error: %v", err)
				}
				return
			}
			if got, want := len(invoices), tt.RecordsNo; got != want {
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

// TestBankTransactionsQuery tests the database bank transactions.
func TestBankTransactionsQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	sqlDir := os.DirFS("sql")

	db, err := New("testdata/test.db", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		name                 string
		reconciliationStatus string
		dateFrom             time.Time
		dateTo               time.Time
		searchString         string
		limit, offset        int

		err error

		RecordsNo       int
		lastTransaction BankTransaction
	}{

		{
			name:                 "get 7 not reconciled records",
			reconciliationStatus: "NotReconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                -1,
			offset:               0,
			RecordsNo:            7,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-06",
				Reference:     "STRIPE-PAYOUT-2025-05-04",
				Date:          time.Date(2025, time.May, 4, 9, 0, 0, 0, time.UTC),
				ContactName:   "Stripe",
				Status:        "RECONCILED",
				Total:         332.5,
				DonationTotal: 340,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      7, // for pagination
			},
		},
		{
			name:                 "get 1 reconciled record",
			reconciliationStatus: "Reconciled",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                -1,
			offset:               0,
			RecordsNo:            1,
			lastTransaction: BankTransaction{
				ID:            "bt-001",
				Reference:     "JG-PAYOUT-2025-04-15",
				Date:          time.Date(2025, time.April, 15, 14, 0, 0, 0, time.UTC),
				ContactName:   "JustGiving",
				Status:        "RECONCILED",
				Total:         337.25,
				DonationTotal: 355.0,
				CRMSTotal:     355.0,
				IsReconciled:  true,
				RowCount:      1,
			},
		},
		{
			name:                 "get all 8 records",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                -1,
			offset:               0,
			RecordsNo:            8,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-06",
				Reference:     "STRIPE-PAYOUT-2025-05-04",
				Date:          time.Date(2025, time.May, 4, 9, 0, 0, 0, time.UTC),
				ContactName:   "Stripe",
				Status:        "RECONCILED",
				Total:         332.5,
				DonationTotal: 340,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      8,
			},
		},
		{
			name:                 "get 1 of 8 records offset 7",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			searchString:         "",
			limit:                1,
			offset:               7,
			RecordsNo:            1, // number of returned records
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-06",
				Reference:     "STRIPE-PAYOUT-2025-05-04",
				Date:          time.Date(2025, time.May, 4, 9, 0, 0, 0, time.UTC),
				ContactName:   "Stripe",
				Status:        "RECONCILED",
				Total:         332.5,
				DonationTotal: 340,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      8, // for pagination
			},
		},
		{
			name:                 "get no records",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2023, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2024, 3, 31, 0, 0, 0, 0, time.Local),
			limit:                -1,
			offset:               0,
			searchString:         "",
			RecordsNo:            0,
			err:                  sql.ErrNoRows,
		},
		{
			name:                 "search record",
			reconciliationStatus: "All",
			dateFrom:             time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:               time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			limit:                -1,
			offset:               0,
			searchString:         "ENTH.*04-28", // a regex which is a lower() to lower() match so (sort of) an iregex
			RecordsNo:            1,
			lastTransaction: BankTransaction{
				ID:            "bt-unrec-03",
				Reference:     "ENTHUSE-PAYOUT-2025-04-28",
				Date:          time.Date(2025, time.April, 28, 10, 0, 0, 0, time.UTC),
				ContactName:   "Enthuse",
				Status:        "RECONCILED",
				Total:         112,
				DonationTotal: 115,
				CRMSTotal:     0,
				IsReconciled:  false,
				RowCount:      1,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			transactions, err := db.GetBankTransactions(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString, tt.limit, tt.offset)
			if err != nil {
				if err != tt.err {
					t.Fatalf("got bank transactions error: %v", err)
				}
				return
			}
			if got, want := len(transactions), tt.RecordsNo; got != want {
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

// TestDonationsQuery tests the donation SQL records.
func TestDonationsQuery(t *testing.T) {

	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	sqlDir := os.DirFS("sql")

	db, err := New("testdata/test.db", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		name            string
		dateFrom        time.Time
		dateTo          time.Time
		linkageStatus   string
		payoutReference string
		searchString    string
		limit, offset   int

		err error

		RecordsNo  int
		lastRecord Donation
	}{
		{
			name:            "all 21 records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "All",
			payoutReference: "",
			searchString:    "",
			limit:           -1,
			offset:          0,
			RecordsNo:       21,
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
				IsLinked:        true,
				RowCount:        21,
			},
		},
		{
			name:            "all 21 records limited to 0",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "All",
			payoutReference: "",
			searchString:    "",
			limit:           0,
			offset:          0,
			err:             sql.ErrNoRows,
		},
		{
			name:            "all 17 linked records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "",
			searchString:    "",
			limit:           20,
			offset:          0,
			RecordsNo:       17,
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
				IsLinked:        true,
				RowCount:        17,
			},
		},
		{
			name:            "all 17 linked records limited to last 7",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "",
			searchString:    "",
			limit:           10,
			offset:          10,
			RecordsNo:       7, // number of records after limiting
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
				IsLinked:        true,
				RowCount:        17, // for pagination
			},
		},
		{
			name:            "search for 1 linked record",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "Linked",
			payoutReference: "INV-2025-101",
			searchString:    "data entry", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:           -1,
			offset:          0,
			RecordsNo:       1,
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
				IsLinked:        true,
				RowCount:        1,
			},
		},
		{
			name:            "list 4 unlinked records",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "",
			limit:           -1,
			offset:          0,
			RecordsNo:       4,
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
				IsLinked:        false,
				RowCount:        4,
			},
		},
		{
			name:            "search 1 unlinked record",
			dateFrom:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.Local),
			dateTo:          time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local),
			linkageStatus:   "NotLinked",
			payoutReference: "",
			searchString:    "unlinked donation", // a regex which is a lower() to lower() match so (sort of) an iregex
			limit:           -1,
			offset:          0,
			RecordsNo:       1,
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
				IsLinked:        false,
				RowCount:        1,
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			donations, err := db.GetDonations(ctx, tt.dateFrom, tt.dateTo, tt.linkageStatus, tt.payoutReference, tt.searchString, tt.limit, tt.offset)
			if err != nil {
				if err != tt.err {
					t.Fatalf("get donations error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("get donations error: %v", err)
			}
			if got, want := len(donations), tt.RecordsNo; got != want {
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

// printInvoiceLineItems is a helper template print function.
func printInvoiceLineItems(t *testing.T, invoice WRInvoice, lineItems []WRLineItem) {
	t.Helper()
	tpl := `template output:
Invoice: {{ .Invoice.ID }} No {{ .Invoice.InvoiceNumber }} {{ .Invoice.Total }}
	{{- range .LineItems}}
	Line item: {{ .AccountName }} {{ .LineAmount }}
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

	sqlDir := os.DirFS("sql")

	db, err := New("testdata/test.db", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		invoiceID string
		err       error
		invoice   WRInvoice
		lineItems []WRLineItem
	}{
		{
			invoiceID: "inv-002",
			err:       nil,
			invoice: WRInvoice{
				ID:               "inv-002",
				InvoiceNumber:    "INV-2025-102",
				Date:             time.Date(2025, 4, 12, 11, 0, 0, 0, time.UTC),
				Type:             nil,
				Status:           "PAID",
				Reference:        nil,
				ContactName:      "Generous Individual",
				Total:            196.5,
				DonationTotal:    200,
				CRMSTotal:        200,
				TotalOutstanding: -3.5,
				IsReconciled:     true,
			},
			lineItems: []WRLineItem{
				WRLineItem{
					AccountCode:    ptrStr("5301"),
					AccountName:    ptrStr("Fundraising Dinners"),
					Description:    ptrStr("Pledged donation via Stripe"),
					LineAmount:     ptrFloat64(200),
					DonationAmount: ptrFloat64(200),
				},
				WRLineItem{
					AccountCode:    ptrStr("429"),
					AccountName:    ptrStr("Platform Fees"),
					Description:    ptrStr("Stripe processing fee"),
					LineAmount:     ptrFloat64(-3.5),
					DonationAmount: ptrFloat64(0),
				},
			},
		},
		{
			invoiceID: "inv-unrec-04",
			err:       nil,
			invoice: WRInvoice{
				ID:               "inv-unrec-04",
				InvoiceNumber:    "INV-2025-106",
				Date:             time.Date(2025, 4, 25, 13, 0, 0, 0, time.UTC),
				Type:             nil,
				Status:           "PAID",
				Reference:        nil,
				ContactName:      "Small Pledge",
				Total:            50,
				DonationTotal:    50,
				CRMSTotal:        0,
				TotalOutstanding: 50,
				IsReconciled:     false,
			},
			lineItems: []WRLineItem{
				{
					AccountCode:    ptrStr("5501"),
					AccountName:    ptrStr("General Giving"),
					Description:    ptrStr("Donation"),
					LineAmount:     ptrFloat64(50),
					DonationAmount: ptrFloat64(50),
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
			invoice, lineItems, err := db.GetInvoiceWR(ctx, tt.invoiceID)
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

func TestBankTransactionsWithLineItemsQuery(t *testing.T) {
	accountCodes := "^(53|55|57)"
	ctx := context.Background()

	sqlDir := os.DirFS("sql")

	db, err := New("testdata/test.db", sqlDir, accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	tests := []struct {
		transactionID string
		err           error
		transaction   WRTransaction
		lineItems     []WRLineItem
	}{
		{
			transactionID: "bt-prev-fy-01",
			err:           nil,
			transaction: WRTransaction{
				ID:            "bt-prev-fy-01",
				Reference:     ptrStr("JG-PAYOUT-2025-02-28"),
				Date:          time.Date(2025, 2, 28, 14, 0, 0, 0, time.UTC),
				Type:          nil,
				Status:        "RECONCILED",
				ContactName:   "JustGiving",
				Total:         190,
				DonationTotal: 200,
				CRMSTotal:     200,
				IsReconciled:  true,
			},
			lineItems: []WRLineItem{
				{
					AccountCode:    ptrStr("5501"),
					AccountName:    ptrStr("General Giving"),
					Description:    ptrStr("Donation Payout"),
					TaxAmount:      nil,
					LineAmount:     ptrFloat64(200),
					DonationAmount: ptrFloat64(200),
				},
				{
					AccountCode:    ptrStr("429"),
					AccountName:    ptrStr("Platform Fees"),
					Description:    ptrStr("Fee"),
					TaxAmount:      nil,
					LineAmount:     ptrFloat64(-10),
					DonationAmount: ptrFloat64(0),
				},
			},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			// Run query
			transaction, lineItems, err := db.GetTransactionWR(ctx, tt.transactionID)
			if err != nil && !errors.Is(tt.err, err) {
				t.Fatalf("query execute error: %v", err)
				return
			}
			if err == nil && tt.err != nil {
				t.Fatalf("expected rows error: (rows %d) %v", len(lineItems), tt.err)
			}
			if diff := cmp.Diff(transaction, tt.transaction); diff != "" {
				t.Errorf("transaction diff error: %v", diff)
			}
			if diff := cmp.Diff(lineItems, tt.lineItems); diff != "" {
				t.Errorf("invoice diff error: %v", diff)
			}

		})
	}
}
