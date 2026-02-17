package salesforce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reconciler/config"
	"strings"
	"testing"
	"time"
)

// setup creates a test environment for running API client tests. It
// returns a request multiplexer for registering handlers, the API
// Client configured to use the test server, and a teardown function to
// close the server.
func setup(t *testing.T) (mux *http.ServeMux, client *Client, teardown func()) {
	t.Helper()
	mux = http.NewServeMux()
	server := httptest.NewServer(mux)
	client = &Client{
		httpClient:  server.Client(),
		instanceURL: server.URL,
		apiVersion:  SalesforceAPIVersionNumber,
		config: config.Config{
			Salesforce: config.SalesforceConfig{
				LoginDomain: server.URL,
			},
		},
		log: slog.New(slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: slog.LevelDebug},
		)),
	}
	teardown = func() {
		server.Close()
	}
	return mux, client, teardown
}

// testBatching is a generic test helper for verifying batched API calls.
func testBatching[T any](
	t *testing.T,
	endpointPathTPL string, // endpoint template
	jsonFiles []string, // files to call for each batch
	getFunc func(client *Client) (T, error),
) (T, error) {

	t.Helper()

	mux, client, teardown := setup(t)
	defer teardown()

	endpointPath := fmt.Sprintf("/services/data/%s/query", client.apiVersion)

	// Load json files.
	expectedCallCount := len(jsonFiles)
	if expectedCallCount == 0 {
		t.Fatal("At least one test json file is needed for a batch test.")
	}
	testContent := make([][]byte, len(jsonFiles))
	for i, j := range jsonFiles {
		var err error
		testContent[i], err = os.ReadFile(filepath.Join("testdata", j))
		if err != nil {
			t.Fatalf("failed to read json file %s: %v", j, err)
		}
		// Replace any nextRecordsUrl field with the current
		// endpointPath.
		testContent[i] = bytes.ReplaceAll(testContent[i], []byte("REPLACE-ME"), []byte(endpointPath))
	}

	var callCount int
	mux.HandleFunc(endpointPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		callCount++
		if callCount > expectedCallCount {
			t.Fatalf("expected batch no %d, got %d", expectedCallCount, callCount)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(testContent[callCount-1])
	})

	return getFunc(client)
}

// testPatch is a generic test helper for verifying PATCH API calls.
func testPatch(
	t *testing.T,
	endpointPathTPL string,
	errorID string,
	getFunc func(client *Client) error,
) error {

	t.Helper()

	mux, client, teardown := setup(t)
	defer teardown()

	endpointPath := fmt.Sprintf("/services/data/%s/composite/sobjects", client.apiVersion)

	mux.HandleFunc(endpointPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected method PATCH, got %s", r.Method)
		}

		// Decode the request body.
		decoder := json.NewDecoder(r.Body)
		var payload CollectionsUpdateRequest
		err := decoder.Decode(&payload)
		if err != nil {
			t.Fatalf("patch body read error: %v", err)
		}

		cur := CollectionsUpdateResponse(make([]SaveResult, len(payload.Records)))

		for i, r := range payload.Records {
			cur[i].ID = r["id"].(string)
			if cur[i].ID != errorID {
				// Simulate ok donation.
				cur[i].Success = true
				cur[i].Errors = []ErrorDetail{}
			} else {
				// Simulate error.
				cur[i].Success = false
				cur[i].Errors = []ErrorDetail{
					ErrorDetail{
						StatusCode: "404",
						Message:    "simulated error",
						Fields:     []string{"simulated"},
						ErrorCode:  "simulated error code",
					},
				}
			}
		}

		// Encode the response.
		response, err := json.Marshal(cur)
		if err != nil {
			t.Fatalf("response encoding error: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	})

	return getFunc(client)
}

// TestGetOpportunities_OneBatch tests the GetOpportunities client call
// for a single batch of donations.
func TestGetOpportunities_OneBatch(t *testing.T) {

	getOpportunitiesFunc := func(client *Client) ([]Donation, error) {
		return client.GetOpportunities(context.Background(), time.Now(), time.Time{})
	}

	donations, err := testBatching(
		t,
		"/services/data/%s/query",            // endpoint
		[]string{"salesforce_response.json"}, // json files to serve
		getOpportunitiesFunc,                 // the api function to call
	)
	if err != nil {
		t.Fatalf("testBatching returned an unexpected error: %v", err)
	}

	if got, want := len(donations), 34; got != want {
		t.Errorf("expected %d donations, got %d", want, got)
	}
}

// TestGetOpportunities_TwoBatch tests the GetOpportunities client call
// for two batches of donations.
func TestGetOpportunities_TwoBatch(t *testing.T) {

	getOpportunitiesFunc := func(client *Client) ([]Donation, error) {
		return client.GetOpportunities(context.Background(), time.Now(), time.Time{})
	}

	donations, err := testBatching(
		t,
		"/services/data/%s/query", // endpoint
		[]string{"salesforce_batch1.json", "salesforce_batch2.json"}, // json files to serve
		getOpportunitiesFunc, // the api function to call
	)
	if err != nil {
		t.Fatalf("testBatching returned an unexpected error: %v", err)
	}

	if got, want := len(donations), 3; got != want {
		t.Errorf("expected %d donations, got %d", want, got)
	}
}

// TestBatchUpdateOpportunityRefs_Succeed tests batch PATCH updates to
// update donations.
func TestBatchUpdateOpportunityRefs_Succeed(t *testing.T) {

	var (
		// allOrNone is the atomic failure flag.
		allOrNone bool            = false
		ctx       context.Context = context.Background()
		errorID   string          = ""
	)

	getBatchUpdate := func(client *Client) error {
		_, err := client.BatchUpdateOpportunityRefs(ctx, "ref-abc", []string{"a", "b", "c"}, allOrNone)
		return err
	}

	err := testPatch(
		t,
		"/services/data/%s/composite/sobjects", // endpoint template
		errorID,                                // ID to error
		getBatchUpdate,                         // the api function to call
	)
	if err != nil {
		t.Fatalf("testPatch returned an unexpected error: %v", err)
	}
}

// TestBatchUpdateOpportunityRefs_Fail tests a partial batch PATCH
// update failure to update donations.
func TestBatchUpdateOpportunityRefs_Fail(t *testing.T) {

	var (
		// allOrNone is the atomic failure flag.
		allOrNone bool            = false
		ctx       context.Context = context.Background()
		errorID   string          = "b"
	)

	var capturedResponse CollectionsUpdateResponse
	getBatchUpdateWithResponse := func(client *Client) error {
		var err error
		capturedResponse, err = client.BatchUpdateOpportunityRefs(ctx, "ref-abc", []string{"a", "b", "c"}, allOrNone)
		return err
	}

	err := testPatch(
		t,
		"/services/data/%s/composite/sobjects", // endpoint template
		errorID,                                // ID to error
		getBatchUpdateWithResponse,             // the api function to call
	)

	if err == nil {
		t.Fatal("expected an error, but got nil")
	}

	expectedErrorSubString := "failed to update donation b"
	if got, want := err.Error(), expectedErrorSubString; !strings.Contains(got, want) {
		t.Errorf("expected error string %q in %q", want, got)
	}

	if got, want := len(capturedResponse), 3; got != want {
		t.Errorf("got %d donations want %d", got, want)
	}

}
