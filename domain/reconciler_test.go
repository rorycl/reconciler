package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	mounts "github.com/rorycl/reconciler/internal/mounts"
)

// a mockXeroClient that only succeeds.
type mockXeroClient struct {
	getCount int
	log      *slog.Logger
}

func (mxc *mockXeroClient) GetOrganisation(ctx context.Context) (xero.Organisation, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("GetOrganisation %d", mxc.getCount))
	return xero.Organisation{Name: "Test", ShortCode: fmt.Sprintf("BCD-%d", mxc.getCount)}, nil
}
func (mxc *mockXeroClient) GetAccounts(ctx context.Context, ifModifiedSince time.Time) ([]xero.Account, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("GetAccounts %d", mxc.getCount))
	return []xero.Account{{AccountID: fmt.Sprintf("accountId-%d", mxc.getCount)}}, nil
}
func (mxc *mockXeroClient) GetBankTransactions(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.BankTransaction, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("GetBankTransactions %d", mxc.getCount))
	return []xero.BankTransaction{{BankTransactionID: fmt.Sprintf("btId-%d", mxc.getCount)}}, nil
}
func (mxc *mockXeroClient) GetInvoices(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.Invoice, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("Invoices %d", mxc.getCount))
	return []xero.Invoice{{InvoiceID: fmt.Sprintf("iId-%d", mxc.getCount)}}, nil
}

// mockXeroErrorClient raises an error for GetOrganisation.
type mockXeroErrorClient struct {
	mockXeroClient
}

func (mxec *mockXeroErrorClient) GetOrganisation(ctx context.Context) (xero.Organisation, error) {
	return xero.Organisation{}, errors.New("error")
}

// setupRefreshTestDB sets up a test database connection.
func setupRefreshTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	accountCodes := "^(53|55|57)"

	// Mount the sql fs either using the embedded fs or via the provided path. Note ".."
	// type paths are not accepted by fs.FS mounting.
	sqlFS, err := mounts.NewFileMount("sql", db.SQLEmbeddedFS, "../db/sql")
	if err != nil {
		t.Fatalf("mount error: %v", err)
	}

	testDB, err := db.NewConnectionInTestMode("file::memory:?cache=shared", sqlFS, accountCodes, nil)
	if err != nil {
		t.Fatalf("in-memory test database opening error: %v", err)
	}

	// Set log level to Error.
	testDB.SetLogLevel(slog.LevelError)

	// closeDBFunc is a closure for running by the function consumer.
	closeDBFunc := func() {
		err := testDB.Close()
		if err != nil {
			t.Fatalf("unexpected db close error: %v", err)
		}
	}

	return testDB, closeDBFunc
}

// TestReconcilerRefreshXeroRecords tests refreshing Xero records and upserting them
// into the database. The test database is used, but the xero api client is mocked.
func TestReconcilerRefreshXeroRecords(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	ctx := t.Context()
	logger := slog.Default()

	cfg := &config.Config{
		DataStartDate: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	reconciler := NewReconciler(testDB, logger)

	xeroClient := &mockXeroClient{getCount: 0, log: slog.Default()}

	results, err := reconciler.XeroRecordsRefresh(
		ctx,
		xeroClient,
		cfg.DataStartDate,
		time.Time{},
		regexp.MustCompile("."),
		true,
	)

	if err != nil {
		t.Fatalf("refreshXeroRecords failed: %v", err)
	}

	// Verify DB contents
	var shortCode string
	err = testDB.Get(&shortCode, "select shortcode from organisation")
	if err != nil {
		t.Fatalf("failed to get org from db: %v", err)
	}
	if shortCode != "BCD-1" {
		t.Errorf("shortcode got %s want %s", shortCode, "BCD-1")
	}
	if results.ShortCode != "BCD-1" {
		t.Errorf("results xero-shortcode got %s want %s", results.ShortCode, "BCD-1")
	}
	if got, want := results.FullRefresh, true; got != want {
		t.Errorf("got %t want %t for full refresh", got, want)
	}

	// Run a partial update, checking only bank transactions and invoices are updated.
	xeroClient.getCount = 10

	results, err = reconciler.XeroRecordsRefresh(
		ctx,
		xeroClient,
		cfg.DataStartDate,
		time.Now().Add(-2*time.Second),
		regexp.MustCompile("."),
		false,
	)
	if err != nil {
		t.Fatalf("refreshXeroRecords partial refresh failed: %v", err)
	}
	if got, want := results.FullRefresh, false; got != want {
		t.Errorf("got %t want %t for full refresh", got, want)
	}
	if got, want := results.InvoicesNo, 1; got != want {
		t.Errorf("got %d want %d for invoices", got, want)
	}

	var invoiceID string
	err = testDB.Get(&invoiceID, "SELECT id FROM invoices WHERE id = 'iId-12'")
	if err != nil {
		t.Fatalf("failed to get row from invoices: %v", err)
	}

	xeroErrorClient := &mockXeroErrorClient{
		mockXeroClient: mockXeroClient{log: slog.Default()},
	}
	_, err = reconciler.XeroRecordsRefresh(
		ctx,
		xeroErrorClient,
		cfg.DataStartDate,
		time.Time{},
		regexp.MustCompile("."),
		true,
	)
	if err == nil {
		t.Fatal("expected xero error")
	}
	es, ok := errors.AsType[ErrSystem](err)
	if !ok {
		t.Errorf("expected ErrSystem error, got %T", err)
	}
	if got, want := es.Detail, "xero GetOrganisation error"; !strings.Contains(got, want) {
		t.Errorf("expected error %q to contain %q", got, want)
	}

}

type mockSalesforceClient struct {
	getCount int
	log      *slog.Logger
}

func (msc *mockSalesforceClient) GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]salesforce.Donation, error) {
	msc.getCount++
	msc.log.Info(fmt.Sprintf("GetOpportunities %d", msc.getCount))
	return []salesforce.Donation{{CoreFields: salesforce.CoreFields{ID: fmt.Sprintf("ID-%d", msc.getCount)}}}, nil
}

func (msc *mockSalesforceClient) BatchUpdateOpportunityRefs(ctx context.Context, idRefs []salesforce.IDRef, allOrNone bool) (salesforce.CollectionsUpdateResponse, error) {
	msc.getCount++
	msc.log.Info(fmt.Sprintf("CollectionsUpdateResponse %d", msc.getCount))
	return salesforce.CollectionsUpdateResponse{
		salesforce.SaveResult{
			ID:      fmt.Sprintf("Id-%d", msc.getCount),
			Success: true,
			Errors:  nil,
		},
	}, nil
}

// TestReconcilerRefreshSalesforceRecords tests refreshing Salesforce records and
// upserting them into the database. The test database is used, but the Salesforce
// API client is mocked.
func TestReconcilerRefreshSalesforceRecords(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	ctx := t.Context()

	logger := slog.Default()

	cfg := &config.Config{
		DataStartDate: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	reconciler := NewReconciler(testDB, logger)

	results, err := reconciler.SalesforceRecordsRefresh(
		ctx,
		&mockSalesforceClient{log: logger},
		cfg.DataStartDate,
		time.Time{},
	)
	if err != nil {
		t.Fatalf("RefreshSalesforceRecords failed: %v", err)
	}
	if got, want := results.FullRefresh, true; got != want {
		t.Errorf("got %t want %t for full refresh", got, want)
	}
	if got, want := results.RecordsNo, 1; got != want {
		t.Errorf("got %d want %d records", got, want)
	}

}

// TestReconcilerLinkUnlink tests linking and updating Salesforce records and
// upserting them into the database. The test database is used, but the Salesforce
// API client is mocked.
func TestReconcilerLinkUnlink(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	ctx := t.Context()

	logger := slog.Default()

	cfg := &config.Config{
		DataStartDate: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	reconciler := NewReconciler(testDB, logger)

	err := reconciler.DonationsLinkUnlink(
		ctx,
		&mockSalesforceClient{log: logger},
		[]salesforce.IDRef{}, // empty
		cfg.DataStartDate,
		time.Time{},
	)
	if err == nil {
		t.Fatal("expected a salesforce link/unlink error")
	}
	if _, ok := errors.AsType[ErrUsage](err); !ok {
		t.Errorf("expected ErrUsage type got %T", err)
	}

	msc := &mockSalesforceClient{log: logger}
	err = reconciler.DonationsLinkUnlink(
		ctx,
		msc,
		[]salesforce.IDRef{{"a", "b"}, {"c", "d"}},
		cfg.DataStartDate,
		time.Time{},
	)
	if err != nil {
		t.Fatal("expected a salesforce link/unlink to proceed")
	}
	if got, want := msc.getCount, 2; got != want {
		t.Errorf("got link/unlink count %d want %d", got, want) // len of idrefs
	}

}

// TestReconcilerDBComponents tests the database related methods.
func TestReconcilerDBComponents(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	reconciler := NewReconciler(testDB, slog.Default())

	if got, want := reconciler.DBIsInMemory(), true; got != want {
		t.Errorf("DBIsInMemory got %t want %t", got, want)
	}
	if got, want := reconciler.DBPath(), reconciler.db.Path; got != want {
		t.Errorf("db path got %s want %s", got, want)
	}
	err := reconciler.Close()
	if err != nil {
		t.Fatalf("unexpected reconciler close error: %v", err)
	}
}

// TestReconcilerDBListings tests the database list functions.
func TestReconcilerDBListings(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	reconciler := NewReconciler(testDB, slog.Default())

	tests := []struct {
		proc            func() (int, error)
		expectedRecords int
		expectedError   error
	}{
		{
			proc: func() (int, error) {
				recs, err := reconciler.InvoicesGet(
					t.Context(),
					"All",
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"", // search
					20, // pagelen
					0,  // offset
				)
				return len(recs), err
			},
			expectedRecords: 9,
			expectedError:   nil,
		},
		{
			proc: func() (int, error) {
				recs, err := reconciler.InvoicesGet(
					t.Context(),
					"All",
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"no search results", // search
					20,                  // pagelen
					0,                   // offset
				)
				return len(recs), err
			},
			expectedError: sql.ErrNoRows,
		},
		{
			proc: func() (int, error) {
				recs, err := reconciler.TransactionsGet(
					t.Context(),
					"All",
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"", // search
					20, // pagelen
					0,  // offset
				)
				return len(recs), err
			},
			expectedRecords: 9,
			expectedError:   nil,
		},
		{
			proc: func() (int, error) {
				recs, err := reconciler.TransactionsGet(
					t.Context(),
					"All",
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"no transactions found", // search
					20,                      // pagelen
					0,                       // offset
				)
				return len(recs), err
			},
			expectedError: sql.ErrNoRows,
		},
		{
			proc: func() (int, error) {
				recs, err := reconciler.DonationsGet(
					t.Context(),
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"All", // linkage
					"",    // payout reference
					"",    // search
					20,    // pagelen
					0,     // offset
				)
				return len(recs), err
			},
			expectedRecords: 20,
			expectedError:   nil,
		},
		{
			proc: func() (int, error) {
				recs, err := reconciler.DonationsGet(
					t.Context(),
					time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
					"All",          // linkage
					"no ref found", // payout reference
					"",             // search
					20,             // pagelen
					0,              // offset
				)
				return len(recs), err
			},
			expectedError: sql.ErrNoRows,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {
			recNo, err := tt.proc()
			if err != nil {
				if tt.expectedError == nil {
					t.Fatalf("unexpected error %v", err)
				}
				if got, want := fmt.Sprintf("%T", err), fmt.Sprintf("%T", tt.expectedError); got != want {
					fmt.Sprintf("got error type %s want %s", got, want)
				}
				return
			}
			if err == nil && tt.expectedError != nil {
				t.Fatalf("expected error type %v", tt.expectedError)
			}
			if got, want := recNo, tt.expectedRecords; got != want {
				t.Errorf("got %d records want %d", got, want)
			}
		})
	}
}

// TestReconcilerDBDetail tests the database detail functions.
func TestReconcilerDBDetail(t *testing.T) {

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	reconciler := NewReconciler(testDB, slog.Default())

	tests := []struct {
		proc         func() (string, error)
		expectedInfo string
		expectedErr  error
	}{

		{
			proc: func() (string, error) {
				i, _, err := reconciler.InvoiceDetailGet(t.Context(), "inv-002")
				if err != nil {
					return "", err
				}
				return i.InvoiceNumber, err
			},
			expectedInfo: "INV-2025-102",
			expectedErr:  nil,
		},
		{
			proc: func() (string, error) {
				_, _, err := reconciler.InvoiceDetailGet(t.Context(), "inv-does-not-exist")
				return "", err
			},
			expectedErr: ErrUsage{Msg: "The requested invoice was not found"},
		},
		{
			proc: func() (string, error) {
				tr, _, err := reconciler.TransactionDetailGet(t.Context(), "bt-001")
				if err != nil {
					return "", err
				}
				return *tr.Reference, err
			},
			expectedInfo: "JG-PAYOUT-2025-04-15",
			expectedErr:  nil,
		},
		{
			proc: func() (string, error) {
				_, _, err := reconciler.TransactionDetailGet(t.Context(), "bt-does-not-exist")
				return "", err
			},
			expectedErr: ErrUsage{Msg: "The requested transaction was not found"},
		},
		{
			proc: func() (string, error) {
				_, dt, err := reconciler.InvoiceOrBankTransactionInfoGet(t.Context(), "invoice", "inv-002")
				if err != nil {
					return "", err
				}
				return dt.Format("2006-01-02"), err
			},
			expectedInfo: "2025-04-12",
			expectedErr:  nil,
		},
		{
			proc: func() (string, error) {
				_, dt, err := reconciler.InvoiceOrBankTransactionInfoGet(t.Context(), "bank-transaction", "bt-001")
				if err != nil {
					return "", err
				}
				return dt.Format("2006-01-02"), err
			},
			expectedInfo: "2025-04-15",
			expectedErr:  nil,
		},
		{
			proc: func() (string, error) {
				_, _, err := reconciler.InvoiceOrBankTransactionInfoGet(t.Context(), "bank-transaction", "invalid")
				return "", err
			},
			expectedErr: ErrUsage{Msg: "Transaction \"invalid\" could not be found"},
		},
		{
			proc: func() (string, error) {
				_, _, err := reconciler.InvoiceOrBankTransactionInfoGet(t.Context(), "nonsense", "invalid")
				return "", err
			},
			expectedErr: ErrSystem{Msg: "InvoiceOrBankTransactionInfoGet error"},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", ii), func(t *testing.T) {

			info, err := tt.proc()
			if err != nil {
				if tt.expectedErr == nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got, want := fmt.Sprintf("%T", err), fmt.Sprintf("%T", tt.expectedErr); got != want {
					t.Fatalf("got error type %q want %q", got, want)
				}
				// errors based on Error() for ErrUsage and ErrSystem are in format "a: b" or "a: b: c"
				matcher := "no match"
				eParts := strings.Split(tt.expectedErr.Error(), ":")
				if len(eParts) > 0 {
					matcher = strings.TrimSpace(eParts[1])
				}
				if got, want := err.Error(), matcher; !strings.Contains(got, want) {
					t.Fatalf("got error %q did not contain %q", got, want)
				}
				return
			}
			if err == nil && tt.expectedErr != nil {
				t.Fatalf("expected err %T %v", tt.expectedErr, tt.expectedErr)
			}
			if got, want := info, tt.expectedInfo; !strings.Contains(got, want) {
				t.Errorf("got %q expected to contain %q", got, want)
			}

		})
	}

}
