package xero

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setup creates a test environment for running API client tests.
// It returns a request multiplexer for registering handlers, the APIClient configured
// to use the test server, and a teardown function to close the server.
func setup(t *testing.T) (mux *http.ServeMux, client *APIClient, teardown func()) {
	t.Helper()
	mux = http.NewServeMux()
	server := httptest.NewServer(mux)
	client = NewAPIClient("fake-tenant-id", server.Client())
	client.baseURL = server.URL // Override the default base URL.
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
	getFunc func(client *APIClient) (T, error),
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
			w.Write(jsonContent)
		case 2: // Second page request
			if page := r.URL.Query().Get("page"); page != "2" {
				t.Errorf("expected page 2, got %s", page)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(emptyResponseJSON))
		default:
			t.Fatalf("handler called too many times: %d", callCount)
		}
	})

	return getFunc(client)
}

// TestGetInvoices_PaginationAndTermination verifies Invoice API pagination
// and termination.
func TestGetInvoices_PaginationAndTermination(t *testing.T) {

	getInvoicesFunc := func(client *APIClient) ([]Invoice, error) {
		return client.GetInvoices(context.Background(), time.Now(), time.Time{})
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

	getBankTransactionsFunc := func(client *APIClient) ([]BankTransaction, error) {
		return client.GetBankTransactions(context.Background(), time.Now(), time.Time{})
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

	getAccountsFunc := func(client *APIClient) ([]Account, error) {
		return client.GetAccounts(context.Background(), time.Time{})
	}

	accounts, err := testPagination(
		t,
		"/Accounts",        // endpoint
		"accounts.json",    // json file to serve
		`{"Accounts": []}`, // empty response
		getAccountsFunc,    // the api function to call
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

	mux.HandleFunc("/Invoices", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") == "" {
			t.Error("expected If-Modified-Since header to be set, but it was empty")
		}
		w.WriteHeader(http.StatusNotModified)
	})

	// The actual time used here doesn't matter, as long as it's not the zero value.
	ifModifiedSince := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	invoices, err := client.GetInvoices(context.Background(), time.Now(), ifModifiedSince)

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

	const apiErrorBody = `{"Message": "An internal error occurred"}`

	// Register a handler that always returns an error status.
	mux.HandleFunc("/Invoices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError) // 500
		w.Write([]byte(apiErrorBody))
	})

	_, err := client.GetInvoices(context.Background(), time.Now(), time.Time{})

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
