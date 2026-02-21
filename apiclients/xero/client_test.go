package xero

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// setup creates a test environment for running API client tests. It returns a request
// multiplexer for registering handlers, the Client configured to use the test server,
// and a teardown function to close the server.
func setup(t *testing.T) (mux *http.ServeMux, client *Client, teardown func()) {

	t.Helper()

	mux = http.NewServeMux()
	server := httptest.NewServer(mux)

	accountsRegexp := regexp.MustCompile("")
	logger := slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))

	client = &Client{
		httpClient:     server.Client(),
		tenantID:       "fake-tenant-id",
		baseURL:        server.URL,
		accountsRegexp: accountsRegexp,
		log:            logger,
	}

	teardown = func() {
		server.Close()
	}

	return mux, client, teardown
}

// testPagination is a generic test helper for verifying paginated API calls.
func testPagination[T any](
	t *testing.T,
	endpointPath string,
	jsonFile string,
	emptyResponseJSON string,
	getFunc func(client *Client) (T, error),
) (T, error) {

	t.Helper()

	mux, client, teardown := setup(t)
	defer teardown()

	jsonContent, err := os.ReadFile(filepath.Join("testdata", jsonFile))
	if err != nil {
		t.Fatalf("failed to read json file %s: %v", jsonFile, err)
	}

	var callCount int
	mux.HandleFunc(endpointPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		callCount++
		switch callCount {
		case 1: // First page request
			if page := r.URL.Query().Get("page"); page != "1" {
				t.Errorf("expected page 1, got %s", page)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonContent)
		case 2: // Second page request
			if page := r.URL.Query().Get("page"); page != "2" {
				t.Errorf("expected page 2, got %s", page)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(emptyResponseJSON))
		default:
			t.Fatalf("handler called too many times: %d", callCount)
		}
	})

	return getFunc(client)
}

// testNoPagination is a generic test helper for verifying non-paginated API calls.
func testNoPagination[T any](
	t *testing.T,
	endpointPath string,
	jsonFile string,
	getFunc func(client *Client) (T, error),
) (T, error) {

	t.Helper()

	mux, client, teardown := setup(t)
	defer teardown()

	jsonContent, err := os.ReadFile(filepath.Join("testdata", jsonFile))
	if err != nil {
		t.Fatalf("failed to read json file %s: %v", jsonFile, err)
	}

	var callCount int
	mux.HandleFunc(endpointPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}
		// It should not have a "page" query parameter.
		if r.URL.Query().Get("page") != "" {
			t.Errorf("unexpected 'page' query parameter found for non-paginated endpoint")
		}

		callCount++
		if callCount > 1 {
			t.Fatalf("handler for non-paginated endpoint called more than once")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jsonContent)
	})

	return getFunc(client)
}

// TestGetInvoices_PaginationAndTermination verifies Invoice API pagination
// and termination.
func TestGetInvoices_PaginationAndTermination(t *testing.T) {

	// invoices.json has no line items
	// allAccountsRegexp := regexp.MustCompile("^[0-9]+")

	getInvoicesFunc := func(client *Client) ([]Invoice, error) {
		return client.GetInvoices(context.Background(), time.Now(), time.Time{}, nil)
	}

	invoices, err := testPagination(
		t,
		"/Invoices",        // endpoint
		"invoices.json",    // json file to serve
		`{"Invoices": []}`, // empty response
		getInvoicesFunc,    // the api function to call
	)
	if err != nil {
		t.Fatalf("testPagination returned an unexpected error: %v", err)
	}

	if got, want := len(invoices), 88; got != want {
		t.Errorf("expected %d invoices, got %d", want, got)
	}
}

// TestGetBankTransactions_PaginationAndTermination verifies
// BankTransaction API pagination and termination.
func TestGetBankTransactions_PaginationAndTermination(t *testing.T) {

	getBankTransactionsFunc := func(client *Client) ([]BankTransaction, error) {
		return client.GetBankTransactions(context.Background(), time.Now(), time.Time{}, nil)
	}

	bankTransactions, err := testPagination(
		t,
		"/BankTransactions",        // endpoint
		"bank_transactions.json",   // json file to serve
		`{"BankTransactions": []}`, // empty response
		getBankTransactionsFunc,    // the api function to call
	)

	if err != nil {
		t.Fatalf("testPagination returned an unexpected error: %v", err)
	}

	if got, want := len(bankTransactions), 29; got != want {
		t.Errorf("expected %d invoices, got %d", want, got)
	}
}

// TestGetAccounts_PaginationAndTermination verifies Accounts  API
// pagination and termination.
func TestGetAccounts_PaginationAndTermination(t *testing.T) {

	getAccountsFunc := func(client *Client) ([]Account, error) {
		return client.GetAccounts(context.Background(), time.Time{})
	}

	// Note that the Xero Accounts endpoint does not support pagination.
	accounts, err := testNoPagination(
		t,
		"/Accounts",     // endpoint
		"accounts.json", // json file to serve
		getAccountsFunc, // the api function to call
	)

	if err != nil {
		t.Fatalf("testPagination returned an unexpected error: %v", err)
	}

	if got, want := len(accounts), 90; got != want {
		t.Errorf("expected %d invoices, got %d", want, got)
	}
}

// TestGetInvoices_NotModified tests the client's handling of an HTTP 304 Not Modified
// status. The client should make one request and immediately stop processing,
// returning an empty slice of invoices without an error.
func TestGetInvoices_NotModified(t *testing.T) {
	mux, client, teardown := setup(t)
	defer teardown()

	allAccountsRegexp := regexp.MustCompile("^[0-9]+")

	mux.HandleFunc("/Invoices", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") == "" {
			t.Error("expected If-Modified-Since header to be set, but it was empty")
		}
		w.WriteHeader(http.StatusNotModified)
	})

	// The actual time used here doesn't matter, as long as it's not the zero value.
	ifModifiedSince := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	invoices, err := client.GetInvoices(context.Background(), time.Now(), ifModifiedSince, allAccountsRegexp)

	if err != nil {
		t.Fatalf("GetInvoices returned an unexpected error: %v", err)
	}

	// When a 304 is received, the function should return an empty slice.
	if len(invoices) != 0 {
		t.Errorf("expected 0 invoices on 304 Not Modified, got %d", len(invoices))
	}
}

// TestGetInvoices_APIError verifies that the client correctly handles and propagates
// errors from the API, such as a 4xx or 5xx status code.
func TestGetInvoices_APIError(t *testing.T) {
	mux, client, teardown := setup(t)
	defer teardown()

	allAccountsRegexp := regexp.MustCompile("^[0-9]+")

	const apiErrorBody = `{"Message": "An internal error occurred"}`

	// Register a handler that always returns an error status.
	mux.HandleFunc("/Invoices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError) // 500
		_, _ = w.Write([]byte(apiErrorBody))
	})

	_, err := client.GetInvoices(context.Background(), time.Now(), time.Time{}, allAccountsRegexp)

	if err == nil {
		t.Fatal("expected an error, but got nil")
	}

	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error message should contain status code 500, but was: %q", err.Error())
	}
	if !strings.Contains(err.Error(), apiErrorBody) {
		t.Errorf("error message should contain API response body, but was: %q", err.Error())
	}
}

// TestLineItemHasWantedAccount checks if an invoice or bank-transaction has a line item
// (which is commen to both) which matches the account regexp.
func TestLineItemHasWantedAccount(t *testing.T) {

	lis := []LineItem{
		LineItem{AccountCode: "a"},
		LineItem{AccountCode: "b"},
	}
	if !lineItemHasWantedAccount(lis, regexp.MustCompile("b")) {
		t.Error("unexpected false return from lineItemHasWantedAccount")
	}
	if lineItemHasWantedAccount(lis, regexp.MustCompile("d")) {
		t.Error("unexpected true return from lineItemHasWantedAccount")
	}
}
