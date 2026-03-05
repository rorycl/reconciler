package web

import (
	"context"
	"encoding/gob"
	"fmt"
	"log/slog"
	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/apiclients/xero"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/db"
	mounts "github.com/rorycl/reconciler/internal/mounts"
	"github.com/rorycl/reconciler/internal/token"
	"regexp"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/oauth2"
)

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
	return []xero.Account{xero.Account{AccountID: fmt.Sprintf("accountId-%d", mxc.getCount)}}, nil
}
func (mxc *mockXeroClient) GetBankTransactions(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.BankTransaction, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("GetBankTransactions %d", mxc.getCount))
	return []xero.BankTransaction{xero.BankTransaction{BankTransactionID: fmt.Sprintf("btId-%d", mxc.getCount)}}, nil
}
func (mxc *mockXeroClient) GetInvoices(ctx context.Context, fromDate time.Time, ifModifiedSince time.Time, accountsRegexp *regexp.Regexp) ([]xero.Invoice, error) {
	mxc.getCount++
	mxc.log.Info(fmt.Sprintf("Invoices %d", mxc.getCount))
	return []xero.Invoice{xero.Invoice{InvoiceID: fmt.Sprintf("iId-%d", mxc.getCount)}}, nil
}

// not good for parallel tests.
var counter = 0

func NewMockXeroClient(ctx context.Context, logger *slog.Logger, accountsRegexp *regexp.Regexp, et *token.ExtendedToken) (xeroClienter, error) {
	return &mockXeroClient{getCount: counter, log: logger}, nil
}

// setupRefreshTestDB sets up a test database connection.
func setupRefreshTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	accountCodes := "^(53|55|57)"

	// mount the sql fs either using the embedded fs or via the provided path.
	// The path is likely to need to be relative to "here" as ".." type paths are not
	// accepted by fs mounting.
	sqlFS, err := mounts.NewFileMount("sql", db.SQLEmbeddedFS, "../db/sql")
	if err != nil {
		t.Fatalf("mount error: %v", err)
	}

	testDB, err := db.NewConnectionInTestMode("file::memory:?cache=shared", sqlFS, accountCodes, nil)
	if err != nil {
		t.Fatalf("in-memory test database opening error: %v", err)
	}

	// set log level to Info/Debug
	testDB.SetLogLevel(slog.LevelInfo)

	// closeDBFunc is a closure for running by the function consumer.
	closeDBFunc := func() {
		err := testDB.Close()
		if err != nil {
			t.Fatalf("unexpected db close error: %v", err)
		}
	}

	return testDB, closeDBFunc
}

// TestRefreshXeroRecords tests refreshing Xero records and upserting them into the
// database. The full test database is used, but the xero api client is mocked.
func TestRefreshXeroRecords(t *testing.T) {

	// Register types for scs.
	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	logger := slog.Default()

	sessionStore := scs.New()
	sessionStore.Lifetime = 1 * time.Hour

	webApp := &WebApp{
		log:            logger,
		db:             testDB,
		sessions:       sessionStore,
		accountsRegexp: regexp.MustCompile(".*"),
		cfg: &config.Config{
			DataStartDate:           time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			DonationAccountPrefixes: []string{"53", "55", "57"},
		},

		// client factory funcs
		newXeroClient: NewMockXeroClient,
	}

	ctx, err := sessionStore.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("could not load session store: %v", err)
	}

	validToken := token.ExtendedToken{
		Type:        token.XeroToken,
		InstanceURL: "https://example.com",
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}

	key := validToken.Type.SessionName()
	webApp.sessions.Put(ctx, key, validToken)
	webApp.sessions.Put(ctx, token.XeroToken.SessionName(), validToken)

	results, err := webApp.refreshXeroRecords(ctx)
	if err != nil {
		t.Fatalf("refreshXeroRecords failed: %v", err)
	}

	lastRefresh := sessionStore.GetTime(ctx, "xero-refreshed-datetime")
	if lastRefresh.IsZero() {
		t.Error("Expected xero-refreshed-datetime in session")
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
	if results["xero-shortcode"] != "BCD-1" {
		t.Errorf("results xero-shortcode got %s want %s", results["xero-shortcode"], "BCD-1")
	}

	// Run a partial update, checking only bank transactions and invoices are updated.
	counter = 10
	webApp.sessions.Put(ctx, "xero-refreshed-datetime", time.Now().Add(-2*time.Second))
	_, err = webApp.refreshXeroRecords(ctx)
	if err != nil {
		t.Fatalf("refreshXeroRecords partial refresh failed: %v", err)
	}

	err = testDB.Get(&shortCode, "select shortcode from organisation")
	if err != nil {
		t.Fatalf("failed to get org from db: %v", err)
	}
	if shortCode != "BCD-1" { // will increment past -1 if refreshed
		t.Errorf("shortcode got %s want %s", shortCode, "BCD-1")
	}

	var invoiceID string
	err = testDB.Get(&invoiceID, "SELECT id FROM invoices WHERE id = 'iId-12'")
	if err != nil {
		t.Fatalf("failed to get row from invoices: %v", err)
	}

}

type mockSalesforceClient struct {
	getCount int
	log      *slog.Logger
}

func (msc *mockSalesforceClient) GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]salesforce.Donation, error) {
	msc.getCount++
	msc.log.Info(fmt.Sprintf("GetOpportunities %d", msc.getCount))
	return []salesforce.Donation{salesforce.Donation{CoreFields: salesforce.CoreFields{ID: fmt.Sprintf("ID-%d", msc.getCount)}}}, nil
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

func NewMockSFClient(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error) {
	return &mockSalesforceClient{getCount: counter, log: logger}, nil
}

// TestRefreshSalesforceRecords tests refreshing Salesforce records and upserting them
// into the database. The full test database is used, but the Salesforce API client is
// mocked.
func TestRefreshSalesforceRecords(t *testing.T) {

	// Register types for scs.
	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	testDB, closeDB := setupRefreshTestDB(t)
	t.Cleanup(closeDB)

	logger := slog.Default()

	sessionStore := scs.New()
	sessionStore.Lifetime = 1 * time.Hour

	webApp := &WebApp{
		log:            logger,
		db:             testDB,
		sessions:       sessionStore,
		accountsRegexp: regexp.MustCompile(".*"),
		cfg: &config.Config{
			DataStartDate:           time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			DonationAccountPrefixes: []string{"53", "55", "57"},
		},

		// client factory funcs
		newSFClient: NewMockSFClient,
	}

	ctx, err := sessionStore.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("could not load session store: %v", err)
	}

	validToken := token.ExtendedToken{
		Type:        token.SalesforceToken,
		InstanceURL: "https://example.com",
		Token: &oauth2.Token{
			AccessToken: "valid-token-234",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}

	key := validToken.Type.SessionName()
	webApp.sessions.Put(ctx, key, validToken)
	webApp.sessions.Put(ctx, token.SalesforceToken.SessionName(), validToken)

	err = webApp.refreshSalesforceRecords(ctx)
	if err != nil {
		t.Fatalf("refreshSalesforceRecords failed: %v", err)
	}

}
