package db

// tests for xero-related database queries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reconciler/apiclients/xero"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Index:
// These tests test each testDB.go database funcion.
//
// Test OrganisationUpsert(ctx context.Context, org xero.Organisation) error
// Test AccountsUpsert(ctx context.Context, accounts []xero.Account) error
// Test InvoicesGet(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]Invoice, error)
// Test InvoicesUpsert(ctx context.Context, invoices []xero.Invoice) error
// Test BankTransactionsGet(ctx context.Context, reconciliationStatus string, dateFrom, dateTo time.Time, search string, limit, offset int) ([]BankTransaction, error)
// Test BankTransactionsUpsert(ctx context.Context, transactions []xero.BankTransaction) error
// Test InvoiceWRGet(ctx context.Context, invoiceID string) (WRInvoice, []WRLineItem, error)
// Test BankTransactionWRGet(ctx context.Context, transactionID string) (WRTransaction, []WRLineItem, error)

func Test_OrganisationUpsert(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

	org := xero.Organisation{
		Name:                  "abc",
		LegalName:             "def",
		OrganisationType:      "charity",
		FinancialYearEndDay:   1,
		FinancialYearEndMonth: 5,
		Timezone:              "StrangeXeroString",
		ShortCode:             "!NXpl!",
		OrganisationID:        "709b07f5-100b-11f1-aab3-7404f143aa1c",
	}

	err := testDB.OrganisationUpsert(ctx, org)
	if err != nil {
		t.Errorf("unexpected organisation error: %v", err)
	}

	var count int
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM organisation")
	if err != nil || count != 1 {
		t.Errorf("Expected to find 1 organisation after upsert, but got count %d, err: %v", count, err)
	}

	result, err := testDB.ExecContext(ctx, "DELETE FROM organisation")
	if err != nil {
		t.Errorf("unexpected error in organisation deletion: %v", err)
	}
	rowNo, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("could not get rows affected: %v", err)
	}
	if got, want := int(rowNo), 1; got != want {
		t.Errorf("Deleted count got %d want %d", got, want)
	}
}

func Test_AccountsUpsert(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

	now := time.Now()

	accounts := []xero.Account{
		{
			AccountID:     "7404f143aa1c",
			Code:          "9987",
			Name:          "Test Account",
			Description:   "Go test description",
			Type:          "AUTHORISED",
			TaxType:       "Unknown",
			Status:        "PAID",
			SystemAccount: "No",
			CurrencyCode:  "GBP",
			Updated:       now,
		},
		{
			AccountID:     "7404f143aa1d",
			Code:          "9986",
			Name:          "Test Account",
			Description:   "Go test description",
			Type:          "AUTHORISED",
			TaxType:       "Unknown",
			Status:        "PAID",
			SystemAccount: "No",
			CurrencyCode:  "GBP",
			Updated:       now.Add(1 * time.Hour),
		},
	}

	err := testDB.AccountsUpsert(ctx, accounts)
	if err != nil {
		t.Errorf("unexpected accounts error: %v", err)
	}

	var count int
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM accounts WHERE id IN (?, ?)", accounts[0].AccountID, accounts[1].AccountID)
	if err != nil || count != 2 {
		t.Errorf("Expected to find 2 accounts after upsert, but got count %d, err: %v", count, err)
	}

	result, err := testDB.ExecContext(ctx, "DELETE FROM accounts WHERE id IN (?, ?)", accounts[0].AccountID, accounts[1].AccountID)
	if err != nil {
		t.Errorf("unexpected error in accounts deletion: %v", err)
	}
	rowNo, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("could not get rows affected: %v", err)
	}
	if got, want := int(rowNo), 2; got != want {
		t.Errorf("Deleted count got %d want %d", got, want)
	}
}

// Test_InvoicesQuery tests searching the database invoice records.
func Test_InvoicesQuery(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

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
				Contact:       "Major Donor Pledge",
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
				Contact:       "Generous Individual",
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
				Contact:       "Major Donor Pledge",
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
				Contact:       "Major Donor Pledge",
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
				Contact:       "Example Corp Ltd",
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
				Contact:       "Example Corp Ltd",
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

			invoices, err := testDB.InvoicesGet(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString, tt.limit, tt.offset)
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

// Test_InvoicesUpsert tests upserting invoices.
func Test_InvoicesUpsert(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

	invoices := []xero.Invoice{
		xero.Invoice{
			Type:          "ACCREC",
			InvoiceID:     "9fe6d963-fa41",
			InvoiceNumber: "INV-TEST-01",
			Contact:       "Contact Name",
			Date:          xero.XeroDateTime{Time: time.Now().Add(-2 * time.Hour)},
			Updated:       xero.XeroDateTime{Time: time.Now()},
			Status:        "PAID",
			Reference:     "A reference",
			Total:         212.20,
			AmountPaid:    212.20,
			LineItems: []xero.LineItem{
				xero.LineItem{
					Description: "A line item",
					UnitAmount:  210.20,
					AccountCode: "5501", // general giving
					LineItemID:  "9fe6d963-fa41-a",
					Quantity:    1,
					TaxAmount:   0,
					LineAmount:  210.20,
				},
				xero.LineItem{
					Description: "Second line item",
					UnitAmount:  2.0,
					AccountCode: "429", // fees
					LineItemID:  "9fe6d963-fa41-b",
					Quantity:    1,
					TaxAmount:   0,
					LineAmount:  2.0,
				},
			},
		},
	}

	err := testDB.InvoicesUpsert(ctx, invoices)
	if err != nil {
		t.Errorf("unexpected invoices error: %v", err)
	}

	// run a second time.
	err = testDB.InvoicesUpsert(ctx, invoices)
	if err != nil {
		t.Errorf("unexpected invoices error: %v", err)
	}

	var count int
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM invoice_line_items WHERE invoice_id = ?", "9fe6d963-fa41")
	if err != nil || count != 2 {
		t.Errorf("Expected to find 2 line items after invoice upsert, but got count %d, err: %v", count, err)
	}

	_, err = testDB.ExecContext(ctx, "DELETE FROM invoices WHERE id = ?", "9fe6d963-fa41")
	if err != nil {
		t.Errorf("unexpected error in invoice deletion: %v", err)
	}
	count = 0
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM invoice_line_items WHERE invoice_id = ?", "9fe6d963-fa41")
	if err != nil || count != 0 {
		t.Errorf("Expected to find 0 line items after invoice deletion, but got count %d, err: %v", count, err)
	}

}

// Test_BankTransactionsQuery tests searching the database bank transactions.
func Test_BankTransactionsQuery(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

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
				Contact:       "Stripe",
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
				Contact:       "JustGiving",
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
				Contact:       "Stripe",
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
				Contact:       "Stripe",
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
				Contact:       "Enthuse",
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

			transactions, err := testDB.BankTransactionsGet(ctx, tt.reconciliationStatus, tt.dateFrom, tt.dateTo, tt.searchString, tt.limit, tt.offset)
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

// Test BankTransactionsUpsert(ctx context.Context, transactions []xero.BankTransaction) error
// Todo: remove bank account data except for name.
func Test_BankTransactionsUpsert(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

	transactions := []xero.BankTransaction{
		xero.BankTransaction{
			BankTransactionID: "27104cb7-fac4",
			Type:              "RECEIVE",
			Contact:           "Contact Name2",
			IsReconciled:      true, // most transactions will be
			Reference:         "TEST-REF-20251101",
			Status:            "AUTHORISED", // or DELETED
			Date:              xero.XeroDateTime{Time: time.Now()},
			Updated:           xero.XeroDateTime{Time: time.Now()},
			Total:             20.00,
			BankAccount:       "current",
			LineItems: []xero.LineItem{
				xero.LineItem{
					Description: "bank transaction line item",
					AccountCode: "9999",
					LineItemID:  "5f117b7b",
					UnitAmount:  20.00,
					Quantity:    1,
					TaxAmount:   0,
					LineAmount:  20.00,
				},
			},
		},
	}

	err := testDB.BankTransactionsUpsert(ctx, transactions)
	if err != nil {
		t.Fatalf("could not upsert bank transactions: %v", err)
	}

	// run again
	err = testDB.BankTransactionsUpsert(ctx, transactions)
	if err != nil {
		t.Fatalf("could not upsert bank transactions for the second time: %v", err)
	}

	var count int
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM bank_transaction_line_items WHERE transaction_id = ?", "27104cb7-fac4")
	if err != nil || count != 1 {
		t.Errorf("Expected to find 1 line items after bank transaction upsert, but got count %d, err: %v", count, err)
	}

	_, err = testDB.ExecContext(ctx, "DELETE FROM bank_transactions WHERE id = ?", "9fe6d963-fa41")
	if err != nil {
		t.Errorf("unexpected error in invoice deletion: %v", err)
	}
	count = 0
	err = testDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM invoice_line_items WHERE invoice_id = ?", "27104cb7-fac4")
	if err != nil || count != 0 {
		t.Errorf("Expected to find 0 line items after bank transaction cascade deletion, but got count %d, err: %v", count, err)
	}

}

// Test_InvoiceWithLineItemsQuery tests retrieving an invoice with line items.
func Test_InvoiceWithLineItemsQuery(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

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
				Contact:          "Generous Individual",
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
				Contact:          "Small Pledge",
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
			invoice, lineItems, err := testDB.InvoiceWRGet(ctx, tt.invoiceID)
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

// Test_BankTransactionsWithLineItemsQuery tests retrieving a bank transactions with
// line items.
func Test_BankTransactionsWithLineItemsQuery(t *testing.T) {

	testDB, closeDB := setupTestDB(t)
	t.Cleanup(closeDB)
	ctx := context.Background()

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
				Contact:       "JustGiving",
				BankAccountID: "7404f143aa1c",
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
			transaction, lineItems, err := testDB.BankTransactionWRGet(ctx, tt.transactionID)
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

/*
// printInvoiceLineItems is an invoice helper template print function.
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
	err := parsedTemplate.Execute(os.Stdout, data)
	if err != nil {
		t.Fatal(err)
	}
}
*/
